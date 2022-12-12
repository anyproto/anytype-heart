package importer

import (
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type RelationService struct {
	service block.Service
}

// NewRelationCreator constructor for RelationService
func NewRelationCreator(service block.Service) RelationCreator {
	return &RelationService{
		service: service,
	}
}

// Create read relations link from snaphot and create according relations in anytype, also set details for according relation in object
// for files it loads them in ipfs
func (rc *RelationService) Create(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	relations []*converter.Relation,
	pageID string) ([]string, map[string]*model.Block, error) {
	var (
		object                *types.Struct
		relationID            string
		err                   error
		filesToDelete         = make([]string, 0)
		oldRelationBlockToNew = make(map[string]*model.Block, 0)
	)

	for _, r := range relations {
		detail := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():           pbtypes.String(r.Name),
				bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(r.Format)),
			},
		}
		if _, object, err = rc.service.CreateRelation(detail); err != nil && err != editor.ErrSubObjectAlreadyExists {
			log.Errorf("create relation %s", err)
			continue
		}

		if object != nil && object.Fields != nil && object.Fields[bundle.RelationKeyRelationKey.String()] != nil {
			relationID = object.Fields[bundle.RelationKeyRelationKey.String()].GetStringValue()
		} else {
			continue
		}

		if err := rc.service.AddExtraRelations(ctx, pageID, []string{relationID}); err != nil {
			log.Errorf("add extra relation %s", err)
			continue
		}

		if snapshot.Details != nil && snapshot.Details.Fields != nil && object != nil {
			if snapshot.Details.Fields[r.Name].GetListValue() != nil {
				rc.handleListValue(ctx, snapshot, r, relationID)
			}

			if r.Format == model.RelationFormat_file {
				rc.handleFileRelation(ctx, snapshot, r, filesToDelete)
			}
			details := make([]*pb.RpcObjectSetDetailsDetail, 0)
			details = append(details, &pb.RpcObjectSetDetailsDetail{
				Key:   relationID,
				Value: snapshot.Details.Fields[r.Name],
			})
			err = rc.service.SetDetails(ctx, pb.RpcObjectSetDetailsRequest{
				ContextId: pageID,
				Details:   details,
			})
			if err != nil {
				log.Errorf("set details %s", err)
				continue
			}
		}
		if r.BlockID != "" {
			original, new := rc.linkRelationsBlocks(snapshot, r.BlockID, relationID)
			if original != nil && new != nil {
				oldRelationBlockToNew[original.GetId()] = new
			}
		}
	}
	return filesToDelete, oldRelationBlockToNew, nil
}

func (rc *RelationService) handleListValue(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, r *converter.Relation, relationID string) {
	var (
		optionsIds = make([]string, 0)
		id         string
		err        error
	)
	for _, v := range snapshot.Details.Fields[r.Name].GetListValue().Values {
		if r.Format == model.RelationFormat_tag || r.Format == model.RelationFormat_status {
			if id, _, err = rc.service.CreateRelationOption(&types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyName.String():        pbtypes.String(v.GetStringValue()),
					bundle.RelationKeyRelationKey.String(): pbtypes.String(relationID),
					bundle.RelationKeyType.String():        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
					bundle.RelationKeyLayout.String():      pbtypes.Float64(float64(model.ObjectType_relationOption)),
				},
			}); err != nil {
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
	r *converter.Relation,
	filesToDelete []string) {
	files := snapshot.Details.Fields[r.Name].GetListValue()

	if files == nil {
		return
	}
	allFilesHashes := make([]string, 0)
	for _, f := range files.Values {
		file := f.GetStringValue()
		if file != "" {
			req := pb.RpcFileUploadRequest{LocalPath: file}
			if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
				req.Url = file
				req.LocalPath = ""
			}
			hash, err := rc.service.UploadFile(req)
			if err != nil {
				log.Errorf("file uploading %s", err)
			} else {
				file = hash
			}
			filesToDelete = append(filesToDelete, file)
			allFilesHashes = append(allFilesHashes, file)
		}
	}
	snapshot.Details.Fields[r.Name] = pbtypes.StringList(allFilesHashes)
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
