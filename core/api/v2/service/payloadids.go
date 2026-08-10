package v2service

// payloadids.go resolves the id slots a PATCH *payload* carries — the
// counterpart of resolveRef, which resolves the id slots a PATCH *reference*
// carries. Both use the same rule (matchBlockRef: exact id first, then a
// unique suffix, §9a/C4), so there is ONE id vocabulary on the whole edit
// surface and no channel takes an id literally.
//
// Why it exists. The default read is the EDIT shape: machine-minted ids are
// relabeled to their 5-char tails, and every write channel is supposed to
// resolve a label back to the id it labels. Reference slots did; payload
// slots did not — they were handed to the format importer verbatim, which
// takes an id as an identity. So the documented loop
//
//	GET ?block=aaaa1  →  PATCH replaceSubtree {id:"aaaa1", blocks:<that array>}
//
// answered 200 and PERMANENTLY renamed the block to "aaaa1": other clients'
// cached ids 404ed, the id was no longer minted-shaped so it never relabeled
// again (and reserved that label in the exporter's avoid-set forever), and
// the CRDT recorded a delete plus a create instead of an edit. The same held
// for updateBlock set:{rows|columns|views} and setCell value — the ops whose
// payload REPLACES the label's own owner, which is exactly the set the
// leaving-subtree subtraction in the old checkFreshIds excluded from its
// tail scan.
//
// Resolving instead of refusing makes that loop CORRECT rather than merely
// rejected: a payload id names the block it was read from, identity is
// preserved, and echoing a subtree back unchanged is a genuine no-op —
// diffStats 0, not "2 added, 2 removed".
//
// An id that resolves to nothing is REFUSED, not minted (§8.29): a payload
// id names an existing block, exactly as every other id slot does, and
// omitting the id stays the one way to author new content. Minting over an
// unresolvable id would silently turn a stale or mistyped reference into a
// new block — the same class of silent-wrong-thing this file exists to
// close — while the refusal costs a caller who meant new content one edit.
//
// Which leaves a payload with NO existing content to name — insertBlocks —
// with an id slot in which every value is an error. Those ops resolve
// nothing: they reject the field outright (rejectOrMintSlot below), and
// their op schema does not publish it (§8.30). Two meanings ("name this
// existing block" / "choose an id for this new one") shared one slot, and
// that sharing is what produced F1; the split is C2 applied to a field.
//
// ONE WALK, THREE PASSES (§8.31). The id rule has two halves — what an id
// may NAME (resolution, here) and what an id may CLAIM (the collision guard,
// claimPayloadIds in stateops.go) — and they have to agree about which slots
// exist and about which of them already do. They did not: resolution walked
// blocks, table internals AND dataview views, while the guard walked the
// imported []*model.Block, in which a view is not an element at all. So a
// view id was resolvable but unclaimable, and two documented bugs followed
// from that one disagreement. Every slot is now visited by ONE walker
// (walkPayloadIdSlots) whose visitors are the three things a slot can need —
// resolve, reject, mint — and "already exists" is one union on both sides
// (payloadIdExists).

import (
	"fmt"
	"net/http"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// v2IdBearingBlockFields are the payload block fields whose entries carry a
// doc-local id of their own: table columns and rows (SPEC §6.1) and dataview
// views (§6.2). Cells are absent on purpose — a cell block's id is DERIVED
// (rowId-colId) and the importer forces it, so the cell object itself has no
// id slot; a cell's flat DESCENDANTS do, and the walker covers those.
var v2IdBearingBlockFields = []string{"columns", "rows", "views"}

// idBearingBlockField reports whether a block field's entries carry ids.
func idBearingBlockField(field string) bool {
	for _, f := range v2IdBearingBlockFields {
		if f == field {
			return true
		}
	}
	return false
}

// maxListedIdCandidates bounds how many candidates an ambiguity refusal
// enumerates.
const maxListedIdCandidates = 8

// payloadIdVocabulary is the pre-op id domain a payload id may name: every
// doc-local id the object's CURRENT document exposes in an id slot — block
// ids, table column and row ids, cell-descendant ids, dataview view ids.
// That is exactly the domain compact relabeling covers (§9a), so every id a
// read can serve resolves back through it.
//
// "Pre-op" includes the subtree the op is about to replace — the whole point:
// replaceSubtree's payload legitimately names the blocks it is replacing, and
// subtracting them (as the old freshness guard did) is what left those ops
// with no id rule at all.
func (a *v2StateApplier) payloadIdVocabulary() ([]string, error) {
	doc, err := a.doc()
	if err != nil {
		return nil, err
	}
	return doc.localIds(), nil
}

// payloadIdExists reports what "already exists in this object" means — the
// question the collision guard asks, which MUST have the same answer as the
// question the resolver asks (§8.31). It is the union of two views of one
// object, and the union is the fix:
//
//   - a.st.Exists sees every state BLOCK, including the ones the served
//     document does not carry at all (the root, title, header,
//     featuredRelations) — so a payload cannot adopt one of those either.
//   - the pre-op document's localIds() is the payload id VOCABULARY, and it
//     additionally holds the doc-local ids that are NOT blocks: a dataview's
//     VIEW ids.
//
// The guard used to ask st.Exists alone. A view id was therefore resolvable
// but never claimable: `updateBlock set:{views:[…]}` could store two views
// under one id (every later view op then addressed the first one forever),
// and a payload block could adopt a live view's id — an identity collision
// no read can distinguish.
func (a *v2StateApplier) payloadIdExists() (func(string) bool, error) {
	vocab, err := a.payloadIdVocabulary()
	if err != nil {
		return nil, err
	}
	local := make(map[string]bool, len(vocab))
	for _, id := range vocab {
		local[id] = true
	}
	return func(id string) bool { return local[id] || a.st.Exists(id) }, nil
}

// resolvePayloadId maps one payload id onto the stored id it names.
func (a *v2StateApplier) resolvePayloadId(vocab []string, id, path string) (string, error) {
	idx, matches := matchBlockRef(vocab, id)
	switch {
	case matches == 1:
		if vocab[idx] != id {
			// remember the spelling the caller wrote, so a later collision
			// refusal can name both (the pasted-compact-label diagnosis)
			a.payloadIdOrigin[vocab[idx]] = id
		}
		return vocab[idx], nil
	case matches > 1:
		return "", ambiguousPayloadIdError(path, id, suffixCandidates(vocab, id))
	default:
		return "", unresolvedPayloadIdError(path, id)
	}
}

//
// ---- the one walk over a payload's id slots ----
//

// payloadIdSlot is one id-bearing object of a payload, with the path that
// ADDRESSES it and whether it is a dataview view. path is the ELEMENT path
// ("ops[0].blocks[0].rows[1]"): error paths append ".id", and a minted id is
// reported under the path itself — which is what makes the slot the caller
// left empty the key it gets its answer back on.
type payloadIdSlot struct {
	m    map[string]any
	path string
	view bool
}

// slotVisitor handles one id slot. The three implementations below are the
// three things a slot can need, and sharing the walk is what keeps them from
// covering different sets of slots.
type slotVisitor func(payloadIdSlot) error

// walkPayloadIdSlots visits every id slot one payload block carries: the
// block itself, each entry of its columns/rows/views, and the flat
// DESCENDANTS inside its rows' cells.
func walkPayloadIdSlots(block map[string]any, path string, fn slotVisitor) error {
	if err := fn(payloadIdSlot{m: block, path: path}); err != nil {
		return err
	}
	for _, field := range v2IdBearingBlockFields {
		entries, ok := block[field].([]any)
		if !ok {
			continue
		}
		if err := walkEntryIdSlots(entries, field, path, fn); err != nil {
			return err
		}
	}
	return nil
}

// walkEntryIdSlots visits the id slots of ONE rows/columns/views array.
// updateBlock resolves the caller's arrays field by field — the live fields
// merged in already carry stored ids — so this half of the walk is reachable
// on its own.
func walkEntryIdSlots(entries []any, field, path string, fn slotVisitor) error {
	for j, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue // shape errors belong to the format validation, not here
		}
		entryPath := fmt.Sprintf("%s.%s[%d]", path, field, j)
		if err := fn(payloadIdSlot{m: m, path: entryPath, view: field == "views"}); err != nil {
			return err
		}
		if field != "rows" {
			continue
		}
		cells, _ := m["cells"].([]any)
		for k, cell := range cells {
			run, ok := cell.([]any)
			if !ok {
				continue // string, null and bare-object cells carry no id slot
			}
			if err := walkCellRunIdSlots(run, fmt.Sprintf("%s.cells[%d]", entryPath, k), fn); err != nil {
				return err
			}
		}
	}
	return nil
}

// walkCellRunIdSlots visits the id slots of a cell's array form (§6.1 F10).
// Element 0 is the cell block itself, whose id is DERIVED (rowId-colId) and
// forced by the importer, so the walk starts at 1; the rest are ordinary
// flat blocks. setCell's value channel is the same shape, so it shares this.
func walkCellRunIdSlots(run []any, path string, fn slotVisitor) error {
	for i := 1; i < len(run); i++ {
		el, ok := run[i].(map[string]any)
		if !ok {
			continue
		}
		if err := fn(payloadIdSlot{m: el, path: fmt.Sprintf("%s[%d]", path, i)}); err != nil {
			return err
		}
	}
	return nil
}

//
// ---- the three visitors ----
//

// resolveOrMintSlot is the EXISTING-content visitor: an id that is there
// names an existing element and resolves to it; an id that is not is MINTED
// here rather than by the format importer, so the response can report it.
func (a *v2StateApplier) resolveOrMintSlot(vocab []string) slotVisitor {
	return func(s payloadIdSlot) error {
		id, _ := s.m["id"].(string)
		if id == "" {
			a.mintSlotId(s)
			return nil
		}
		full, err := a.resolvePayloadId(vocab, id, s.path+".id")
		if err != nil {
			return err
		}
		s.m["id"] = full
		return nil
	}
}

// rejectOrMintSlot is the NEW-content visitor (§8.30): every id is refused
// as not part of the op — there is no value of the field that could succeed
// — and every empty slot is minted and reported.
//
// The refusal runs INSTEAD of resolution so the verdict reads as "not part
// of this op" rather than as a duplicate or an unresolvable id — both of
// which were true before and neither of which told the caller the field
// itself is wrong. The op schema does not publish an id for such a payload,
// which is what stops a constrained decoder from emitting one; this is the
// check for the caller who is not decoding against the schema.
func (a *v2StateApplier) rejectOrMintSlot(op string) slotVisitor {
	return func(s payloadIdSlot) error {
		if id, _ := s.m["id"].(string); id != "" {
			return newContentIdError(op, id, s.path+".id")
		}
		a.mintSlotId(s)
		return nil
	}
}

// mintSlotId fills an empty id slot and REPORTS what it minted under that
// slot's path.
//
// Minting here rather than leaving it to the format importer is what makes
// the refusals' promise true. Both of them tell the caller to omit the id
// because "the server mints one and returns it in createdBlocks" — but
// createdBlocks used to be written only for TOP-LEVEL run blocks, so a
// minted view id, a minted cell descendant and the row/column ids of a table
// created through insertBlocks were all unreported. Those are precisely the
// slots the refusals fire on, so the API was telling a model to do something
// and then withholding the answer it had promised; the model's only recovery
// was a re-read, a whole round trip to learn an id it had just created.
//
// A view id goes to createdViews, not createdBlocks: a view is not a block,
// and createdViews is already the view-family twin of that map.
func (a *v2StateApplier) mintSlotId(s payloadIdSlot) {
	id := a.mintBlockId()
	s.m["id"] = id
	if s.view {
		a.createdViews[s.path] = id
		return
	}
	a.createdBlocks[s.path] = id
}

// resolveIdEntries resolves (and mints into) the id slots of one payload
// rows/columns/views array — updateBlock's set channel, which hands over one
// field at a time.
func (a *v2StateApplier) resolveIdEntries(vocab []string, entries []any, field, path string) error {
	return walkEntryIdSlots(entries, field, path, a.resolveOrMintSlot(vocab))
}

// resolveCellValueIds resolves (and mints into) the id slots a setCell value
// carries. Only the array form has any, and only past element 0 (the cell
// block, derived id).
func (a *v2StateApplier) resolveCellValueIds(vocab []string, value any, path string) error {
	run, ok := value.([]any)
	if !ok {
		return nil
	}
	return walkCellRunIdSlots(run, path, a.resolveOrMintSlot(vocab))
}

//
// ---- refusals ----
//

// newContentIdError is the refusal for an id in a payload that only ever
// creates. It names the field as not belonging to the op, because that is
// the repair — there is no value that would have worked.
func newContentIdError(op, id, path string) error {
	return v2model.ValidationFailed(
		fmt.Sprintf("%s takes no id — its payload authors NEW content", op),
		v2model.Issue{
			Path:    path,
			Message: fmt.Sprintf("id %q is not part of this op: an id names an EXISTING element, and %s only creates them; no value of this field can succeed, so the op's schema does not have it", id, op),
			Hint:    fmt.Sprintf("drop the id — the server mints one and reports it under this exact path in createdBlocks (createdViews for a view); see GET /v2/schemas/ops/%s. To change an existing block use updateBlock, or replaceSubtree to swap it whole", op),
		})
}

// suffixCandidates lists the ids ref is a suffix of, bounded.
func suffixCandidates(ids []string, ref string) []string {
	var out []string
	for _, id := range ids {
		if id != ref && strings.HasSuffix(id, ref) {
			out = append(out, id)
			if len(out) == maxListedIdCandidates {
				break
			}
		}
	}
	return out
}

// unresolvedPayloadIdError refuses a payload id that names no block of this
// object. See the file header for why this is a refusal and not a fresh mint.
func unresolvedPayloadIdError(path, id string) error {
	return v2model.ValidationFailed(
		fmt.Sprintf("id %q in the payload matches no block, row, column or view of this object", id),
		v2model.Issue{
			Path:    path,
			Message: "a payload id names an EXISTING element whose identity the op keeps — a full id or a unique suffix, the same rule every other id slot follows; it is not a way to choose the id of new content",
			Hint:    "omit id to author something new — the server mints one and reports it under this exact path in createdBlocks (createdViews for a view); if you meant an existing element, re-read the object (GET ?outline=true lists block ids) — it may have changed under you",
		})
}

// ambiguousPayloadIdError refuses a payload id that is a suffix of several
// stored ids — the reference slots' 400, with the candidates named.
func ambiguousPayloadIdError(path, id string, candidates []string) error {
	msg := fmt.Sprintf("id %q in the payload matches more than one element — use the full id", id)
	return v2model.AmbiguousInput(msg, v2model.Issue{
		Path:    path,
		Message: "the id is a suffix of: " + strings.Join(candidates, ", "),
	})
}

// duplicateIdError refuses a payload id that resolved to an element this op
// may not reuse — a second holder of an id the document already owns. When
// the caller wrote a shorter spelling (a compact label off a default read),
// both spellings are named: "bbbb1" reads as a fresh id but resolves to the
// block it labels, and inserting a COPY under that identity is what the
// refusal is about.
func (a *v2StateApplier) duplicateIdError(path string, id string) error {
	detail := fmt.Sprintf("duplicate id %q — it already exists in the document", id)
	if origin, ok := a.payloadIdOrigin[id]; ok {
		detail = fmt.Sprintf("id %q resolves to %q, which already exists in the document — a compact label off a default read names the block it labels, it does not mint a new one", origin, id)
	}
	return v2model.NewError(http.StatusBadRequest, v2model.CodeValidationFailed, v2InvalidDocMessage,
		v2model.Issue{
			Path:    path,
			Message: detail,
			Hint:    "omit id on new content — the server mints one; to change the existing element, address it with updateBlock or replaceSubtree",
		})
}

// viewIdPath addresses the j-th view of the payload block whose own id slot
// is base: "ops[0].blocks[0].id" → "ops[0].blocks[0].views[1].id", and a base
// that is not itself an id slot ("ops[0].set") simply gains the suffix.
func viewIdPath(base string, j int) string {
	return fmt.Sprintf("%s.views[%d].id", strings.TrimSuffix(base, ".id"), j)
}
