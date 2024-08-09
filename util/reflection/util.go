package reflection

import (
	"errors"
	"reflect"
	"strings"
)

func GetError(obj any) (code int64, description string, err error) {
	val := reflect.ValueOf(obj)

	if !val.IsValid() {
		return code, description, errors.New("response is absent")
	}

	elem := val.Elem()
	for i := 0; i < elem.NumField(); i++ {
		f := elem.Field(i)
		if f.Kind() != reflect.Pointer {
			continue
		}
		el := f.Elem()
		if !el.IsValid() {
			continue
		}
		if strings.Contains(el.Type().Name(), "ResponseError") {
			code = el.FieldByName("Code").Int()
			description = el.FieldByName("Description").String()
			return
		}
	}
	err = errors.New("can't extract the error field")
	return
}
