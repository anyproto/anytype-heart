package core

import (
	"testing"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
)

type testCode int32

func TestErrorCodeMapping(t *testing.T) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	err3 := errors.New("err3")

	wrapped1 := errors.Join(err1, fmt.Errorf("description of error"))
	wrapped2 := fmt.Errorf("description of error: %w", err2)

	mapper := func(err error) testCode {
		return mapErrorCode(err,
			errToCode(err1, testCode(2)),
			errToCode(err2, testCode(3)),
			errToCode(err3, testCode(4)),
		)
	}

	assert.True(t, 0 == mapper(nil))
	assert.True(t, 1 == mapper(errors.New("unknown error")))
	assert.True(t, 2 == mapper(wrapped1))
	assert.True(t, 3 == mapper(wrapped2))
	assert.True(t, 4 == mapper(err3))
}
