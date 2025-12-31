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
	rt := quickjs.NewRuntime()
	defer rt.Close()
	defer rt.RunGC() // Run GC before closing to free remaining JS objects

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

	// Load OpenAI module
	openaiModule := ctx.Eval(openaiJS)
	if openaiModule.IsException() {
		openaiModule.Free()
		return "", getTrace(), fmt.Errorf("load openai module: %w", ctx.Exception())
	}
	openaiModule.Free()

	jsCode := `
	function main(args) {
	  const result = openai.complete(args.text);
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

	// define main
	def := ctx.Eval(jsCode)
	if def.IsException() {
		def.Free()
		return "", getTrace(), ctx.Exception()
	}
	def.Free()

	// call main
	ret := ctx.Globals().Call("main", jsMessageVal)
	if ret.IsException() {
		ret.Free()
		return "", getTrace(), ctx.Exception()
	}
	defer ret.Free()

	return ret.ToString(), getTrace(), nil
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
