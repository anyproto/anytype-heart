package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"os"
	"strings"
)

func (mw *Middleware) UnsplashSearch(req *pb.RpcUnsplashSearchRequest) *pb.RpcUnsplashSearchResponse {
	response := func(resp []*pb.RpcUnsplashSearchResponsePicture, err error) *pb.RpcUnsplashSearchResponse {
		m := &pb.RpcUnsplashSearchResponse{
			Error:    &pb.RpcUnsplashSearchResponseError{Code: pb.RpcUnsplashSearchResponseError_NULL},
			Pictures: resp,
		}
		if err != nil {
			m.Error.Code = pb.RpcUnsplashSearchResponseError_UNKNOWN_ERROR
			if strings.Contains(err.Error(), "Rate limit exhausted") {
				m.Error.Code = pb.RpcUnsplashSearchResponseError_RATE_LIMIT_EXCEEDED
			}
			m.Error.Description = err.Error()
		}
		return m
	}
	var pictures []*pb.RpcUnsplashSearchResponsePicture
	err := mw.doBlockService(func(bs block.Service) (err error) {
		pictures, err = bs.UnsplashSearch(req.Query, int(req.Limit))
		return
	})

	return response(pictures, err)
}

func (mw *Middleware) UnsplashDownload(req *pb.RpcUnsplashDownloadRequest) *pb.RpcUnsplashDownloadResponse {
	response := func(hash string, err error) *pb.RpcUnsplashDownloadResponse {
		m := &pb.RpcUnsplashDownloadResponse{
			Error: &pb.RpcUnsplashDownloadResponseError{Code: pb.RpcUnsplashDownloadResponseError_NULL},
			Hash:  hash,
		}
		if err != nil {
			m.Error.Code = pb.RpcUnsplashDownloadResponseError_UNKNOWN_ERROR
			if strings.Contains(err.Error(), "Rate limit exhausted") {
				m.Error.Code = pb.RpcUnsplashDownloadResponseError_RATE_LIMIT_EXCEEDED
			}
			m.Error.Description = err.Error()
		}
		return m
	}

	var hash string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		imagePath, err := bs.UnsplashDownload(req.PictureId)
		if err != nil {
			return err
		}
		hash, err = bs.UploadFile(pb.RpcUploadFileRequest{
			LocalPath: imagePath,
			Type:      model.BlockContentFile_Image,
			Style:     model.BlockContentFile_Embed,
		})
		if err != nil {
			return err
		}
		_ = os.Remove(imagePath)
		return
	})

	return response(hash, err)
}
