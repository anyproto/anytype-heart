package objectgraph

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.LoggerNotSugared("object-graph")

// relationsEdgesSkipList contains relations that SHOULD NOT be included in the graph. These relations of Object/File type that make no sense in the graph for user
var relationsEdgesSkipList = []domain.RelationKey{
	// Type is excluded optionally via IncludeTypeEdges argument
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
	ObjectGraph(req ObjectGraphRequest) ([]*domain.Details, []*pb.RpcObjectGraphEdge, error)
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

type ObjectGraphRequest struct {
	Filters          []database.FilterRequest
	Limit            int32
	ObjectTypeFilter []string
	Keys             []string
	SpaceId          string
	CollectionId     string
	SetSource        []string
	IncludeTypeEdges bool
}

func (gr *Builder) ObjectGraph(req ObjectGraphRequest) ([]*domain.Details, []*pb.RpcObjectGraphEdge, error) {
	if req.SpaceId == "" {
		return nil, nil, fmt.Errorf("spaceId is required")
	}
	relations, err := gr.objectStore.SpaceIndex(req.SpaceId).ListAllRelations()
	if err != nil {
		return nil, nil, err
	}
	req.Keys = append(req.Keys, bundle.RelationKeyLinks.String())
	req.Keys = append(req.Keys, lo.FilterMap(relations, func(rel *relationutils.Relation, _ int) (string, bool) {
		return rel.Key, isRelationShouldBeIncludedAsEdge(rel, req.IncludeTypeEdges)
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
		make([]*domain.Details, 0, len(resp.Records)),
		req,
		relations,
		make([]*pb.RpcObjectGraphEdge, 0, len(resp.Records)*2),
	)
	return nodes, edges, nil
}

func isRelationShouldBeIncludedAsEdge(rel *relationutils.Relation, includeTypeEdges bool) bool {
	if rel == nil {
		return false
	}

	// Check if relation is in skip list
	isInSkipList := lo.Contains(relationsEdgesSkipList, domain.RelationKey(rel.Key))

	// Special handling for type relation based on includeTypeEdges parameter
	if rel.Key == bundle.RelationKeyType.String() {
		// If includeTypeEdges is true, we want to include it regardless of skip list
		return includeTypeEdges
	}

	// For all other relations, follow the normal skip list logic
	return (rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_file) && !isInSkipList
}

func (gr *Builder) buildGraph(
	records []*domain.Details,
	nodes []*domain.Details,
	req ObjectGraphRequest,
	relations relationutils.Relations,
	edges []*pb.RpcObjectGraphEdge,
) ([]*domain.Details, []*pb.RpcObjectGraphEdge) {
	existedNodes := fillExistedNodes(records)
	for _, rec := range records {
		sourceId := rec.GetString(bundle.RelationKeyId)

		if len(req.Keys) == 0 {
			nodes = append(nodes, rec)
		} else {
			nodes = append(nodes, rec.CopyOnlyKeys(slice.StringsInto[domain.RelationKey](req.Keys)...))
		}

		outgoingRelationLink := make(map[string]struct{}, 10)
		edges = gr.appendRelations(rec, relations, edges, existedNodes, sourceId, outgoingRelationLink, req.IncludeTypeEdges)
		var nodesToAdd []*domain.Details
		nodesToAdd, edges = gr.appendLinks(req.SpaceId, rec, outgoingRelationLink, existedNodes, edges, sourceId)

		nodesToAdd = lo.Map(nodesToAdd, func(item *domain.Details, _ int) *domain.Details {
			return item.CopyOnlyKeys(slice.StringsInto[domain.RelationKey](req.Keys)...)
		})
		nodes = append(nodes, nodesToAdd...)
	}
	return nodes, edges
}

func (gr *Builder) appendRelations(
	rec *domain.Details,
	relations relationutils.Relations,
	edges []*pb.RpcObjectGraphEdge,
	existedNodes map[string]struct{},
	sourceId string,
	outgoingRelationLink map[string]struct{},
	includeTypeEdges bool,
) []*pb.RpcObjectGraphEdge {
	for relKey, relValue := range rec.Iterate() {
		rel := relations.GetByKey(string(relKey))
		if !isRelationShouldBeIncludedAsEdge(rel, includeTypeEdges) {
			continue
		}
		stringValues, _ := relValue.TryWrapToStringList()
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

func fillExistedNodes(records []*domain.Details) map[string]struct{} {
	existedNodes := make(map[string]struct{}, len(records))
	for _, rec := range records {
		id := rec.GetString(bundle.RelationKeyId)
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
	rec *domain.Details,
	outgoingRelationLink map[string]struct{},
	existedNodes map[string]struct{},
	edges []*pb.RpcObjectGraphEdge,
	id string,
) (nodes []*domain.Details, resultEdges []*pb.RpcObjectGraphEdge) {
	links := rec.GetStringList(bundle.RelationKeyLinks)
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
			if _, exists := existedNodes[link]; !exists {
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
		}

		if _, exists := outgoingRelationLink[link]; exists {
			continue
		}

		if _, exists := existedNodes[link]; exists {
			edges = append(edges, &pb.RpcObjectGraphEdge{
				Source: id,
				Target: link,
				Name:   "",
				Type:   pb.RpcObjectGraphEdge_Link,
			})
		}
	}
	return nodes, edges
}
