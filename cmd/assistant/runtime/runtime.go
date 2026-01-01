package runtime

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/buke/quickjs-go"
)

//go:embed openai.js
var openaiJS string

type jsChatMessage struct {
	Identity string `js:"identity"`
	Text     string `js:"text"`
}

func HandleChatMsg(chatAddEv *pb.EventChatAdd, openAIKey string) (reply string, trace *Trace, err error) {
	// Enable module imports
	rt := quickjs.NewRuntime(quickjs.WithModuleImport(true))
	defer rt.Close()
	defer rt.RunGC()

	ctx := rt.NewContext()
	defer ctx.Close()

	// Install tracer first (before other side effects)
	getTrace, tracerCleanup, err := installTracer(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tracerCleanup()

	client := &http.Client{Timeout: 60 * time.Second}

	fetchCleanup, err := installFetch(ctx, client)
	if err != nil {
		return "", nil, err
	}
	defer fetchCleanup()

	// Set OpenAI key for the module
	ctx.Globals().Set("__openaiKey", ctx.NewString(openAIKey))

	// Set up and preload modules
	moduleLoader := NewMapModuleLoader()
	moduleLoader.Register("openai", openaiJS)

	if err := PreloadModules(ctx, moduleLoader); err != nil {
		return "", getTrace(), fmt.Errorf("preload modules: %w", err)
	}

	// User's main program module with export main convention
	jsCode := `
	import { complete } from "openai";

	export function main(args) {
	  const result = complete(args.text);
	  return result;
	}
	`

	jsMessage := jsChatMessage{
		Text:     chatAddEv.Message.Message.Text,
		Identity: chatAddEv.Message.Creator,
	}

	jsMessageVal, err := ctx.Marshal(jsMessage)
	if err != nil {
		return "", getTrace(), err
	}
	defer jsMessageVal.Free()

	// Register the user's module
	moduleLoader.Register("__main__", jsCode)
	userModule := ctx.LoadModule(jsCode, "__main__", quickjs.EvalLoadOnly(true))
	if userModule.IsException() {
		userModule.Free()
		return "", getTrace(), fmt.Errorf("load user module: %w", ctx.Exception())
	}
	userModule.Free()

	// Wrapper that imports user module and exposes main to globals
	wrapperCode := `
	import * as userMod from "__main__";
	if (typeof userMod.main === "function") {
	  globalThis.__main = userMod.main;
	} else {
	  throw new Error("Module must export a 'main' function");
	}
	`

	wrapper := ctx.LoadModule(wrapperCode, "__wrapper__")
	if wrapper.IsException() {
		wrapper.Free()
		return "", getTrace(), fmt.Errorf("load wrapper: %w", ctx.Exception())
	}
	wrapper.Free()

	// Call the exposed main function
	result := ctx.Globals().Call("__main", jsMessageVal)
	if result.IsException() {
		result.Free()
		return "", getTrace(), ctx.Exception()
	}
	defer result.Free()

	return result.ToString(), getTrace(), nil
}

// TraceToJSON converts trace to a formatted JSON string for printing
func TraceToJSON(trace *Trace) string {
	if trace == nil {
		return "{}"
	}
	data, err := json.MarshalIndent(trace.Effects, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
