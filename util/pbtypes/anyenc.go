package pbtypes

import (
	"bytes"
	"fmt"

	"github.com/anyproto/any-store/anyenc"
)

type AnyEncDiffType int

const (
	AnyEncDiffTypeAdd AnyEncDiffType = iota
	AnyEncDiffTypeRemove
	AnyEncDiffTypeUpdate
)

type AnyEncDiff struct {
	Type  AnyEncDiffType
	Key   string
	Value *anyenc.Value
}

func DiffAnyEnc(a *anyenc.Value, b *anyenc.Value) ([]AnyEncDiff, error) {
	objA, err := a.Object()
	if err != nil {
		return nil, fmt.Errorf("param a is not an object: %w", err)
	}
	objB, err := b.Object()
	if err != nil {
		return nil, fmt.Errorf("param b is not an object: %w", err)
	}

	var diffs []AnyEncDiff
	existsA := make(map[string]struct{}, objA.Len())

	objA.Visit(func(key []byte, v *anyenc.Value) {
		existsA[string(key)] = struct{}{}
	})

	var (
		stop     bool
		visitErr error
	)
	objB.Visit(func(key []byte, v *anyenc.Value) {
		if stop {
			return
		}
		sKey := string(key)
		if _, ok := existsA[sKey]; ok {
			eq, err := compareValue(a.Get(sKey), v)
			if err != nil {
				visitErr = err
				stop = true
			}
			if !eq {
				diffs = append(diffs, AnyEncDiff{
					Type:  AnyEncDiffTypeUpdate,
					Key:   sKey,
					Value: v, // Holden value, be cautious
				})
			}
			delete(existsA, sKey)
		} else {
			diffs = append(diffs, AnyEncDiff{
				Type:  AnyEncDiffTypeAdd,
				Key:   sKey,
				Value: v, // Holden value, be cautious
			})
		}
	})
	if visitErr != nil {
		return nil, fmt.Errorf("visit b: %w", visitErr)
	}

	for key := range existsA {
		diffs = append(diffs, AnyEncDiff{
			Type: AnyEncDiffTypeRemove,
			Key:  key,
		})
	}
	return diffs, nil
}

func compareValue(a *anyenc.Value, b *anyenc.Value) (bool, error) {
	if a.Type() != b.Type() {
		// Return true, as we have checked that types are equal
		return false, nil
	}
	switch a.Type() {
	case anyenc.TypeNull:
		return true, nil
	case anyenc.TypeNumber:
		af, err := a.Float64()
		if err != nil {
			return false, fmt.Errorf("a: get float64: %w", err)
		}
		bf, err := b.Float64()
		if err != nil {
			return false, fmt.Errorf("b: get float64: %w", err)
		}
		return af == bf, nil
	case anyenc.TypeString:
		as, err := a.StringBytes()
		if err != nil {
			return false, fmt.Errorf("a: get string: %w", err)
		}
		bs, err := b.StringBytes()
		if err != nil {
			return false, fmt.Errorf("b: get string: %w", err)
		}
		return bytes.Compare(as, bs) == 0, nil
	case anyenc.TypeTrue, anyenc.TypeFalse:
		// Return true, as we have checked that types are equal
		return true, nil
	case anyenc.TypeArray:
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
	case anyenc.TypeObject:
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
		ao.Visit(func(k []byte, va *anyenc.Value) {
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

func AnyEncArrayToStrings(arr []*anyenc.Value) []string {
	res := make([]string, 0, len(arr))
	for _, v := range arr {
		res = append(res, string(v.GetStringBytes()))
	}
	return res
}

func StringsToAnyEnc(arena *anyenc.Arena, arr []string) *anyenc.Value {
	res := arena.NewArray()
	for i, v := range arr {
		res.SetArrayItem(i, arena.NewString(v))
	}
	return res
}
