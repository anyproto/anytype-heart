package slice

type DiffOperation int

const (
	OperationAdd     DiffOperation = iota
	OperationMove    DiffOperation = iota
	OperationRemove  DiffOperation = iota
	OperationReplace DiffOperation = iota
)

type Change struct {
	Op      DiffOperation
	Ids     []string
	AfterId string
}

func Diff(origin, changed []string) []Change {
	return []Change{{Op: OperationReplace, Ids: changed}}
}

func ApplyChanges(origin []string, change []Change) []string {
	for _, ch := range change {
		pos := 0
		if ch.AfterId != "" {
			pos = FindPos(origin, ch.AfterId)
		}

		switch ch.Op {
		case OperationAdd:
			Insert(origin,pos)
		case OperationMove:
			// TODO
		case OperationRemove:
			// TODO
		case OperationReplace:
			origin = ch.Ids
		}
	}

	return origin
}
