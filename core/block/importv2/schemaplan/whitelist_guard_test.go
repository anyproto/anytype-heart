package schemaplan_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// An unassigned container falls back to a bundled typesuggest verdict. If a
// minted kind's key collided with that bundled key, sanitize's rename table
// would retype the unassigned container onto the minted type — joining a type
// the coverage gate never approved it for.
func TestUnassignedContainerIsNotCapturedByAMintedKind(t *testing.T) {
	// given — c1 is a sprint board the model called "Task"; c2 is a grocery
	// list the model never assigned, which naive-types as bundled task.
	schemas := []schemaplan.ContainerSchema{
		{Id: "c1", Name: "Sprint Board", Properties: []schemaplan.PropertySchema{
			{Id: "p1", Name: "Summary", Format: model.RelationFormat_longtext},
			{Id: "p2", Name: "Points", Format: model.RelationFormat_number},
		}},
		{Id: "c2", Name: "Groceries", Properties: []schemaplan.PropertySchema{
			{Id: "q1", Name: "Done", Format: model.RelationFormat_checkbox},
		}},
	}
	kinds := []schemaplan.KindPlan{{Name: "Task", ContainerIds: []string{"c1"}}}

	// when
	plan := schemaplan.CompleteKinds(kinds, schemas)
	clean := schemaplan.Sanitize(plan, schemas, nil)

	// then
	require.Contains(t, clean.Containers, "c1")
	require.Contains(t, clean.Containers, "c2")
	assert.NotEqual(t, clean.Containers["c1"].TypeKey, clean.Containers["c2"].TypeKey,
		"unassigned container joined the minted kind's type")
	assert.Equal(t, "task", string(clean.Containers["c2"].TypeKey),
		"unassigned container must keep its bundled naive verdict")
}

// When the option-vocabulary guard vetoes a share, each member keeps its own
// relation — but the merged type must not then advertise the same label once
// per member.
func TestVetoedShareIsRecommendedOnce(t *testing.T) {
	// given — three trackers of one kind whose Status vocabularies are disjoint
	schemas := []schemaplan.ContainerSchema{
		{Id: "c1", Name: "Sprint", Properties: []schemaplan.PropertySchema{
			{Id: "p1", Name: "Status", Format: model.RelationFormat_status,
				Options: []string{"Todo", "Doing"}},
			{Id: "p2", Name: "Owner", Format: model.RelationFormat_longtext},
		}},
		{Id: "c2", Name: "Support", Properties: []schemaplan.PropertySchema{
			{Id: "q1", Name: "Status", Format: model.RelationFormat_status,
				Options: []string{"New", "Escalated"}},
			{Id: "q2", Name: "Owner", Format: model.RelationFormat_longtext},
		}},
		{Id: "c3", Name: "Bugs", Properties: []schemaplan.PropertySchema{
			{Id: "r1", Name: "Status", Format: model.RelationFormat_status,
				Options: []string{"Open", "Fixed"}},
			{Id: "r2", Name: "Owner", Format: model.RelationFormat_longtext},
		}},
	}
	kinds := []schemaplan.KindPlan{{Name: "Ticket", ContainerIds: []string{"c1", "c2", "c3"}}}

	// when
	plan := schemaplan.CompleteKinds(kinds, schemas)
	clean := schemaplan.Sanitize(plan, schemas, nil)

	// then — the kind survived the coverage gate (every member covers 2/2)
	require.Len(t, clean.NewTypes, 1)
	byName := map[string]int{}
	for _, property := range clean.NewTypes[0].Properties {
		byName[property.Name]++
	}
	assert.Equal(t, 1, byName["Status"], "Status recommended once, not once per member")

	// and the members' Status relations still differ, so option pools stay apart
	first := clean.Containers["c1"].Properties["p1"].Key
	second := clean.Containers["c2"].Properties["q1"].Key
	assert.NotEqual(t, first, second, "disjoint vocabularies must not share a relation")
}
