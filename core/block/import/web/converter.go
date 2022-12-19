package web

import (
	"fmt"

	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web/parsers"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
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

func (c *Converter) GetSnapshots(req *pb.RpcObjectImportRequest,
	progress *process.Progress) (*converter.Response, converter.ConvertError) {
	we := converter.NewError()
	url, err := c.getParams(req.Params)

	progress.SetTotal(1)

	if err != nil {
		we.Add(url, err)
		return nil, we
	}
	p := c.GetParser(url)
	if p == nil {
		we.Add(url, fmt.Errorf("unknown url format"))
		return nil, we
	}

	progress.SetProgressMessage("Start parsing url to snapshot")
	snapshots, err := p.ParseUrl(url)

	progress.AddDone(1)

	if err != nil {
		we.Add(url, err)
		return nil, we
	}

	tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
	if err != nil {
		we.Add(url, err)
		return nil, we
	}
	s := &converter.Snapshot{
		Id:       tid.String(),
		FileName: url,
		Snapshot: snapshots,
	}
	res := &converter.Response{
		Snapshots: []*converter.Snapshot{s},
	}

	return res, nil
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
