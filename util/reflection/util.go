package reflection

import (
	"errors"
	"reflect"
	"strings"

	"github.com/anyproto/anytype-heart/pb"
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

func GetChangeContent(val pb.IsChangeContentValue) (name string) {
	t := reflect.TypeOf(val)
	if t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t != nil {
		name = t.Name()
		name, _ = strings.CutPrefix(name, "ChangeContentValueOf")
	}
	return
}

func GetMessageContent(val pb.IsEventMessageValue) (name string) {
	t := reflect.TypeOf(val)
	if t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t != nil {
		name = t.Name()
		name, _ = strings.CutPrefix(name, "EventMessageValueOf")
	}
	return
}
