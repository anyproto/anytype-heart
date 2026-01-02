package runtime

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	apiservice "github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/buke/quickjs-go"
)

//go:embed openai.js
var openaiJS string

type jsChatMessage struct {
	Identity string `js:"identity"`
	Text     string `js:"text"`
}

// HandleChatMsgParams contains all parameters for HandleChatMsg
type HandleChatMsgParams struct {
	ChatAddEv      *pb.EventChatAdd
	OpenAIKey      string
	ApiService     *apiservice.Service
	CurrentSpaceId string
	MainProgram    string // program specifier, e.g. "my_bot@v1"
}

func HandleChatMsg(goCtx context.Context, params HandleChatMsgParams) (reply string, trace *Trace, err error) {
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
	ctx.Globals().Set("__openaiKey", ctx.NewString(params.OpenAIKey))

	// Set up module loader with Anytype backend
	moduleLoader := NewAnytypeModuleLoader(goCtx, params.ApiService, params.CurrentSpaceId)

	// Register built-in modules
	moduleLoader.Register("openai", openaiJS)

	// Recursively preload the main program and all its dependencies
	loaded := make(map[string]bool)
	if err := PreloadModuleRecursively(ctx, moduleLoader, params.MainProgram, loaded); err != nil {
		return "", getTrace(), fmt.Errorf("preload modules: %w", err)
	}

	jsMessage := jsChatMessage{
		Text:     params.ChatAddEv.Message.Message.Text,
		Identity: params.ChatAddEv.Message.Creator,
	}

	jsMessageVal, err := ctx.Marshal(jsMessage)
	if err != nil {
		return "", getTrace(), err
	}
	defer jsMessageVal.Free()

	// Wrapper that imports the main program and exposes main() to globals
	wrapperCode := fmt.Sprintf(`
	import * as userMod from %q;
	if (typeof userMod.main === "function") {
	  globalThis.__main = userMod.main;
	} else {
	  throw new Error("Module must export a 'main' function");
	}
	`, params.MainProgram)

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
