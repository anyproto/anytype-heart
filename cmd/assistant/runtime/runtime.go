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

	// Main program using ES module import
	// We load it as a module that sets a global function
	jsCode := `
	import { complete } from "openai";

	globalThis.main = function(args) {
	  const result = complete(args.text);
	  return result;
	};
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

	// Load and evaluate the main module
	mainModule := ctx.LoadModule(jsCode, "main.js")
	if mainModule.IsException() {
		mainModule.Free()
		return "", getTrace(), fmt.Errorf("load main module: %w", ctx.Exception())
	}
	mainModule.Free()

	// Call main from globals
	result := ctx.Globals().Call("main", jsMessageVal)
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
