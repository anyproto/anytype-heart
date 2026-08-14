package api

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

// objectProvenanceAdapter implements apicore.ObjectProvenance — the DELETE
// enforcement read (APIV2_OBJECT_DELETE.md §10). It reads provenance from
// cryptographically validated change storage, never from details:
//
//  1. the tree is built from local storage the way version history is
//     (BuildHistoryTree over the full history) — the in-memory live tree
//     may be snapshot-reduced and MUST NOT be used, because the creating
//     change can predate its base snapshot;
//  2. the root clause: the signed root header's identity must be this
//     account (the §2-grade guarantee that this account created the
//     object; derived trees have no root identity and fail here);
//  3. the key clause: the FIRST non-root change, in tree order, must carry
//     the same identity, and its pb.Change.IntegrationName is the recorded
//     provenance (the raw app name, served verbatim — the caller compares).
//
// Cost: one full-history read of one tree from local storage — the same
// class as opening that object's version history, on a human-scale,
// write-rate-limited DELETE endpoint.
type objectProvenanceAdapter struct {
	spaces  space.Service
	account accountIdProvider
}

// accountIdProvider is the slice of account.Service this adapter needs: the
// account identity string, the same value space as a signed root header's
// Identity.Account() (both derive from the account sign key).
type accountIdProvider interface {
	AccountID() string
}

func newObjectProvenanceAdapter(spaces space.Service, account accountIdProvider) apicore.ObjectProvenance {
	return &objectProvenanceAdapter{spaces: spaces, account: account}
}

func (a *objectProvenanceAdapter) CreatorProvenance(ctx context.Context, spaceId string, objectId string) (accountMatch bool, integrationName string, err error) {
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return false, "", fmt.Errorf("get space %s: %w", spaceId, err)
	}
	treeBuilder := spc.TreeBuilder()
	if treeBuilder == nil {
		return false, "", fmt.Errorf("space %s has no tree builder", spaceId)
	}
	// empty opts = the full history: root and first change guaranteed present
	ht, err := treeBuilder.BuildHistoryTree(ctx, objectId, objecttreebuilder.HistoryTreeOpts{})
	if err != nil {
		return false, "", fmt.Errorf("build history tree for %s: %w", objectId, err)
	}
	return creatorProvenanceFromTree(ht, a.account.AccountID())
}

// provenanceTree is the slice of objecttree.ReadableObjectTree the
// provenance read consumes — narrowed so the clause logic is testable
// against fixture trees (§15).
type provenanceTree interface {
	Id() string
	UnmarshalledHeader() *objecttree.Change
	IterateRoot(convert objecttree.ChangeConvertFunc, iterate objecttree.ChangeIterateFunc) error
}

// creatorProvenanceFromTree evaluates the §10 clauses on a built tree.
// Every ambiguity resolves toward "no provenance" — the caller refuses on
// anything short of a full match, so the safe direction here is empty, not
// guessed.
func creatorProvenanceFromTree(tree provenanceTree, ownAccount string) (accountMatch bool, integrationName string, err error) {
	root := tree.UnmarshalledHeader()
	if root == nil || root.Identity == nil || ownAccount == "" || root.Identity.Account() != ownAccount {
		// derived trees (no signed root identity) and other members' objects:
		// the root clause fails, the key clause is not consulted
		return false, "", nil
	}

	// take the FIRST non-root change in tree order; the same convert version
	// history's state build uses decrypts and unmarshals it
	var first *objecttree.Change
	iterErr := tree.IterateRoot(sourceimpl.NewUnmarshalTreeChange(), func(change *objecttree.Change) bool {
		if change.Id == tree.Id() {
			return true // the root itself — not a content change
		}
		first = change
		return false
	})
	if iterErr != nil {
		return false, "", fmt.Errorf("iterate history tree %s: %w", tree.Id(), iterErr)
	}
	if first == nil {
		// no content change yet (the §10 creation-race window): the account
		// owns the tree but nothing records a key — fail-closed as "no stamp"
		return true, "", nil
	}
	if first.Identity == nil || first.Identity.Account() != ownAccount {
		// first content change signed by someone else (same-account other
		// device cannot happen — but fail closed rather than reason about it)
		return true, "", nil
	}
	model, ok := first.Model.(*pb.Change)
	if !ok || model == nil {
		return true, "", nil
	}
	return true, model.IntegrationName, nil
}
