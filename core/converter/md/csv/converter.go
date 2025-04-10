package csv

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/converter/md/csv/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("csv-export")

type ExportCtx struct {
	filters           []*model.BlockContentDataviewFilter
	sorts             []*model.BlockContentDataviewSort
	relationKeys      []string
	includeSetObjects bool
}

type ExportCtxOption func(*ExportCtx)

func NewExportCtx(opts ...ExportCtxOption) *ExportCtx {
	e := &ExportCtx{}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func WithFilters(filters []*model.BlockContentDataviewFilter) ExportCtxOption {
	return func(e *ExportCtx) {
		e.filters = filters
	}
}

func WithSorts(sorts []*model.BlockContentDataviewSort) ExportCtxOption {
	return func(e *ExportCtx) {
		e.sorts = sorts
	}
}

func WithIncludeSetObjects(includeSetObjects bool) ExportCtxOption {
	return func(e *ExportCtx) {
		e.includeSetObjects = includeSetObjects
	}
}

func WithRelationKeys(relationKeys []string) ExportCtxOption {
	return func(e *ExportCtx) {
		e.relationKeys = relationKeys
	}
}

type Converter struct {
	knownDocs map[string]*domain.Details
	store     objectstore.ObjectStore
	ctx       *ExportCtx
}

func NewConverter(
	ctx *ExportCtx,
	store objectstore.ObjectStore,
	knownDocs map[string]*domain.Details,
) *Converter {
	return &Converter{
		ctx:       ctx,
		store:     store,
		knownDocs: knownDocs,
	}
}

func (c *Converter) Convert(st *state.State) []byte {
	headers, headerLabels, err := c.resolveHeaders(st)
	if err != nil {
		log.Errorf("failed resolve header: %v", err)
		return nil
	}
	if len(headers) == 0 {
		return nil
	}
	csvRows := [][]string{headerLabels}
	objectIDs := c.collectObjectIDs(st)
	for _, id := range objectIDs {
		if details, ok := c.knownDocs[id]; ok {
			csvRows = append(csvRows, c.buildCSVRow(details, headers))
		}
	}

	result, err := common.WriteCSV(csvRows)
	if err != nil {
		log.Errorf("CSV writing failed: %v", err)
		return nil
	}
	return result.Bytes()
}

func (c *Converter) resolveHeaders(st *state.State) ([]string, []string, error) {
	spaceIndex := c.store.SpaceIndex(st.SpaceID())

	if len(c.ctx.relationKeys) > 0 {
		return common.ExtractHeaders(spaceIndex, c.ctx.relationKeys)
	}

	block := findDataviewBlock(st)
	if block == nil {
		return nil, nil, nil
	}
	var relationKeys []string
	for _, link := range block.RelationLinks {
		relationKeys = append(relationKeys, link.Key)
	}
	return common.ExtractHeaders(spaceIndex, relationKeys)
}

func (c *Converter) collectObjectIDs(st *state.State) []string {
	layout, _ := st.Layout()
	spaceIndex := c.store.SpaceIndex(st.SpaceID())
	var (
		ids []string
		err error
	)

	switch {
	case c.ctx.includeSetObjects && layout == model.ObjectType_set:
		ids, err = c.querySetObjects(st, spaceIndex)
	case layout == model.ObjectType_collection:
		ids, err = c.queryLinkedObjects(st, spaceIndex)
	}

	if err != nil {
		log.Errorf("object ID query failed: %v", err)
		return nil
	}
	return ids
}

func (c *Converter) querySetObjects(st *state.State, spaceIndex spaceindex.Store) ([]string, error) {
	setOf := st.Details().GetStringList(bundle.RelationKeySetOf)
	if len(setOf) == 0 {
		return nil, nil
	}
	uk, err := spaceIndex.GetUniqueKeyById(setOf[0])
	if err != nil {
		return nil, err
	}

	query := database.Query{Filters: c.composeSetFilters(uk, setOf[0])}
	query.Sorts = database.SortsFromProto(c.ctx.sorts)

	return c.executeQuery(spaceIndex, query)
}

func (c *Converter) composeSetFilters(uk domain.UniqueKey, setOf string) []database.FilterRequest {
	var filters []database.FilterRequest

	switch uk.SmartblockType() {
	case smartblock.SmartBlockTypeRelation:
		filters = append(filters, database.FilterRequest{
			RelationKey: domain.RelationKey(uk.InternalKey()),
			Condition:   model.BlockContentDataviewFilter_Exists,
		})
	case smartblock.SmartBlockTypeObjectType:
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(setOf),
		})
	}
	if len(c.ctx.filters) > 0 {
		filters = append(filters, database.FiltersFromProto(c.ctx.filters)...)
	}
	return filters
}

func (c *Converter) queryLinkedObjects(st *state.State, spaceIndex spaceindex.Store) ([]string, error) {
	ids := st.GetStoreSlice(template.CollectionStoreKey)
	if len(c.ctx.filters) == 0 && len(c.ctx.sorts) == 0 {
		return ids, nil
	}

	query := database.Query{
		Filters: append(
			database.FiltersFromProto(c.ctx.filters),
			database.FilterRequest{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(ids),
			},
		),
		Sorts: database.SortsFromProto(c.ctx.sorts),
	}
	return c.executeQuery(spaceIndex, query)
}

func (c *Converter) executeQuery(spaceIndex spaceindex.Store, q database.Query) ([]string, error) {
	ids, _, err := spaceIndex.QueryObjectIds(q)
	return ids, err
}

func (c *Converter) buildCSVRow(details *domain.Details, headers []string) []string {
	values := make([]string, len(headers))
	for i, key := range headers {
		values[i] = common.GetValueAsString(details, nil, domain.RelationKey(key))
	}
	return values
}

func findDataviewBlock(st *state.State) *model.BlockContentDataview {
	for _, block := range st.Blocks() {
		if dv := block.GetDataview(); dv != nil {
			return dv
		}
	}
	return nil
}
