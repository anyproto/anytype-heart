package v2service

// settype.go is the set_type op: the one PATCH op that changes the document
// envelope rather than a property or a block. Heart's type change carries
// guardrails an agent cannot see from the outside — the TypeChange
// restriction, the template refusal, and a layout conversion table that
// allows only page-family moves — so the op reads the same table before the
// editor runs and turns each refusal into a path-addressed error with the
// repair in it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// v2MaxCompatibleTypesInHint bounds the alternatives a layout refusal lists;
// the hint also names the listing that has them all.
const v2MaxCompatibleTypesInHint = 12

// applySetType resolves the type the way create's `type` resolves, refuses
// what heart would refuse — with the reason and the alternatives — and then
// hands the change to the editor hook, or records the type keys alone when
// the run is a simulation.
func (a *v2StateApplier) applySetType(op opSetType, opPath string) error {
	path := opPath + ".type"
	if op.Type == "" {
		return v2model.ValidationFailed("set_type needs a type",
			v2model.Issue{Path: path, Message: "type is empty — name the type the object becomes, by key, api key or name"}.
				Hintf("list the space's types with %s", v2model.RefListTypes(a.spaceId)))
	}
	entries, err := a.types()
	if err != nil {
		return err
	}
	entry, ok, ambiguous, err := a.s.resolveTypeInput(a.spaceId, op.Type, entries)
	if err != nil {
		return err
	}
	if len(ambiguous) > 0 {
		return ambiguousKeyError(a.v.typeWord(), op.Type, path, ambiguous)
	}
	if !ok {
		return a.s.unknownTypeKeyError(a.spaceId, op.Type, path, a.v)
	}
	key := domain.TypeKey(entry.Key)
	if key == bundle.TypeKeyTemplate {
		return v2model.ValidationFailed("an object cannot become a template",
			v2model.Issue{Path: path, Message: "template is not a type an existing object can take"}.
				Hintf("create a template of a type with %s", v2model.NewRef(v2model.OpCreateTemplate, "space_id", a.spaceId)))
	}
	current := a.st.ObjectTypeKeys()
	if len(current) > 0 && current[0] == key {
		return nil // already this type: nothing to convert, nothing to say
	}
	fromLayout, err := a.currentLayout(current, entries)
	if err != nil {
		return err
	}
	toLayout := typeLayout(entry)
	if !converter.IsConversionAllowed(fromLayout, toLayout) {
		return a.layoutConversionRefusal(op.Type, current, fromLayout, toLayout, entries, path)
	}
	from := a.spellType(current)
	switch {
	case a.simulate:
		// no editor behind this state by design (a dry run, or the
		// create-missing probe): the conversion the new layout needs cannot
		// run, so the validation above is the whole answer, and saying so
		// keeps the receipt honest about what a commit would still do
		a.st.SetObjectTypeKeys([]domain.TypeKey{key})
		a.warnings = append(a.warnings, v2model.Issue{Path: opPath,
			Message: fmt.Sprintf("dry run: the change to %q is valid, but the layout conversion from %s to %s is not simulated — the committed edit rewrites the document for the new layout",
				op.Type, layoutName(fromLayout), layoutName(toLayout))})
	case a.setObjectType == nil:
		// a committing run whose editor cannot change types: refuse rather
		// than write the keys bare and leave a document the app renders in
		// a layout it was never converted to
		return fmt.Errorf("object %s: the editor behind this object does not support a type change", a.objectId)
	default:
		if err := a.setObjectType(a.st, key); err != nil {
			return err
		}
	}
	// the editor only resolves the layout at Apply; within the batch the
	// state has to say what the conversion just made it, or a second
	// set_type — and the editor's own conversion under it — would start
	// from the pre-batch layout
	a.st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(toLayout)))
	a.mutated() // the conversion may have added or moved blocks
	if a.typeChanged == nil {
		a.typeChanged = &v2model.TypeChange{From: from}
	}
	a.typeChanged.To = a.spellType([]domain.TypeKey{key})
	return nil
}

// spellType is the receipt's spelling of a type: exactly what the caller's
// own reads serve in the document envelope, from the same two vocabularies
// a read uses — the api-key vocabulary by default, the resolver's raw-name
// one (shipped names for bundled types, granted labels with collision
// suffixes for custom ones) under ?keys=name.
func (a *v2StateApplier) spellType(keys []domain.TypeKey) string {
	if len(keys) == 0 {
		return ""
	}
	key := string(keys[0])
	a.marshalOptions() // primes the resolver and the key vocabulary once
	if a.v.names {
		return a.marshalResolver.TypeSlug(key)
	}
	if a.marshalKeys != nil {
		return a.marshalKeys.TypeSlug(key)
	}
	return bundle.TypeApiSlug(key)
}

// types is the memoized live type set of this PATCH.
func (a *v2StateApplier) types() ([]typeEntry, error) {
	if !a.typeEntriesLoaded {
		entries, err := a.s.liveTypes(a.spaceId)
		if err != nil {
			return nil, fmt.Errorf("load types of space %s: %w", a.spaceId, err)
		}
		a.typeEntries, a.typeEntriesLoaded = entries, true
	}
	return a.typeEntries, nil
}

// currentLayout is the object's layout as the editor sees it: the resolved
// layout when the state carries one, else the layout of its current type.
func (a *v2StateApplier) currentLayout(current []domain.TypeKey, entries []typeEntry) (model.ObjectTypeLayout, error) {
	if layout, ok := a.st.Layout(); ok {
		return layout, nil
	}
	if len(current) == 0 {
		return 0, fmt.Errorf("object %s has no type to convert from", a.objectId)
	}
	for _, entry := range entries {
		if entry.Key == string(current[0]) {
			return entry.Layout, nil
		}
	}
	if t, err := bundle.GetType(current[0]); err == nil {
		return t.Layout, nil
	}
	return 0, fmt.Errorf("type %s of object %s has no known layout", current[0], a.objectId)
}

// typeLayout is the layout a type's objects get: the installed type's
// recommended layout, or the bundle's for a bundled type the space has not
// installed yet (installSetTypeTargets installs it before the lock, so the
// bundle's layout is what will apply).
func typeLayout(entry typeEntry) model.ObjectTypeLayout {
	if entry.Id != "" {
		return entry.Layout
	}
	if t, err := bundle.GetType(domain.TypeKey(entry.Key)); err == nil {
		return t.Layout
	}
	return entry.Layout
}

// layoutConversionRefusal is the 400 for a type whose layout the object
// cannot convert to: both layouts by name, the rule, and the types this
// object CAN take — the alternatives are the repair, and a listing costs no
// lookup because the entries carry their layout.
func (a *v2StateApplier) layoutConversionRefusal(spelled string, current []domain.TypeKey, from, to model.ObjectTypeLayout, entries []typeEntry, path string) error {
	return layoutConversionRefusal(a.spaceId, spelled, current, from, to, entries, path, a.v)
}

func layoutConversionRefusal(spaceId, spelled string, current []domain.TypeKey, from, to model.ObjectTypeLayout, entries []typeEntry, path string, v errKeys) error {
	compatible := compatibleTypeKeys(from, current, entries, v)
	hint := v2model.Hintf("list all with %s", v2model.RefListTypes(spaceId))
	if len(compatible) > 0 {
		hint = v2model.Hintf("types this object can take: %s — list all with %s", strings.Join(compatible, ", "), v2model.RefListTypes(spaceId))
	}
	return v2model.ValidationFailed(
		fmt.Sprintf("an object with the %s layout cannot become %q, whose objects have the %s layout", layoutName(from), spelled, layoutName(to)),
		v2model.Issue{Path: path,
			Message: "a type change converts the document to the new type's layout, and a conversion exists only between the page layouts (basic, todo, note, profile, bookmark) or between two types of the same layout",
		}.WithHint(hint))
}

// compatibleTypeKeys lists, in served spelling, the visible live types an
// object with layout from can become — other than the one it already is.
func compatibleTypeKeys(from model.ObjectTypeLayout, current []domain.TypeKey, entries []typeEntry, v errKeys) []string {
	keyTaken, slugHolders := servedTypeKeySets(entries)
	var keys []string
	for _, entry := range entries {
		if entry.Hidden || entry.Key == string(bundle.TypeKeyTemplate) {
			continue
		}
		if len(current) > 0 && entry.Key == string(current[0]) {
			continue
		}
		if !converter.IsConversionAllowed(from, entry.Layout) {
			continue
		}
		keys = append(keys, v.spell(servedTypeKeyOf(entry.Key, entry.Slug, keyTaken, slugHolders), entry.Name))
	}
	sort.Strings(keys)
	if len(keys) > v2MaxCompatibleTypesInHint {
		keys = append(keys[:v2MaxCompatibleTypesInHint], "…")
	}
	return keys
}

// layoutName spells a layout the way the type schemas do (basic, todo, note,
// profile, bookmark, set, collection).
func layoutName(layout model.ObjectTypeLayout) string {
	if name, ok := model.ObjectTypeLayout_name[int32(layout)]; ok {
		return name
	}
	return fmt.Sprintf("layout %d", layout)
}

// checkSetTypeTargets is the pre-lock validation of every set_type op in
// the batch, run BEFORE the create-missing guard and prewarm so that a batch
// refused here creates no permanent option: a malformed op (strict decode),
// a bundled type the space REMOVED (refuseRemovedType — the same rule create
// applies; the install would otherwise resurrect it), and, for a bundled
// target not yet installed, a conversion the layout table forbids (so an
// impossible request installs nothing). It returns the uninstalled bundled
// targets a committing run then installs (installSetTypeTargets).
//
// A resolution that is not clean (unknown, ambiguous) is left for the
// applier to report with its own path-addressed error. The batch is NOT
// probed as a whole here: a probe cannot run the conversion, so it refused
// batches whose later ops depend on the converted document (a locator that
// becomes unique once the first block turned into the name) — a false
// refusal the caller cannot repair. The residual is documented in the op's
// description: a later op that fails leaves the type installed.
func (s *Service) checkSetTypeTargets(ctx context.Context, spaceId string, ops []json.RawMessage, cur apicore.ObjectRead) ([]domain.TypeKey, error) {
	v := errKeysFor(ctx)
	var entries []typeEntry
	var toInstall []domain.TypeKey
	// the type and layout as they stand BEFORE each set_type op: a second
	// set_type in the batch starts from what the first one made, exactly as
	// the applier will see it under the lock — read from the original
	// snapshot the check would call a return to the original type a no-op
	// and skip installing it, or refuse from the wrong layout
	current := readTypeKeys(cur.Snapshot.GetObjectTypes())
	from, haveFrom := readLayout(cur, current, nil)
	for i, raw := range ops {
		var probe struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil || probe.Op != "set_type" {
			continue
		}
		opPath := fmt.Sprintf("ops[%d]", i)
		var op opSetType
		if err := decodeStrictOp(raw, probe.Op, opPath, &op); err != nil {
			return nil, err
		}
		if op.Type == "" {
			continue // the applier's own refusal
		}
		if entries == nil {
			var err error
			if entries, err = s.liveTypes(spaceId); err != nil {
				return nil, fmt.Errorf("load types of space %s: %w", spaceId, err)
			}
			if !haveFrom {
				from, haveFrom = readLayout(cur, current, entries)
			}
		}
		entry, ok, ambiguous, err := s.resolveTypeInput(spaceId, op.Type, entries)
		if err != nil || !ok || len(ambiguous) > 0 {
			continue
		}
		key := domain.TypeKey(entry.Key)
		if len(current) > 0 && current[0] == key {
			continue // a no-op for the applier too
		}
		if entry.Id == "" && bundle.HasObjectTypeByKey(key) && key != bundle.TypeKeyTemplate {
			if err := s.refuseRemovedType(ctx, spaceId, entry.Key, op.Type, opPath+".type"); err != nil {
				return nil, err
			}
			if haveFrom && !converter.IsConversionAllowed(from, typeLayout(entry)) {
				return nil, layoutConversionRefusal(spaceId, op.Type, current, from, typeLayout(entry), entries, opPath+".type", v)
			}
			toInstall = append(toInstall, key)
		}
		current, from, haveFrom = []domain.TypeKey{key}, typeLayout(entry), true
	}
	return toInstall, nil
}

// readTypeKeys parses a snapshot's object types (raw unique keys such as
// "ot-page"; a bare key is accepted too) into type keys.
func readTypeKeys(raw []string) []domain.TypeKey {
	keys := make([]domain.TypeKey, 0, len(raw))
	for _, r := range raw {
		if key, err := domain.GetTypeKeyFromRawUniqueKey(r); err == nil {
			keys = append(keys, key)
		} else if r != "" {
			keys = append(keys, domain.TypeKey(r))
		}
	}
	return keys
}

// readLayout is currentLayout over a read instead of a state: the resolved
// layout the snapshot carries, else the current type's layout (from the
// live entries when given, else the bundle).
func readLayout(cur apicore.ObjectRead, current []domain.TypeKey, entries []typeEntry) (model.ObjectTypeLayout, bool) {
	if details := cur.Snapshot.GetDetails(); details != nil {
		if v, ok := details.Fields[bundle.RelationKeyResolvedLayout.String()]; ok && v != nil {
			return model.ObjectTypeLayout(pbtypes.GetInt64(details, bundle.RelationKeyResolvedLayout.String())), true //nolint:gosec
		}
	}
	if len(current) == 0 {
		return 0, false
	}
	for _, entry := range entries {
		if entry.Key == string(current[0]) {
			return entry.Layout, true
		}
	}
	if t, err := bundle.GetType(current[0]); err == nil {
		return t.Layout, true
	}
	return 0, false
}

// v2InstalledTypeVisibleTimeout bounds how long a committing run waits for a
// type it just installed to become readable in the space's store: the
// editor's type change reads the target by unique key there, and an install
// that raced another one (or whose indexing lags) is otherwise a 500.
var v2InstalledTypeVisibleTimeout = 3 * time.Second

// installSetTypeTargets installs, BEFORE the object lock, the uninstalled
// bundled targets checkSetTypeTargets returned (an install is its own object
// create, which must not run under the lock), then waits until each is
// readable by unique key so the editor's own lookup under the lock cannot
// miss it.
func (s *Service) installSetTypeTargets(ctx context.Context, spaceId string, keys []domain.TypeKey) error {
	if len(keys) == 0 || s.creator == nil {
		return nil
	}
	for _, key := range keys {
		if err := s.creator.InstallBundledType(ctx, spaceId, key); err != nil {
			if errors.Is(err, apicore.ErrSpaceReadOnly) {
				return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
					fmt.Sprintf("space %q is read-only for this account", spaceId),
					v2model.Issue{Path: "/ops", Message: "the account can read this space but not write it, so nothing in it can be changed or installed"})
			}
			return fmt.Errorf("install type %s for set_type: %w", key, err)
		}
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key.String())
		if err != nil {
			return fmt.Errorf("unique key of type %s: %w", key, err)
		}
		deadline := time.Now().Add(v2InstalledTypeVisibleTimeout)
		for {
			if _, err := s.store.SpaceIndex(spaceId).GetObjectByUniqueKey(uk); err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("type %s was installed for set_type but is not readable yet — retry the request", key)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	return nil
}
