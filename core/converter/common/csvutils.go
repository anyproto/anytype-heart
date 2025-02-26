package common

import (
	"bytes"
	"encoding/csv"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func ExtractHeaders(spaceIndex spaceindex.Store, keys []string) ([]string, error) {
	records, err := spaceIndex.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyUniqueKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(keys),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	recordMap := make(map[string]string, len(records))
	for _, record := range records {
		recordMap[record.Details.GetString(bundle.RelationKeyRelationKey)] = record.Details.GetString(bundle.RelationKeyName)
	}

	headersNames := make([]string, len(keys))
	for i, key := range keys {
		if name, exists := recordMap[key]; exists {
			headersNames[i] = name
		} else {
			headersNames[i] = key
		}
	}

	return headersNames, nil
}

func WriteCSV(csvRows [][]string) (*bytes.Buffer, error) {
	buffer := bytes.NewBuffer(nil)
	csvWriter := csv.NewWriter(buffer)
	defer csvWriter.Flush()

	err := csvWriter.WriteAll(csvRows)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}
