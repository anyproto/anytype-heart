package core

import (
	"io/ioutil"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pb"
	core2 "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/ipfs"
)

func (mw *Middleware) IpfsGetFile(req *pb.RpcIpfsGetFileRequest) *pb.RpcIpfsGetFileResponse {
	response := func(data []byte, media string, name string, code pb.RpcIpfsGetFileResponseErrorCode, err error) *pb.RpcIpfsGetFileResponse {
		m := &pb.RpcIpfsGetFileResponse{Data: data, Media: media, Error: &pb.RpcIpfsGetFileResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	reader, info, err := mw.Anytype.Textile.Node().FileContent(req.Id)
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, "", "", pb.RpcIpfsGetFileResponseError_NOT_FOUND, err)
		}

		return response(nil, "", "", pb.RpcIpfsGetFileResponseError_UNKNOWN_ERROR, err)
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return response(nil, "", "", pb.RpcIpfsGetFileResponseError_UNKNOWN_ERROR, err)
	}

	return response(data, info.Media, info.Name, pb.RpcIpfsGetFileResponseError_NULL, nil)
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
		if err == core2.ErrFileNotFound {
			return response(nil, pb.RpcIpfsGetDataResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsGetDataResponseError_UNKNOWN_ERROR, err)
	}

	return response(data, pb.RpcIpfsGetDataResponseError_NULL, nil)
}

*/

func (mw *Middleware) ImageGetBlob(req *pb.RpcImageGetBlobRequest) *pb.RpcImageGetBlobResponse {
	response := func(blob []byte, code pb.RpcImageGetBlobResponseErrorCode, err error) *pb.RpcImageGetBlobResponse {
		m := &pb.RpcImageGetBlobResponse{Blob: blob, Error: &pb.RpcImageGetBlobResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), req.Id+"/0/"+strings.ToLower(req.GetSize_().String())+"/content")
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, pb.RpcImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}

	return response(data, pb.RpcImageGetBlobResponseError_NULL, nil)
}
