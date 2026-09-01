package export

// collection.go is the collection half of the exporter — the dependency
// closure over an export request, complete before anything is written — and
// the implementation behind the format-agnostic collect.Collector seam.
//
// The closure mode used to travel as a bare `isProtobuf bool`; it is now an
// explicit collect.Closure, so the native AnyBlock JSON writer asks for the
// derived closure by name instead of impersonating protobuf. Everything here
// reads DETAILS from the store and loads an object only to walk its links
// (getViewDependentObjects, collectDerivedObjects, addNestedObject,
// fillLinkedFiles); content stays out of the collection, which is what keeps
// its resident cost O(all details) rather than O(all content).

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/export/collect"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"

	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

// Collect implements collect.Collector: it runs the requested closure and
// returns the complete doc set before anything is written. The legacy
// formats reach the same code through exportContext.docsForExport; the
// native AnyBlock JSON exporter consumes only this method.
func (e *export) Collect(ctx context.Context, req collect.Request) (collect.Docs, error) {
	ec := &exportContext{
		spaceId:          req.SpaceId,
		docs:             Docs{},
		includeArchive:   req.IncludeArchived,
		includeNested:    req.IncludeNested,
		includeFiles:     req.IncludeFiles,
		reqIds:           req.Ids,
		closure:          req.Closure,
		linkStateFilters: req.StateFilters,
		includeBackLinks: req.IncludeBacklinks,
		includeSpace:     req.IncludeSpace,
		setOfList:        make(map[string]struct{}),
		objectTypes:      make(map[string]struct{}),
		relations:        make(map[string]struct{}),
		export:           e,
	}
	if err := ec.collectDocs(ctx); err != nil {
		return nil, err
	}
	return ec.docs, nil
}

// collectDocs runs the closure the context asks for — the body the legacy
// docsForExport dispatched on `isProtobuf`.
func (e *exportContext) collectDocs(ctx context.Context) (err error) {
	if len(e.reqIds) == 0 {
		return e.getExistedObjects(e.closure)
	}
	return e.getObjectsByIDs(ctx, e.closure)
}

func (e *exportContext) getObjectsByIDs(ctx context.Context, closure collect.Closure) error {
	res, err := e.queryAndFilterObjectsByRelation(e.spaceId, e.reqIds, bundle.RelationKeyId)
	if err != nil {
		return fmt.Errorf("query and filter objects by relation: %w", err)
	}
	for _, object := range res {
		id := object.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: object.Details}
	}
	if e.includeSpace {
		err = e.addSpaceToDocs(ctx)
		if err != nil {
			return fmt.Errorf("add space to docs: %w", err)
		}
	}
	if closure == collect.ClosureDerived {
		if err := e.processDerived(); err != nil {
			return fmt.Errorf("process derived closure: %w", err)
		}
		return nil
	}
	if err := e.processContent(); err != nil {
		return fmt.Errorf("process content closure: %w", err)
	}
	return nil
}

func (e *exportContext) queryAndFilterObjectsByRelation(spaceId string, reqIds []string, relationKey domain.RelationKey) ([]database.Record, error) {
	var allObjects []database.Record
	const singleBatchCount = 50
	for j := 0; j < len(reqIds); {
		if j+singleBatchCount < len(reqIds) {
			records, err := e.queryObjectsByRelation(spaceId, reqIds[j:j+singleBatchCount], relationKey)
			if err != nil {
				return nil, fmt.Errorf("query objects by relation: %w", err)
			}
			allObjects = append(allObjects, records...)
		} else {
			records, err := e.queryObjectsByRelation(spaceId, reqIds[j:], relationKey)
			if err != nil {
				return nil, fmt.Errorf("query objects by relation: %w", err)
			}
			allObjects = append(allObjects, records...)
		}
		j += singleBatchCount
	}
	return allObjects, nil
}

func (e *exportContext) queryObjectsByRelation(spaceId string, reqIds []string, relationKey domain.RelationKey) ([]database.Record, error) {
	return e.objectStore.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(reqIds),
			},
		},
	})
}

func (e *exportContext) addSpaceToDocs(ctx context.Context) error {
	space, err := e.spaceService.Get(ctx, e.spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	workspaceId := space.DerivedIDs().Workspace
	records, err := e.objectStore.SpaceIndex(e.spaceId).QueryByIds([]string{workspaceId})
	if err != nil {
		return fmt.Errorf("query workspace details: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("no objects found for space %s", workspaceId)
	}
	e.docs[workspaceId] = &Doc{Details: records[0].Details, IsLink: true}
	return nil
}

// processContent is the ClosureContent closure: linked files and, behind
// IncludeNested, content-linked objects — nothing derived.
func (e *exportContext) processContent() error {
	ids := listObjectIds(e.docs)
	if e.includeFiles {
		fileObjectsIds, err := e.processFiles(ids)
		if err != nil {
			return fmt.Errorf("process files: %w", err)
		}
		ids = append(ids, fileObjectsIds...)
	}
	if e.includeNested {
		for _, id := range ids {
			e.addNestedObject(id, map[string]*Doc{})
		}
	}
	return nil
}

// processDerived is the ClosureDerived closure: types, relations, options,
// templates, dataview dependencies and recommended relations ride along, so
// a snapshot export stands alone.
func (e *exportContext) processDerived() error {
	if !e.includeNested {
		err := e.addDependentObjectsFromDataview()
		if err != nil {
			return fmt.Errorf("add dependent objects from dataview: %w", err)
		}
	}
	ids := listObjectIds(e.docs)
	if e.includeFiles {
		err := e.addFileObjects(ids)
		if err != nil {
			return fmt.Errorf("add file objects: %w", err)
		}
	}

	err := e.addDerivedObjects()
	if err != nil {
		return fmt.Errorf("add derived objects: %w", err)
	}
	ids = e.listTargetTypesFromTemplates(ids)
	if e.includeNested {
		err = e.addNestedObjects(ids)
		if err != nil {
			return fmt.Errorf("add nested objects: %w", err)
		}
	}
	return nil
}

func (e *exportContext) addDependentObjectsFromDataview() error {
	var (
		viewDependentObjectsIds []string
		err                     error
	)
	for id, doc := range e.docs {
		if isExcludedFromExport(doc.Details) {
			continue
		}
		if isObjectWithDataview(doc.Details) {
			viewDependentObjectsIds, err = e.getViewDependentObjects(id, viewDependentObjectsIds)
			if err != nil {
				return fmt.Errorf("get view dependent objects: %w", err)
			}
		}
	}
	viewDependentObjects, err := e.queryAndFilterObjectsByRelation(e.spaceId, viewDependentObjectsIds, bundle.RelationKeyId)
	if err != nil {
		return fmt.Errorf("query dependent objects: %w", err)
	}
	templates, err := e.queryAndFilterObjectsByRelation(e.spaceId, viewDependentObjectsIds, bundle.RelationKeyTargetObjectType)
	if err != nil {
		return fmt.Errorf("query templates: %w", err)
	}
	for _, object := range append(viewDependentObjects, templates...) {
		id := object.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{
			Details: object.Details,
			IsLink:  e.isLinkProcess,
		}
	}
	return nil
}

func (e *exportContext) getViewDependentObjects(id string, viewDependentObjectsIds []string) ([]string, error) {
	err := cache.Do(e.picker, id, func(sb sb.SmartBlock) error {
		st := sb.NewState().Copy().Filter(e.getStateFilters(id))
		viewDependentObjectsIds = append(viewDependentObjectsIds,
			objectlink.DependentObjectIDs(st, sb.Space(), e.formatFetcher, objectlink.Flags{Blocks: true})...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get object from cache: %w", err)
	}
	return viewDependentObjectsIds, nil
}

func (e *exportContext) addFileObjects(ids []string) error {
	fileObjectsIds, err := e.processFiles(ids)
	if err != nil {
		return fmt.Errorf("process files: %w", err)
	}
	if e.includeNested {
		err = e.addNestedObjects(fileObjectsIds)
		if err != nil {
			return fmt.Errorf("add nested objects: %w", err)
		}
	}
	return nil
}

func (e *exportContext) processFiles(ids []string) ([]string, error) {
	var fileObjectsIds []string
	for _, id := range ids {
		objectFiles, err := e.fillLinkedFiles(id)
		if err != nil {
			return nil, fmt.Errorf("fill linked files: %w", err)
		}
		fileObjectsIds = lo.Union(fileObjectsIds, objectFiles)
	}
	return fileObjectsIds, nil
}

func (e *exportContext) addDerivedObjects() error {
	processedObjects := make(map[string]struct{}, 0)
	err := e.getRelationsAndTypes(e.docs, processedObjects)
	if err != nil {
		return fmt.Errorf("get relations and types: %w", err)
	}

	err = e.getTemplatesRelationsAndTypes(processedObjects)
	if err != nil {
		return fmt.Errorf("get templates relations and types: %w", err)
	}
	err = e.addRelationsAndTypes()
	if err != nil {
		return fmt.Errorf("add relations and types: %w", err)
	}
	return nil
}

func (e *exportContext) getRelationsAndTypes(notProcessedObjects map[string]*Doc, processedObjects map[string]struct{}) error {
	err := e.collectDerivedObjects(notProcessedObjects)
	if err != nil {
		return fmt.Errorf("collect derived objects: %w", err)
	}
	// get derived objects only from types,
	// because relations currently have only system relations and object type
	if len(e.objectTypes) > 0 || len(e.setOfList) > 0 {
		err = e.getDerivedObjectsForTypes(processedObjects)
		if err != nil {
			return fmt.Errorf("get derived objects for types: %w", err)
		}
	}
	return nil
}

func (e *exportContext) collectDerivedObjects(objects map[string]*Doc) error {
	for id, doc := range objects {
		if doc != nil && isExcludedFromExport(doc.Details) {
			continue
		}
		err := cache.Do(e.picker, id, func(b sb.SmartBlock) error {
			state := b.NewState().Copy().Filter(e.getStateFilters(id))
			objectRelations := state.AllRelationKeys()
			fillObjectsMap(e.relations, slice.IntoStrings(objectRelations))
			details := state.CombinedDetails()
			if isObjectWithDataview(details) {
				dataviewRelations, err := getDataviewRelations(state)
				if err != nil {
					return fmt.Errorf("get dataview relations: %w", err)
				}
				fillObjectsMap(e.relations, dataviewRelations)
			}
			var objectTypes []string
			if details.Has(bundle.RelationKeyType) {
				objectTypes = append(objectTypes, details.GetString(bundle.RelationKeyType))
			}
			if details.Has(bundle.RelationKeyTargetObjectType) {
				objectTypes = append(objectTypes, details.GetString(bundle.RelationKeyTargetObjectType))
			}
			fillObjectsMap(e.objectTypes, objectTypes)
			setOfList := details.GetStringList(bundle.RelationKeySetOf)
			fillObjectsMap(e.setOfList, setOfList)
			return nil
		})
		if err != nil {
			return fmt.Errorf("get object from cache: %w", err)
		}
	}
	return nil
}

func fillObjectsMap(dst map[string]struct{}, objectsToAdd []string) {
	for _, objectId := range objectsToAdd {
		dst[objectId] = struct{}{}
	}
}

func isObjectWithDataview(details *domain.Details) bool {
	return details.GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_collection) ||
		details.GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_set)
}

func getDataviewRelations(state *state.State) ([]string, error) {
	var relations []string
	err := state.Iterate(func(b simple.Block) (isContinue bool) {
		if dataview := b.Model().GetDataview(); dataview != nil {
			for _, view := range dataview.Views {
				for _, relation := range view.Relations {
					relations = append(relations, relation.Key)
				}
			}
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("iterate state blocks: %w", err)
	}
	return relations, nil
}

func (e *exportContext) getDerivedObjectsForTypes(processedObjects map[string]struct{}) error {
	notProceedTypes := make(map[string]*Doc)
	for object := range e.objectTypes {
		e.fillNotProcessedTypes(processedObjects, object, notProceedTypes)
	}
	for object := range e.setOfList {
		e.fillNotProcessedTypes(processedObjects, object, notProceedTypes)
	}
	if len(notProceedTypes) == 0 {
		return nil
	}
	err := e.getRelationsAndTypes(notProceedTypes, processedObjects)
	if err != nil {
		return fmt.Errorf("get relations and types: %w", err)
	}
	return nil
}

func (e *exportContext) fillNotProcessedTypes(processedObjects map[string]struct{}, object string, notProceedTypes map[string]*Doc) {
	if _, ok := processedObjects[object]; ok {
		return
	}
	notProceedTypes[object] = nil
	processedObjects[object] = struct{}{}
}

func (e *exportContext) getTemplatesRelationsAndTypes(processedObjects map[string]struct{}) error {
	allTypes := lo.MapToSlice(e.objectTypes, func(key string, value struct{}) string { return key })
	templates, err := e.queryAndFilterObjectsByRelation(e.spaceId, allTypes, bundle.RelationKeyTargetObjectType)
	if err != nil {
		return fmt.Errorf("query templates by target type: %w", err)
	}
	if len(templates) == 0 {
		return nil
	}
	templatesToProcess := make(map[string]*Doc, len(templates))
	for _, template := range templates {
		id := template.Details.GetString(bundle.RelationKeyId)
		if _, ok := e.docs[id]; !ok {
			templateDoc := &Doc{Details: template.Details, IsLink: e.isLinkProcess}
			e.docs[id] = templateDoc
			templatesToProcess[id] = templateDoc
		}
	}
	err = e.getRelationsAndTypes(templatesToProcess, processedObjects)
	if err != nil {
		return fmt.Errorf("get relations and types for templates: %w", err)
	}
	return nil
}

func (e *exportContext) addRelationsAndTypes() error {
	types := lo.MapToSlice(e.objectTypes, func(key string, value struct{}) string { return key })
	setOfList := lo.MapToSlice(e.setOfList, func(key string, value struct{}) string { return key })
	relations := lo.MapToSlice(e.relations, func(key string, value struct{}) string { return key })

	err := e.addRelations(relations)
	if err != nil {
		return fmt.Errorf("add relations: %w", err)
	}
	err = e.processObjectTypesAndSetOfList(types, setOfList)
	if err != nil {
		return fmt.Errorf("process object types and set of list: %w", err)
	}
	return nil
}

func (e *exportContext) addRelations(relations []string) error {
	storeRelations, err := e.getRelationsFromStore(relations)
	if err != nil {
		return fmt.Errorf("get relations from store: %w", err)
	}
	for _, storeRelation := range storeRelations {
		e.addRelation(storeRelation)
		err := e.addOptionIfTag(storeRelation)
		if err != nil {
			return fmt.Errorf("add option if tag: %w", err)
		}
	}
	return nil
}

func (e *exportContext) getRelationsFromStore(relations []string) ([]database.Record, error) {
	uniqueKeys := make([]string, 0, len(relations))
	for _, relation := range relations {
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relation)
		if err != nil {
			return nil, fmt.Errorf("create unique key for relation: %w", err)
		}
		uniqueKeys = append(uniqueKeys, uniqueKey.Marshal())
	}
	storeRelations, err := e.queryAndFilterObjectsByRelation(e.spaceId, uniqueKeys, bundle.RelationKeyUniqueKey)
	if err != nil {
		return nil, fmt.Errorf("query relations by unique key: %w", err)
	}
	return storeRelations, nil
}

func (e *exportContext) addRelation(relation database.Record) {
	relationKey := domain.RelationKey(relation.Details.GetString(bundle.RelationKeyRelationKey))
	if relationKey != "" {
		id := relation.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: relation.Details, IsLink: e.isLinkProcess}
	}
}

func (e *exportContext) addOptionIfTag(relation database.Record) error {
	format := relation.Details.GetInt64(bundle.RelationKeyRelationFormat)
	relationKey := relation.Details.GetString(bundle.RelationKeyRelationKey)
	if format == int64(model.RelationFormat_tag) || format == int64(model.RelationFormat_status) {
		err := e.addRelationOptions(relationKey)
		if err != nil {
			return fmt.Errorf("add relation options: %w", err)
		}
	}
	return nil
}

func (e *exportContext) addRelationOptions(relationKey string) error {
	relationOptions, err := e.getRelationOptions(relationKey)
	if err != nil {
		return fmt.Errorf("get relation options: %w", err)
	}
	for _, option := range relationOptions {
		id := option.Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: option.Details, IsLink: e.isLinkProcess}
	}
	return nil
}

func (e *exportContext) getRelationOptions(relationKey string) ([]database.Record, error) {
	relationOptionsDetails, err := e.objectStore.SpaceIndex(e.spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_relationOption),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(relationKey),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query relation options: %w", err)
	}
	return relationOptionsDetails, nil
}

func (e *exportContext) processObjectTypesAndSetOfList(objectTypes, setOfList []string) error {
	objectDetails, err := e.queryAndFilterObjectsByRelation(e.spaceId, lo.Union(objectTypes, setOfList), bundle.RelationKeyId)
	if err != nil {
		return fmt.Errorf("query object types: %w", err)
	}
	if len(objectDetails) == 0 {
		return nil
	}
	recommendedRelations, err := e.addObjectsAndCollectRecommendedRelations(objectDetails)
	if err != nil {
		return fmt.Errorf("collect recommended relations: %w", err)
	}
	err = e.addRecommendedRelations(recommendedRelations)
	if err != nil {
		return fmt.Errorf("add recommended relations: %w", err)
	}
	return nil
}

func (e *exportContext) addObjectsAndCollectRecommendedRelations(objectTypes []database.Record) ([]string, error) {
	recommendedRelations := make([]string, 0, len(objectTypes))
	for i := 0; i < len(objectTypes); i++ {
		rawUniqueKey := objectTypes[i].Details.GetString(bundle.RelationKeyUniqueKey)
		uniqueKey, err := domain.UnmarshalUniqueKey(rawUniqueKey)
		if err != nil {
			return nil, fmt.Errorf("unmarshal unique key: %w", err)
		}
		id := objectTypes[i].Details.GetString(bundle.RelationKeyId)
		e.docs[id] = &Doc{Details: objectTypes[i].Details, IsLink: e.isLinkProcess}
		if uniqueKey.SmartblockType() == smartblock.SmartBlockTypeObjectType {
			key, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
			if err != nil {
				return nil, fmt.Errorf("get type key from unique key: %w", err)
			}
			if bundle.IsInternalType(key) {
				continue
			}
			recommendedRelations = lo.Uniq(slices.Concat(recommendedRelations,
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedRelations),
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations),
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations),
				objectTypes[i].Details.GetStringList(bundle.RelationKeyRecommendedFileRelations),
			))
		}
	}
	return recommendedRelations, nil
}

func (e *exportContext) addRecommendedRelations(recommendedRelations []string) error {
	relations, err := e.queryAndFilterObjectsByRelation(e.spaceId, recommendedRelations, bundle.RelationKeyId)
	if err != nil {
		return fmt.Errorf("query recommended relations: %w", err)
	}
	for _, relation := range relations {
		id := relation.Details.GetString(bundle.RelationKeyId)
		if id == addr.MissingObject {
			continue
		}

		relationKey := relation.Details.GetString(bundle.RelationKeyUniqueKey)
		uniqueKey, err := domain.UnmarshalUniqueKey(relationKey)
		if err != nil {
			return fmt.Errorf("unmarshal relation unique key: %w", err)
		}
		if bundle.IsSystemRelation(domain.RelationKey(uniqueKey.InternalKey())) {
			continue
		}
		e.docs[id] = &Doc{Details: relation.Details, IsLink: e.isLinkProcess}
	}
	return nil
}

func (e *exportContext) addNestedObjects(ids []string) error {
	nestedDocs := make(map[string]*Doc, 0)
	for _, id := range ids {
		e.addNestedObject(id, nestedDocs)
	}
	if len(nestedDocs) == 0 {
		return nil
	}
	exportCtxChild := e.copy()
	exportCtxChild.includeNested = false
	exportCtxChild.docs = nestedDocs
	exportCtxChild.isLinkProcess = true
	err := exportCtxChild.processDerived()
	if err != nil {
		return fmt.Errorf("process nested derived closure: %w", err)
	}
	for id, object := range exportCtxChild.docs {
		if _, ok := e.docs[id]; !ok {
			e.docs[id] = object
		}
	}
	return nil
}

func (e *exportContext) addNestedObject(id string, nestedDocs map[string]*Doc) {
	if doc, ok := e.docs[id]; ok && isExcludedFromExport(doc.Details) {
		return
	}
	var links []string
	err := cache.Do(e.picker, id, func(sb sb.SmartBlock) error {
		st := sb.NewState().Copy().Filter(e.getStateFilters(id))
		links = objectlink.DependentObjectIDs(st, sb.Space(), e.formatFetcher, objectlink.Flags{
			Blocks:                   true,
			Details:                  true,
			Collection:               true,
			NoHiddenBundledRelations: true,
			NoBackLinks:              !e.includeBackLinks,
			CreatorModifierWorkspace: true,
		})
		return nil
	})
	if err != nil {
		return
	}
	for _, link := range links {
		if _, exists := e.docs[link]; !exists {
			sbt, sbtErr := e.sbtProvider.Type(e.spaceId, link)
			if sbtErr != nil {
				log.Errorf("failed to get smartblocktype of id %s", link)
				continue
			}
			if !validType(sbt) {
				continue
			}
			rec, qErr := e.objectStore.SpaceIndex(e.spaceId).QueryByIds([]string{link})
			if qErr != nil {
				log.Errorf("failed to query id %s, err: %s", qErr, err)
				continue
			}
			if isLinkedObjectExist(rec) {
				exportDoc := &Doc{Details: rec[0].Details, IsLink: true}
				nestedDocs[link] = exportDoc
				e.docs[link] = exportDoc
				e.addNestedObject(link, nestedDocs)
			}
		}
	}
}

func (e *exportContext) fillLinkedFiles(id string) ([]string, error) {
	if doc, ok := e.docs[id]; ok && isExcludedFromExport(doc.Details) {
		return nil, nil
	}
	spaceIndex := e.objectStore.SpaceIndex(e.spaceId)
	var fileObjectsIds []string
	err := cache.Do(e.picker, id, func(b sb.SmartBlock) error {
		b.NewState().Copy().Filter(e.getStateFilters(id)).IterateLinkedFiles(e.formatFetcher, func(fileObjectId string) {
			res, err := spaceIndex.Query(database.Query{
				Filters: []database.FilterRequest{
					{
						RelationKey: bundle.RelationKeyId,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String(fileObjectId),
					},
				},
			})
			if err != nil {
				log.Errorf("failed to get details for file object id %s: %v", fileObjectId, err)
				return
			}
			if len(res) == 0 {
				return
			}
			e.docs[fileObjectId] = &Doc{Details: res[0].Details, IsLink: e.isLinkProcess}
			fileObjectsIds = append(fileObjectsIds, fileObjectId)
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get object from cache: %w", err)
	}
	return fileObjectsIds, nil
}

func (e *exportContext) getExistedObjects(closure collect.Closure) error {
	spaceIndex := e.objectStore.SpaceIndex(e.spaceId)
	res, err := spaceIndex.List(false)
	if err != nil {
		return fmt.Errorf("list objects: %w", err)
	}
	if e.includeArchive {
		archivedObjects, err := spaceIndex.List(true)
		if err != nil {
			return fmt.Errorf("list archived objects: %w", err)
		}
		res = append(res, archivedObjects...)
	}
	e.docs = make(map[string]*Doc, len(res))
	for _, info := range res {
		objectSpaceID := e.spaceId
		if objectSpaceID == "" {
			objectSpaceID = info.Details.GetString(bundle.RelationKeySpaceId)
		}
		sbType, err := e.sbtProvider.Type(objectSpaceID, info.Id)
		if err != nil {
			log.With("objectId", info.Id).Errorf("failed to get smartblock type: %v", err)
			continue
		}
		if !objectValid(sbType, info, e.includeArchive, closure) {
			continue
		}
		e.docs[info.Id] = &Doc{Details: info.Details}
	}
	return nil
}

func (e *exportContext) listTargetTypesFromTemplates(ids []string) []string {
	for id, object := range e.docs {
		if object.Details.Has(bundle.RelationKeyTargetObjectType) {
			ids = append(ids, id)
		}
	}
	return ids
}

func isExcludedFromExport(details *domain.Details) bool {
	return collect.Excluded(details)
}

func objectValid(sbType smartblock.SmartBlockType, info *database.ObjectInfo, includeArchived bool, closure collect.Closure) bool {
	if info.Id == addr.AnytypeProfileId {
		return false
	}
	if closure == collect.ClosureContent && (!validTypeForContentClosure(sbType) || !validLayoutForContentClosure(info.Details)) {
		return false
	}
	if closure == collect.ClosureDerived && !validType(sbType) {
		return false
	}
	if strings.HasPrefix(info.Id, addr.BundledObjectTypeURLPrefix) || strings.HasPrefix(info.Id, addr.BundledRelationURLPrefix) {
		return false
	}
	if info.Details.GetBool(bundle.RelationKeyIsArchived) && !includeArchived {
		return false
	}
	return true
}

func validType(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage ||
		sbType == smartblock.SmartBlockTypeTemplate ||
		sbType == smartblock.SmartBlockTypeWorkspace ||
		sbType == smartblock.SmartBlockTypeWidget ||
		sbType == smartblock.SmartBlockTypeObjectType ||
		sbType == smartblock.SmartBlockTypeRelation ||
		sbType == smartblock.SmartBlockTypeRelationOption ||
		sbType == smartblock.SmartBlockTypeFileObject ||
		sbType == smartblock.SmartBlockTypeParticipant
}

func validTypeForContentClosure(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage ||
		sbType == smartblock.SmartBlockTypeFileObject
}

func validLayoutForContentClosure(details *domain.Details) bool {
	return details.GetInt64(bundle.RelationKeyResolvedLayout) != int64(model.ObjectType_collection) &&
		details.GetInt64(bundle.RelationKeyResolvedLayout) != int64(model.ObjectType_set)
}

func listObjectIds(docs map[string]*Doc) []string {
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	return ids
}

func isLinkedObjectExist(rec []database.Record) bool {
	return len(rec) > 0 && !rec[0].Details.GetBool(bundle.RelationKeyIsDeleted)
}
