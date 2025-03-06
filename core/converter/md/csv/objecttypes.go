package csv

import (
	"io"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter/md/csv/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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
	fileName    string
	csvRows     [][]string
	headers     []string
	headersName []string
	spaceIndex  spaceindex.Store
}

func newObjectType(fileName string, spaceIndex spaceindex.Store) *objectType {
	return &objectType{fileName: fileName, spaceIndex: spaceIndex}
}

func (o *objectType) WriteRecord(state *state.State, filename string) error {
	details := state.Details()
	localDetails := state.LocalDetails()
	var err error
	if len(o.csvRows) == 0 {
		o.headers, o.headersName, err = o.collectHeaders(details, localDetails)
		if err != nil {
			return err
		}
		o.csvRows = append(o.csvRows, o.headersName)
	}
	o.fillCSVRows(o.headers, filename, details, localDetails)
	return nil
}

func (o *objectType) fillCSVRows(headers []string, filename string, details, localDetails *domain.Details) {
	values := make([]string, 0, len(o.csvRows[0]))
	for _, header := range headers {
		if header == bundle.RelationKeySourceFilePath.String() {
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
	headersKeys = append(headersKeys, bundle.RelationKeySourceFilePath.String())

	for key := range details.Iterate() {
		headersKeys = append(headersKeys, key.String())
	}

	for key := range localDetails.Iterate() {
		headersKeys = append(headersKeys, key.String())
	}

	headersName, err := common.ExtractHeaders(o.spaceIndex, headersKeys)
	if err != nil {
		return nil, nil, err
	}
	return headersKeys, headersName, nil
}

func (o *objectType) Flush(fn writer) error {
	if len(o.csvRows) == 0 {
		return nil
	}
	buffer, err := common.WriteCSV(o.csvRows)
	if err != nil {
		return err
	}
	return fn.WriteFile(o.fileName, buffer, time.Now().Unix())
}
