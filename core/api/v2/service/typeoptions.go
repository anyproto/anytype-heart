package v2service

// typeoptions.go applies the select vocabulary a type document declares on its
// property definitions.
//
// `property_definitions[].options` is part of the format's propertyDefinition
// and decodes cleanly on both type channels, and until now neither channel did
// anything with it: BuildRecommendedLists resolves property IDENTITIES, and the
// option minter only fires when a property VALUE needs an option. So a caller
// declaring a select property together with its vocabulary got a 200, a
// property, and no options — with nothing said. That is the failure this file
// exists to end, and it is the one the resolver's own comment forbids:
// "Refusing beats returning 'unresolved': that would store the NAME where an
// option id belongs — a value matching nothing that reads as if it matched."

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// rejectMisplacedPropertyArray answers the mistake every agent makes on this
// endpoint: sending the shape a REST API would have, with the field list as a
// root `properties` array beside a flat name/layout/plural_name.
//
// The format layer refuses it correctly and unhelpfully — the document routes
// as a property dictionary, because a root `properties` ARRAY is what a
// dictionary is, so the verdict describes a document the caller never meant to
// send. Here we know the endpoint, so we can name the two things they have to
// move and where each one goes.
func rejectMisplacedPropertyArray(body []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil // malformed JSON is the format validation's verdict to give
	}
	raw, present := root["properties"]
	if !present || len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	issues := []v2model.Issue{{
		Path:    "/properties",
		Message: "a field list, not property values: at the root `properties` maps a property's spelling to its VALUE, so an array here reads as a property dictionary rather than a document",
		Hint:    "move the list to `type_settings.property_definitions`, and put the display name in `properties` as {\"name\": \"Plant\"}",
	}}
	for member, home := range map[string]string{
		"layout":      "type_settings.layout",
		"plural_name": "type_settings.plural_name",
		"api_key":     "type_settings.api_key",
	} {
		if _, ok := root[member]; ok {
			issues = append(issues, v2model.Issue{
				Path:    "/" + member,
				Message: fmt.Sprintf("%s belongs to the type, not the document root", member),
				Hint:    "move it to " + home,
			})
		}
	}
	return v2model.ValidationFailed("the field list is in the wrong place", issues...)
}

// applyDeclaredOptions creates the options a type's property definitions
// declare, through the same resolver every other create-missing path uses — so
// consent (?create_missing_options=true), dry-run previewing and the side-effect
// report all behave identically to setting an option name as a value.
//
// It fails closed on a property it cannot address. A property minted by THIS
// request has no stored key yet, so its declared options cannot be created in
// the same call; refusing says so, where skipping would reproduce the silent
// drop in a narrower case.
func (s *Service) applyDeclaredOptions(defs []anyblockjson.TypeProperty, resolvers *creatingResolvers, path string) error {
	return s.applyDeclaredOptionsAt(defs, resolvers, indexedOptionsPath(path))
}

// applyDeclaredOptionsAt is applyDeclaredOptions with the JSON pointer of
// each entry supplied by the caller. The two channels that declare a select
// vocabulary index it differently — a document states a list and an op batch
// states one per op — and a pointer into a list the caller never sent is not
// an address they can act on.
func (s *Service) applyDeclaredOptionsAt(defs []anyblockjson.TypeProperty, resolvers *creatingResolvers, pathAt func(int) string) error {
	for i, def := range defs {
		if len(def.Options) == 0 {
			continue
		}
		// identity is the spelling, then the stored key, then the name the
		// spelling derives from — the same order the format states
		docKey := def.Property
		if docKey == "" {
			docKey = def.InternalKey
		}
		if docKey == "" {
			docKey = def.Name
		}
		defPath := pathAt(i)

		storedKey, ok := resolvers.PropertyStoredKey(docKey)
		if !ok && resolvers.dryRun && resolvers.MintingProperty(docKey) {
			// a DRY RUN only: the property is not minted, so there is no key to
			// create against. Preview the options instead of refusing — the
			// real run creates the property first and then these. Consent is
			// still required, because the real run would require it.
			//
			// Gated on dryRun, not on MintingProperty alone. On a REAL run the
			// stored-key read-back can also miss (the mint happened, the row
			// did not come back), and this branch then reported the options as
			// created while creating none — exactly the silent drop the file
			// exists to end, and the opposite of the refusal
			// PropertyStoredKey's own comment promises.
			if !resolvers.CreateMissingConsent() {
				return optionConsentErrorAt(resolvers.spaceId, docKey, def.Options[0].Name, defPath)
			}
			for _, opt := range def.Options {
				if opt.Name != "" {
					resolvers.PlanOption(docKey, opt.Name)
				}
			}
			continue
		}
		if !ok {
			names := make([]string, 0, len(def.Options))
			for _, opt := range def.Options {
				names = append(names, opt.Name)
			}
			return v2model.ValidationFailed("declared options need an existing property",
				v2model.Issue{
					Path: defPath,
					Message: fmt.Sprintf(
						"property %q is created by this request, so its options (%s) cannot be created in the same call",
						docKey, joinQuoted(names)),
				}.Hintf("create the property first with %s, which takes its options, then reference it here", v2model.NewRef(v2model.OpCreateProperty)))
		}
		for _, opt := range def.Options {
			if opt.Name == "" {
				continue
			}
			resolvers.DeclareOptionColor(domain.RelationKey(storedKey), opt.Name, opt.Color)
			if _, created := resolvers.OptionId(domain.RelationKey(storedKey), opt.Name); !created {
				// the resolver has recorded the reason (no consent, ambiguity,
				// an RPC failure) and resolvers.err() surfaces it to the caller
				continue
			}
		}
	}
	return resolvers.err()
}

func joinQuoted(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%q", n)
	}
	return out
}

// storedRelationKeyById reads the stored relation key of a property object.
// The mint returns an id; the key it assigned is only on the object, and an
// option is created against the key.
func (s *Service) storedRelationKeyById(ctx context.Context, spaceId, objectId string) (string, bool) {
	details, err := s.store.SpaceIndex(spaceId).GetDetails(objectId)
	if err != nil || details == nil {
		return "", false
	}
	key := details.GetString(bundle.RelationKeyRelationKey)
	return key, key != ""
}

// declaredOptionKey is the spelling a definition identifies its property by,
// in the order the format states: the document-facing spelling, then the
// stored key, then the name the spelling derives from.
func declaredOptionKey(def anyblockjson.TypeProperty) string {
	if def.Property != "" {
		return def.Property
	}
	if def.InternalKey != "" {
		return def.InternalKey
	}
	return def.Name
}

// guardDeclaredOptionConsent refuses a body whose declared options would need
// consent BEFORE anything is written.
//
// Without it the refusal arrives too late to matter: anyblockjson.Unmarshal
// mints the missing properties first, applyDeclaredOptions hits the consent
// gate second, and the failed request leaves the minted properties behind as
// orphan relations. CreateType's own comment states the rule this restores —
// reject before any side effect — and the PATCH channel already has its
// equivalent in guardCreateMissing.
//
// It refuses only what the resolver would refuse. A declared option already
// present on an existing property creates nothing, so it needs no consent and
// is not reported here.
func (s *Service) guardDeclaredOptions(spaceId string, defs []anyblockjson.TypeProperty, path string, createMissingOptions bool) error {
	return s.guardDeclaredOptionsAt(spaceId, defs, indexedOptionsPath(path), createMissingOptions)
}

// indexedOptionsPath is the pointer a definition LIST gives its entries.
func indexedOptionsPath(path string) func(int) string {
	return func(i int) string { return fmt.Sprintf("%s/%d/options", path, i) }
}

// guardDeclaredOptionsAt is guardDeclaredOptions with caller-supplied
// pointers — see applyDeclaredOptionsAt.
func (s *Service) guardDeclaredOptionsAt(spaceId string, defs []anyblockjson.TypeProperty, pathAt func(int) string, createMissingOptions bool) error {
	var declaring bool
	for _, def := range defs {
		if len(def.Options) > 0 {
			declaring = true
			break
		}
	}
	if !declaring {
		return nil
	}

	properties, err := s.liveProperties(spaceId)
	if err != nil {
		return err
	}
	find := func(spelling string) (propertyEntry, bool) {
		for _, entry := range properties {
			if entry.Key == spelling || (entry.Slug != "" && entry.Slug == spelling) || entry.Name == spelling {
				return entry, true
			}
		}
		return propertyEntry{}, false
	}

	for i, def := range defs {
		if len(def.Options) == 0 {
			continue
		}
		defPath := pathAt(i)
		docKey := declaredOptionKey(def)
		entry, exists := find(docKey)

		// options need a select format, the rule POST /properties states at
		// its own /options and the format layer enforces on a create-time
		// document. UpdateType validates no document at all, so without this
		// a PATCH could hang a vocabulary on a text property.
		declaredFormat := def.Format
		if declaredFormat == "" && exists {
			declaredFormat = anyblockjson.FormatName(entry.Format)
		}
		if declaredFormat != "" && declaredFormat != "select" && declaredFormat != "multi_select" {
			return v2model.ValidationFailed("options need a select format",
				v2model.Issue{
					Path:    defPath,
					Message: fmt.Sprintf("options apply to select and multi_select properties, and %q is %s", docKey, declaredFormat),
				})
		}

		if createMissingOptions {
			continue // the format rule above still applied; consent is granted
		}

		// a property this request mints holds no options at all, so every
		// name declared against it is one the request would create
		if !exists {
			return optionConsentErrorAt(spaceId, docKey, def.Options[0].Name, defPath)
		}
		held := map[string]bool{}
		options, err := s.store.SpaceIndex(spaceId).ListRelationOptions(domain.RelationKey(entry.Key))
		if err != nil {
			return fmt.Errorf("list options of property %s: %w", entry.Key, err)
		}
		for _, option := range options {
			if option != nil {
				held[option.Text] = true
			}
		}
		for _, opt := range def.Options {
			if opt.Name != "" && !held[opt.Name] {
				return optionConsentErrorAt(spaceId, docKey, opt.Name, defPath)
			}
		}
	}
	return nil
}

// detachedProperties names the properties a replaced property_definitions list
// dropped: present in the type's recommended lists before the write, absent
// after.
//
// Reported as rows rather than ids because an id is not an address on this
// surface — a caller who sent `location` cannot match a bson id back to the
// field they wrote by hand. An id that no longer resolves to a live property
// is skipped rather than guessed at.
func (s *Service) detachedProperties(spaceId string, before, kept map[string]bool) []v2model.PropertyRow {
	var gone []string
	for id := range before {
		if !kept[id] {
			gone = append(gone, id)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return nil
	}
	byId := make(map[string]propertyEntry, len(entries))
	for _, entry := range entries {
		byId[entry.Id] = entry
	}
	keyTaken, slugHolders := servedPropertyKeySets(entries)

	rows := make([]v2model.PropertyRow, 0, len(gone))
	for _, id := range gone {
		entry, ok := byId[id]
		if !ok {
			continue
		}
		rows = append(rows, v2model.PropertyRow{
			Key:    servedKey(entry.Key, entry.Slug, keyTaken, slugHolders),
			Name:   entry.Name,
			Format: anyblockjson.FormatName(entry.Format),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	return rows
}
