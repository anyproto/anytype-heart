package parsers

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const wikiRegexp = "\\/wiki\\/([\\w%]+)"

type DumbWikiParser struct{}

func New() Parser {
	return new(DumbWikiParser)
}

func (w *DumbWikiParser) ParseUrl(url string) (*model.SmartBlockSnapshotBase, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("WikiParser: ParseUrl: %w", err)
	}
	defer resp.Body.Close()

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("WikiParser: ParseUrl: %w", err)
	}
	blocks, _, err := anymark.HTMLToBlocks(bytes, "")
	if err != nil {
		return nil, fmt.Errorf("WikiParser: ParseUrl: %w", err)
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
			bundle.RelationKeyType.String():   pbtypes.String(bundle.TypeKeyBookmark.String()),
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
