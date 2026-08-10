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
// nothing: they reject the field outright (rejectPayloadIds below), and
// their op schema does not publish it (§8.30). Two meanings ("name this
// existing block" / "choose an id for this new one") shared one slot, and
// that sharing is what produced F1; the split is C2 applied to a field.

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
// id slot; a cell's flat DESCENDANTS do, and resolveCellIds walks those.
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

// resolveIdField resolves the "id" of one payload object in place. An
// omitted id is left alone — that is how new content is authored; the
// importer mints one and the response reports it in createdBlocks.
func (a *v2StateApplier) resolveIdField(vocab []string, m map[string]any, path string) error {
	id, _ := m["id"].(string)
	if id == "" {
		return nil
	}
	full, err := a.resolvePayloadId(vocab, id, path)
	if err != nil {
		return err
	}
	m["id"] = full
	return nil
}

// resolveIdEntries resolves the id of every entry of a rows/columns/views
// array, and (for rows) the ids inside their cells.
func (a *v2StateApplier) resolveIdEntries(vocab []string, entries []any, field, path string) error {
	for j, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue // shape errors belong to the format validation, not here
		}
		entryPath := fmt.Sprintf("%s.%s[%d]", path, field, j)
		if err := a.resolveIdField(vocab, m, entryPath+".id"); err != nil {
			return err
		}
		if field == "rows" {
			if err := a.resolveCellIds(vocab, m, entryPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveCellIds resolves the ids a row's cells carry. Only the F10 array
// form has any: its first element is the cell block (derived id, forced by
// the importer — skipped) and the rest are ordinary flat blocks with ids.
// The string, null and bare-object forms carry no id slot at all.
func (a *v2StateApplier) resolveCellIds(vocab []string, row map[string]any, path string) error {
	cells, _ := row["cells"].([]any)
	for k, cell := range cells {
		run, ok := cell.([]any)
		if !ok {
			continue
		}
		for i := 1; i < len(run); i++ {
			el, ok := run[i].(map[string]any)
			if !ok {
				continue
			}
			if err := a.resolveIdField(vocab, el, fmt.Sprintf("%s.cells[%d][%d].id", path, k, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolvePayloadBlock resolves every id slot one payload block object
// carries: its own id and the ids nested in its columns, rows (incl. cell
// descendants) and views.
func (a *v2StateApplier) resolvePayloadBlock(vocab []string, block map[string]any, path string) error {
	if err := a.resolveIdField(vocab, block, path+".id"); err != nil {
		return err
	}
	for _, field := range v2IdBearingBlockFields {
		entries, ok := block[field].([]any)
		if !ok {
			continue
		}
		if err := a.resolveIdEntries(vocab, entries, field, path); err != nil {
			return err
		}
	}
	return nil
}

//
// ---- new-content payloads: no id slot at all ----
//

// rejectPayloadIds refuses every id one NEW-CONTENT payload block carries —
// its own and the ones nested in columns, rows (incl. cell descendants) and
// views. It walks the same slots resolvePayloadBlock does, so the two cannot
// drift apart.
//
// The op schema does not publish an id for such a payload (§8.30), which is
// what stops a constrained decoder from emitting one; this is the check for
// the caller who is not decoding against the schema. It runs INSTEAD of
// resolution so the verdict reads as "not part of this op" rather than as a
// duplicate or an unresolvable id — both of which were true before and
// neither of which told the caller the field itself is wrong.
func rejectPayloadIds(op string, block map[string]any, path string) error {
	if err := rejectIdField(op, block, path+".id"); err != nil {
		return err
	}
	for _, field := range v2IdBearingBlockFields {
		entries, ok := block[field].([]any)
		if !ok {
			continue
		}
		for j, e := range entries {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			entryPath := fmt.Sprintf("%s.%s[%d]", path, field, j)
			if err := rejectIdField(op, m, entryPath+".id"); err != nil {
				return err
			}
			if field != "rows" {
				continue
			}
			cells, _ := m["cells"].([]any)
			for k, cell := range cells {
				run, ok := cell.([]any)
				if !ok {
					continue
				}
				for i := 1; i < len(run); i++ {
					el, ok := run[i].(map[string]any)
					if !ok {
						continue
					}
					if err := rejectIdField(op, el, fmt.Sprintf("%s.cells[%d][%d].id", entryPath, k, i)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// rejectIdField refuses one id slot of a new-content payload.
func rejectIdField(op string, m map[string]any, path string) error {
	id, _ := m["id"].(string)
	if id == "" {
		return nil
	}
	return newContentIdError(op, id, path)
}

// newContentIdError is the refusal for an id in a payload that only ever
// creates. It names the field as not belonging to the op, because that is
// the repair — there is no value that would have worked.
func newContentIdError(op, id, path string) error {
	return v2model.ValidationFailed(
		fmt.Sprintf("%s takes no id — its payload authors NEW content", op),
		v2model.Issue{
			Path:    path,
			Message: fmt.Sprintf("id %q is not part of this op: an id names an EXISTING element, and %s only creates them; no value of this field can succeed, so the op's schema does not have it", id, op),
			Hint:    fmt.Sprintf("drop the id — the server mints one and returns it in createdBlocks (GET /v2/schemas/ops/%s); to change an existing block use updateBlock, or replaceSubtree to swap it whole", op),
		})
}

// resolveCellValueIds resolves the ids a setCell value carries. Only the
// array form has any, and only past element 0 (the cell block, derived id).
func (a *v2StateApplier) resolveCellValueIds(vocab []string, value any, path string) error {
	run, ok := value.([]any)
	if !ok {
		return nil
	}
	for i := 1; i < len(run); i++ {
		el, ok := run[i].(map[string]any)
		if !ok {
			continue
		}
		if err := a.resolveIdField(vocab, el, fmt.Sprintf("%s[%d].id", path, i)); err != nil {
			return err
		}
	}
	return nil
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
			Hint:    "omit id to author something new — the server mints one and returns it in createdBlocks; if you meant an existing element, re-read the object (GET ?outline=true lists block ids) — it may have changed under you",
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
