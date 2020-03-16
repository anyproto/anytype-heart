package file

import "github.com/anytypeio/go-anytype-middleware/pb"

type File interface {
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)
	Upload(id string, localPath, url string) error
}
