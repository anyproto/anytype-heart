package block

func findPosInSlice(s []string, v string) int {
	for i, sv := range s {
		if sv == v {
			return i
		}
	}
	return -1
}

func insertToSlice(s []string, v string, pos int) []string {
	if len(s) <= pos {
		return append(s, v)
	}
	if pos == 0 {
		return append([]string{v}, s[pos:]...)
	}
	return append(s[:pos], append([]string{v}, s[pos:]...)...)
}
