package pbtypes

import (
	"bytes"
	"fmt"

	"github.com/valyala/fastjson"
)

type JsonDiffType int

const (
	JsonDiffTypeAdd JsonDiffType = iota
	JsonDiffTypeRemove
	JsonDiffTypeUpdate
)

type JsonDiff struct {
	Type  JsonDiffType
	Key   string
	Value *fastjson.Value
}

func DiffJson(a *fastjson.Value, b *fastjson.Value) ([]JsonDiff, error) {
	objA, err := a.Object()
	if err != nil {
		return nil, fmt.Errorf("param a is not an object: %w", err)
	}
	objB, err := b.Object()
	if err != nil {
		return nil, fmt.Errorf("param b is not an object: %w", err)
	}

	var diffs []JsonDiff
	existsA := make(map[string]struct{}, objA.Len())

	objA.Visit(func(key []byte, v *fastjson.Value) {
		existsA[string(key)] = struct{}{}
	})

	var (
		stop     bool
		visitErr error
	)
	objB.Visit(func(key []byte, v *fastjson.Value) {
		if stop {
			return
		}
		strKey := string(key)
		if _, ok := existsA[strKey]; ok {
			eq, err := compareValue(a.Get(strKey), v)
			if err != nil {
				visitErr = err
				stop = true
			}
			if !eq {
				diffs = append(diffs, JsonDiff{
					Type:  JsonDiffTypeUpdate,
					Key:   strKey,
					Value: v, // Holden value, be cautious
				})
			}
			delete(existsA, strKey)
		} else {
			diffs = append(diffs, JsonDiff{
				Type:  JsonDiffTypeAdd,
				Key:   strKey,
				Value: v, // Holden value, be cautious
			})
		}
	})
	if visitErr != nil {
		return nil, fmt.Errorf("visit b: %w", visitErr)
	}

	for key := range existsA {
		diffs = append(diffs, JsonDiff{
			Type: JsonDiffTypeRemove,
			Key:  key,
		})
	}
	return diffs, nil
}

func compareValue(a *fastjson.Value, b *fastjson.Value) (bool, error) {
	if a.Type() != b.Type() {
		// Return true, as we have checked that types are equal
		return false, nil
	}
	switch a.Type() {
	case fastjson.TypeNull:
		return true, nil
	case fastjson.TypeNumber:
		af, err := a.Float64()
		if err != nil {
			return false, fmt.Errorf("a: get float64: %w", err)
		}
		bf, err := b.Float64()
		if err != nil {
			return false, fmt.Errorf("b: get float64: %w", err)
		}
		return af == bf, nil
	case fastjson.TypeString:
		as, err := a.StringBytes()
		if err != nil {
			return false, fmt.Errorf("a: get string: %w", err)
		}
		bs, err := b.StringBytes()
		if err != nil {
			return false, fmt.Errorf("b: get string: %w", err)
		}
		return bytes.Compare(as, bs) == 0, nil
	case fastjson.TypeTrue, fastjson.TypeFalse:
		// Return true, as we have checked that types are equal
		return true, nil
	case fastjson.TypeArray:
		aa, err := a.Array()
		if err != nil {
			return false, fmt.Errorf("a: get array: %w", err)
		}
		ba, err := b.Array()
		if err != nil {
			return false, fmt.Errorf("b: get array: %w", err)
		}
		if len(aa) != len(ba) {
			return false, nil
		}
		for i := range aa {
			eq, err := compareValue(aa[i], ba[i])
			if err != nil {
				return false, err
			}
			if !eq {
				return false, nil
			}
		}
		return true, nil
	case fastjson.TypeObject:
		ao, err := a.Object()
		if err != nil {
			return false, fmt.Errorf("a: get object: %w", err)
		}
		bo, err := b.Object()
		if err != nil {
			return false, fmt.Errorf("b: get object: %w", err)
		}
		if ao.Len() != bo.Len() {
			return false, nil
		}
		var (
			eq       bool
			stop     bool
			visitErr error
		)
		ao.Visit(func(k []byte, va *fastjson.Value) {
			if stop {
				return
			}
			vb := bo.Get(string(k))
			// TODO Test nil values
			if vb == nil {
				eq = false
				stop = true
				return
			}
			eq, visitErr = compareValue(va, vb)
			if visitErr != nil {
				stop = true
				return
			}
			if !eq {
				stop = true
			}
		})
		if visitErr != nil {
			return false, fmt.Errorf("compare objects: %w", visitErr)
		}
		return eq, nil
	}
	return false, nil
}
