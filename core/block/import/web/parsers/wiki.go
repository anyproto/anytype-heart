package parsers

import (
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/markdown/anymark"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const wikiRegexp = "\\/wiki\\/([\\w%]+)"

type DumbWikiParser struct{}

func New() Parser {
	return new(DumbWikiParser)
}

func (w *DumbWikiParser) ParseUrl(url string) (*model.SmartBlockSnapshotBase, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "WikiParser: ParseUrl: ")
	}
	defer resp.Body.Close()

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "WikiParser: ParseUrl: ")
	}
	blocks, _, err := anymark.HTMLToBlocks(bytes)
	if err != nil {
		return nil, errors.Wrap(err, "WikiParser: ParseUrl: ")
	}

	snapshots := &model.SmartBlockSnapshotBase{}
	snapshots.Blocks = blocks
	var name string
	for _, b := range blocks {
		if text := b.GetText(); text != nil && text.Style == model.BlockContentText_Header1 {
			name = text.Text
		}
	}
	if name == "" {
		name = filepath.Base(url)
	}
	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyName.String():   pbtypes.String(name),
			bundle.RelationKeySource.String(): pbtypes.String(url),
			bundle.RelationKeyType.String():   pbtypes.String(bundle.TypeKeyBookmark.URL()),
			bundle.RelationKeySource.String(): pbtypes.String(url),
		},
	}
	snapshots.Details = details
	return snapshots, nil
}

func (w *DumbWikiParser) MatchUrl(url string) bool {
	match, _ := regexp.MatchString(wikiRegexp, url)
	return match
}
