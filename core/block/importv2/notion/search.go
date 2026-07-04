// Package notion is the streaming Notion converter: the /search crawl is
// pass 1 (identity only — ids, titles, parents; bodies are released), pass 2
// re-fetches each page and streams databases, pages, relations, options and
// files through the sink one object at a time. See docs/ImportV2Design.md
// §11.2.
package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
)

const searchPageSize = 100

// entityKind distinguishes the pass-1 stub flavors under the data-sources
// API: collections come from data_source objects (a database owns one or
// more of them); a bare database result is handled defensively by resolving
// its data sources in pass 2.
type entityKind int

const (
	kindPage entityKind = iota
	kindDataSource
	kindDatabase
)

// Entity is the pass-1 stub of one workspace object: everything the
// converter retains about it between passes (small strings only).
type Entity struct {
	Id    string
	Kind  entityKind
	Title string
	// Parent is the hierarchy location: for pages the page/data-source
	// parent; for data sources the owning database (their location parent
	// arrives separately as database_parent).
	Parent Parent
	// DatabaseId is the owning database for data-source stubs — the alias
	// child_database blocks and rich-text mentions still reference.
	DatabaseId string
}

func (e Entity) isCollectionLike() bool {
	return e.Kind != kindPage
}

// Parent locates an entity in the workspace hierarchy.
type Parent struct {
	Type         string `json:"type"` // workspace | page_id | database_id | data_source_id | block_id
	PageId       string `json:"page_id"`
	DatabaseId   string `json:"database_id"`
	DataSourceId string `json:"data_source_id"`
	BlockId      string `json:"block_id"`
	Workspace    bool   `json:"workspace"`
}

type richText struct {
	PlainText string `json:"plain_text"`
	Href      string `json:"href"`
	Type      string `json:"type"`
	Text      *struct {
		Content string `json:"content"`
		Link    *struct {
			Url string `json:"url"`
		} `json:"link"`
	} `json:"text"`
	Annotations *annotations `json:"annotations"`
	Mention     *mention     `json:"mention"`
	Equation    *struct {
		Expression string `json:"expression"`
	} `json:"equation"`
}

type annotations struct {
	Bold          bool   `json:"bold"`
	Italic        bool   `json:"italic"`
	Strikethrough bool   `json:"strikethrough"`
	Underline     bool   `json:"underline"`
	Code          bool   `json:"code"`
	Color         string `json:"color"`
}

type mention struct {
	Type string `json:"type"` // user | page | database | date | link_preview | custom_emoji | template_mention
	Page *struct {
		Id string `json:"id"`
	} `json:"page"`
	Database *struct {
		Id string `json:"id"`
	} `json:"database"`
	Date *dateValue `json:"date"`
	User *struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	LinkPreview *struct {
		Url string `json:"url"`
	} `json:"link_preview"`
	CustomEmoji     *customEmoji `json:"custom_emoji"`
	TemplateMention *struct {
		Type string `json:"type"` // template_mention_date | template_mention_user
		Date string `json:"template_mention_date"`
		User string `json:"template_mention_user"`
	} `json:"template_mention"`
}

type dateValue struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	TimeZone string `json:"time_zone"`
}

type customEmoji struct {
	Name string `json:"name"`
	Url  string `json:"url"`
}

// searchResult decodes only the stub fields of one /search result; the rest
// of the body is released with the response.
type searchResult struct {
	Object string `json:"object"` // page | data_source | database
	Id     string `json:"id"`
	Parent Parent `json:"parent"`
	// DatabaseParent is a data source's LOCATION (page/workspace); its
	// plain parent is the owning database.
	DatabaseParent *Parent    `json:"database_parent"`
	Title          []richText `json:"title"` // data sources/databases carry it directly
	Name           string     `json:"name"`  // data-source display name fallback
	// Properties is the page's property VALUES or a data source's property
	// SCHEMA — the title field is a rich-text array in the former and an
	// empty config object in the latter, so it stays raw and is parsed
	// leniently (only pages read a title out of it).
	Properties map[string]struct {
		Type  string          `json:"type"`
		Title json.RawMessage `json:"title"`
	} `json:"properties"`
}

type searchResponse struct {
	Results    []searchResult `json:"results"`
	HasMore    bool           `json:"has_more"`
	NextCursor *string        `json:"next_cursor"`
}

// crawlSearch paginates POST /search over everything the integration can
// see, yielding one stub per entity. truncated=true reports a pagination
// inconsistency after which the crawl stopped early — the caller surfaces
// it as a data-loss warning instead of failing the run (a v1-observed
// has_more+null-cursor response used to abort the whole import).
func crawlSearch(ctx context.Context, c *client.Client, yield func(Entity) error) (truncated bool, err error) {
	body := map[string]any{"page_size": searchPageSize}
	for {
		var response searchResponse
		if err := c.Request(ctx, http.MethodPost, "/search", body, &response); err != nil {
			return false, fmt.Errorf("search: %w", err)
		}
		for _, result := range response.Results {
			entity := Entity{
				Id:     result.Id,
				Parent: result.Parent,
				Title:  titleOf(result),
			}
			switch result.Object {
			case "data_source":
				entity.Kind = kindDataSource
				entity.DatabaseId = result.Parent.DatabaseId
				if result.DatabaseParent != nil {
					entity.Parent = *result.DatabaseParent
				}
			case "database":
				entity.Kind = kindDatabase
				entity.DatabaseId = result.Id
			}
			if err := yield(entity); err != nil {
				return false, err
			}
		}
		if !response.HasMore {
			return false, nil
		}
		// v1 dereferenced next_cursor unconditionally and panicked on the
		// (observed in the wild) has_more=true + null cursor combination.
		if response.NextCursor == nil || *response.NextCursor == "" {
			return true, nil
		}
		body["start_cursor"] = *response.NextCursor
	}
}

func titleOf(result searchResult) string {
	if result.Object == "database" || result.Object == "data_source" {
		if title := plainText(result.Title); title != "" {
			return title
		}
		return result.Name
	}
	for _, property := range result.Properties {
		if property.Type != "title" {
			continue
		}
		// A page's title value is a rich-text array; a data source's title
		// schema is an object — accept only the array form.
		if len(property.Title) > 0 && property.Title[0] == '[' {
			var runs []richText
			if json.Unmarshal(property.Title, &runs) == nil {
				return plainText(runs)
			}
		}
		return ""
	}
	return ""
}

func plainText(runs []richText) string {
	text := ""
	for _, run := range runs {
		text += run.PlainText
	}
	return text
}
