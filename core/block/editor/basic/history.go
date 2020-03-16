package basic

type History interface {
	Undo() (err error)
	Redo() (err error)
}
