package core

import (
	"io/ioutil"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pb"
	core2 "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/ipfs"
)

func (mw *Middleware) IpfsGetFile(req *pb.IpfsGetFileRequest) *pb.IpfsGetFileResponse {
	response := func(data []byte, media string, name string, code pb.IpfsGetFileResponse_Error_Code, err error) *pb.IpfsGetFileResponse {
		m := &pb.IpfsGetFileResponse{Data: data, Media: media, Error: &pb.IpfsGetFileResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	reader, info, err := mw.Anytype.Textile.Node().FileContent(req.Id)
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, "", "", pb.IpfsGetFileResponse_Error_NOT_FOUND, err)
		}

		return response(nil, "", "", pb.IpfsGetFileResponse_Error_UNKNOWN_ERROR, err)
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return response(nil, "", "", pb.IpfsGetFileResponse_Error_UNKNOWN_ERROR, err)
	}

	return response(data, info.Media, info.Name, pb.IpfsGetFileResponse_Error_NULL, nil)
}

/*
//exportMobile IpfsGetData
func IpfsGetData(b []byte) []byte {
	response := func(data []byte, code pb.IpfsGetDataResponse_Error_Code, err error) []byte {
		m := &pb.IpfsGetDataResponse{Data: data, Error: &pb.IpfsGetDataResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.IpfsGetDataRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, pb.IpfsGetDataResponse_Error_BAD_INPUT, err)
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), q.Id)
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, pb.IpfsGetDataResponse_Error_NOT_FOUND, err)
		}

		return response(nil, pb.IpfsGetDataResponse_Error_UNKNOWN_ERROR, err)
	}

	return response(data, pb.IpfsGetDataResponse_Error_NULL, nil)
}

*/

func (mw *Middleware) ImageGetBlob(req *pb.ImageGetBlobRequest) *pb.ImageGetBlobResponse {
	response := func(blob []byte, code pb.ImageGetBlobResponse_Error_Code, err error) *pb.ImageGetBlobResponse {
		m := &pb.ImageGetBlobResponse{Blob: blob, Error: &pb.ImageGetBlobResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), req.Id+"/0/"+strings.ToLower(req.GetSize_().String())+"/content")
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, pb.ImageGetBlobResponse_Error_NOT_FOUND, err)
		}

		return response(nil, pb.ImageGetBlobResponse_Error_UNKNOWN_ERROR, err)
	}

	return response(data, pb.ImageGetBlobResponse_Error_NULL, nil)
}
