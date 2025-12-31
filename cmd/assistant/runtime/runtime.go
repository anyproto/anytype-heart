package runtime

import (
	"net/http"
	"time"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/buke/quickjs-go"
)

type jsChatMessage struct {
	Identity string `js:"identity"`
	Text     string `js:"text"`
}

func HandleChatMsg(chatAddEv *pb.EventChatAdd) (reply string, err error) {
	rt := quickjs.NewRuntime()
	defer rt.Close()
	defer rt.RunGC() // Run GC before closing to free remaining JS objects

	ctx := rt.NewContext()
	defer ctx.Close()

	client := &http.Client{Timeout: 20 * time.Second}

	cleanup, err := installFetch(ctx, client)
	if err != nil {
		return "", err
	}

	jsCode := `
	function main(args) {
	  const r = fetch("https://httpbin.org/anything", {
	    method: "POST",
	    headers: { "Content-Type": "application/json" },
	    body: JSON.stringify({ message: args.text, from: args.identity })
	  });
	  const data = r.json();
	  return { ok: r.ok, status: r.status, echo: data?.json?.message ?? null };
	}
	`

	jsMessage := jsChatMessage{
		Text:     chatAddEv.Message.Message.Text,
		Identity: chatAddEv.Message.Creator,
	}

	defer cleanup()

	jsMessageVal, err := ctx.Marshal(jsMessage)
	if err != nil {
		return "", err
	}
	defer jsMessageVal.Free()

	// define main
	def := ctx.Eval(jsCode)
	if def.IsException() {
		def.Free()
		return "", ctx.Exception()
	}
	def.Free()

	// call main
	ret := ctx.Globals().Call("main", jsMessageVal)
	if ret.IsException() {
		ret.Free()
		return "", ctx.Exception()
	}
	defer ret.Free()

	out := ret.JSONStringify()
	return out, nil

}
