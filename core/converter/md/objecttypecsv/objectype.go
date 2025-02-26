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
)

const (
	objectTypesDirectory = "object_types"
	ext                  = ".csv"
)

type writer interface {
	WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error)
}

type File interface {
	WriteRecord(state *state.State, filename string)
	Flush(fn writer) error
}

type ObjectTypeFiles map[string]File

func (c ObjectTypeFiles) GetFileOrCreate(name string) (File, error) {
	fileName := filepath.Join(objectTypesDirectory, name+ext)
	converter, ok := c[fileName]
	if ok {
		return converter, nil
	}
	newConverter := newObjectType(fileName)
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
	fileName string
	csvRows  [][]string
}

func newObjectType(fileName string) *objectType {
	return &objectType{fileName: fileName}
}

func (c *objectType) WriteRecord(state *state.State, filename string) {
	details := state.Details()
	localDetails := state.LocalDetails()

	if len(c.csvRows) == 0 {
		headers := []string{bundle.RelationKeySourceFilePath.String()}
		headers = append(headers, collectHeaders(details, localDetails)...)
		c.csvRows = append(c.csvRows, headers)
	}
	values := make([]string, 0, len(c.csvRows[0]))
	for _, header := range c.csvRows[0] {
		if header == bundle.RelationKeySourceFilePath.String() {
			values = append(values, filename)
			continue
		}
		relationKey := domain.RelationKey(header)
		values = append(values, common.GetValueAsString(details, localDetails, relationKey))
	}

	c.csvRows = append(c.csvRows, values)
}

func collectHeaders(details, localDetails *domain.Details) []string {
	headers := make([]string, 0, details.Len()+localDetails.Len())

	for key, _ := range details.Iterate() {
		headers = append(headers, key.String())
	}

	for key, _ := range localDetails.Iterate() {
		headers = append(headers, key.String())
	}

	return headers
}

func (c *objectType) Flush(fn writer) error {
	if len(c.csvRows) == 0 {
		return nil
	}
	buffer := bytes.NewBuffer(nil)
	csvWriter := csv.NewWriter(buffer)
	defer csvWriter.Flush()
	err := csvWriter.WriteAll(c.csvRows)
	if err != nil {
		return err
	}
	return fn.WriteFile(c.fileName, buffer, time.Now().Unix())
}
