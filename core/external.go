package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/gallery"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/unsplash"
)

func (mw *Middleware) UnsplashSearch(cctx context.Context, req *pb.RpcUnsplashSearchRequest) *pb.RpcUnsplashSearchResponse {
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
	un := mw.applicationService.GetApp().Component(unsplash.CName).(unsplash.Unsplash)
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

func (mw *Middleware) UnsplashDownload(cctx context.Context, req *pb.RpcUnsplashDownloadRequest) *pb.RpcUnsplashDownloadResponse {
	response := func(objectId string, err error) *pb.RpcUnsplashDownloadResponse {
		m := &pb.RpcUnsplashDownloadResponse{
			Error:    &pb.RpcUnsplashDownloadResponseError{Code: pb.RpcUnsplashDownloadResponseError_NULL},
			ObjectId: objectId,
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

	var objectId string
	un := mw.applicationService.GetApp().Component(unsplash.CName).(unsplash.Unsplash)
	if un == nil {
		return response("", fmt.Errorf("node not started"))
	}
	imagePath, err := un.Download(context.TODO(), req.PictureId)
	if err != nil {
		return response("", err)
	}
	defer os.Remove(imagePath)

	err = mw.doBlockService(func(bs *block.Service) (err error) {
		objectId, err = bs.UploadFile(cctx, req.SpaceId, block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: imagePath,
				Type:      model.BlockContentFile_Image,
				Style:     model.BlockContentFile_Embed,
			},
		})
		if err != nil {
			return err
		}
		return
	})

	return response(objectId, err)
}

func (mw *Middleware) DownloadManifest(_ context.Context, req *pb.RpcDownloadManifestRequest) *pb.RpcDownloadManifestResponse {
	response := func(info *pb.RpcDownloadManifestResponseManifestInfo, err error) *pb.RpcDownloadManifestResponse {
		m := &pb.RpcDownloadManifestResponse{
			Error: &pb.RpcDownloadManifestResponseError{Code: pb.RpcDownloadManifestResponseError_NULL},
			Info:  info,
		}
		if err != nil {
			m.Error.Code = pb.RpcDownloadManifestResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	info, err := gallery.DownloadManifest(req.Url)
	return response(info, err)
}
