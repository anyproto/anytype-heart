package core

import (
	"io/ioutil"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pb"
	core2 "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/ipfs"
)

func (mw *Middleware) IpfsGetFile(req *pb.Rpc_Ipfs_Get_File_Request) *pb.Rpc_Ipfs_Get_File_Response {
	response := func(data []byte, media string, name string, code pb.Rpc_Ipfs_Get_File_Response_Error_Code, err error) *pb.Rpc_Ipfs_Get_File_Response {
		m := &pb.Rpc_Ipfs_Get_File_Response{Data: data, Media: media, Error: &pb.Rpc_Ipfs_Get_File_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	reader, info, err := mw.Anytype.Textile.Node().FileContent(req.Id)
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, "", "", pb.Rpc_Ipfs_Get_File_Response_Error_NOT_FOUND, err)
		}

		return response(nil, "", "", pb.Rpc_Ipfs_Get_File_Response_Error_UNKNOWN_ERROR, err)
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return response(nil, "", "", pb.Rpc_Ipfs_Get_File_Response_Error_UNKNOWN_ERROR, err)
	}

	return response(data, info.Media, info.Name, pb.Rpc_Ipfs_Get_File_Response_Error_NULL, nil)
}

/*
//exportMobile IpfsGetData
func IpfsGetData(b []byte) []byte {
	response := func(data []byte, code pb.Rpc_Ipfs_GetData_Response_Error_Code, err error) []byte {
		m := &pb.Rpc_Ipfs_GetData_Response{Data: data, Error: &pb.Rpc_Ipfs_GetData_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.Rpc_Ipfs_GetData_Request
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, pb.Rpc_Ipfs_GetData_Response_Error_BAD_INPUT, err)
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), q.Id)
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, pb.Rpc_Ipfs_GetData_Response_Error_NOT_FOUND, err)
		}

		return response(nil, pb.Rpc_Ipfs_GetData_Response_Error_UNKNOWN_ERROR, err)
	}

	return response(data, pb.Rpc_Ipfs_GetData_Response_Error_NULL, nil)
}

*/

func (mw *Middleware) ImageGetBlob(req *pb.Rpc_Image_Get_Blob_Request) *pb.Rpc_Image_Get_Blob_Response {
	response := func(blob []byte, code pb.Rpc_Image_Get_Blob_Response_Error_Code, err error) *pb.Rpc_Image_Get_Blob_Response {
		m := &pb.Rpc_Image_Get_Blob_Response{Blob: blob, Error: &pb.Rpc_Image_Get_Blob_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	data, err := ipfs.DataAtPath(mw.Anytype.Textile.Node().Ipfs(), req.Id+"/0/"+strings.ToLower(req.GetSize_().String())+"/content")
	if err != nil {
		if err == core2.ErrFileNotFound {
			return response(nil, pb.Rpc_Image_Get_Blob_Response_Error_NOT_FOUND, err)
		}

		return response(nil, pb.Rpc_Image_Get_Blob_Response_Error_UNKNOWN_ERROR, err)
	}

	return response(data, pb.Rpc_Image_Get_Blob_Response_Error_NULL, nil)
}
