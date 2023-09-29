package block

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var logger = logging.Logger("notion-get-blocks")

const (
	// Page is also a block, so we use endpoint to retrieve its children
	endpoint    = "/blocks/%s/children"
	startCursor = "start_cursor"
	pageSize    = "page_size"
)

type Service struct {
	client *client.Client
}

func New(client *client.Client) *Service {
	return &Service{
		client: client,
	}
}

type Response struct {
	Results    []interface{} `json:"results"`
	HasMore    bool          `json:"has_more"`
	NextCursor *string       `json:"next_cursor"`
	Block      Block         `json:"block"`
}

func (s *Service) GetBlocksAndChildren(ctx context.Context,
	pageID, apiKey string,
	pageSize int64,
	mode pb.RpcObjectImportRequestMode) ([]interface{}, *converter.ConvertError) {
	converterError := converter.NewError(mode)
	allBlocks := make([]interface{}, 0)
	blocks, err := s.getBlocks(ctx, pageID, apiKey, pageSize)
	if err != nil {
		converterError.Add(err)
		if converterError.ShouldAbortImport(0, pb.RpcObjectImportRequest_Notion) {
			return nil, converterError
		}
	}
	for _, b := range blocks {
		cs, ok := b.(ChildSetter)
		if !ok {
			allBlocks = append(allBlocks, b)
			continue
		}
		var (
			children []interface{}
			childErr *converter.ConvertError
		)
		if cs.HasChild() {
			children, childErr = s.GetBlocksAndChildren(ctx, cs.GetID(), apiKey, pageSize, mode)
			if !childErr.IsEmpty() {
				converterError.Merge(childErr)
				if childErr.ShouldAbortImport(0, pb.RpcObjectImportRequest_Notion) {
					return nil, childErr
				}
			}
		}
		cs.SetChildren(children)
		allBlocks = append(allBlocks, b)
	}
	return allBlocks, nil
}

func (s *Service) MapNotionBlocksToAnytype(req *api.NotionImportContext, blocks []interface{}, pageID string) *MapResponse {
	return MapBlocks(req, blocks, pageID)
}

func (s *Service) getBlocks(ctx context.Context, pageID, apiKey string, pagination int64) ([]interface{}, error) {
	var (
		hasMore = true
		blocks  = make([]interface{}, 0)
		cursor  string
	)

	for hasMore {
		objects, err := s.getBlocksResponse(ctx, pageID, apiKey, cursor, pagination)
		if err != nil {
			return nil, err
		}

		for _, b := range objects.Results {
			buffer, err := json.Marshal(b)
			if err != nil {
				logger.Errorf("GetBlocks: failed to marshal: %s", err)
				continue
			}
			blockMap := b.(map[string]interface{})
			blocks = append(blocks, s.fillBlocks(Type(blockMap["type"].(string)), buffer)...)
		}

		if !objects.HasMore {
			hasMore = false
			continue
		}

		cursor = *objects.NextCursor
		time.Sleep(time.Millisecond * 5) // to avoid rate limit

	}
	return blocks, nil
}

//nolint:funlen
func (*Service) fillBlocks(blockType Type, buffer []byte) []interface{} {
	blocks := make([]interface{}, 0)
	switch blockType {
	case TypeParagraph:
		var p ParagraphBlock
		err := json.Unmarshal(buffer, &p)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &p)
	case TypeHeading1:
		var h Heading1Block
		err := json.Unmarshal(buffer, &h)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &h)
	case TypeHeading2:
		var h Heading2Block
		err := json.Unmarshal(buffer, &h)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &h)
	case TypeHeading3:
		var h Heading3Block
		err := json.Unmarshal(buffer, &h)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &h)
	case TypeCallout:
		var c CalloutBlock
		err := json.Unmarshal(buffer, &c)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &c)
	case TypeQuote:
		var q QuoteBlock
		err := json.Unmarshal(buffer, &q)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &q)
	case TypeBulletList:
		var list BulletedListBlock
		err := json.Unmarshal(buffer, &list)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &list)
	case TypeNumberList:
		var nl NumberedListBlock
		err := json.Unmarshal(buffer, &nl)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &nl)
	case TypeToggle:
		var t ToggleBlock
		err := json.Unmarshal(buffer, &t)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &t)
	case TypeCode:
		var c CodeBlock
		err := json.Unmarshal(buffer, &c)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &c)
	case TypeEquation:
		var e EquationBlock
		err := json.Unmarshal(buffer, &e)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &e)
	case TypeToDo:
		var t ToDoBlock
		err := json.Unmarshal(buffer, &t)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &t)
	case TypeFile:
		var f FileBlock
		err := json.Unmarshal(buffer, &f)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &f)
	case TypeImage:
		var i ImageBlock
		err := json.Unmarshal(buffer, &i)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &i)
	case TypeVideo:
		var v VideoBlock
		err := json.Unmarshal(buffer, &v)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &v)
	case TypeAudio:
		var v AudioBlock
		err := json.Unmarshal(buffer, &v)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &v)
	case TypePdf:
		var p PdfBlock
		err := json.Unmarshal(buffer, &p)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &p)
	case TypeBookmark:
		var b BookmarkBlock
		err := json.Unmarshal(buffer, &b)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &b)
	case TypeDivider:
		var d DividerBlock
		err := json.Unmarshal(buffer, &d)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &d)
	case TypeTableOfContents:
		var t TableOfContentsBlock
		err := json.Unmarshal(buffer, &t)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &t)
	case TypeEmbed:
		var e EmbedBlock
		err := json.Unmarshal(buffer, &e)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &e)
	case TypeLinkPreview:
		var lp LinkPreviewBlock
		err := json.Unmarshal(buffer, &lp)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &lp)
	case TypeChildDatabase:
		var c ChildDatabaseBlock
		err := json.Unmarshal(buffer, &c)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &c)
	case TypeChildPage:
		var c ChildPageBlock
		err := json.Unmarshal(buffer, &c)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &c)
	case TypeLinkToPage:
		var l LinkToPageBlock
		err := json.Unmarshal(buffer, &l)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &l)
	case TypeUnsupported, TypeTemplate, TypeSyncedBlock:
		var u UnsupportedBlock
		err := json.Unmarshal(buffer, &u)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &u)
	case TypeTable:
		var t TableBlock
		err := json.Unmarshal(buffer, &t)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &t)
	case TypeTableRow:
		var t TableRowBlock
		err := json.Unmarshal(buffer, &t)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &t)
	case TypeColumnList:
		var cl ColumnListBlock
		err := json.Unmarshal(buffer, &cl)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		cl.SetChildren([]interface{}{})
		blocks = append(blocks, &cl)
	case TypeColumn:
		var cb ColumnBlock
		err := json.Unmarshal(buffer, &cb)
		if err != nil {
			logger.With(zap.String("method", "getBlocks")).Error(err)
			return blocks
		}
		blocks = append(blocks, &cb)
	}
	return blocks
}

func (s *Service) getBlocksResponse(ctx context.Context,
	pageID, apiKey, cursor string,
	pagination int64) (Response, error) {

	url := fmt.Sprintf(endpoint, pageID)
	req, err := s.client.PrepareRequest(ctx, apiKey, http.MethodGet, url, nil)

	if err != nil {
		return Response{}, fmt.Errorf("GetBlocks: %s", err)
	}
	query := req.URL.Query()

	if cursor != "" {
		query.Add(startCursor, cursor)
	}

	query.Add(pageSize, strconv.FormatInt(pagination, 10))

	req.URL.RawQuery = query.Encode()
	res, err := s.client.DoWithRetry(endpoint, 0, req)
	if err != nil {
		return Response{}, fmt.Errorf("GetBlocks: %s", err)
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)

	if err != nil {
		return Response{}, fmt.Errorf("GetBlocks: %s", err)
	}
	var objects Response
	if res.StatusCode != http.StatusOK {
		notionErr := client.TransformHTTPCodeToError(b)
		if notionErr == nil {
			return Response{}, fmt.Errorf("GetBlocks: failed http request, %d code", res.StatusCode)
		}

		return Response{}, notionErr
	}

	err = json.Unmarshal(b, &objects)

	if err != nil {
		return Response{}, fmt.Errorf("GetBlocks: %s", err)
	}
	return objects, nil
}
