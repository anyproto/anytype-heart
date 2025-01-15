package parsers

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const wikiRegexp = "\\/wiki\\/([\\w%]+)"

type DumbWikiParser struct{}

func New() Parser {
	return new(DumbWikiParser)
}

func (w *DumbWikiParser) ParseUrl(url string) (*common.StateSnapshot, error) {
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

	snapshot := &common.StateSnapshot{}
	snapshot.Blocks = blocks
	var name string
	for _, b := range blocks {
		if text := b.GetText(); text != nil && text.Style == model.BlockContentText_Header1 {
			name = text.Text
		}
	}
	if name == "" {
		name = filepath.Base(url)
	}
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	details.SetString(bundle.RelationKeySource, url)
	details.SetString(bundle.RelationKeyType, bundle.TypeKeyBookmark.String())
	snapshot.Details = details
	return snapshot, nil
}

func (w *DumbWikiParser) MatchUrl(url string) bool {
	match, _ := regexp.MatchString(wikiRegexp, url)
	return match
}
