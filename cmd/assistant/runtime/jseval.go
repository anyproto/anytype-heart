package runtime

import (
	"encoding/json"
	"errors"
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
func installJsEval(parentCtx *quickjs.Context, client *http.Client, openaiKey string) (cleanup func(), err error) {
	// Create the raw eval function
	evalFn := parentCtx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) < 1 {
			return ctx.ThrowError(errors.New("js.eval: source code is required"))
		}

		source := args[0].ToString()

		// Execute in a new isolated runtime
		result := executeIsolated(source, client, openaiKey)

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

// executeIsolated runs JS code in a fresh isolated runtime
func executeIsolated(source string, client *http.Client, openaiKey string) JsEvalResult {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	// Create new isolated runtime
	rt := quickjs.NewRuntime()
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

	// Set OpenAI key if provided
	if openaiKey != "" {
		ctx.Globals().Set("__openaiKey", ctx.NewString(openaiKey))
	}

	// Execute the code
	result := ctx.Eval(source)
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
