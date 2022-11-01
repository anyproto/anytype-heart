package converter

import (
	"bytes"
	"fmt"
)

type ConvertError map[string]error

func NewError() ConvertError {
	return ConvertError{}
}

func (ce ConvertError) Add(objectName string, err error) {
	ce[objectName] = err
}

func (ce ConvertError) Merge(err ConvertError) {
	for fileName, errPb := range err {
		ce[fileName] = errPb
	}
}

func (ce ConvertError) IsEmpty() bool {
	return len(ce) == 0
}

func (ce ConvertError) Error() error {
	var pattern = "source: %s, error: %s" + "\n"
	var errorString bytes.Buffer
	if ce.IsEmpty() {
		return nil
	}
	for name, err := range ce {
		errorString.WriteString(fmt.Sprintf(pattern, name, err))
	}
	return fmt.Errorf(errorString.String())
}

func (ce ConvertError) Get(objectName string) error {
	return ce[objectName]
}