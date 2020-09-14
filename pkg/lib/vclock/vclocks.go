package vclock

type VClocks []VClock

func (vcs VClocks) Len() int {
	return len(vcs)
}

func (vcs VClocks) Less(i, j int) bool {
	if vcs[i].Compare(vcs[j], Descendant) {
		return true
	}

	return false
}

func (vcs VClocks) Swap(i, j int) {
	vcs[i], vcs[j] = vcs[j], vcs[i]
}
