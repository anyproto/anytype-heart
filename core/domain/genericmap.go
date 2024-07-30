package domain

func (d *GenericMap[K]) Len() int {
	return len(d.data)
}

func (d *GenericMap[K]) Set(key K, value any) {
	d.data[key] = value
}

func (d *GenericMap[K]) Delete(key K) {
	delete(d.data, key)
}

func (d *GenericMap[K]) Iterate(proc func(K, any) bool) {
	for k, v := range d.data {
		if !proc(k, v) {
			return
		}
	}
}

func (d *GenericMap[K]) GetRaw(key K) (any, bool) {
	v, ok := d.data[key]
	return v, ok
}

func (d *GenericMap[K]) Get(key K) Value {
	v, ok := d.data[key]
	return Value{ok, v}
}

func (d *GenericMap[K]) Has(key K) bool {
	_, ok := d.data[key]
	return ok
}

func (d *GenericMap[K]) GetBool(key K) (bool, bool) {
	return d.Get(key).Bool()
}

func (d *GenericMap[K]) GetBoolOrDefault(key K, def bool) bool {
	return d.Get(key).BoolOrDefault(def)
}

func (d *GenericMap[K]) GetString(key K) (string, bool) {
	return d.Get(key).String()
}

func (d *GenericMap[K]) GetStringOrDefault(key K, def string) string {
	return d.Get(key).StringOrDefault(def)
}

func (d *GenericMap[K]) GetInt(key K) (int, bool) {
	return d.Get(key).Int()
}

func (d *GenericMap[K]) GetIntOrDefault(key K, def int) int {
	return d.Get(key).IntOrDefault(def)
}

func (d *GenericMap[K]) GetFloat(key K) (float64, bool) {
	return d.Get(key).Float()
}

func (d *GenericMap[K]) GetFloatOrDefault(key K, def float64) float64 {
	return d.Get(key).FloatOrDefault(def)
}

func (d *GenericMap[K]) GetStringList(key K) ([]string, bool) {
	return d.Get(key).StringList()
}

func (d *GenericMap[K]) GetStringListOrDefault(key K, def []string) []string {
	return d.Get(key).StringListOrDefault(def)
}

func (d *GenericMap[K]) GetIntList(key K) ([]int, bool) {
	return d.Get(key).IntList()
}

func (d *GenericMap[K]) GetIntListOrDefault(key K, def []int) []int {
	return d.Get(key).IntListOrDefault(def)
}

func (d *GenericMap[K]) ShallowCopy() *GenericMap[K] {
	newData := make(map[K]any, len(d.data))
	for k, v := range d.data {
		newData[k] = v
	}
	return &GenericMap[K]{data: newData}
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
