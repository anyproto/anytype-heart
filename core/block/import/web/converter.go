package web

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web/parsers"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

const name = "web"

type Converter struct{}

func NewConverter() converter.Converter {
	return new(Converter)
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
	// TODO: [MR] fix imports
	panic("can't convert")
	//tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
	if err != nil {
		we.Add(url, err)
		return &converter.Response{Error: we}
	}
	s := &converter.Snapshot{
		//Id:       tid.String(),
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
	return name
}

func (p *Converter) getParams(params pb.IsRpcObjectImportRequestParams) (string, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfBookmarksParams); ok {
		return p.BookmarksParams.GetUrl(), nil
	}
	return "", fmt.Errorf("PB: GetParams wrong parameters format")
}
