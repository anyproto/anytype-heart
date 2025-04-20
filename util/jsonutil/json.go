package jsonutil

import (
	"encoding/json"
	"math"
	"reflect"
)

func Stringify(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func MarshalSafely(v any) ([]byte, error) {
	clearStruct(v)
	return json.Marshal(v)
}

func clearStruct(res interface{}) {
	elem := reflect.ValueOf(res).Elem()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		if kind := field.Kind(); kind == reflect.Float64 {
			if math.IsNaN(field.Float()) || math.IsInf(field.Float(), 0) {
				field.SetFloat(0)
			}
		}
	}
}
