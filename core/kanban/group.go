package kanban

type Group struct {
	Id string
	Data GroupData
}

type GroupData struct {
	Ids []string
}