package csv

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/converter/md/csv/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("csv-export")

type Converter struct {
	knownDocs   map[string]*domain.Details
	store       objectstore.ObjectStore
	filters     []*model.BlockContentDataviewFilter
	sorts       []*model.BlockContentDataviewSort
	relationKey []string
}

func NewConverter(
	store objectstore.ObjectStore,
	knownDocs map[string]*domain.Details,
	filters []*model.BlockContentDataviewFilter,
	sorts []*model.BlockContentDataviewSort,
	relationKey []string,
) *Converter {
	return &Converter{
		store:       store,
		knownDocs:   knownDocs,
		filters:     filters,
		sorts:       sorts,
		relationKey: relationKey,
	}
}

func (c *Converter) Convert(st *state.State) []byte {
	var headers, headersName []string
	var err error
	if len(c.relationKey) == 0 {
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
		headers, headersName, err = common.ExtractHeaders(c.store.SpaceIndex(st.SpaceID()), c.relationKey)
		if err != nil {
			log.Errorf("failed extracting headers for csv export: %v", err)
			return nil
		}
	}
	csvRows := [][]string{headersName}
	if len(c.filters) > 0 || len(c.sorts) > 0 {
		records, err := c.store.SpaceIndex(st.SpaceID()).Query(database.Query{Filters: database.FiltersFromProto(c.filters), Sorts: database.SortsFromProto(c.sorts)})
		if err != nil {
			return nil
		}
	}
	objects := st.GetStoreSlice(template.CollectionStoreKey)
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
