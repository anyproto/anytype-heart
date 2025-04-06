package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testErrorType struct {
	payload int
}

func (t testErrorType) Error() string {
	return "error type!"
}

type testCode int32

func TestErrorCodeMapping(t *testing.T) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	err3 := errors.New("err3")
	err4 := testErrorType{}

	wrapped1 := errors.Join(err1, fmt.Errorf("description of error"))
	wrapped2 := fmt.Errorf("description of error: %w", err2)

	mapper := func(err error) testCode {
		return mapErrorCode(err,
			errToCode(err1, testCode(2)),
			errToCode(err2, testCode(3)),
			errToCode(err3, testCode(4)),
			errTypeToCode(&testErrorType{}, testCode(5)),
		)
	}

	assert.Equal(t, testCode(0), mapper(nil))
	assert.Equal(t, testCode(1), mapper(errors.New("unknown error")))
	assert.Equal(t, testCode(2), mapper(wrapped1))
	assert.Equal(t, testCode(3), mapper(wrapped2))
	assert.Equal(t, testCode(4), mapper(err3))
	assert.Equal(t, testCode(5), mapper(err4))
}

func TestErrorCodeMappingWithPayload(t *testing.T) {
	errPrototype := &testErrorType{}

	err := &testErrorType{payload: 42}
	code := mapErrorCode(err, errTypeToCode(&errPrototype, testCode(123)))

	assert.Equal(t, 42, errPrototype.payload)
	assert.Equal(t, testCode(123), code)
}
