package change

type Change struct {
	Id          string
	PreviousIds []string
	Next        []*Change
	Active      bool
}
