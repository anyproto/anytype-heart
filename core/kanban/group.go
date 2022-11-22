package kanban


type GroupSlice []Group

func(gs GroupSlice) Len() int {
	return len(gs)
}

func (gs GroupSlice) Less(i, j int) bool {
	return len(gs[i].Id) < len(gs[j].Id)
}

func (gs GroupSlice) Swap(i, j int) {
	gs[i], gs[j] = gs[j], gs[i]
}


type Group struct {
	Id string
	Data GroupData
}


type GroupData struct {
	Ids []string
}
