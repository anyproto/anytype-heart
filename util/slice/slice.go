package slice

import (
	"hash/fnv"
	"math/rand"
	"sort"
)

func FindPos(s []string, v string) int {
	for i, sv := range s {
		if sv == v {
			return i
		}
	}
	return -1
}

func Insert(s []string, pos int, v ...string) []string {
	if len(s) <= pos {
		return append(s, v...)
	}
	if pos == 0 {
		return append(v, s[pos:]...)
	}
	return append(s[:pos], append(v, s[pos:]...)...)
}

func Remove(s []string, v string) []string {
	var n int
	for _, x := range s {
		if x != v {
			s[n] = x
			n++
		}
	}
	return s[:n]
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

func SortedEquals(s1, s2 []string) bool {
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

func UnsortedEquals(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}

	s1Sorted := make([]string, len(s1))
	s2Sorted := make([]string, len(s2))
	copy(s1, s1Sorted)
	copy(s2, s2Sorted)
	sort.Strings(s1Sorted)
	sort.Strings(s2Sorted)

	return SortedEquals(s1Sorted, s2Sorted)
}
