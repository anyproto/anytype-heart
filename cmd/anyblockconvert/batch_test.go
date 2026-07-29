package main

// Options sort on orderId+name concatenated (database.OrderMap.BuildOrder),
// so an option with no order id is compared by *name* against everyone
// else's order id: "Abandoned" would sort ahead of the whole declared
// vocabulary because 'A' < the lexid alphabet, while "Zebra" would sort
// behind it. Every option therefore needs an order id.

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// mintedOptions returns option names in the order BuildOrder would compare
// them: the orderId and name concatenated.
func mintedOptions(t *testing.T, b *batch) []string {
	t.Helper()
	type opt struct{ sortKey, name string }
	var opts []opt
	for _, p := range b.pending {
		if p.sbType != model.SmartBlockType_STRelationOption {
			continue
		}
		d := p.snapshot.Details.Fields
		name := d[detailName].GetStringValue()
		order := d[detailOrderId].GetStringValue()
		require.NotEmpty(t, order, "option %q has no order id", name)
		opts = append(opts, opt{order + name, name})
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].sortKey < opts[j].sortKey })
	names := make([]string, 0, len(opts))
	for _, o := range opts {
		names = append(names, o.name)
	}
	return names
}

func TestBatch_DeclaredVocabularyKeepsDeclarationOrder(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {
			Format:  model.RelationFormat_status,
			Options: []string{"Backlog", "In progress", "In review", "Blocked", "Done"},
		},
	})
	assert.Equal(t,
		[]string{"Backlog", "In progress", "In review", "Blocked", "Done"},
		mintedOptions(t, b),
		"declaration order, not alphabetical")
}

// a value no vocabulary declares must land after the declared ones whatever
// its name — this is the case that used to get an empty order id
func TestBatch_UndeclaredValueSortsAfterVocabulary(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {
			Format:  model.RelationFormat_status,
			Options: []string{"Backlog", "In progress", "Done"},
		},
	})
	// "Abandoned" sorts first alphabetically and would jump the queue
	b.OptionId(domain.RelationKey("stage"), "Abandoned")
	b.OptionId(domain.RelationKey("stage"), "Zebra")

	assert.Equal(t,
		[]string{"Backlog", "In progress", "Done", "Abandoned", "Zebra"},
		mintedOptions(t, b))
}

// with nothing declared, discovery order still produces real order ids
func TestBatch_UndeclaredOnlyStillGetsOrderIds(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {Format: model.RelationFormat_status},
	})
	b.OptionId(domain.RelationKey("stage"), "Zebra")
	b.OptionId(domain.RelationKey("stage"), "Abandoned")
	assert.Equal(t, []string{"Zebra", "Abandoned"}, mintedOptions(t, b),
		"first seen first, not alphabetical")
}

// order ids are per property, so two selects do not interleave
func TestBatch_OrderIdsArePerProperty(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage":    {Format: model.RelationFormat_status, Options: []string{"A", "B"}},
		"priority": {Format: model.RelationFormat_status, Options: []string{"Low", "High"}},
	})
	firsts := map[string]string{}
	for _, p := range b.pending {
		if p.sbType != model.SmartBlockType_STRelationOption {
			continue
		}
		d := p.snapshot.Details.Fields
		key := d[detailRelationKey].GetStringValue()
		if _, seen := firsts[key]; !seen {
			firsts[key] = d[detailOrderId].GetStringValue()
		}
	}
	require.Len(t, firsts, 2)
	assert.Equal(t, firsts["stage"], firsts["priority"],
		"each property starts its own sequence at the same midpoint")
}
