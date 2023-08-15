package objectgraph

import (
	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/opentracing/opentracing-go/log"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// relationsSkipList contains relations that SHOULD NOT be included in the graph. These relations of Object/File type that make no sense in the graph for user
var relationsSkipList = []bundle.RelationKey{
	bundle.RelationKeyType,
	bundle.RelationKeySetOf,
	bundle.RelationKeyCreator,
	bundle.RelationKeyLastModifiedBy,
	bundle.RelationKeyWorkspaceId,
	bundle.RelationKeyIconImage,
	bundle.RelationKeyCoverId,
}

type Service interface {
	ObjectGraph(req *pb.RpcObjectGraphRequest) ([]*types.Struct, []*pb.RpcObjectGraphEdge, error)
}

type Builder struct {
	graphService    Service //nolint:unused
	relationService relation.Service
	sbtProvider     typeprovider.SmartBlockTypeProvider
	coreService     core.Service
	objectStore     objectstore.ObjectStore

	*app.App
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (gr *Builder) Init(a *app.App) (err error) {
	gr.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	gr.relationService = app.MustComponent[relation.Service](a)
	gr.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	gr.coreService = app.MustComponent[core.Service](a)
	return nil
}

const CName = "graphBuilder"

func (gr *Builder) Name() (name string) {
	return CName
}

func (gr *Builder) ObjectGraph(req *pb.RpcObjectGraphRequest) ([]*types.Struct, []*pb.RpcObjectGraphEdge, error) {
	records, err := gr.queryRecords(req)
	if err != nil {
		return nil, nil, err
	}

	nodes := make([]*types.Struct, 0, len(records))
	edges := make([]*pb.RpcObjectGraphEdge, 0, len(records)*2)

	existedNodes := fillExistedNodes(records)

	relations, err := gr.provideRelations(req.SpaceId)
	if err != nil {
		return nil, nil, err
	}

	nodes, edges = gr.extractGraph(req.SpaceId, records, nodes, req, relations, edges, existedNodes)
	return nodes, edges, nil
}

func isRelationShouldBeIncludedAsEdge(rel *relationutils.Relation) bool {
	return rel != nil && (rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_file) && !lo.Contains(relationsSkipList, bundle.RelationKey(rel.Key))
}

func (gr *Builder) extractGraph(
	spaceID string,
	records []database.Record,
	nodes []*types.Struct,
	req *pb.RpcObjectGraphRequest,
	relations relationutils.Relations,
	edges []*pb.RpcObjectGraphEdge,
	existedNodes map[string]struct{},
) ([]*types.Struct, []*pb.RpcObjectGraphEdge) {
	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())

		nodes = append(nodes, pbtypes.Map(rec.Details, req.Keys...))

		outgoingRelationLink := make(map[string]struct{}, 10)
		for k, v := range rec.Details.GetFields() {
			rel := relations.GetByKey(k)
			if !isRelationShouldBeIncludedAsEdge(rel) {
				continue
			}

			edges = appendRelations(v, existedNodes, rel, edges, id, outgoingRelationLink)
		}

		edges = gr.appendLinks(spaceID, rec, outgoingRelationLink, existedNodes, edges, id)
	}
	return nodes, edges
}

func (gr *Builder) provideRelations(spaceID string) (relationutils.Relations, error) {
	relations, err := gr.relationService.ListAll(relation.WithWorkspaceId(gr.coreService.PredefinedObjects(spaceID).Account))
	return relations, err
}

func (gr *Builder) queryRecords(req *pb.RpcObjectGraphRequest) ([]database.Record, error) {
	records, _, err := gr.objectStore.Query(
		nil,
		database.Query{
			Filters: req.Filters,
			Limit:   int(req.Limit),
		},
	)
	return records, err
}

func fillExistedNodes(records []database.Record) map[string]struct{} {
	existedNodes := make(map[string]struct{}, len(records))
	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		existedNodes[id] = struct{}{}
	}
	return existedNodes
}

func appendRelations(
	v *types.Value,
	existedNodes map[string]struct{},
	rel *relationutils.Relation,
	edges []*pb.RpcObjectGraphEdge,
	id string,
	outgoingRelationLink map[string]struct{},
) []*pb.RpcObjectGraphEdge {
	stringValues := pbtypes.GetStringListValue(v)
	if len(stringValues) == 0 || unallowedRelation(rel) {
		return edges
	}

	for _, l := range stringValues {
		if _, exists := existedNodes[l]; exists {
			edges = append(edges, &pb.RpcObjectGraphEdge{
				Source:      id,
				Target:      l,
				Name:        rel.Name,
				Type:        pb.RpcObjectGraphEdge_Relation,
				Description: rel.Description,
				Hidden:      rel.Hidden,
			})
			outgoingRelationLink[l] = struct{}{}
		}
	}
	return edges
}

func unallowedRelation(rel *relationutils.Relation) bool {
	return rel.Hidden ||
		rel.Key == bundle.RelationKeyId.String() ||
		rel.Key == bundle.RelationKeyCreator.String() ||
		rel.Key == bundle.RelationKeyWorkspaceId.String() ||
		rel.Key == bundle.RelationKeyLastModifiedBy.String()
}

func (gr *Builder) appendLinks(
	spaceID string,
	rec database.Record,
	outgoingRelationLink map[string]struct{},
	existedNodes map[string]struct{},
	edges []*pb.RpcObjectGraphEdge,
	id string,
) []*pb.RpcObjectGraphEdge {
	links := pbtypes.GetStringList(rec.Details, bundle.RelationKeyLinks.String())
	for _, link := range links {
		sbType, err := gr.sbtProvider.Type(spaceID, link)
		if err != nil {
			log.Error(err)
		}
		// ignore files because we index all file blocks as outgoing links
		if sbType != smartblock.SmartBlockTypeFile {
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
	return edges
}
