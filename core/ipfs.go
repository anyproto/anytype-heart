package core

import (
	"io/ioutil"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pb"
	core2 "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/ipfs"
)

func (mw *Middleware) IpfsGetFile(req *pb.RpcIpfsFileGetRequest) *pb.RpcIpfsFileGetResponse {
	response := func(data []byte, media string, name string, code pb.RpcIpfsFileGetResponseErrorCode, err error) *pb.RpcIpfsFileGetResponse {
		m := &pb.RpcIpfsFileGetResponse{Data: data, Media: media, Error: &pb.RpcIpfsFileGetResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	reader, info, err := mw.Anytype.Textile.Node().FileContent(req.Id)
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, "", "", pb.RpcIpfsFileGetResponseError_NOT_FOUND, err)
		}

		return response(nil, "", "", pb.RpcIpfsFileGetResponseError_UNKNOWN_ERROR, err)
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return response(nil, "", "", pb.RpcIpfsFileGetResponseError_UNKNOWN_ERROR, err)
	}

	return response(data, info.Media, info.Name, pb.RpcIpfsFileGetResponseError_NULL, nil)
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

func (mw *Middleware) ImageGetBlob(req *pb.RpcIpfsImageGetBlobRequest) *pb.RpcIpfsImageGetBlobResponse {
	response := func(blob []byte, code pb.RpcIpfsImageGetBlobResponseErrorCode, err error) *pb.RpcIpfsImageGetBlobResponse {
		m := &pb.RpcIpfsImageGetBlobResponse{Blob: blob, Error: &pb.RpcIpfsImageGetBlobResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), req.Id+"/0/"+strings.ToLower(req.GetSize_().String())+"/content")
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}

	return response(data, pb.RpcIpfsImageGetBlobResponseError_NULL, nil)
}
