package slice

import (
	"hash/fnv"
	"iter"
	"math/rand"
	"sort"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
)

func Union(a, b []string) []string {
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; !ok {
			a = append(a, v)
		}
	}
	return a
}

func DifferenceRemovedAdded(a, b []string) (removed []string, added []string) {
	var amap = map[string]struct{}{}
	var bmap = map[string]struct{}{}

	for _, item := range a {
		amap[item] = struct{}{}
	}

	for _, item := range b {
		if _, exists := amap[item]; !exists {
			added = append(added, item)
		}
		bmap[item] = struct{}{}
	}

	for _, item := range a {
		if _, exists := bmap[item]; !exists {
			removed = append(removed, item)
		}
	}
	return
}

func FindPos[T comparable](s []T, v T) int {
	for i, sv := range s {
		if sv == v {
			return i
		}
	}
	return -1
}

func Find[T comparable](s []T, cond func(T) bool) int {
	for i, sv := range s {
		if cond(sv) {
			return i
		}
	}
	return -1
}

// Difference returns the elements in `a` that aren't in `b`.
func Difference(a, b []string) []string {
	var diff = make([]string, 0, len(a))
	for _, a1 := range a {
		if FindPos(b, a1) == -1 {
			diff = append(diff, a1)
		}
	}
	return diff
}

func Insert[T any](s []T, pos int, v ...T) []T {
	if len(s) <= pos {
		return append(s, v...)
	}
	if pos == 0 {
		return append(v, s[pos:]...)
	}
	return append(s[:pos], append(v, s[pos:]...)...)
}

// RemoveMut reuses provided slice capacity. Provided s slice should not be used after without reassigning to the func return!
func RemoveMut[T comparable](s []T, v T) []T {
	var n int
	for _, x := range s {
		if x != v {
			s[n] = x
			n++
		}
	}
	return s[:n]
}

// Remove is an immutable analog of RemoveMut function. Input slice is copied and then modified
func Remove[T comparable](s []T, v T) []T {
	var n int
	sc := slices.Clone(s)
	for _, x := range s {
		if x != v {
			sc[n] = x
			n++
		}
	}
	return sc[:n]
}

// RemoveIndex reuses provided slice capacity. Provided s slice should not be used after without reassigning to the func return!
func RemoveIndex[T any](s []T, idx int) []T {
	var n int
	for i, x := range s {
		if i != idx {
			s[n] = x
			n++
		}
	}
	return s[:n]
}

// RemoveN is an analog of Remove function, but removing a range of values
func RemoveN[T comparable](s []T, vs ...T) []T {
	var (
		n     int
		found bool
		sc    = slices.Clone(s)
	)
	if len(vs) == 0 {
		return sc
	}
	for _, x := range s {
		found = false
		for _, v := range vs {
			if x == v {
				found = true
				break
			}
		}
		if !found {
			sc[n] = x
			n++
		}
	}
	return sc[:n]
}

func Filter[T any](vals []T, cond func(T) bool) []T {
	var result = make([]T, 0, len(vals))
	for i := range vals {
		if cond(vals[i]) {
			result = append(result, vals[i])
		}
	}
	return result
}

func FilterMut[T any](vals []T, cond func(T) bool) []T {
	result := vals[:0]
	for i := range vals {
		if cond(vals[i]) {
			result = append(result, vals[i])
		}
	}
	return result
}

func GetRandomString(s []string, seed string) string {
	rand.Seed(int64(hash(seed)))
	return s[rand.Intn(len(s))]
}

func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func SortedEquals[K constraints.Ordered](s1, s2 []K) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func UnsortedEqual[K constraints.Ordered](s1, s2 []K) bool {
	if len(s1) != len(s2) {
		return false
	}

	s1Sorted := make([]K, len(s1))
	s2Sorted := make([]K, len(s2))
	copy(s1Sorted, s1)
	copy(s2Sorted, s2)
	slices.Sort(s1Sorted)
	slices.Sort(s2Sorted)

	return SortedEquals(s1Sorted, s2Sorted)
}

func HasPrefix(value, prefix []string) bool {
	if len(value) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if value[i] != p {
			return false
		}
	}
	return true
}

func Copy[T any](list []T) []T {
	newList := make([]T, len(list))
	copy(newList, list)
	return newList
}

func Intersection(a, b []string) (res []string) {
	sort.Strings(a)
	sort.Strings(b)
	aIdx := 0
	bIdx := 0
	for aIdx < len(a) && bIdx < len(b) {
		cmp := strings.Compare(a[aIdx], b[bIdx])
		switch cmp {
		case 0:
			res = append(res, a[aIdx])
			aIdx++
			bIdx++
		case -1:
			aIdx++
		case 1:
			bIdx++
		}
	}
	return
}

func ReplaceFirstBy[T comparable](s []T, el T, pred func(el T) bool) []T {
	for i, el2 := range s {
		if pred(el2) {
			s[i] = el
			break
		}
	}
	return s
}

func FilterCID(cids []string) []string {
	return lo.Filter(cids, func(item string, index int) bool {
		_, err := cid.Parse(item)
		return err == nil
	})
}

func StringsInto[T ~string](from []string) []T {
	to := make([]T, len(from))
	for i, v := range from {
		to[i] = T(v)
	}
	return to
}

func IntoStrings[T ~string](from []T) []string {
	to := make([]string, len(from))
	for i, v := range from {
		to[i] = string(v)
	}
	return to
}

type numeric interface {
	constraints.Integer | constraints.Float
}

func FloatsInto[T numeric](from []float64) []T {
	to := make([]T, len(from))
	for i, v := range from {
		to[i] = T(v)
	}
	return to
}

// MergeUniqBy merges two slices with comparator. Resulting slice saves values' order and uniqueness.
// Input slices MUST contain only unique values
func MergeUniqBy[T comparable](s1, s2 []T, equal func(v1, v2 T) bool) (result []T) {
	result = make([]T, len(s1))
	copy(result, s1)
	for _, v2 := range s2 {
		if !slices.ContainsFunc(s1, func(v1 T) bool {
			return equal(v1, v2)
		}) {
			result = append(result, v2)
		}
	}

	return result
}

// ContainsBySeq searches for value v1 using iterator over sequence seq
func ContainsBySeq[T comparable](seq iter.Seq[T], v1 T) (found bool) {
	seq(func(v2 T) bool {
		if v1 == v2 {
			found = true
			return false
		}
		return true
	})
	return
}

// DeleteOrApplyFunc deletes elements that satisfy del predicate, otherwise apply func
func DeleteOrApplyFunc[S ~[]E, E any](s S, del func(E) bool, apply func(E) E) S {
	i := 0
	for j := 0; j < len(s); j++ {
		if v := s[j]; !del(v) {
			s[i] = apply(v)
			i++
		}
	}
	clear(s[i:])
	return s[:i]
}
