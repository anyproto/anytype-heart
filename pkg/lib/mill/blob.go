package mill

import "io"

type Blob struct{}

func (m *Blob) ID() string {
	return "/blob"
}

func (m *Blob) Pin() bool {
	return false
}

func (m *Blob) AcceptMedia(media string) error {
	return nil
}

func (m *Blob) Options(add map[string]interface{}) (string, error) {
	return hashOpts(make(map[string]string), add)
}

func (m *Blob) Mill(r io.ReadSeeker, name string) (*Result, error) {
	return &Result{File: r}, nil
}
