package source

import (
	"github.com/anytypeio/go-anytype-library/core"
)

type Source interface {
	ReadVersion() (*core.SmartBlockVersion, error)
	WriteVersion(v *core.SmartBlockVersion) (err error)
	Close() (err error)
}
