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
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("csv-export")

type ExportCtx struct {
	filters           []*model.BlockContentDataviewFilter
	sorts             []*model.BlockContentDataviewSort
	relationKeys      []string
	includeSetObjects bool
	includeLinked     bool
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

func WithIncludeLinked(includeLinked bool) ExportCtxOption {
	return func(e *ExportCtx) {
		e.includeLinked = includeLinked
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
	var headers, headersName, objects []string
	var err error
	if len(c.ctx.relationKeys) == 0 {
		block := findDataviewBlock(st)
		if block == nil {
			return nil
		}
		if len(block.Views) == 0 {
			return nil
		}
		headers, headersName, err = c.extractHeaders(block, st.SpaceID())
		if err != nil {
			log.Errorf("failed extracting headers for csv export: %v", err)
			return nil
		}
	} else {
		headers, headersName, err = common.ExtractHeaders(c.store.SpaceIndex(st.SpaceID()), c.ctx.relationKeys)
		if err != nil {
			log.Errorf("failed extracting headers for csv export: %v", err)
			return nil
		}
	}
	csvRows := [][]string{headersName}
	layout, _ := st.Layout()
	spaceIndex := c.store.SpaceIndex(st.SpaceID())
	if c.ctx.includeSetObjects && layout == model.ObjectType_set {
		setOf := st.Details().GetString(bundle.RelationKeySetOf)
		uk, err := spaceIndex.GetUniqueKeyById(setOf)
		if err != nil {
			return nil
		}
		query := database.Query{}
		switch uk.SmartblockType() {
		case smartblock.SmartBlockTypeRelation:
			query.Filters = append(query.Filters, database.FilterRequest{
				RelationKey: domain.RelationKey(uk.InternalKey()),
				Condition:   model.BlockContentDataviewFilter_Exists,
			})
		case smartblock.SmartBlockTypeObjectType:
			query.Filters = append(query.Filters, database.FilterRequest{
				RelationKey: bundle.RelationKeyType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(setOf),
			})
		}
		if len(c.ctx.filters) > 0 {
			proto := database.FiltersFromProto(c.ctx.filters)
			query.Filters = append(query.Filters, proto...)
		}
		query.Sorts = database.SortsFromProto(c.ctx.sorts)
		objects, _, err = spaceIndex.QueryObjectIds(query)
		if err != nil {
			return nil
		}
	}
	if c.ctx.includeLinked && layout == model.ObjectType_collection {
		objects = st.GetStoreSlice(template.CollectionStoreKey)
		if len(c.ctx.filters) > 0 || len(c.ctx.sorts) > 0 {
			query := database.Query{}
			query.Filters = database.FiltersFromProto(c.ctx.filters)
			query.Sorts = database.SortsFromProto(c.ctx.sorts)
			query.Filters = append(query.Filters, database.FilterRequest{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(objects),
			})
			objects, _, err = spaceIndex.QueryObjectIds(query)
		}
	}
	for _, object := range objects {
		if objectDetail, ok := c.knownDocs[object]; ok {
			csvRows = append(csvRows, c.getCSVRow(objectDetail, headers))
		}
	}
	result, err := common.WriteCSV(csvRows)
	if err != nil {
		log.Errorf("failed writing csv: %v", err)
		return nil
	}
	return result.Bytes()
}

func (c *Converter) getCSVRow(details *domain.Details, headers []string) []string {
	values := make([]string, len(headers))
	for i, header := range headers {
		relationKey := domain.RelationKey(header)
		values[i] = common.GetValueAsString(details, nil, relationKey)
	}
	return values
}

func (c *Converter) extractHeaders(dataview *model.BlockContentDataview, spaceId string) ([]string, []string, error) {
	var headersKeys []string
	for _, relation := range dataview.RelationLinks {
		headersKeys = append(headersKeys, relation.Key)
	}
	return common.ExtractHeaders(c.store.SpaceIndex(spaceId), headersKeys)
}

func findDataviewBlock(st *state.State) *model.BlockContentDataview {
	for _, block := range st.Blocks() {
		if block.GetDataview() != nil {
			return block.GetDataview()
		}
	}
	return nil
}
