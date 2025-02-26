package csv

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/converter/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("csv-export")

type FileNamer interface {
	Get(path, hash, title, ext string) (name string)
}

type Csv struct {
	knownDocs  map[string]*domain.Details
	fn         FileNamer
	spaceIndex spaceindex.Store
}

func NewCsv(fn FileNamer, spaceIndex spaceindex.Store) *Csv {
	return &Csv{fn: fn, spaceIndex: spaceIndex}
}

func (c *Csv) Convert(st *state.State, sbType model.SmartBlockType, filename string) []byte {
	block := findDataviewBlock(st)
	if block == nil {
		return nil
	}
	dataview := block.GetDataview()
	if len(dataview.Views) == 0 {
		return nil
	}
	headers, headersName, err := c.extractHeaders(dataview)
	if err != nil {
		log.Errorf("failed extracting headers for csv export: %v", err)
		return nil
	}
	csvRows := [][]string{headersName}

	objects := st.GetStoreSlice(template.CollectionStoreKey)
	for _, object := range objects {
		if objectDetail, ok := c.knownDocs[object]; ok {
			csvRows = append(csvRows, c.getCSVRow(objectDetail, headers, filename))
		}
	}
	result, err := common.WriteCSV(csvRows)
	if err != nil {
		log.Errorf("failed writing csv: %v", err)
		return nil
	}
	return result.Bytes()
}

func (c *Csv) getCSVRow(details *domain.Details, headers []string, filename string) []string {
	values := make([]string, len(headers))
	for i, header := range headers {
		if header == bundle.RelationKeySourceFilePath.URL() {
			values[i] = filename
			continue
		}
		relationKey := domain.RelationKey(header)
		values[i] = common.GetValueAsString(details, nil, relationKey)
	}
	return values
}

func (c *Csv) extractHeaders(dataview *model.BlockContentDataview) ([]string, []string, error) {
	headersKeys := []string{bundle.RelationKeySourceFilePath.String()}
	for _, relation := range dataview.Views[0].Relations {
		if relation.IsVisible {
			headersKeys = append(headersKeys, relation.Key)
		}
	}
	headersName, err := common.ExtractHeaders(c.spaceIndex, headersKeys)
	if err != nil {
		return nil, nil, err
	}
	return headersKeys, headersName, nil
}

func findDataviewBlock(st *state.State) *model.Block {
	for _, block := range st.Blocks() {
		if block.GetDataview() != nil {
			return block
		}
	}
	return nil
}

func (c *Csv) SetKnownDocs(docs map[string]*domain.Details) {
	c.knownDocs = docs
}

func (c *Csv) FileHashes() []string {
	return nil
}

func (c *Csv) ImageHashes() []string {
	return nil
}

func (c *Csv) Ext() string {
	return ".csv"
}
