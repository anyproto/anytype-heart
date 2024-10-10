package kanban

type GroupSlice []Group

func (gs GroupSlice) Len() int {
	return len(gs)
}

func (gs GroupSlice) Less(i, j int) bool {
	return len(gs[i].Id) > len(gs[j].Id)
}

func (gs GroupSlice) Swap(i, j int) {
	gs[i], gs[j] = gs[j], gs[i]
}

type Group struct {
	Id   string
	Data GroupData
}

type GroupData struct {
	Ids []string
}

type GroupCounts []*GroupCount

func (gc GroupCounts) Len() int {
	return len(gc)
}

func (gc GroupCounts) Less(i, j int) bool {
	return gc[i].Count > gc[j].Count
}

func (gc GroupCounts) Swap(i, j int) {
	gc[i], gc[j] = gc[j], gc[i]
}

type GroupCount struct {
	Group
	Count int
}
