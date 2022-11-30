package block

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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
	mapper *Mapper
}

func New(client *client.Client) *Service {
	return &Service{
		client: client,
		mapper: &Mapper{},
	}
}

type Response struct {
	Results    []interface{} `json:"results"`
	HasMore    bool          `json:"has_more"`
	NextCursor *string       `json:"next_cursor"`
	Block      Block         `json:"block"`
}

func (s *Service) GetBlocksAndChildren(ctx context.Context, pageID, apiKey string, pageSize int64, mode pb.RpcObjectImportRequestMode) ([]interface{}, *converter.ConvertError) {
	ce := &converter.ConvertError{}
	allBlocks := make([]interface{}, 0)
	blocks, err := s.getBlocks(ctx, pageID, apiKey, pageSize)
	if err != nil {
		ce.Add(endpoint, err)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, ce
		}
	}

	for _, b := range blocks {
		switch bl := b.(type) {
		case *Heading1Block, *Heading2Block, *Heading3Block, *CodeBlock, *EquationBlock, *FileBlock, *ImageBlock, *VideoBlock, *PdfBlock, *DividerBlock:
			allBlocks = append(allBlocks, bl)
		case *ParagraphBlock:
			if bl.HasChildren {
				children, err := s.GetBlocksAndChildren(ctx, bl.ID, apiKey, pageSize, mode)
				if err != nil {
					ce.Merge(*err)
					if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return nil, ce
					}
				}
				bl.Paragraph.Children = children
			}
			allBlocks = append(allBlocks, bl)
		case *CalloutBlock:
			if bl.HasChildren {
				children, err := s.GetBlocksAndChildren(ctx, bl.ID, apiKey, pageSize, mode)
				if err != nil {
					ce.Merge(*err)
					if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return nil, ce
					}
				}
				bl.Callout.Children = children
			}
			allBlocks = append(allBlocks, bl)
		case *QuoteBlock:
			if bl.HasChildren {
				children, err := s.GetBlocksAndChildren(ctx, bl.ID, apiKey, pageSize, mode)
				if err != nil {
					ce.Merge(*err)
					if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return nil, ce
					}
				}
				bl.Quote.Children = children
			}
			allBlocks = append(allBlocks, bl)
		case *BulletedListBlock:
			if bl.HasChildren {
				children, err := s.GetBlocksAndChildren(ctx, bl.ID, apiKey, pageSize, mode)
				if err != nil {
					ce.Merge(*err)
					if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return nil, ce
					}
				}
				bl.BulletedList.Children = children
			}
			allBlocks = append(allBlocks, bl)
		case *NumberedListBlock:
			if bl.HasChildren {
				children, err := s.GetBlocksAndChildren(ctx, bl.ID, apiKey, pageSize, mode)
				if err != nil {
					ce.Merge(*err)
					if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return nil, ce
					}
				}
				bl.NumberedList.Children = children
			}
			allBlocks = append(allBlocks, bl)
		case *ToggleBlock:
			if bl.HasChildren {
				children, err := s.GetBlocksAndChildren(ctx, bl.ID, apiKey, pageSize, mode)
				if err != nil {
					ce.Merge(*err)
					if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return nil, ce
					}
				}
				bl.Toggle.Children = children
			}
			allBlocks = append(allBlocks, bl)
		case *ToDoBlock:
			if bl.HasChildren {
				children, err := s.GetBlocksAndChildren(ctx, bl.ID, apiKey, pageSize, mode)
				if err != nil {
					ce.Merge(*err)
					if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
						return nil, ce
					}
				}
				bl.ToDo.Children = children
			}
			allBlocks = append(allBlocks, bl)
		}
	}
	return allBlocks, nil
}

func (s *Service) MapNotionBlocksToAnytype(blocks []interface{}, notionPagesIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID map[string]string) []*model.Block {
	allBlocks, _ := s.mapper.MapBlocks(blocks, notionPagesIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
	return allBlocks
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
				p := ParagraphBlock{}
				err = json.Unmarshal(buffer, &p)
				if err != nil {
					continue
				}
				blocks = append(blocks, &p)
			case Heading1:
				h := Heading1Block{}
				err = json.Unmarshal(buffer, &h)
				if err != nil {
					continue
				}
				blocks = append(blocks, &h)
			case Heading2:
				h := Heading2Block{}
				err = json.Unmarshal(buffer, &h)
				if err != nil {
					continue
				}
				blocks = append(blocks, &h)
			case Heading3:
				h := Heading3Block{}
				err = json.Unmarshal(buffer, &h)
				if err != nil {
					continue
				}
				blocks = append(blocks, &h)
			case Callout:
				c := CalloutBlock{}
				err = json.Unmarshal(buffer, &c)
				if err != nil {
					continue
				}
				blocks = append(blocks, &c)
			case Quote:
				q := QuoteBlock{}
				err = json.Unmarshal(buffer, &q)
				if err != nil {
					continue
				}
				blocks = append(blocks, &q)
			case BulletList:
				list := BulletedListBlock{}
				err = json.Unmarshal(buffer, &list)
				if err != nil {
					continue
				}
				blocks = append(blocks, &list)
			case NumberList:
				nl := NumberedListBlock{}
				err = json.Unmarshal(buffer, &nl)
				if err != nil {
					continue
				}
				blocks = append(blocks, &nl)
			case Toggle:
				t := ToggleBlock{}
				err = json.Unmarshal(buffer, &t)
				if err != nil {
					continue
				}
				blocks = append(blocks, &t)
			case Code:
				c := CodeBlock{}
				err = json.Unmarshal(buffer, &c)
				if err != nil {
					continue
				}
				blocks = append(blocks, &c)
			case Equation:
				e := EquationBlock{}
				err = json.Unmarshal(buffer, &e)
				if err != nil {
					continue
				}
				blocks = append(blocks, &e)
			case ToDo:
				t := ToDoBlock{}
				err = json.Unmarshal(buffer, &t)
				if err != nil {
					continue
				}
				blocks = append(blocks, &t)
			case File:
				f := FileBlock{}
				err = json.Unmarshal(buffer, &f)
				if err != nil {
					continue
				}
				blocks = append(blocks, &f)
			case Image:
				i := ImageBlock{}
				err = json.Unmarshal(buffer, &i)
				if err != nil {
					continue
				}
				blocks = append(blocks, &i)
			case Video:
				v := VideoBlock{}
				err = json.Unmarshal(buffer, &v)
				if err != nil {
					continue
				}
				blocks = append(blocks, &v)
			case Pdf:
				p := PdfBlock{}
				err = json.Unmarshal(buffer, &p)
				if err != nil {
					continue
				}
				blocks = append(blocks, &p)
			case Divider:
				d := DividerBlock{}
				err = json.Unmarshal(buffer, &d)
				if err != nil {
					continue
				}
				blocks = append(blocks, &d)
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
