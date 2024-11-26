package objectgraph

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.LoggerNotSugared("object-graph")

// relationsSkipList contains relations that SHOULD NOT be included in the graph. These relations of Object/File type that make no sense in the graph for user
var relationsSkipList = []domain.RelationKey{
	bundle.RelationKeyType,
	bundle.RelationKeySetOf,
	bundle.RelationKeyCreator,
	bundle.RelationKeyLastModifiedBy,
	bundle.RelationKeyWorkspaceId,
	bundle.RelationKeyIconImage,
	bundle.RelationKeyCoverId,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyLinks,
	bundle.RelationKeySourceObject,
}

type Service interface {
	ObjectGraph(req *pb.RpcObjectGraphRequest) ([]*types.Struct, []*pb.RpcObjectGraphEdge, error)
}

type Builder struct {
	sbtProvider         typeprovider.SmartBlockTypeProvider
	objectStore         objectstore.ObjectStore
	subscriptionService subscription.Service

	*app.App
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (gr *Builder) Init(a *app.App) (err error) {
	gr.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	gr.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	gr.subscriptionService = app.MustComponent[subscription.Service](a)
	return nil
}

const CName = "graphBuilder"

func (gr *Builder) Name() (name string) {
	return CName
}

func (gr *Builder) ObjectGraph(req *pb.RpcObjectGraphRequest) ([]*types.Struct, []*pb.RpcObjectGraphEdge, error) {
	if req.SpaceId == "" {
		return nil, nil, fmt.Errorf("spaceId is required")
	}
	relations, err := gr.objectStore.SpaceIndex(req.SpaceId).ListAllRelations()
	if err != nil {
		return nil, nil, err
	}
	req.Keys = append(req.Keys, bundle.RelationKeyLinks.String())
	req.Keys = append(req.Keys, lo.FilterMap(relations, func(rel *relationutils.Relation, _ int) (string, bool) {
		return rel.Key, isRelationShouldBeIncludedAsEdge(rel)
	})...)

	resp, err := gr.subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId:      req.SpaceId,
		Source:       req.SetSource,
		Filters:      req.Filters,
		Keys:         lo.Map(relations.Models(), func(rel *model.Relation, _ int) string { return rel.Key }),
		CollectionId: req.CollectionId,
		Limit:        int64(req.Limit),
		Internal:     true,
	})

	if err != nil {
		return nil, nil, err
	}

	err = gr.subscriptionService.Unsubscribe(resp.SubId)
	// workaround should be reviewed in GO-3332
	if err != nil {
		log.Error("unsubscribe", zap.Error(err))
	}

	nodes, edges := gr.buildGraph(
		resp.Records,
		make([]*types.Struct, 0, len(resp.Records)),
		req,
		relations,
		make([]*pb.RpcObjectGraphEdge, 0, len(resp.Records)*2),
	)
	return nodes, edges, nil
}

func isRelationShouldBeIncludedAsEdge(rel *relationutils.Relation) bool {
	return rel != nil && (rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_file) && !lo.Contains(relationsSkipList, domain.RelationKey(rel.Key))
}

func (gr *Builder) buildGraph(
	records []*types.Struct,
	nodes []*types.Struct,
	req *pb.RpcObjectGraphRequest,
	relations relationutils.Relations,
	edges []*pb.RpcObjectGraphEdge,
) ([]*types.Struct, []*pb.RpcObjectGraphEdge) {
	existedNodes := fillExistedNodes(records)
	for _, rec := range records {
		sourceId := pbtypes.GetString(rec, bundle.RelationKeyId.String())

		nodes = append(nodes, pbtypes.Map(rec, req.Keys...))

		outgoingRelationLink := make(map[string]struct{}, 10)
		edges = gr.appendRelations(rec, relations, edges, existedNodes, sourceId, outgoingRelationLink)
		nodesToAdd := make([]*types.Struct, 0)
		nodesToAdd, edges = gr.appendLinks(req.SpaceId, rec, outgoingRelationLink, existedNodes, edges, sourceId)

		if len(nodesToAdd) != 0 {
			nodes = append(nodes, nodesToAdd...)
		}
	}
	return nodes, edges
}

func (gr *Builder) appendRelations(
	rec *types.Struct,
	relations relationutils.Relations,
	edges []*pb.RpcObjectGraphEdge,
	existedNodes map[string]struct{},
	sourceId string,
	outgoingRelationLink map[string]struct{},
) []*pb.RpcObjectGraphEdge {
	for relKey, relValue := range rec.GetFields() {
		rel := relations.GetByKey(relKey)
		if !isRelationShouldBeIncludedAsEdge(rel) {
			continue
		}
		stringValues := pbtypes.GetStringListValue(relValue)
		if len(stringValues) == 0 || isExcludedRelation(rel) {
			continue
		}

		for _, strValue := range stringValues {
			if _, exists := existedNodes[strValue]; exists {
				edges = append(edges, &pb.RpcObjectGraphEdge{
					Source:      sourceId,
					Target:      strValue,
					Name:        rel.Name,
					Type:        pb.RpcObjectGraphEdge_Relation,
					Description: rel.Description,
					Hidden:      rel.Hidden,
				})
				outgoingRelationLink[strValue] = struct{}{}
			}
		}
	}
	return edges
}

func fillExistedNodes(records []*types.Struct) map[string]struct{} {
	existedNodes := make(map[string]struct{}, len(records))
	for _, rec := range records {
		id := pbtypes.GetString(rec, bundle.RelationKeyId.String())
		existedNodes[id] = struct{}{}
	}
	return existedNodes
}

func isExcludedRelation(rel *relationutils.Relation) bool {
	return rel.Hidden ||
		rel.Key == bundle.RelationKeyId.String() ||
		rel.Key == bundle.RelationKeyCreator.String() ||
		rel.Key == bundle.RelationKeyLastModifiedBy.String()
}

func (gr *Builder) appendLinks(
	spaceID string,
	rec *types.Struct,
	outgoingRelationLink map[string]struct{},
	existedNodes map[string]struct{},
	edges []*pb.RpcObjectGraphEdge,
	id string,
) (nodes []*types.Struct, resultEdges []*pb.RpcObjectGraphEdge) {
	links := pbtypes.GetStringList(rec, bundle.RelationKeyLinks.String())
	for _, link := range links {
		sbType, err := gr.sbtProvider.Type(spaceID, link)
		if err != nil {
			log.Error("get smartblock type", zap.String("objectId", link), zap.Error(err))
		}

		switch sbType {
		case smartblock.SmartBlockTypeFileObject:
			// ignore files because we index all file blocks as outgoing links
			continue
		case smartblock.SmartBlockTypeDate:
			details, err := gr.objectStore.SpaceIndex(spaceID).QueryByIds([]string{link})
			if err == nil && len(details) != 1 {
				err = fmt.Errorf("expected to get 1 date object, got %d", len(details))
			}
			if err != nil {
				log.Error("get details of Date object", zap.String("objectId", link), zap.Error(err))
				continue
			}
			existedNodes[link] = struct{}{}
			nodes = append(nodes, details[0].Details)
		}

		if sbType != smartblock.SmartBlockTypeFileObject {
			if _, exists := outgoingRelationLink[link]; !exists {
				if _, exists := existedNodes[link]; exists {
					edges = append(edges, &pb.RpcObjectGraphEdge{
						Source: id,
						Target: link,
						Name:   "",
						Type:   pb.RpcObjectGraphEdge_Link,
					})
				}
			}
		}
	}
	return nodes, edges
}
