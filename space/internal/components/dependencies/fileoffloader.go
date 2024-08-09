package dependencies

import "context"

type FileOffloader interface {
	FileSpaceOffload(ctx context.Context, spaceId string, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error)
}
