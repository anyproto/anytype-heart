package notion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/database"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/search"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestNotion_GetSnapshots(t *testing.T) {
	t.Run("internal error from Notion", func(t *testing.T) {
		// given
		converter := &Notion{}
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"object":"error","status":500,"code":"internal_error","message":"internal server error"}`))
		}))
		defer s.Close()
		c := client.NewClient()
		c.BasePath = s.URL
		converter.search = search.New(c)
		p := process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})

		// when
		_, ce := converter.GetSnapshots(
			context.Background(),
			&pb.RpcObjectImportRequest{
				Params: &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{ApiKey: "key"}},
				Type:   model.Import_Markdown,
				Mode:   pb.RpcObjectImportRequest_IGNORE_ERRORS,
			},
			p,
		)

		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Notion), common.ErrNotionServerIsUnavailable))
	})
	t.Run("rate limit error from Notion", func(t *testing.T) {
		// given
		converter := &Notion{}
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"object":"error","status":429,"code":"rate_limit_error","message":"rate limit error"}`))
		}))
		defer s.Close()
		c := client.NewClient()
		c.BasePath = s.URL
		converter.search = search.New(c)
		p := process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})

		// when
		_, ce := converter.GetSnapshots(
			context.Background(),
			&pb.RpcObjectImportRequest{
				Params: &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{ApiKey: "key"}},
				Type:   model.Import_Markdown,
				Mode:   pb.RpcObjectImportRequest_IGNORE_ERRORS,
			},
			p,
		)

		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Notion), common.ErrNotionServerExceedRateLimit))
	})
	t.Run("no objects in integration", func(t *testing.T) {
		// given
		converter := &Notion{}
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"object":"list","results":[]}`))
		}))
		defer s.Close()
		c := client.NewClient()
		c.BasePath = s.URL
		converter.search = search.New(c)
		p := process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})

		// when
		_, ce := converter.GetSnapshots(
			context.Background(),
			&pb.RpcObjectImportRequest{
				Params: &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{ApiKey: "key"}},
				Type:   model.Import_Markdown,
				Mode:   pb.RpcObjectImportRequest_IGNORE_ERRORS,
			},
			p,
		)

		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Notion), common.ErrNoObjectInIntegration))
	})
}

func TestNotion_getUniqueProperties(t *testing.T) {
	t.Run("Page and Database have the same property - 1 unique item", func(t *testing.T) {
		// given
		converter := &Notion{}

		databases := []database.Database{
			{
				Properties: map[string]property.DatabasePropertyHandler{
					"Name": &property.DatabaseTitle{},
				},
			},
		}
		pages := []page.Page{
			{
				Properties: map[string]property.Object{
					"Name": &property.TitleItem{},
				},
			},
		}

		// when
		properties := converter.getUniqueProperties(databases, pages)

		// then
		assert.Len(t, properties, 1)
	})
	t.Run("Page and Database have the different properties - 2 unique item", func(t *testing.T) {
		// given
		converter := &Notion{}
		db := []database.Database{
			{
				Properties: map[string]property.DatabasePropertyHandler{
					"Name": &property.DatabaseTitle{},
				},
			},
		}
		pages := []page.Page{
			{
				Properties: map[string]property.Object{
					"Name1": &property.TitleItem{},
				},
			},
		}

		// when
		properties := converter.getUniqueProperties(db, pages)

		// then
		assert.Len(t, properties, 2)
	})
	t.Run("Page and Database have the 2 different properties and 1 same property - 3 unique item", func(t *testing.T) {
		// given
		converter := &Notion{}
		databases := []database.Database{
			{
				Properties: map[string]property.DatabasePropertyHandler{
					"Name":   &property.DatabaseTitle{},
					"Name 1": &property.DatabaseTitle{},
				},
			},
		}
		pages := []page.Page{
			{
				Properties: map[string]property.Object{
					"Name":   &property.TitleItem{},
					"Name 2": &property.TitleItem{},
				},
			},
		}

		// when
		properties := converter.getUniqueProperties(databases, pages)

		// then
		assert.Len(t, properties, 3)
	})
}
