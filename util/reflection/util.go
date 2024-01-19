package reflection

import (
	"errors"
	"reflect"
	"strings"
)

func GetError(i interface{}) (code int64, description string, err error) {
	v := reflect.ValueOf(i).Elem()

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
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
