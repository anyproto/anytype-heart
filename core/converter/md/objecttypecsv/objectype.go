package objecttypecsv

import (
	"bytes"
	"encoding/csv"
	"io"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	objectTypesDirectory = "object_types"
	ext                  = ".csv"
)

type writer interface {
	WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error)
}

type File interface {
	WriteRecord(state *state.State, filename string) error
	Flush(fn writer) error
}

type ObjectTypeFiles map[string]File

func (c ObjectTypeFiles) GetFileOrCreate(name string, spaceIndex spaceindex.Store) (File, error) {
	fileName := filepath.Join(objectTypesDirectory, name+ext)
	converter, ok := c[fileName]
	if ok {
		return converter, nil
	}
	newConverter := newObjectType(fileName, spaceIndex)
	c[fileName] = newConverter
	return newConverter, nil
}

func (c ObjectTypeFiles) Flush(wr writer) error {
	var multiErr error
	for _, converter := range c {
		err := converter.Flush(wr)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	return multiErr
}

type objectType struct {
	fileName   string
	csvRows    [][]string
	spaceIndex spaceindex.Store
}

func newObjectType(fileName string, spaceIndex spaceindex.Store) *objectType {
	return &objectType{fileName: fileName, spaceIndex: spaceIndex}
}

func (o *objectType) WriteRecord(state *state.State, filename string) error {
	details := state.Details()
	localDetails := state.LocalDetails()
	var (
		headers, headersName []string
		err                  error
	)
	if len(o.csvRows) == 0 {
		headers, headersName, err = o.collectHeaders(details, localDetails)
		if err != nil {
			return err
		}
		o.csvRows = append(o.csvRows, headersName)
	}
	o.fillCSVRows(headers, filename, details, localDetails)
	return nil
}

func (o *objectType) fillCSVRows(headers []string, filename string, details, localDetails *domain.Details) {
	values := make([]string, 0, len(o.csvRows[0]))
	for _, header := range headers {
		if header == bundle.RelationKeySourceFilePath.URL() {
			values = append(values, filename)
			continue
		}
		relationKey := domain.RelationKey(header)
		values = append(values, common.GetValueAsString(details, localDetails, relationKey))
	}

	o.csvRows = append(o.csvRows, values)
}

func (o *objectType) collectHeaders(details, localDetails *domain.Details) ([]string, []string, error) {
	headersKeys := make([]string, 0, details.Len()+localDetails.Len()+1)
	headersKeys = append(headersKeys, bundle.RelationKeySourceFilePath.URL())

	for key, _ := range details.Iterate() {
		headersKeys = append(headersKeys, key.URL())
	}

	for key, _ := range localDetails.Iterate() {
		headersKeys = append(headersKeys, key.URL())
	}

	records, err := o.spaceIndex.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyUniqueKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(headersKeys),
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	recordMap := make(map[string]string, len(records))
	for _, record := range records {
		recordMap[record.Details.GetString(bundle.RelationKeyRelationKey)] = record.Details.GetString(bundle.RelationKeyName)
	}
	headersName := make([]string, 0, len(headersKeys))
	for _, key := range headersKeys {
		if name, exists := recordMap[key]; exists {
			headersName = append(headersName, name)
		} else {
			headersName = append(headersName, key)
		}
	}
	return headersKeys, headersName, nil
}

func (o *objectType) Flush(fn writer) error {
	if len(o.csvRows) == 0 {
		return nil
	}
	buffer := bytes.NewBuffer(nil)
	csvWriter := csv.NewWriter(buffer)
	defer csvWriter.Flush()
	err := csvWriter.WriteAll(o.csvRows)
	if err != nil {
		return err
	}
	return fn.WriteFile(o.fileName, buffer, time.Now().Unix())
}
