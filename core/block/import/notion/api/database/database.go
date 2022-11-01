package database

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
)

type DatabaseID string

const (
	endpoint = "/search"
	pageSize = 100
	objectType = "database"
)

type DatabaseService struct {
	client *client.Client
}

func New() *DatabaseService {
	return &DatabaseService{
		client: client.NewClient(),
	}
}

type Database struct {
	Object         string         `json:"object"`
	ID             string         `json:"id"`
	CreatedTime    time.Time      `json:"created_time"`
	LastEditedTime time.Time      `json:"last_edited_time"`
	CreatedBy      api.User       `json:"created_by,omitempty"`
	LastEditedBy   api.User       `json:"last_edited_by,omitempty"`
	Title          []api.RichText `json:"title"`
	Parent         Parent         `json:"parent"`
	URL            string         `json:"url"`
	Properties     interface{}    `json:"properties"` // can't support it for databases yet
	Description    []api.RichText `json:"description"`
	IsInline       bool           `json:"is_inline"`
	Archived       bool           `json:"archived"`
	Icon           *api.Icon      `json:"icon,omitempty"`
	Cover          *block.Image   `json:"cover,omitempty"`
}

type Parent struct {
	Type   string `json:"type,omitempty"`
	PageID string `json:"page_id"`
}

type ListDatabasesResponse struct {
	Results    []Database `json:"results"`
	HasMore    bool       `json:"has_more"`
	NextCursor *string    `json:"next_cursor"`
}

func (ds *DatabaseService) GetDatabase(ctx context.Context, mode pb.RpcObjectImportRequestMode, apiKey string) *converter.Response {
	var convereterError = converter.ConvertError{}
	databases, notionErr, err := ds.listDatabases(ctx, apiKey, pageSize)
	if err != nil {
		convereterError.Add(endpoint, err)
		return &converter.Response{Error: convereterError} 
	}
	if notionErr != nil {
		convereterError.Add(endpoint, notionErr.Error())
		return &converter.Response{Error: convereterError}
	}
	return ds.mapDatabasesToSnaphots(ctx, mode, databases, convereterError)
}

func (ds *DatabaseService) listDatabases(ctx context.Context, apiKey string, pageSize int64) ([]Database, *client.NotionErrorResponse, error) {
	var (
		hasMore         = true
		body            = &bytes.Buffer{}
		resultDatabases = make([]Database, 0)
		startCursor     int64
	)
	type Option struct {
		PageSize    int64 `json:"page_size,omitempty"`
		StartCursor int64 `json:"start_cursor,omitempty"`
	}
	err := json.NewEncoder(body).Encode(&Option{PageSize: pageSize, StartCursor: startCursor})

	if err != nil {
		return nil, nil, fmt.Errorf("ListDatabases: %s", err)
	}

	req, err := ds.client.PrepareRequest(ctx, apiKey, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, nil, fmt.Errorf("ListDatabases: %s", err)
	}

	for hasMore {
		res, err := ds.client.HttpClient.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("ListDatabases: %s", err)
		}
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)

		if err != nil {
			return nil, nil, err
		}
		var databases ListDatabasesResponse
		if res.StatusCode != http.StatusOK {
			notionErr := client.TransformHttpCodeToError(b)
			if notionErr == nil {
				return nil, nil, fmt.Errorf("failed http request, %d code", res.StatusCode)
			}
			return nil, notionErr, nil
		}

		err = json.Unmarshal(b, &databases)

		if err != nil {
			return nil, nil, err
		}

		for _, d := range databases.Results {
			if d.Object == objectType {
				resultDatabases = append(resultDatabases, d)
			}
		}

		if !databases.HasMore {
			hasMore = false
			continue
		}

		startCursor += pageSize

	}
	return resultDatabases, nil, nil
}

func (ds *DatabaseService) mapDatabasesToSnaphots(ctx context.Context, mode pb.RpcObjectImportRequestMode, databases []Database, convereterError converter.ConvertError) *converter.Response {
	var allSnapshots = make([]*converter.Snapshot, 0)
	for _, d := range databases {
		tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
		if err != nil {
			convereterError.Add(d.ID, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return &converter.Response{Error: convereterError}
			} else {
				continue
			}
		}
		snapshot := ds.transformDatabase(d)
		
		allSnapshots = append(allSnapshots, &converter.Snapshot{
			Id:       tid.String(),
			FileName: d.URL,
			Snapshot: snapshot,
		})
	}
	if convereterError.IsEmpty() {
		return &converter.Response{Snapshots: allSnapshots, Error: nil} 
	}
	return &converter.Response{Snapshots: allSnapshots, Error: convereterError} 
}

func (ds *DatabaseService) transformDatabase(d Database) *model.SmartBlockSnapshotBase {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(d.URL)
	if len(d.Title) > 0{
		details[bundle.RelationKeyName.String()] = pbtypes.String(d.Title[0].PlainText)
	}
	if d.Icon != nil && d.Icon.Emoji != nil {
		details[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*d.Icon.Emoji)
	}
	details[bundle.RelationKeyCreatedDate.String()] = pbtypes.String(d.CreatedTime.String())
	details[bundle.RelationKeyCreator.String()] = pbtypes.String(d.CreatedBy.Name)
	details[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(d.Archived)
	details[bundle.RelationKeyLastModifiedDate.String()] = pbtypes.String(d.LastEditedTime.String())
	details[bundle.RelationKeyLastModifiedBy.String()] = pbtypes.String(d.LastEditedBy.Name)
	details[bundle.RelationKeyDescription.String()] = pbtypes.String(api.RichTextToDescription(d.Description))

	snapshot := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{},
		Details: &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
		Collections: nil,
	}

	return snapshot
}
