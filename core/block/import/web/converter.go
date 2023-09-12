package web

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/web/parsers"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

const Name = "web"

type Converter struct {
	otc converter.ObjectTreeCreator
}

func NewConverter() converter.Converter {
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

func (c *Converter) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, _ int64) (*converter.Response, *converter.ConvertError) {
	we := converter.NewError()
	url, err := c.getParams(req.Params)
	progress.SetTotal(1)
	if err != nil {
		we.Add(err)
		return nil, we
	}
	p := c.GetParser(url)
	if p == nil {
		we.Add(fmt.Errorf("unknown url format"))
		return nil, we
	}
	snapshots, err := p.ParseUrl(url)
	progress.AddDone(1)

	if err != nil {
		we.Add(err)
		return nil, we
	}

	s := &converter.Snapshot{
		Id:       uuid.New().String(),
		FileName: url,
		Snapshot: &pb.ChangeSnapshot{Data: snapshots},
	}
	res := &converter.Response{
		Snapshots: []*converter.Snapshot{s},
	}
	return res, nil
}

func (p *Converter) Name() string {
	return Name
}

func (p *Converter) getParams(params pb.IsRpcObjectImportRequestParams) (string, error) {
	if p, ok := params.(*pb.RpcObjectImportRequestParamsOfBookmarksParams); ok {
		return p.BookmarksParams.GetUrl(), nil
	}
	return "", fmt.Errorf("PB: getParams wrong parameters format")
}
