package csv

import (
	"bytes"
	"encoding/csv"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/converter/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("csv-export")

type FileNamer interface {
	Get(path, hash, title, ext string) (name string)
}

type Csv struct {
	knownDocs map[string]*domain.Details
	fn        FileNamer
}

func NewCsv(fn FileNamer) *Csv {
	return &Csv{fn: fn}
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

	headers := []string{bundle.RelationKeySourceFilePath.String()}
	headers = append(headers, extractHeaders(dataview)...)
	csvRows := [][]string{headers}

	objects := st.GetStoreSlice(template.CollectionStoreKey)
	for _, object := range objects {
		if objectDetail, ok := c.knownDocs[object]; ok {
			csvRows = append(csvRows, c.getCSVRow(objectDetail, headers, filename))
		}
	}
	return writeCSV(csvRows)
}

func (c *Csv) getCSVRow(details *domain.Details, headers []string, filename string) []string {
	values := make([]string, len(headers))
	for i, header := range headers {
		if header == bundle.RelationKeySourceFilePath.String() {
			values[i] = filename
			continue
		}
		relationKey := domain.RelationKey(header)
		values[i] = common.GetValueAsString(details, nil, relationKey)
	}
	return values
}

func findDataviewBlock(st *state.State) *model.Block {
	for _, block := range st.Blocks() {
		if block.GetDataview() != nil {
			return block
		}
	}
	return nil
}

func extractHeaders(dataview *model.BlockContentDataview) []string {
	var headers []string
	for _, relation := range dataview.Views[0].Relations {
		if relation.IsVisible {
			headers = append(headers, relation.Key)
		}
	}
	return headers
}

func writeCSV(csvRows [][]string) []byte {
	buffer := bytes.NewBuffer(nil)
	csvWriter := csv.NewWriter(buffer)

	if err := csvWriter.WriteAll(csvRows); err != nil {
		log.Errorf("failed to write CSV rows: %s", err.Error())
		return nil
	}

	csvWriter.Flush()
	return buffer.Bytes()
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
