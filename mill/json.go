package mill

import (
	"encoding/json"
)

type Json struct{}

func (m *Json) ID() string {
	return "/json"
}

func (m *Json) Encrypt() bool {
	return true
}

func (m *Json) Pin() bool {
	return false
}

func (m *Json) AcceptMedia(media string) error {
	return accepts([]string{
		"application/json",
	}, media)
}

func (m *Json) Options(add map[string]interface{}) (string, error) {
	return hashOpts(make(map[string]string), add)
}

func (m *Json) Mill(input []byte, name string) (*Result, error) {
	var any interface{}
	if err := json.Unmarshal(input, &any); err != nil {
		return nil, err
	}

	data, err := json.Marshal(&any)
	if err != nil {
		return nil, err
	}

	log.Debugf("/json: %s", string(data))

	return &Result{File: data}, nil
}
