package converter

import (
	"io"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// Functions to create in-tree and plugin converters
var converterCreators []ConverterCreator

// Function to register converter
type ConverterCreator = func(s core.Service) Converter

// RegisterFunc add converter creation function to converterCreators
func RegisterFunc(c ConverterCreator) {
	converterCreators = append(converterCreators, c)
}

// Converter incapsulate logic with transforming some data to smart blocks
type Converter interface {
	GetSnapshots(req *pb.RpcObjectImportRequest) *Response
	Name() string
}

// ImageGetter returns image for given converter in frontend
type ImageGetter interface {
	GetImage() ([]byte, int64, int64, error)
}

// IOReader combine name of the file and it's io reader
type IOReader struct {
	Name   string
	Reader io.ReadCloser
}
type Snapshot struct {
	Id       string
	FileName string
	Snapshot *model.SmartBlockSnapshotBase
}

// Response expected response of each converter, incapsulate blocks snapshots and converting errors
type Response struct {
	Snapshots []*Snapshot
	Error     ConvertError
}

func GetConverters() []func(s core.Service) Converter {
	return converterCreators
}
