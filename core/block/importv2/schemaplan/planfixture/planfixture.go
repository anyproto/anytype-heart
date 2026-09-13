// Package planfixture loads the synthetic workspace fixtures the planner
// quality tests run against.
//
// A fixture is one synthetic workspace: the planner's input (which loads into
// []schemaplan.ContainerSchema) plus machine-checkable expectations about what
// a correct plan must do with it. A fixture without expectations is not a test.
//
// The on-disk format deliberately does not marshal ContainerSchema directly.
// That type carries no JSON tags, so its natural encoding is Go field names and
// numeric relation formats ({"Format": 4}) — unreadable and unwritable by hand.
// Fixtures instead speak the same string format vocabulary the planner prompt
// uses ("text", "select", "date", ...), and this package is the one place that
// translation lives. See fixtures/FORMAT.md for the authoring rules.
package planfixture

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

//go:embed fixtures/*.json
var fixturesFS embed.FS

const fixturesDir = "fixtures"

// formats is the closed fixture vocabulary. It mirrors the planner prompt's own
// format names so a fixture and a plan describe a property the same way; note
// "text" maps to longtext because shortText is deprecated and we mint one text
// format.
var formats = map[string]model.RelationFormat{
	"text":        model.RelationFormat_longtext,
	"select":      model.RelationFormat_status,
	"multiSelect": model.RelationFormat_tag,
	"date":        model.RelationFormat_date,
	"number":      model.RelationFormat_number,
	"checkbox":    model.RelationFormat_checkbox,
	"url":         model.RelationFormat_url,
	"email":       model.RelationFormat_email,
	"phone":       model.RelationFormat_phone,
	"files":       model.RelationFormat_file,
	"objects":     model.RelationFormat_object,
}

// optioned are the formats that carry (and require) an option vocabulary.
var optioned = map[string]bool{"select": true, "multiSelect": true}

// Fixture is one synthetic workspace and what a correct plan must do with it.
type Fixture struct {
	Id          string
	Name        string
	Family      string
	Inspiration string
	Notes       string
	Containers  []schemaplan.ContainerSchema
	Expect      Expectations
}

// Expectations are the fixture's assertions. Container and property ids are
// fixture-local; a property is addressed as "container:property" in the
// relation groups.
type Expectations struct {
	// SameKind groups containers that must end up sharing one object type.
	SameKind [][]string
	// DifferentKind groups containers that must not share a type.
	DifferentKind [][]string
	// Bundled maps container id → property id → the bundled relation the
	// property must land on.
	Bundled map[string]map[string]domain.RelationKey
	// NotBundled lists, per container, properties that must NOT be redirected
	// onto any bundled relation — the false-positive traps.
	NotBundled map[string][]string
	// SharedRelation groups properties that must land on one relation.
	SharedRelation [][]string
	// SeparateRelation groups properties that must stay distinct despite
	// sharing a name.
	SeparateRelation [][]string
}

// wire mirrors the on-disk shape.
type wireFixture struct {
	Id          string          `json:"id"`
	Name        string          `json:"name"`
	Family      string          `json:"family"`
	Inspiration string          `json:"inspiration"`
	Notes       string          `json:"notes"`
	Containers  []wireContainer `json:"containers"`
	Expect      wireExpect      `json:"expect"`
}

type wireContainer struct {
	Id         string         `json:"id"`
	Name       string         `json:"name"`
	Properties []wireProperty `json:"properties"`
	Titles     []string       `json:"titles"`
}

type wireProperty struct {
	Id      string   `json:"id"`
	Name    string   `json:"name"`
	Format  string   `json:"format"`
	Options []string `json:"options"`
}

type wireExpect struct {
	SameKind         [][]string                   `json:"sameKind"`
	DifferentKind    [][]string                   `json:"differentKind"`
	Bundled          map[string]map[string]string `json:"bundled"`
	NotBundled       map[string][]string          `json:"notBundled"`
	SharedRelation   [][]string                   `json:"sharedRelation"`
	SeparateRelation [][]string                   `json:"separateRelation"`
}

// Load reads one fixture by its id (the filename without .json).
func Load(id string) (Fixture, error) {
	raw, err := fixturesFS.ReadFile(path.Join(fixturesDir, id+".json"))
	if err != nil {
		return Fixture{}, fmt.Errorf("read fixture %q: %w", id, err)
	}
	fixture, err := decode(raw)
	if err != nil {
		return Fixture{}, fmt.Errorf("decode fixture %q: %w", id, err)
	}
	return fixture, nil
}

// All reads every embedded fixture, ordered by id so tests are deterministic.
func All() ([]Fixture, error) {
	entries, err := fs.ReadDir(fixturesFS, fixturesDir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir: %w", err)
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), ".json"))
	}
	sort.Strings(ids)

	fixtures := make([]Fixture, 0, len(ids))
	for _, id := range ids {
		fixture, err := Load(id)
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures, nil
}

// bundledTargetAllowed reports whether a fixture may assert this target.
// schemaplan keeps the closed set as a slice (with descriptions for the prompt)
// and its lookup map unexported, so a fixture's assertion is checked against
// the same source of truth by scanning it.
func bundledTargetAllowed(key domain.RelationKey) bool {
	for _, target := range schemaplan.AllowedBundledTargets {
		if target.Key == key {
			return true
		}
	}
	return false
}

// Schemas is the shorthand a planner test needs: just the container schemas.
func (f Fixture) Schemas() []schemaplan.ContainerSchema {
	return f.Containers
}

func decode(raw []byte) (Fixture, error) {
	var wire wireFixture
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Fixture{}, fmt.Errorf("unmarshal: %w", err)
	}

	fixture := Fixture{
		Id:          wire.Id,
		Name:        wire.Name,
		Family:      wire.Family,
		Inspiration: wire.Inspiration,
		Notes:       wire.Notes,
	}
	for _, container := range wire.Containers {
		schema := schemaplan.ContainerSchema{Id: container.Id, Name: container.Name}
		for _, property := range container.Properties {
			format, ok := formats[property.Format]
			if !ok {
				return Fixture{}, fmt.Errorf("container %q property %q: unknown format %q",
					container.Id, property.Id, property.Format)
			}
			if optioned[property.Format] == (len(property.Options) == 0) {
				return Fixture{}, fmt.Errorf("container %q property %q: format %q %s options",
					container.Id, property.Id, property.Format,
					map[bool]string{true: "requires", false: "must not carry"}[optioned[property.Format]])
			}
			schema.Properties = append(schema.Properties, schemaplan.PropertySchema{
				Id:      property.Id,
				Name:    property.Name,
				Format:  format,
				Options: property.Options,
			})
		}
		if len(container.Titles) > 0 {
			schema.Samples = &schemaplan.ContainerSamples{Titles: container.Titles}
		}
		fixture.Containers = append(fixture.Containers, schema)
	}

	expect, err := decodeExpect(wire.Expect)
	if err != nil {
		return Fixture{}, err
	}
	fixture.Expect = expect
	return fixture, nil
}

func decodeExpect(wire wireExpect) (Expectations, error) {
	expect := Expectations{
		SameKind:         wire.SameKind,
		DifferentKind:    wire.DifferentKind,
		NotBundled:       wire.NotBundled,
		SharedRelation:   wire.SharedRelation,
		SeparateRelation: wire.SeparateRelation,
	}
	if len(wire.Bundled) > 0 {
		expect.Bundled = make(map[string]map[string]domain.RelationKey, len(wire.Bundled))
		for containerId, mapping := range wire.Bundled {
			keys := make(map[string]domain.RelationKey, len(mapping))
			for propertyId, target := range mapping {
				if !bundledTargetAllowed(domain.RelationKey(target)) {
					return Expectations{}, fmt.Errorf(
						"container %q property %q: %q is not an allowed bundled target",
						containerId, propertyId, target)
				}
				keys[propertyId] = domain.RelationKey(target)
			}
			expect.Bundled[containerId] = keys
		}
	}
	return expect, nil
}
