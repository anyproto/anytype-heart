package service

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/gogo/protobuf/proto"

	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw")

var mw = core.New()

func init() {
	fmt.Printf("mw jsaddon: %s\n", core.GetVersionDescription())
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

func SetEventHandlerMobile(eh MessageHandler) {
	SetEventHandler(func(event *pb.Event) {
		b, err := proto.Marshal(event)
		if err != nil {
			log.Errorf("eventHandler failed to marshal error: %s", err.Error())
		}
		eh.Handle(b)
	})
}
