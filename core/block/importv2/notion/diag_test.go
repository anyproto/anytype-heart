package notion

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestCassetteDiagnostic runs the full converter over the recorded cassette
// and prints a breakdown of what real workspace data converts into: object
// counts by type, issue counts by code, and block-kind coverage. Not an
// assertion test — a lens on fidelity. Run with -v.
func TestCassetteDiagnostic(t *testing.T) {
	if _, err := cassette.Load(workspaceCassette); err != nil {
		t.Skip("no cassette")
	}
	rec, err := recorder.New(workspaceCassette,
		recorder.WithMode(recorder.ModeReplayOnly),
		recorder.WithSkipRequestLatency(true),
		recorder.WithMatcher(cassette.NewDefaultMatcher(cassette.WithIgnoreAuthorization())),
	)
	require.NoError(t, err)
	defer rec.Stop()

	apiClient := client.NewClient("cassette",
		client.WithTransport(&http.Client{Transport: rec, Timeout: time.Minute}),
		client.WithRateLimit(100000),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	_, err = converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	sbTypes := map[coresb.SmartBlockType]int{}
	fileObjs := 0
	for _, o := range sink.objects {
		sbTypes[o.SbType]++
		if o.File != nil {
			fileObjs++
		}
	}
	t.Logf("objects emitted: %d (file objects: %d)", len(sink.objects), fileObjs)
	for sb, n := range sbTypes {
		t.Logf("  sbType %-24s %d", sb.String(), n)
	}

	issueCodes := map[importv2.IssueCode]int{}
	sample := map[importv2.IssueCode]string{}
	maxSev := importv2.SeverityWarning
	for _, i := range sink.issues {
		issueCodes[i.Code]++
		if _, ok := sample[i.Code]; !ok {
			sample[i.Code] = i.Error()
		}
		if i.Severity > maxSev {
			maxSev = i.Severity
		}
	}
	// Break unsupportedBlock and dataLoss down by the type named in the message.
	typeRe := regexp.MustCompile(`(?:block type|type) "([^"]+)"`)
	unsupByType := map[string]int{}
	lossByType := map[string]int{}
	for _, i := range sink.issues {
		m := typeRe.FindStringSubmatch(i.Message)
		if m == nil {
			continue
		}
		switch i.Code {
		case importv2.IssueUnsupportedBlock:
			unsupByType[m[1]]++
		case importv2.IssueDataLoss:
			lossByType[m[1]]++
		}
	}
	t.Logf("unsupportedBlock by type: %v", unsupByType)
	t.Logf("dataLoss (unsupported prop) by type: %v", lossByType)

	t.Logf("issues: %d total, max severity %s", len(sink.issues), maxSev)
	codes := make([]string, 0)
	for c := range issueCodes {
		codes = append(codes, string(c))
	}
	sort.Strings(codes)
	for _, c := range codes {
		t.Logf("  [%s] x%d  e.g. %s", c, issueCodes[importv2.IssueCode(c)], sample[importv2.IssueCode(c)])
	}

	// Block-kind coverage across every emitted page.
	kinds := map[string]int{}
	for _, o := range sink.objects {
		if o.SbType != coresb.SmartBlockTypePage || o.Payload == nil {
			continue
		}
		for _, b := range o.Payload.Blocks {
			kinds[blockKind(b)]++
		}
	}
	kks := make([]string, 0)
	for k := range kinds {
		kks = append(kks, k)
	}
	sort.Strings(kks)
	t.Logf("block kinds across all pages:")
	for _, k := range kks {
		t.Logf("  %-16s %d", k, kinds[k])
	}

	// Icon coverage: how many objects got an emoji vs image vs nothing.
	var emoji, image, none int
	for _, o := range sink.objects {
		if o.Payload == nil {
			continue
		}
		switch {
		case o.Payload.Details.GetString(bundle.RelationKeyIconEmoji) != "":
			emoji++
		case o.Payload.Details.GetString(bundle.RelationKeyIconImage) != "":
			image++
		default:
			none++
		}
	}
	t.Logf("icons: emoji=%d image=%d none=%d", emoji, image, none)
}

func blockKind(b *model.Block) string {
	switch b.Content.(type) {
	case *model.BlockContentOfText:
		return "text:" + b.GetText().Style.String()
	case *model.BlockContentOfLink:
		return "link"
	case *model.BlockContentOfFile:
		return "file"
	case *model.BlockContentOfBookmark:
		return "bookmark"
	case *model.BlockContentOfLatex:
		return "latex"
	case *model.BlockContentOfDiv:
		return "div"
	case *model.BlockContentOfTable:
		return "table"
	case *model.BlockContentOfTableRow:
		return "tableRow"
	case *model.BlockContentOfTableColumn:
		return "tableColumn"
	case *model.BlockContentOfLayout:
		return "layout"
	case *model.BlockContentOfTableOfContents:
		return "tableOfContents"
	case *model.BlockContentOfSmartblock:
		return "smartblock(root)"
	default:
		return "other"
	}
}
