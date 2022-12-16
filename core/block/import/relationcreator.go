package importer

import (
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type relationIDFormat struct {
	ID     string
	Format model.RelationFormat
}

type relations []relationIDFormat

type RelationService struct {
	core             core.Service
	service          *block.Service
	createdRelations map[string]relations // need this field to avoid creation of the same relations
	store            filestore.FileStore
}

// NewRelationCreator constructor for RelationService
func NewRelationCreator(service *block.Service, store filestore.FileStore, core core.Service) RelationCreator {
	return &RelationService{
		service:          service,
		core:             core,
		createdRelations: make(map[string]relations, 0),
		store:            store,
	}
}

// Create read relations link from snaphot and create according relations in anytype, also set details for according relation in object
// for files it loads them in ipfs
func (rc *RelationService) Create(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	relations []*converter.Relation,
	pageID string) ([]string, map[string]*model.Block, error) {
	var (
		err                   error
		filesToDelete         = make([]string, 0)
		oldRelationBlockToNew = make(map[string]*model.Block, 0)
		createRequest         = make([]*types.Struct, 0)
		existedRelationsIDs   = make([]string, 0)
		setDetailsRequest     = make([]*pb.RpcObjectSetDetailsDetail, 0)
	)

	for _, r := range relations {
		var (
			relationID string
		)
		if rel, ok := rc.createdRelations[r.Name]; ok {
			for _, v := range rel {
				if v.Format == r.Format {
					relationID = v.ID
					existedRelationsIDs = append(existedRelationsIDs, relationID)
					break
				}
			}
		}

		if relationID == "" {
			detail := &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyName.String():           pbtypes.String(r.Name),
					bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(r.Format)),
					bundle.RelationKeyType.String():           pbtypes.String(bundle.TypeKeyRelation.URL()),
					bundle.RelationKeyLayout.String():         pbtypes.Float64(float64(model.ObjectType_relation)),
				},
			}
			createRequest = append(createRequest, detail)
		}
	}
	var objects []*types.Struct
	if _, objects, err = rc.service.CreateSubObjectsInWorkspace(createRequest); err != nil {
		log.Errorf("create relation %s", err)
	}

	ids := make([]string, 0, len(existedRelationsIDs)+len(objects))
	ids = append(ids, existedRelationsIDs...)

	for _, s := range objects {
		name := pbtypes.GetString(s, bundle.RelationKeyName.String())
		id := pbtypes.GetString(s, bundle.RelationKeyRelationKey.String())
		format := model.RelationFormat(pbtypes.GetFloat64(s, bundle.RelationKeyRelationFormat.String()))
		rc.createdRelations[name] = append(rc.createdRelations[name], relationIDFormat{
			ID:     id,
			Format: format,
		})
		ids = append(ids, id)
	}

	if err = rc.service.AddExtraRelations(ctx, pageID, ids); err != nil {
		log.Errorf("add extra relation %s", err)
	}

	for _, r := range relations {
		var relationID string
		if cr, ok := rc.createdRelations[r.Name]; ok {
			for _, rel := range cr {
				if rel.Format == r.Format {
					relationID = rel.ID
				}
			}
		}
		if relationID == "" {
			continue
		}
		if snapshot.Details != nil && snapshot.Details.Fields != nil {
			if snapshot.Details.Fields[r.Name].GetListValue() != nil {
				rc.handleListValue(ctx, snapshot, r, relationID)
			}
			if r.Format == model.RelationFormat_file {
				filesToDelete = append(filesToDelete, rc.handleFileRelation(ctx, snapshot, r.Name)...)
			}
		}
		setDetailsRequest = append(setDetailsRequest, &pb.RpcObjectSetDetailsDetail{
			Key:   relationID,
			Value: snapshot.Details.Fields[r.Name],
		})
		if r.BlockID != "" {
			original, new := rc.linkRelationsBlocks(snapshot, r.BlockID, relationID)
			if original != nil && new != nil {
				oldRelationBlockToNew[original.GetId()] = new
			}
		}
	}

	err = rc.service.SetDetails(ctx, pb.RpcObjectSetDetailsRequest{
		ContextId: pageID,
		Details:   setDetailsRequest,
	})
	if err != nil {
		log.Errorf("set details %s", err)
	}

	if ftd, err := rc.handleCoverRelation(ctx, snapshot, pageID); err != nil {
		log.Errorf("failed to upload cover image %s", err)
	} else {
		filesToDelete = append(filesToDelete, ftd...)
	}

	return filesToDelete, oldRelationBlockToNew, nil
}

func (rc *RelationService) handleCoverRelation(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	pageID string) ([]string, error) {
	filesToDelete := rc.handleFileRelation(ctx, snapshot, bundle.RelationKeyCoverId.String())
	details := make([]*pb.RpcObjectSetDetailsDetail, 0)
	details = append(details, &pb.RpcObjectSetDetailsDetail{
		Key:   bundle.RelationKeyCoverId.String(),
		Value: snapshot.Details.Fields[bundle.RelationKeyCoverId.String()],
	})
	err := rc.service.SetDetails(ctx, pb.RpcObjectSetDetailsRequest{
		ContextId: pageID,
		Details:   details,
	})

	return filesToDelete, err
}

func (rc *RelationService) handleListValue(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, r *converter.Relation, relationID string) {
	var (
		optionsIds = make([]string, 0)
		id         string
		err        error
	)
	for _, v := range snapshot.Details.Fields[r.Name].GetListValue().Values {
		if r.Format == model.RelationFormat_tag || r.Format == model.RelationFormat_status {
			if id, _, err = rc.service.CreateSubObjectInWorkspace(&types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyName.String():        pbtypes.String(v.GetStringValue()),
					bundle.RelationKeyRelationKey.String(): pbtypes.String(relationID),
					bundle.RelationKeyType.String():        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
					bundle.RelationKeyLayout.String():      pbtypes.Float64(float64(model.ObjectType_relationOption)),
				},
			}, rc.core.PredefinedBlocks().Account); err != nil {
				log.Errorf("add extra relation %s", err)
			}
		} else {
			id = v.GetStringValue()
		}
		optionsIds = append(optionsIds, id)
	}
	snapshot.Details.Fields[r.Name] = pbtypes.StringList(optionsIds)
}

func (rc *RelationService) handleFileRelation(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	name string) []string {
	var allFiles []string
	if files := snapshot.Details.Fields[name].GetListValue(); files != nil {
		for _, f := range files.Values {
			allFiles = append(allFiles, f.GetStringValue())
		}
	}

	if files := snapshot.Details.Fields[name].GetStringValue(); files != "" {
		allFiles = append(allFiles, files)
	}

	allFilesHashes := make([]string, 0)

	filesToDelete := make([]string, 0, len(allFiles))
	for _, f := range allFiles {
		if f == "" {
			continue
		}
		if _, err := rc.store.GetByHash(f); err == nil {
			allFilesHashes = append(allFilesHashes, f)
			continue
		}

		req := pb.RpcFileUploadRequest{LocalPath: f}

		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
			req.Url = f
			req.LocalPath = ""
		}

		hash, err := rc.service.UploadFile(req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		} else {
			f = hash
		}

		filesToDelete = append(filesToDelete, f)
		allFilesHashes = append(allFilesHashes, f)
	}

	if snapshot.Details.Fields[name].GetListValue() != nil {
		snapshot.Details.Fields[name] = pbtypes.StringList(allFilesHashes)
	}

	if snapshot.Details.Fields[name].GetStringValue() != "" && len(allFilesHashes) != 0 {
		snapshot.Details.Fields[name] = pbtypes.String(allFilesHashes[0])
	}

	return filesToDelete
}

func (rc *RelationService) linkRelationsBlocks(snapshot *model.SmartBlockSnapshotBase, oldID, newID string) (*model.Block, *model.Block) {
	for _, b := range snapshot.Blocks {
		if rel, ok := b.Content.(*model.BlockContentOfRelation); ok && rel.Relation.GetKey() == oldID {
			return b, &model.Block{
				Id: bson.NewObjectId().Hex(),
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: newID,
					},
				}}
		}
	}
	return nil, nil
}
