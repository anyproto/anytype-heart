package storestate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/pb"
)

const ordersKey = "_o"

func makeModifier(ch ChangeOp, h Handler) (modifier query.Modifier, err error) {
	m := ch.Change.Change.GetModify()
	chain := make(query.ModifierChain, 0, len(m.Keys))
	newModifier := func(mKey *pb.KeyModify, modOp string, val *fastjson.Value) (query.Modifier, error) {
		modJSON := ch.Arena.NewObject()
		valJSON := ch.Arena.NewObject()
		valJSON.Set(strings.Join(mKey.KeyPath, "."), val)
		modJSON.Set(modOp, valJSON)
		anyMod, mErr := query.ParseModifier(modJSON)
		if mErr != nil {
			return nil, mErr
		}
		mod := query.ModifyFunc(func(a *fastjson.Arena, v *fastjson.Value) (result *fastjson.Value, modified bool, err error) {
			if curOrder := getFieldOrder(val, mKey.KeyPath...); curOrder != "" && curOrder >= ch.Change.Order {
				return v, false, nil
			}
			result, modified, err = anyMod.Modify(a, v)
			if err == nil && modified {
				setFieldOrder(a, result, ch.Change.Order, mKey.KeyPath...)
			}
			return
		})
		ch.Value = val
		return h.UpgradeKeyModifier(ch, mKey, mod), nil
	}

	for _, mKey := range m.Keys {
		var (
			val   *fastjson.Value
			mod   query.Modifier
			modOp string
		)
		if len(mKey.KeyPath) == 0 {
			return nil, errors.Join()
		}
		if mKey.ModifyValue != "" {
			if val, err = fastjson.Parse(mKey.ModifyValue); err != nil {
				return nil, err
			}
		}
		switch mKey.ModifyOp {
		case pb.ModifyOp_Set:
			modOp = "$set"
		case pb.ModifyOp_Unset:
			val = ch.Arena.NewTrue()
			modOp = "$unset"
		case pb.ModifyOp_Inc:
			if val == nil || val.Type() != fastjson.TypeNumber {
				return nil, fmt.Errorf("unexpected value for $inc %v: '%s'", mKey.KeyPath, mKey.ModifyValue)
			}
			modOp = "$inc"
		case pb.ModifyOp_AddToSet:
			modOp = "$addToSet"
		case pb.ModifyOp_Pull:
			modOp = "$pull"
		default:
			return nil, fmt.Errorf("unexpected modify op: '%v", mKey.ModifyOp)
		}

		if val == nil {
			return nil, fmt.Errorf("no value for modifier: %v", mKey.KeyPath)
		}
		if mod, err = newModifier(mKey, modOp, val); err != nil {
			return
		}
		chain = append(chain, mod)
	}
	return chain, nil
}

func fillRootOrder(a *fastjson.Arena, v *fastjson.Value, order string) {
	val := a.NewObject()
	v.Set(ordersKey, val)
	iterateKeysByPath(v, func(k string) {
		if k != ordersKey {
			val.Set(k, a.NewString(order))
		}
	})
}

func getFieldOrder(v *fastjson.Value, fieldPath ...string) (order string) {
	obj := v.GetObject(ordersKey)
	if obj == nil {
		return
	}
	for _, field := range fieldPath {
		val := obj.Get(field)
		if val == nil {
			return
		}
		switch val.Type() {
		case fastjson.TypeObject:
			obj, _ = val.Object()
			continue
		case fastjson.TypeString:
			return string(val.GetStringBytes())
		default:
			return
		}
	}
	return
}

func setFieldOrder(a *fastjson.Arena, v *fastjson.Value, order string, fieldPath ...string) {
	val := v.Get(ordersKey)
	if val == nil || val.Type() != fastjson.TypeObject {
		val = a.NewObject()
		v.Set(ordersKey, val)
	}
	for i, field := range fieldPath {
		if i == len(fieldPath)-1 {
			// it's a last element in the path - set order anyway
			val.Set(field, a.NewString(order))
			return
		}
		fieldVal := val.Get(field)
		if fieldVal == nil || (fieldVal.Type() != fastjson.TypeObject && fieldVal.Type() != fastjson.TypeString) {
			fieldVal = a.NewObject()
		}
		switch fieldVal.Type() {
		case fastjson.TypeObject:
			val.Set(field, fieldVal)
			val = fieldVal
			continue
		case fastjson.TypeString:
			prevOrder := string(fieldVal.GetStringBytes())
			fieldVal = a.NewObject()
			val.Set(field, fieldVal)
			val = fieldVal
			iterateKeysByPath(v, func(k string) {
				if k != field {
					val.Set(k, a.NewString(prevOrder))
				}
			}, fieldPath[:i+1]...)
		}
	}
}

func iterateKeysByPath(v *fastjson.Value, f func(k string), fieldPath ...string) {
	if obj := v.GetObject(fieldPath...); obj != nil {
		obj.Visit(func(key []byte, v *fastjson.Value) {
			f(string(key))
		})
	}
}