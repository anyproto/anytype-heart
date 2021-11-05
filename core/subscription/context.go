package subscription

type opChange struct {
	id    string
	subId string
	keys  []string
}

type opRemove struct {
	opCounter
	id string
}

type opPosition struct {
	id      string
	subId   string
	afterId string
}

type opCounter struct {
	subId     string
	total     int
	prevCount int
	nextCount int
}

type opCtx struct {
	// subIds for remove
	remove   []opRemove
	change   []opChange
	add      []opChange
	position []opPosition
	counters []opCounter
}
