package core

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) IpfsGetFile(req *pb.RpcIpfsFileGetRequest) *pb.RpcIpfsFileGetResponse {
	return nil
	/*response := func(data []byte, media string, name string, code pb.RpcIpfsFileGetResponseErrorCode, err error) *pb.RpcIpfsFileGetResponse {
		m := &pb.RpcIpfsFileGetResponse{Data: data, Media: media, Error: &pb.RpcIpfsFileGetResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	reader, info, err := mw.Anytype.Textile.Node().FileContent(req.Id)
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, "", "", pb.RpcIpfsFileGetResponseError_NOT_FOUND, err)
		}

		return response(nil, "", "", pb.RpcIpfsFileGetResponseError_UNKNOWN_ERROR, err)
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return response(nil, "", "", pb.RpcIpfsFileGetResponseError_UNKNOWN_ERROR, err)
	}

	return response(data, info.Media, info.Name, pb.RpcIpfsFileGetResponseError_NULL, nil)*/
}

/*
//exportMobile IpfsGetData
func IpfsGetData(b []byte) []byte {
	response := func(data []byte, code pb.RpcIpfsGetDataResponseErrorCode, err error) []byte {
		m := &pb.RpcIpfsGetDataResponse{Data: data, Error: &pb.RpcIpfsGetDataResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.RpcIpfsGetDataRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, pb.RpcIpfsGetDataResponseError_BAD_INPUT, err)
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), q.Id)
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsGetDataResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsGetDataResponseError_UNKNOWN_ERROR, err)
	}

	return response(data, pb.RpcIpfsGetDataResponseError_NULL, nil)
}

*/

func (mw *Middleware) ImageGetBlob(req *pb.RpcIpfsImageGetBlobRequest) *pb.RpcIpfsImageGetBlobResponse {
	response := func(blob []byte, code pb.RpcIpfsImageGetBlobResponseErrorCode, err error) *pb.RpcIpfsImageGetBlobResponse {
		m := &pb.RpcIpfsImageGetBlobResponse{Blob: blob, Error: &pb.RpcIpfsImageGetBlobResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.Anytype == nil {
		response(nil, pb.RpcIpfsImageGetBlobResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	if !mw.Anytype.IsStarted() {
		response(nil, pb.RpcIpfsImageGetBlobResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype node not started"))
	}

	image, err := mw.Anytype.ImageByHash(context.TODO(), req.GetHash())
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}
	file, err := image.GetFileForWidth(context.TODO(), int(req.WantWidth))
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}

	rd, err := file.Reader()
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}
	return response(data, pb.RpcIpfsImageGetBlobResponseError_NULL, nil)
}
