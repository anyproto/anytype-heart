package clientdebugrpc

import (
	"context"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/debug/clientdebugrpc/clientdebugrpcproto"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
	"io"
	"os"
)

type rpcHandler struct {
	spaceService   space.Service
	storageService storage.ClientStorage
	blockService   *block.Service
	account        accountservice.Service
	file           fileservice.FileService
}

func (r *rpcHandler) Watch(ctx context.Context, request *clientdebugrpcproto.WatchRequest) (resp *clientdebugrpcproto.WatchResponse, err error) {
	space, err := r.spaceService.GetSpace(context.Background(), request.SpaceId)
	if err != nil {
		return
	}
	watcher := space.SyncStatus().(syncstatus.StatusWatcher)
	watcher.Watch(request.TreeId)
	resp = &clientdebugrpcproto.WatchResponse{}
	return
}

func (r *rpcHandler) Unwatch(ctx context.Context, request *clientdebugrpcproto.UnwatchRequest) (resp *clientdebugrpcproto.UnwatchResponse, err error) {
	space, err := r.spaceService.GetSpace(context.Background(), request.SpaceId)
	if err != nil {
		return
	}
	watcher := space.SyncStatus().(syncstatus.StatusWatcher)
	watcher.Unwatch(request.TreeId)
	resp = &clientdebugrpcproto.UnwatchResponse{}
	return
}

func (r *rpcHandler) LoadSpace(ctx context.Context, request *clientdebugrpcproto.LoadSpaceRequest) (resp *clientdebugrpcproto.LoadSpaceResponse, err error) {
	_, err = r.spaceService.GetSpace(context.Background(), request.SpaceId)
	if err != nil {
		return
	}
	resp = &clientdebugrpcproto.LoadSpaceResponse{}
	return
}

func (r *rpcHandler) CreateSpace(ctx context.Context, request *clientdebugrpcproto.CreateSpaceRequest) (resp *clientdebugrpcproto.CreateSpaceResponse, err error) {
	panic("not implemented")
	return
}

func (r *rpcHandler) DeriveSpace(ctx context.Context, request *clientdebugrpcproto.DeriveSpaceRequest) (resp *clientdebugrpcproto.DeriveSpaceResponse, err error) {
	panic("not implemented")
	return
}

func (r *rpcHandler) CreateDocument(ctx context.Context, request *clientdebugrpcproto.CreateDocumentRequest) (resp *clientdebugrpcproto.CreateDocumentResponse, err error) {
	panic("not implemented")
	return
}

func (r *rpcHandler) DeleteDocument(ctx context.Context, request *clientdebugrpcproto.DeleteDocumentRequest) (resp *clientdebugrpcproto.DeleteDocumentResponse, err error) {
	panic("not implemented")
	return
}

func (r *rpcHandler) AddText(ctx context.Context, request *clientdebugrpcproto.AddTextRequest) (resp *clientdebugrpcproto.AddTextResponse, err error) {
	panic("not implemented")
	return
}

func (r *rpcHandler) DumpTree(ctx context.Context, request *clientdebugrpcproto.DumpTreeRequest) (resp *clientdebugrpcproto.DumpTreeResponse, err error) {
	tr, err := r.blockService.GetTree(ctx, request.SpaceId, request.DocumentId)
	if err != nil {
		return
	}
	dump, err := tr.DebugDump(state.ChangeParser{})
	if err != nil {
		return
	}
	resp = &clientdebugrpcproto.DumpTreeResponse{
		Dump: dump,
	}
	return
}

func (r *rpcHandler) AllTrees(ctx context.Context, request *clientdebugrpcproto.AllTreesRequest) (resp *clientdebugrpcproto.AllTreesResponse, err error) {
	sp, err := r.spaceService.GetSpace(ctx, request.SpaceId)
	if err != nil {
		return
	}
	heads := sp.DebugAllHeads()
	var trees []*clientdebugrpcproto.Tree
	for _, head := range heads {
		trees = append(trees, &clientdebugrpcproto.Tree{
			Id:    head.Id,
			Heads: head.Heads,
		})
	}
	resp = &clientdebugrpcproto.AllTreesResponse{Trees: trees}
	return
}

func (r *rpcHandler) AllSpaces(ctx context.Context, request *clientdebugrpcproto.AllSpacesRequest) (resp *clientdebugrpcproto.AllSpacesResponse, err error) {
	ids, err := r.storageService.AllSpaceIds()
	if err != nil {
		return
	}
	resp = &clientdebugrpcproto.AllSpacesResponse{SpaceIds: ids}
	return
}

func (r *rpcHandler) TreeParams(ctx context.Context, request *clientdebugrpcproto.TreeParamsRequest) (resp *clientdebugrpcproto.TreeParamsResponse, err error) {
	tr, err := r.blockService.GetTree(ctx, request.SpaceId, request.DocumentId)
	if err != nil {
		return
	}
	resp = &clientdebugrpcproto.TreeParamsResponse{
		RootId:  tr.Root().Id,
		HeadIds: tr.Heads(),
	}
	return
}

func (r *rpcHandler) PutFile(ctx context.Context, request *clientdebugrpcproto.PutFileRequest) (*clientdebugrpcproto.PutFileResponse, error) {
	f, err := os.Open(request.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	n, err := r.file.AddFile(ctx, f)
	if err != nil {
		return nil, err
	}
	return &clientdebugrpcproto.PutFileResponse{
		Hash: n.Cid().String(),
	}, nil
}

func (r *rpcHandler) GetFile(ctx context.Context, request *clientdebugrpcproto.GetFileRequest) (*clientdebugrpcproto.GetFileResponse, error) {
	c, err := cid.Parse(request.Hash)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(request.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rd, err := r.file.GetFile(ctx, c)
	if err != nil {
		return nil, err
	}
	defer rd.Close()
	wr, err := io.Copy(f, rd)
	if err != nil && err != io.EOF {
		return nil, err
	}
	log.Info("copied bytes", zap.Int64("size", wr))
	return &clientdebugrpcproto.GetFileResponse{
		Path: request.Path,
	}, nil
}

func (r *rpcHandler) DeleteFile(ctx context.Context, request *clientdebugrpcproto.DeleteFileRequest) (*clientdebugrpcproto.DeleteFileResponse, error) {
	//TODO implement me
	panic("implement me")
}
