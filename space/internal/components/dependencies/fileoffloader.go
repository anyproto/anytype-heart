package dependencies

import "context"

type FileOffloader interface {
	FilesSpaceOffload(ctx context.Context, spaceId string) (err error)
}
