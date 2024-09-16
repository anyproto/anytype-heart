package mill

import "io"

type Blob struct{}

const BlobId = "/blob"

func (m *Blob) ID() string {
	return BlobId
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
	return &Result{File: noopCloser(r)}, nil
}
