package web

import (
	"context"
	"fmt"

	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web/parsers"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
)

const Name = "web"

type Converter struct {
	otc converter.ObjectTreeCreator
}

func init() {
	converter.RegisterFunc(NewConverter)
}

func NewConverter(s core.Service, otc converter.ObjectTreeCreator) converter.Converter {
	return &Converter{
		otc: otc,
	}
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

	ctx := context.Background()
	obj, release, err := c.otc.CreateTreeObject(ctx, smartblock.SmartBlockTypePage, func(id string) *sb.InitContext {
		return &sb.InitContext{
			Ctx: ctx,
		}
	})
	defer release()
	if err != nil {
		we.Add(url, err)
		return &converter.Response{Error: we}
	}
	s := &converter.Snapshot{
		Id:       obj.Id(),
		FileName: url,
		Snapshot: snapshots,
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
