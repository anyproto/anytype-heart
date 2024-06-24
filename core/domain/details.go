package domain

type Details struct {
	data map[RelationKey]any
}

func (d *Details) Set(key RelationKey, value any) {
	d.data[key] = value
}

func (d *Details) Delete(key RelationKey) {
	delete(d.data, key)
}

func (d *Details) GetRaw(key RelationKey) (any, bool) {
	v, ok := d.data[key]
	return v, ok
}

func (d *Details) Get(key RelationKey) Value {
	v, ok := d.data[key]
	return Value{ok, v}
}

func (d *Details) Has(key RelationKey) bool {
	_, ok := d.data[key]
	return ok
}

func (d *Details) GetBool(key RelationKey) (bool, bool) {
	return d.Get(key).Bool()
}

func (d *Details) GetBoolOrDefault(key RelationKey, def bool) bool {
	return d.Get(key).BoolOrDefault(def)
}

func (d *Details) GetString(key RelationKey) (string, bool) {
	return d.Get(key).String()
}

func (d *Details) GetStringOrDefault(key RelationKey, def string) string {
	return d.Get(key).StringOrDefault(def)
}

func (d *Details) GetInt(key RelationKey) (int, bool) {
	return d.Get(key).Int()
}

func (d *Details) GetIntOrDefault(key RelationKey, def int) int {
	return d.Get(key).IntOrDefault(def)
}

func (d *Details) GetFloat(key RelationKey) (float64, bool) {
	return d.Get(key).Float()
}

func (d *Details) GetFloatOrDefault(key RelationKey, def float64) float64 {
	return d.Get(key).FloatOrDefault(def)
}

func (d *Details) GetStringList(key RelationKey) ([]string, bool) {
	return d.Get(key).StringList()
}

func (d *Details) GetStringListOrDefault(key RelationKey, def []string) []string {
	return d.Get(key).StringListOrDefault(def)
}

func (d *Details) GetIntList(key RelationKey) ([]int, bool) {
	return d.Get(key).IntList()
}

func (d *Details) GetIntListOrDefault(key RelationKey, def []int) []int {
	return d.Get(key).IntListOrDefault(def)
}

type Value struct {
	ok    bool
	value any
}

func (v Value) Ok() bool {
	return v.ok
}

func (v Value) Bool() (bool, bool) {
	if !v.ok {
		return false, false
	}
	b, ok := v.value.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func (v Value) BoolOrDefault(def bool) bool {
	res, ok := v.Bool()
	if !ok {
		return def
	}
	return res
}

func (v Value) String() (string, bool) {
	if !v.ok {
		return "", false
	}
	s, ok := v.value.(string)
	return s, ok
}

func (v Value) StringOrDefault(def string) string {
	res, ok := v.String()
	if !ok {
		return def
	}
	return res
}

func (v Value) Int() (int, bool) {
	if !v.ok {
		return 0, false
	}
	switch v := v.value.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func (v Value) IntOrDefault(def int) int {
	res, ok := v.Int()
	if !ok {
		return def
	}
	return res
}

func (v Value) Float() (float64, bool) {
	if !v.ok {
		return 0, false
	}
	switch v := v.value.(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func (v Value) FloatOrDefault(def float64) float64 {
	res, ok := v.Float()
	if !ok {
		return def
	}
	return res
}

func (v Value) StringList() ([]string, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]string)
	return l, ok
}

func (v Value) StringListOrDefault(def []string) []string {
	res, ok := v.StringList()
	if !ok {
		return def
	}
	return res
}

func (v Value) IntList() ([]int, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]int)
	return l, ok
}

func (v Value) IntListOrDefault(def []int) []int {
	res, ok := v.IntList()
	if !ok {
		return def
	}
	return res
}
