package web

import (
	"fmt"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web/parsers"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

const Name = "web"

type Converter struct {
	otc converter.ObjectTreeCreator
}

func init() {
	converter.RegisterFunc(NewConverter)
}

func NewConverter(core.Service, *collection.Service) converter.Converter {
	return &Converter{}
}

func (*Converter) GetParser(url string) parsers.Parser {
	for _, ps := range parsers.Parsers {
		p := ps()
		if p.MatchUrl(url) {
			return p
		}
	}
	return nil
}

func (c *Converter) GetSnapshots(req *pb.RpcObjectImportRequest) *converter.Response {
	we := converter.NewError()
	url, err := c.getParams(req.Params)
	if err != nil {
		we.Add(url, err)
		return &converter.Response{Error: we}
	}
	p := c.GetParser(url)
	if p == nil {
		we.Add(url, fmt.Errorf("unknown url format"))
		return &converter.Response{Error: we}
	}
	snapshots, err := p.ParseUrl(url)
	if err != nil {
		we.Add(url, err)
		return &converter.Response{Error: we}
	}

	if err != nil {
		we.Add(url, err)
		return &converter.Response{Error: we}
	}
	s := &converter.Snapshot{
		Id:       uuid.New().String(),
		FileName: url,
		Snapshot: &pb.ChangeSnapshot{Data: snapshots},
	}
	res := &converter.Response{
		Snapshots: []*converter.Snapshot{s},
		Error:     nil,
	}
	return res
}

func (p *Converter) Name() string {
	return Name
}

func (p *Converter) getParams(params pb.IsRpcObjectImportRequestParams) (string, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfBookmarksParams); ok {
		return p.BookmarksParams.GetUrl(), nil
	}
	return "", fmt.Errorf("PB: GetParams wrong parameters format")
}
