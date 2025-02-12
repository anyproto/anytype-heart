package domain

import (
	"fmt"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
)

var log = logger.NewNamed("core.domain")

// Detail is Key-Value pair
type Detail struct {
	Key   RelationKey
	Value Value
}

type Details = GenericMap[RelationKey]

func NewDetails() *Details {
	return &GenericMap[RelationKey]{data: make(map[RelationKey]Value, 20)}
}

func NewDetailsFromProto(st *types.Struct) *Details {
	data := make(map[RelationKey]Value, len(st.GetFields()))
	d := &GenericMap[RelationKey]{data: data}
	for k, v := range st.GetFields() {
		d.SetProtoValue(RelationKey(k), v)
	}
	return d
}

func NewDetailsFromMap(details map[RelationKey]Value) *Details {
	return &GenericMap[RelationKey]{
		data: details,
	}
}

func NewDetailsWithSize(size int) *Details {
	return &GenericMap[RelationKey]{data: make(map[RelationKey]Value, size)}
}

func NewDetailsFromAnyEnc(v *anyenc.Value) (*Details, error) {
	obj, err := v.Object()
	if err != nil {
		return nil, fmt.Errorf("is object: %w", err)
	}
	res := NewDetailsWithSize(obj.Len())
	var visitErr error
	obj.Visit(func(k []byte, v *anyenc.Value) {
		if visitErr != nil {
			return
		}
		// key is copied
		err := setValueFromAnyEnc(res, RelationKey(k), v)
		if err != nil {
			visitErr = fmt.Errorf("key %s: %w", k, err)
		}
	})
	return res, visitErr
}

func setValueFromAnyEnc(d *Details, key RelationKey, val *anyenc.Value) error {
	switch val.Type() {
	case anyenc.TypeNumber:
		v, err := val.Float64()
		if err != nil {
			return fmt.Errorf("number: %w", err)
		}
		d.SetFloat64(key, v)
		return nil

	case anyenc.TypeString:
		v, err := val.StringBytes()
		if err != nil {
			return fmt.Errorf("string: %w", err)
		}
		d.SetString(key, string(v))
		return nil
	case anyenc.TypeTrue:
		d.SetBool(key, true)
		return nil
	case anyenc.TypeFalse:
		d.SetBool(key, false)
		return nil
	case anyenc.TypeArray:
		arrVals, err := val.Array()
		if err != nil {
			return fmt.Errorf("array: %w", err)
		}
		// Assume string as default type
		if len(arrVals) == 0 {
			d.SetStringList(key, nil)
			return nil
		}

		var arrayType anyenc.Type
		for _, arrVal := range arrVals {
			if arrVal.Type() == anyenc.TypeString {
				arrayType = anyenc.TypeString
				break
			}
			if arrVal.Type() == anyenc.TypeNumber {
				arrayType = anyenc.TypeNumber
				break
			}
		}
		if arrayType == anyenc.TypeString {
			res := make([]string, 0, len(arrVals))
			for i, arrVal := range arrVals {
				if arrVal.Type() != anyenc.TypeString {
					// todo: make it not possible to create such an arrays and remove this
					log.With(zap.String("key", key.String())).With(zap.Int("index", i)).Error(fmt.Sprintf("array item: expected string, got %s", arrVal.Type()))
					if arrVal.Type() != anyenc.TypeNull {
						return fmt.Errorf("array item: expected number, got %s", arrVal.Type())
					}
					continue
				}
				v, err := arrVal.StringBytes()
				if err != nil {
					return fmt.Errorf("array item: string: %w", err)
				}
				res = append(res, string(v))
			}
			d.SetStringList(key, res)
			return nil
		} else if arrayType == anyenc.TypeNumber {
			res := make([]float64, 0, len(arrVals))
			for i, arrVal := range arrVals {
				if arrVal.Type() != anyenc.TypeNumber {
					// todo: make it not possible to create such an arrays and remove this
					log.With(zap.String("key", key.String())).With(zap.Int("index", i)).Error(fmt.Sprintf("array item: expected number, got %s", arrVal.Type()))
					if arrVal.Type() != anyenc.TypeNull {
						return fmt.Errorf("array item: expected number, got %s", arrVal.Type())
					}
					continue
				}
				v, err := arrVal.Float64()
				if err != nil {
					return fmt.Errorf("array item: number: %w", err)
				}
				res = append(res, v)
			}
			d.SetFloat64List(key, res)
			return nil
		} else {
			// todo: make it not possible to create such an arrays and remove this
			var elTypes []string
			for _, arrVal := range arrVals {
				elTypes = append(elTypes, arrVal.Type().String())
			}
			return fmt.Errorf("unsupported array type %s; elements' types: %v", arrayType.String(), elTypes)
		}
	}
	d.Set(key, Null())
	return nil
}

// StructDiff returns pb struct which contains:
// - st2 fields that not exist in st1
// - st2 fields that not equal to ones exist in st1
// - absentKeys are st1 fields that do not exist in st2
// In case st1 and st2 are equal returns nil
func StructDiff(st1, st2 *Details) (diff *Details, absentKeys []RelationKey) {
	if st1 == nil {
		return st2, nil
	}
	if st2 == nil {
		diff = NewDetails()
		for k, _ := range st1.Iterate() {
			absentKeys = append(absentKeys, k)
		}
		return nil, absentKeys
	}

	for k2, v2 := range st2.Iterate() {
		v1 := st1.Get(k2)
		if !v1.Ok() || !v1.Equal(v2) {
			if diff == nil {
				diff = NewDetails()
			}
			diff.Set(k2, v2)
		}
	}

	for k, _ := range st1.Iterate() {
		if !st2.Has(k) {
			absentKeys = append(absentKeys, k)
		}
	}

	return diff, absentKeys
}

func DetailsListToProtos(dets []*Details) []*types.Struct {
	res := make([]*types.Struct, 0, len(dets))
	for _, d := range dets {
		res = append(res, d.ToProto())
	}
	return res
}
