package lib

import (
	"io/ioutil"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	core2 "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/ipfs"
)

func IpfsGetFile(b []byte) []byte {
	response := func(data []byte, media string, name string, code pb.IpfsGetFileResponse_Error_Code, err error) []byte {
		m := &pb.IpfsGetFileResponse{Data: data, Media: media, Error: &pb.IpfsGetFileResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.IpfsGetFileRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, "", "", pb.IpfsGetFileResponse_Error_BAD_INPUT, err)
	}

	reader, info, err := mw.Anytype.Textile.Node().FileContent(q.Id)
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

func ImageGetBlob(b []byte) []byte {
	response := func(blob []byte, code pb.ImageGetBlobResponse_Error_Code, err error) []byte {
		m := &pb.ImageGetBlobResponse{Blob: blob, Error: &pb.ImageGetBlobResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.ImageGetBlobRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, pb.ImageGetBlobResponse_Error_BAD_INPUT, err)
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), q.Id+"/0/"+strings.ToLower(q.Size.String())+"/content")
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, pb.ImageGetBlobResponse_Error_NOT_FOUND, err)
		}

		return response(nil, pb.ImageGetBlobResponse_Error_UNKNOWN_ERROR, err)
	}

	return response(data, pb.ImageGetBlobResponse_Error_NULL, nil)
}
