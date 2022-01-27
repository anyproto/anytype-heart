package core

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/unsplash"
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
	un := mw.app.Component(unsplash.CName).(unsplash.Unsplash)
	if un == nil {
		return response(nil, fmt.Errorf("node not started"))
	}

	results, err := un.Search(context.TODO(), req.Query, int(req.Limit))
	pictures := make([]*pb.RpcUnsplashSearchResponsePicture, 0, len(results))
	for _, res := range results {
		pictures = append(pictures, &pb.RpcUnsplashSearchResponsePicture{
			Id:        res.ID,
			Url:       res.PictureThumbUrl,
			Artist:    res.Artist,
			ArtistUrl: res.ArtistURL,
		})
	}
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
	un := mw.app.Component(unsplash.CName).(unsplash.Unsplash)
	if un == nil {
		return response("", fmt.Errorf("node not started"))
	}
	imagePath, err := un.Download(context.TODO(), req.PictureId)
	if err != nil {
		return response("", err)
	}
	defer os.Remove(imagePath)

	err = mw.doBlockService(func(bs block.Service) (err error) {
		hash, err = bs.UploadFile(pb.RpcUploadFileRequest{
			LocalPath: imagePath,
			Type:      model.BlockContentFile_Image,
			Style:     model.BlockContentFile_Embed,
		})
		if err != nil {
			return err
		}
		return
	})

	return response(hash, err)
}
