package logging

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"

	"go.uber.org/zap"
)

type Sugared struct {
	*zap.SugaredLogger
}

func (s *Sugared) With(args ...interface{}) *Sugared {
	cleanupArgs(args)
	return &Sugared{s.SugaredLogger.With(args...)}
}

func (s *Sugared) Warn(args ...interface{}) {
	cleanupArgs(args)
	s.SugaredLogger.Warn(args...)
}

func (s *Sugared) Warnf(template string, args ...interface{}) {
	cleanupArgs(args)
	s.SugaredLogger.Warnf(template, args...)
}

func (s *Sugared) Warnw(msg string, keysAndValues ...interface{}) {
	cleanupArgs(keysAndValues)
	s.SugaredLogger.Warnw(msg, keysAndValues...)
}

func (s *Sugared) Error(args ...interface{}) {
	cleanupArgs(args)
	s.SugaredLogger.Error(args...)
}

func (s *Sugared) Errorf(template string, args ...interface{}) {
	cleanupArgs(args)
	s.SugaredLogger.Errorf(template, args...)
}

func (s *Sugared) Errorw(msg string, keysAndValues ...interface{}) {
	cleanupArgs(keysAndValues)
	s.SugaredLogger.Errorw(msg, keysAndValues...)
}

func (s *Sugared) Info(args ...interface{}) {
	cleanupArgs(args)
	s.SugaredLogger.Info(args...)
}

func (s *Sugared) Infof(template string, args ...interface{}) {
	cleanupArgs(args)
	s.SugaredLogger.Infof(template, args...)
}

func (s *Sugared) Infow(msg string, keysAndValues ...interface{}) {
	cleanupArgs(keysAndValues)
	s.SugaredLogger.Infow(msg, keysAndValues...)
}

func cleanupArgs(args []interface{}) {
	for i, arg := range args {
		if err, ok := arg.(error); ok {
			err = cleanupError(err)
			args[i] = err
		}
	}
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

func cleanupError(err error) error {
	if err == nil {
		return nil
	}
	result := err.Error()

	result = cleanUpCase(result, err, func(pathErr *os.PathError) {
		pathErr.Path = "<masked file path>"
	})
	result = cleanUpCase(result, err, func(urlErr *url.Error) {
		urlErr.URL = "<masked url>"
	})
	result = cleanUpCase(result, err, func(dnsErr *net.DNSError) {
		if dnsErr.Name != "" {
			dnsErr.Name = "<masked host name>"
		}
		if dnsErr.Server != "" {
			dnsErr.Server = "<masked dns server>"
		}
	})
	return errors.New(result)
}
