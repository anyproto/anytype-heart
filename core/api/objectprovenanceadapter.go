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
//     object). NOT every derived tree fails here: BuildDerivedRoot emits
//     unsigned roots, but derivePersonalPayload (objectcache/payload.go)
//     signs derived roots with the ACCOUNT identity — taken whenever
//     personalSpaceId == space.Id() or UseAccountSignature, and
//     objectcreator sets UseAccountSignature for every FileObject — so
//     accountMatch=true means "this account signed the root", never "this
//     is created-tree user content". The DELETE route's sbType allowlist
//     owns that exclusion;
//  3. the key clause: the FIRST non-root change, in tree order, must carry
//     the same identity, and its pb.Change.IntegrationName is the recorded
//     provenance (the raw app name, served verbatim — the caller compares);
//  4. the co-author scan: any change signed by another identity means
//     another member edited the object.
//
// Cost: one full-history read of one tree from local storage — the same
// class as opening that object's version history, on a human-scale,
// write-rate-limited DELETE endpoint. Only the first content change is
// decrypted; the co-author scan reads signed headers alone.
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

func (a *objectProvenanceAdapter) CreatorProvenance(ctx context.Context, spaceId string, objectId string) (apicore.Provenance, error) {
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return apicore.Provenance{}, fmt.Errorf("get space %s: %w", spaceId, err)
	}
	treeBuilder := spc.TreeBuilder()
	if treeBuilder == nil {
		return apicore.Provenance{}, fmt.Errorf("space %s has no tree builder", spaceId)
	}
	// empty opts = the full history: root and first change guaranteed present
	ht, err := treeBuilder.BuildHistoryTree(ctx, objectId, objecttreebuilder.HistoryTreeOpts{})
	if err != nil {
		return apicore.Provenance{}, fmt.Errorf("build history tree for %s: %w", objectId, err)
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
func creatorProvenanceFromTree(tree provenanceTree, ownAccount string) (apicore.Provenance, error) {
	root := tree.UnmarshalledHeader()
	if root == nil || root.Identity == nil || ownAccount == "" || root.Identity.Account() != ownAccount {
		// other members' objects and UNSIGNED derived roots: the root clause
		// fails, the key clause is not consulted. Account-signed derived
		// shapes (personal-space derives, FileObjects everywhere) pass this
		// clause — see the header note; the caller's allowlist excludes the
		// system ones.
		return apicore.Provenance{}, nil
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
		return apicore.Provenance{}, fmt.Errorf("iterate history tree %s: %w", tree.Id(), iterErr)
	}
	// the co-author scan walks every change with no convert, so nothing is
	// decrypted: the signing identity is on the validated header. A change
	// with no identity counts as foreign — fail closed, the caller refuses.
	editedByOthers := false
	iterErr = tree.IterateRoot(nil, func(change *objecttree.Change) bool {
		if change.Identity == nil || change.Identity.Account() != ownAccount {
			editedByOthers = true
			return false
		}
		return true
	})
	if iterErr != nil {
		return apicore.Provenance{}, fmt.Errorf("scan history tree %s: %w", tree.Id(), iterErr)
	}
	owned := apicore.Provenance{AccountMatch: true, EditedByOthers: editedByOthers}
	if first == nil {
		// no content change yet (the §10 creation-race window): the account
		// owns the tree but nothing records a key — fail-closed as "no stamp"
		return owned, nil
	}
	if first.Identity == nil || first.Identity.Account() != ownAccount {
		// first content change signed by someone else (same-account other
		// device cannot happen — but fail closed rather than reason about it)
		return owned, nil
	}
	model, ok := first.Model.(*pb.Change)
	if !ok || model == nil {
		return owned, nil
	}
	owned.IntegrationName = model.IntegrationName
	return owned, nil
}
