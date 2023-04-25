package amplitude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

func mockServer(msgKey string) (chan []byte, chan []byte, *httptest.Server) {
	key, body := make(chan []byte, 1), make(chan []byte, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		k := r.FormValue("api_key")
		id := r.FormValue(msgKey)

		var v interface{}
		err := json.Unmarshal([]byte(id), &v)
		if err != nil {
			panic(err)
		}

		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			panic(err)
		}

		key <- []byte(k)
		body <- b
	}))

	return key, body, server
}
