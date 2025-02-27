package files

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/gabriel-vasile/mimetype"
)

type AddOption func(*AddOptions)

type AddOptions struct {
	Reader               io.ReadSeeker
	Media                string
	Name                 string
	LastModifiedDate     int64
	CustomEncryptionKeys map[string]string

	// checksum of original file, calculated from Reader
	checksum string
}

func WithReader(r io.ReadSeeker) AddOption {
	return func(args *AddOptions) {
		args.Reader = r
	}
}

func WithName(name string) AddOption {
	return func(args *AddOptions) {
		args.Name = name
	}
}

func WithLastModifiedDate(timestamp int64) AddOption {
	return func(args *AddOptions) {
		args.LastModifiedDate = timestamp
	}
}

func WithCustomEncryptionKeys(keys map[string]string) AddOption {
	return func(args *AddOptions) {
		args.CustomEncryptionKeys = keys
	}
}

func (s *service) normalizeOptions(opts *AddOptions) error {
	if opts.checksum == "" && opts.Reader != nil {
		var err error
		opts.checksum, err = checksum(opts.Reader, false)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum: %w", err)
		}
		_, err = opts.Reader.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek reader: %w", err)
		}
	}

	if opts.Media == "" {
		data, err := ioutil.ReadAll(io.LimitReader(opts.Reader, 512))
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to get first 512 bytes to detect content-type: %w", err)
		}

		_, err = opts.Reader.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek underlying reader: %w", err)
		}

		mime := mimetype.Detect(data)
		opts.Media = mime.String()
	}

	return nil
}
