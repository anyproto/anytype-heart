package block

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
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
	mode pb.RpcObjectImportRequestMode) ([]interface{}, converter.ConvertError) {
	ce := converter.ConvertError{}
	allBlocks := make([]interface{}, 0)
	blocks, err := s.getBlocks(ctx, pageID, apiKey, pageSize)
	if err != nil {
		ce.Add(endpoint, err)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, ce
		}
	}
	for _, b := range blocks {
		cs, ok := b.(ChildSetter)
		if !ok {
			allBlocks = append(allBlocks, b)
			continue
		}
		if cs.HasChild() {
			children, err := s.GetBlocksAndChildren(ctx, cs.GetID(), apiKey, pageSize, mode)
			if err != nil {
				ce.Merge(err)
			}
			if err != nil && mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, ce
			}
			cs.SetChildren(children)
		}
		allBlocks = append(allBlocks, b)
	}
	return allBlocks, nil
}

func (s *Service) MapNotionBlocksToAnytype(req *MapRequest) *MapResponse {
	return MapBlocks(req)
}

func (s *Service) getBlocks(ctx context.Context, pageID, apiKey string, pagination int64) ([]interface{}, error) {
	var (
		hasMore = true
		body    = &bytes.Buffer{}
		blocks  = make([]interface{}, 0)
		cursor  string
	)

	for hasMore {
		url := fmt.Sprintf(endpoint, pageID)

		req, err := s.client.PrepareRequest(ctx, apiKey, http.MethodGet, url, body)

		if err != nil {
			return nil, fmt.Errorf("GetBlocks: %s", err)
		}
		query := req.URL.Query()

		if cursor != "" {
			query.Add(startCursor, cursor)
		}

		query.Add(pageSize, strconv.FormatInt(pagination, 10))

		req.URL.RawQuery = query.Encode()

		res, err := s.client.HttpClient.Do(req)

		if err != nil {
			return nil, fmt.Errorf("GetBlocks: %s", err)
		}
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)

		if err != nil {
			return nil, err
		}
		var objects Response
		if res.StatusCode != http.StatusOK {
			notionErr := client.TransformHttpCodeToError(b)
			if notionErr == nil {
				return nil, fmt.Errorf("GetBlocks: failed http request, %d code", res.StatusCode)
			}
			return nil, notionErr
		}

		err = json.Unmarshal(b, &objects)

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
			switch BlockType(blockMap["type"].(string)) {
			case Paragraph:
				var p ParagraphBlock
				err = json.Unmarshal(buffer, &p)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &p)
			case Heading1:
				var h Heading1Block
				err = json.Unmarshal(buffer, &h)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &h)
			case Heading2:
				var h Heading2Block
				err = json.Unmarshal(buffer, &h)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &h)
			case Heading3:
				var h Heading3Block
				err = json.Unmarshal(buffer, &h)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &h)
			case Callout:
				var c CalloutBlock
				err = json.Unmarshal(buffer, &c)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &c)
			case Quote:
				var q QuoteBlock
				err = json.Unmarshal(buffer, &q)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &q)
			case BulletList:
				var list BulletedListBlock
				err = json.Unmarshal(buffer, &list)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &list)
			case NumberList:
				var nl NumberedListBlock
				err = json.Unmarshal(buffer, &nl)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &nl)
			case Toggle:
				var t ToggleBlock
				err = json.Unmarshal(buffer, &t)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &t)
			case Code:
				var c CodeBlock
				err = json.Unmarshal(buffer, &c)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &c)
			case Equation:
				var e EquationBlock
				err = json.Unmarshal(buffer, &e)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &e)
			case ToDo:
				var t ToDoBlock
				err = json.Unmarshal(buffer, &t)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &t)
			case File:
				var f FileBlock
				err = json.Unmarshal(buffer, &f)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &f)
			case Image:
				var i ImageBlock
				err = json.Unmarshal(buffer, &i)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &i)
			case Video:
				var v VideoBlock
				err = json.Unmarshal(buffer, &v)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &v)
			case Pdf:
				var p PdfBlock
				err = json.Unmarshal(buffer, &p)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &p)
			case Bookmark:
				var b BookmarkBlock
				err = json.Unmarshal(buffer, &b)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &b)
			case Divider:
				var d DividerBlock
				err = json.Unmarshal(buffer, &d)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &d)
			case TableOfContents:
				var t TableOfContentsBlock
				err = json.Unmarshal(buffer, &t)

				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}

				blocks = append(blocks, &t)
			case Embed:
				var e EmbedBlock
				err = json.Unmarshal(buffer, &e)
				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}
				blocks = append(blocks, &e)
			case LinkPreview:
				var lp LinkPreviewBlock
				err = json.Unmarshal(buffer, &lp)
				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}
				blocks = append(blocks, &lp)
			case ChildDatabase:
				var c ChildDatabaseBlock
				err = json.Unmarshal(buffer, &c)
				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}
				blocks = append(blocks, &c)
			case ChildPage:
				var c ChildPageBlock
				err = json.Unmarshal(buffer, &c)
				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}
				blocks = append(blocks, &c)
			case LinkToPage:
				var l LinkToPageBlock
				err = json.Unmarshal(buffer, &l)
				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}
				blocks = append(blocks, &l)
			case Unsupported:
				var u UnsupportedBlock
				err = json.Unmarshal(buffer, &u)
				if err != nil {
					logger.With(zap.String("method", "getBlocks")).Error(err)
					continue
				}
				blocks = append(blocks, &u)
			}
		}

		if !objects.HasMore {
			hasMore = false
			continue
		}

		cursor = *objects.NextCursor

	}
	return blocks, nil
}
