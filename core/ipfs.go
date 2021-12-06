package core

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pb"
	pb2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pin"
)

func (mw *Middleware) FileListOffload(req *pb.RpcFileListOffloadRequest) *pb.RpcFileListOffloadResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()
	response := func(filesOffloaded int32, bytesOffloaded uint64, code pb.RpcFileListOffloadResponseErrorCode, err error) *pb.RpcFileListOffloadResponse {
		m := &pb.RpcFileListOffloadResponse{Error: &pb.RpcFileListOffloadResponseError{Code: code}, BytesOffloaded: bytesOffloaded, FilesOffloaded: filesOffloaded}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.app == nil {
		response(0, 0, pb.RpcFileListOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	at := mw.app.MustComponent(core.CName).(core.Service)
	pin := mw.app.MustComponent(pin.CName).(pin.FilePinService)

	if !at.IsStarted() {
		response(0, 0, pb.RpcFileListOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype node not started"))
	}

	files, err := at.FileStore().ListTargets()
	if err != nil {
		return response(0, 0, pb.RpcFileListOffloadResponseError_UNKNOWN_ERROR, err)
	}
	pinStatus := pin.PinStatus(files...)
	var (
		totalBytesOffloaded uint64
		totalFilesOffloaded int32
	)
	ds := mw.app.MustComponent(datastore.CName).(datastore.Datastore)
	for _, fileId := range files {
		if st, exists := pinStatus[fileId]; (!exists || st.Status != pb2.PinStatus_Done) && !req.IncludeNotPinned {
			continue
		}
		bytesRemoved, err := at.FileOffload(fileId)
		if err != nil {
			log.Errorf("failed to offload file %s: %s", fileId, err.Error())
			continue
		}
		if bytesRemoved > 0 {
			totalBytesOffloaded += bytesRemoved
			totalFilesOffloaded++
		}
	}

	freed, err := ds.RunBlockstoreGC()
	if err != nil {
		return response(0, 0, pb.RpcFileListOffloadResponseError_UNKNOWN_ERROR, err)
	}

	return response(totalFilesOffloaded, uint64(freed), pb.RpcFileListOffloadResponseError_NULL, nil)
}

func (mw *Middleware) FileOffload(req *pb.RpcFileOffloadRequest) *pb.RpcFileOffloadResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()
	response := func(bytesOffloaded uint64, code pb.RpcFileOffloadResponseErrorCode, err error) *pb.RpcFileOffloadResponse {
		m := &pb.RpcFileOffloadResponse{BytesOffloaded: bytesOffloaded, Error: &pb.RpcFileOffloadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.app == nil {
		response(0, pb.RpcFileOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	at := mw.app.MustComponent(core.CName).(core.Service)
	pin := mw.app.MustComponent(pin.CName).(pin.FilePinService)

	if !at.IsStarted() {
		response(0, pb.RpcFileOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype node not started"))
	}

	pinStatus := pin.PinStatus(req.Id)
	var (
		totalBytesOffloaded uint64
	)
	for fileId, status := range pinStatus {
		if status.Status != pb2.PinStatus_Done && !req.IncludeNotPinned {
			continue
		}
		bytesRemoved, err := at.FileOffload(fileId)
		if err != nil {
			log.Errorf("failed to offload file %s: %s", fileId, err.Error())
			continue
		}
		totalBytesOffloaded += bytesRemoved
	}

	return response(totalBytesOffloaded, pb.RpcFileOffloadResponseError_NULL, nil)
}
