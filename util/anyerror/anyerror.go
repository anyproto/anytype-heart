package anyerror

import (
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// badgerFileErrPattern is a regular expression pattern to match file paths in badger errors.
// seems like all badger errors with file paths have this pattern.
var badgerFileErrPattern = regexp.MustCompile(`file: (.*?), error:`)

// anonymizeBadgerError anonymizes a non-typed badger errors that contain file paths.
func anonymizeBadgerError(err string, changed bool) (string, bool) {
	if submatch := badgerFileErrPattern.FindStringSubmatch(err); len(submatch) > 0 {
		if len(submatch) > 0 {
			anonymizedPath := "*" + string(os.PathSeparator) + filepath.Base(strings.TrimSpace(submatch[1]))
			err = strings.Replace(err, submatch[1], anonymizedPath, 1)
			changed = true
		}
	}
	return err, changed
}

func CleanupError(err error) error {
	if err == nil {
		return nil
	}
	result := err.Error()
	var errChanged bool
	result = cleanUpCase(result, err, func(pathErr *os.PathError) {
		pathErr.Path = "<masked file path>"
		errChanged = true
	})
	result = cleanUpCase(result, err, func(urlErr *url.Error) {
		urlErr.URL = "<masked url>"
		errChanged = true
	})
	result = cleanUpCase(result, err, func(dnsErr *net.DNSError) {
		if dnsErr.Name != "" {
			dnsErr.Name = "<masked host name>"
			errChanged = true
		}
		if dnsErr.Server != "" {
			dnsErr.Server = "<masked dns server>"
			errChanged = true
		}
	})
	result, errChanged = anonymizeBadgerError(result, errChanged)
	if errChanged {
		return errors.New(result)
	}
	return err
}

func cleanUpCase[T error](result string, originalErr error, proc func(T)) string {
	var wrappedErr T
	if errors.As(originalErr, &wrappedErr) {
		prevWrappedString := wrappedErr.Error()
		proc(wrappedErr)
		result = strings.Replace(result, prevWrappedString, wrappedErr.Error(), 1)
	}
	return result
}
