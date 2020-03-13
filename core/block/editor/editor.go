package editor

type Document interface {
	Init() (err error)
	Open() (err error)
	Show() (err error)
	Close() (err error)
}
