package mill

import "io"

type noopCloserWrapper struct {
	io.ReadSeeker
}

func (n *noopCloserWrapper) Close() error {
	return nil
}

func noopCloser(r io.ReadSeeker) io.ReadSeekCloser {
	return &noopCloserWrapper{r}
}
