package debug

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("debug")

func PlaintextHandler(handlerFunc func(w io.Writer, req *http.Request) error) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "text/plain")

		err := handlerFunc(rw, req)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			_, err := fmt.Fprintf(rw, "error: %s", err)
			if err != nil {
				log.Errorf("write debug response for path %s error: %s", req.URL.Path, err)
			}
			return
		}
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func JSONHandler[T any](handlerFunc func(req *http.Request) (T, error)) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")

		data, err := handlerFunc(req)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			err := json.NewEncoder(rw).Encode(errorResponse{Error: err.Error()})
			if err != nil {
				log.Errorf("encode debug response for path %s error: %s", req.URL.Path, err)
			}
			return
		}
		err = json.NewEncoder(rw).Encode(data)
		if err != nil {
			log.Errorf("encode debug response for path %s error: %s", req.URL.Path, err)
		}
	}
}
