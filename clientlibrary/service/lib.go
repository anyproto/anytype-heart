package service

import (
	"fmt"
	"net/http"
	"os"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"

	_ "net/http/pprof"
)

var log = logging.Logger("anytype-mw")

var mw = core.New()

func init() {
	fmt.Printf("mw jsaddon: %s\n", app.GitSummary)
	registerClientCommandsHandler(mw)
	PanicHandler = mw.OnPanic
	metrics.SharedClient.InitWithKey(metrics.DefaultAmplitudeKey)
	if debug, ok := os.LookupEnv("ANYPROF"); ok && debug != "" {
		go func() {
			http.ListenAndServe(debug, nil)
		}()
	}
}

func SetEventHandler(eh func(event *pb.Event)) {
	mw.EventSender = event.NewCallbackSender(eh)
}

func SetEnv(key, value string) {
	os.Setenv(key, value)
}

func SetEventHandlerMobile(eh MessageHandler) {
	SetEventHandler(func(event *pb.Event) {
		b, err := proto.Marshal(event)
		if err != nil {
			log.Errorf("eventHandler failed to marshal error: %s", err.Error())
		}
		eh.Handle(b)
	})
}
