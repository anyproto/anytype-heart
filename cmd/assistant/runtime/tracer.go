package runtime

import (
	"github.com/buke/quickjs-go"
)

// Trace holds all recorded side effect calls: effectName -> input -> []outputs
// Errors are recorded as "__error__: <message>"
type Trace struct {
	Effects map[string]map[string][]string `json:"effects"`
}

func installTracer(ctx *quickjs.Context) (getTrace func() *Trace, cleanup func(), err error) {
	// Initialize trace storage in JS
	// Structure: { effectName: { inputStr: [outputStr, outputStr, ...] } }
	// Errors are recorded as "__error__: <message>"
	initCode := ctx.Eval(`
		globalThis.__trace = {};
		globalThis.__recordEffect = function(name, input, output) {
			if (!globalThis.__trace[name]) {
				globalThis.__trace[name] = {};
			}
			if (!globalThis.__trace[name][input]) {
				globalThis.__trace[name][input] = [];
			}
			globalThis.__trace[name][input].push(output);
		};
		globalThis.__wrapEffect = function(name, fn) {
			return function(...args) {
				const inputStr = JSON.stringify(args);
				try {
					const result = fn(...args);
					const outputStr = JSON.stringify(result);
					globalThis.__recordEffect(name, inputStr, outputStr);
					return result;
				} catch (e) {
					const errorStr = "__error__: " + String(e);
					globalThis.__recordEffect(name, inputStr, errorStr);
					throw e;
				}
			};
		};
	`)
	if initCode.IsException() {
		defer initCode.Free()
		return nil, nil, ctx.Exception()
	}
	initCode.Free()

	// Function to retrieve the trace from JS
	getTrace = func() *Trace {
		traceVal := ctx.Eval(`globalThis.__trace`)
		if traceVal.IsException() {
			traceVal.Free()
			return &Trace{Effects: map[string]map[string][]string{}}
		}
		defer traceVal.Free()

		var rawTrace map[string]map[string][]string
		if err := ctx.Unmarshal(traceVal, &rawTrace); err != nil {
			return &Trace{Effects: map[string]map[string][]string{}}
		}

		return &Trace{Effects: rawTrace}
	}

	cleanup = func() {
		// Clean up global trace objects
		cleanupCode := ctx.Eval(`
			delete globalThis.__trace;
			delete globalThis.__recordEffect;
			delete globalThis.__wrapEffect;
		`)
		cleanupCode.Free()
	}

	return getTrace, cleanup, nil
}
