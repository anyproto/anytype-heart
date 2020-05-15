package slice

func FindPos(s []string, v string) int {
	for i, sv := range s {
		if sv == v {
			return i
		}
	}
	return -1
}

func Insert(s []string, v string, pos int) []string {
	if len(s) <= pos {
		return append(s, v)
	}
	if pos == 0 {
		return append([]string{v}, s[pos:]...)
	}
	return append(s[:pos], append([]string{v}, s[pos:]...)...)
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
