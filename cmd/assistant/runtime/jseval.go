package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/buke/quickjs-go"
)

// JsEvalResult is the result returned by js.eval
type JsEvalResult struct {
	Result interface{}            `json:"result"`
	Traces map[string]interface{} `json:"traces"`
	Error  string                 `json:"error,omitempty"`
}

// installJsEval installs the js.eval effect that runs JS code in a nested context
// Usage: js.eval(source, args?) - runs source code that must export main(args), returns {result, traces, error?}
func installJsEval(parentCtx *quickjs.Context, client *http.Client, openaiKey string, moduleLoader ModuleLoader) (cleanup func(), err error) {
	return installJsEvalWithParams(parentCtx, client, openaiKey, "", moduleLoader)
}

// installJsEvalWithParams installs js.eval with full parameter control including claudeKey
func installJsEvalWithParams(parentCtx *quickjs.Context, client *http.Client, openaiKey, claudeKey string, moduleLoader ModuleLoader) (cleanup func(), err error) {
	// Create the raw eval function
	evalFn := parentCtx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) < 1 {
			return ctx.ThrowError(errors.New("js.eval: source code is required"))
		}

		source := args[0].ToString()

		// Get optional args parameter to pass to main()
		var mainArgs interface{}
		if len(args) >= 2 && !args[1].IsUndefined() && !args[1].IsNull() {
			if err := ctx.Unmarshal(args[1], &mainArgs); err != nil {
				mainArgs = args[1].ToString()
			}
		}

		// Execute in a new isolated runtime with module support
		result := executeIsolatedWithParams(ExecuteIsolatedParams{
			Source:       source,
			MainArgs:     mainArgs,
			Client:       client,
			OpenAIKey:    openaiKey,
			ClaudeKey:    claudeKey,
			ModuleLoader: moduleLoader,
		})

		// Marshal result to JS value
		jsResult, err := ctx.Marshal(result)
		if err != nil {
			return ctx.ThrowError(errors.New("js.eval: marshal result: " + err.Error()))
		}
		return jsResult
	})

	// Set raw function, then wrap with tracer
	parentCtx.Globals().Set("__rawJsEval", evalFn)
	wrapCode := parentCtx.Eval(`
		globalThis.js = globalThis.js || {};
		globalThis.js.eval = globalThis.__wrapEffect("js.eval", globalThis.__rawJsEval);
	`)
	if wrapCode.IsException() {
		defer wrapCode.Free()
		return nil, parentCtx.Exception()
	}
	wrapCode.Free()

	return func() {
		// Cleanup if needed
	}, nil
}

// ExecuteIsolatedParams contains parameters for executeIsolated
type ExecuteIsolatedParams struct {
	Source       string
	MainArgs     interface{}
	Client       *http.Client
	OpenAIKey    string
	ClaudeKey    string
	ModuleLoader ModuleLoader
}

// executeIsolated runs JS code in a fresh isolated runtime with module support
// The source must export a main(args) function, which will be called with mainArgs
func executeIsolated(source string, mainArgs interface{}, client *http.Client, openaiKey string, moduleLoader ModuleLoader) JsEvalResult {
	return executeIsolatedWithParams(ExecuteIsolatedParams{
		Source:       source,
		MainArgs:     mainArgs,
		Client:       client,
		OpenAIKey:    openaiKey,
		ModuleLoader: moduleLoader,
	})
}

// executeIsolatedWithParams runs JS code with full parameter control
func executeIsolatedWithParams(params ExecuteIsolatedParams) JsEvalResult {
	source := params.Source
	mainArgs := params.MainArgs
	client := params.Client
	openaiKey := params.OpenAIKey
	claudeKey := params.ClaudeKey
	moduleLoader := params.ModuleLoader
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	// Create new isolated runtime with module imports enabled
	rt := quickjs.NewRuntime(quickjs.WithModuleImport(true))
	defer rt.Close()
	defer rt.RunGC()

	ctx := rt.NewContext()
	defer ctx.Close()

	// Install tracer
	getTrace, tracerCleanup, err := installTracer(ctx)
	if err != nil {
		return JsEvalResult{Error: "tracer setup: " + err.Error()}
	}
	defer tracerCleanup()

	// Install fetch
	fetchCleanup, err := installFetch(ctx, client)
	if err != nil {
		return JsEvalResult{Error: "fetch setup: " + err.Error()}
	}
	defer fetchCleanup()

	// Set API keys if provided
	if openaiKey != "" {
		ctx.Globals().Set("__openaiKey", ctx.NewString(openaiKey))
	}
	if claudeKey != "" {
		ctx.Globals().Set("__claudeKey", ctx.NewString(claudeKey))
	}

	// Install js.eval in nested context (allows nested programs to run other programs)
	jsEvalCleanup, err := installJsEvalWithParams(ctx, client, openaiKey, claudeKey, moduleLoader)
	if err != nil {
		return JsEvalResult{Error: "js.eval setup: " + err.Error()}
	}
	defer jsEvalCleanup()

	// If module loader is provided, preload any imports found in the source
	if moduleLoader != nil {
		loaded := make(map[string]bool)
		imports := parseImports(source)
		for _, imp := range imports {
			if err := PreloadModuleRecursively(ctx, moduleLoader, imp, loaded); err != nil {
				return JsEvalResult{Error: fmt.Sprintf("preload module %q: %s", imp, err.Error())}
			}
		}
	}

	// Prepend "export {};" to ensure QuickJS detects this as a module
	source = "export {};\n" + source

	// Load the user module
	userModule := ctx.LoadModule(source, "__eval_module__", quickjs.EvalLoadOnly(true))
	if userModule.IsException() {
		userModule.Free()
		trace := getTrace()
		return JsEvalResult{
			Error:  ctx.Exception().Error(),
			Traces: traceToMap(trace),
		}
	}
	userModule.Free()

	// Create wrapper that imports user module and exposes main to globals (same as parent runtime)
	wrapperCode := `
	import * as userMod from "__eval_module__";
	if (typeof userMod.main === "function") {
	  globalThis.__main = userMod.main;
	} else {
	  throw new Error("Module must export a 'main' function");
	}
	`

	wrapper := ctx.LoadModule(wrapperCode, "__eval_wrapper__")
	if wrapper.IsException() {
		wrapper.Free()
		trace := getTrace()
		return JsEvalResult{
			Error:  ctx.Exception().Error(),
			Traces: traceToMap(trace),
		}
	}
	wrapper.Free()

	// Marshal args to pass to main()
	var jsArgs *quickjs.Value
	if mainArgs != nil {
		var marshalErr error
		jsArgs, marshalErr = ctx.Marshal(mainArgs)
		if marshalErr != nil {
			return JsEvalResult{Error: "marshal args: " + marshalErr.Error()}
		}
		defer jsArgs.Free()
	} else {
		jsArgs = ctx.NewUndefined()
		defer jsArgs.Free()
	}

	// Call the exposed main function
	result := ctx.Globals().Call("__main", jsArgs)
	if result.IsException() {
		result.Free()
		trace := getTrace()
		return JsEvalResult{
			Error:  ctx.Exception().Error(),
			Traces: traceToMap(trace),
		}
	}
	defer result.Free()

	// Get the result value
	var resultVal interface{}
	if err := ctx.Unmarshal(result, &resultVal); err != nil {
		// If unmarshal fails, try to get string representation
		resultVal = result.ToString()
	}

	trace := getTrace()
	return JsEvalResult{
		Result: resultVal,
		Traces: traceToMap(trace),
	}
}

// traceToMap converts Trace to a generic map for JSON marshaling
func traceToMap(trace *Trace) map[string]interface{} {
	if trace == nil || trace.Effects == nil {
		return map[string]interface{}{}
	}

	result := make(map[string]interface{})
	for effectName, inputs := range trace.Effects {
		inputMap := make(map[string]interface{})
		for input, outputs := range inputs {
			// Try to parse outputs as JSON for cleaner structure
			parsedOutputs := make([]interface{}, len(outputs))
			for i, out := range outputs {
				var parsed interface{}
				if err := json.Unmarshal([]byte(out), &parsed); err == nil {
					parsedOutputs[i] = parsed
				} else {
					parsedOutputs[i] = out
				}
			}
			inputMap[input] = parsedOutputs
		}
		result[effectName] = inputMap
	}
	return result
}
