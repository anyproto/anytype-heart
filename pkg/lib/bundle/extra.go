package bundle

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// envExtraRelations holds an os.PathListSeparator-separated list of JSON files with additional
// bundled relations. Internal escape hatch: it lets a build learn about a relation without
// regenerating relation.gen.go. Files use the same schema as relations.json, and a key that
// already exists in the generated bundle is never replaced.
const envExtraRelations = "ANYTYPE_EXTRA_RELATIONS"

var log = logging.Logger("bundle")

var relationKeyRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// relationJSON mirrors the schema of relations.json consumed by ./generator, so an entry can be
// moved between the two unchanged.
type relationJSON struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Format      string   `json:"format"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	MaxCount    int      `json:"maxCount"`
	Hidden      bool     `json:"hidden"`
	Readonly    bool     `json:"readonly"`
	ObjectTypes []string `json:"objectTypes"`
	Revision    int      `json:"revision"`
	IncludeTime bool     `json:"includeTime"` // nolint: tagliatelle
}

func extraRelationsPaths() []string {
	return filepath.SplitList(os.Getenv(envExtraRelations))
}

// loadExtraRelations merges relations declared in the given JSON files into dst. Keys already
// present in dst are skipped with a warning: the generated bundle always wins. Any other problem
// is an error — a misconfigured env var must not produce a half-populated bundle.
func loadExtraRelations(paths []string, dst map[domain.RelationKey]*model.Relation) error {
	declaredIn := make(map[domain.RelationKey]string)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read extra relations file %s: %w", path, err)
		}
		var parsed []relationJSON
		if err = json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("parse extra relations file %s: %w", path, err)
		}
		for i, entry := range parsed {
			relation, err := entry.toModel()
			if err != nil {
				return fmt.Errorf("extra relation #%d in %s: %w", i, path, err)
			}
			key := domain.RelationKey(relation.Key)
			if prevPath, ok := declaredIn[key]; ok {
				return fmt.Errorf("extra relation %q in %s is already declared in %s", key, path, prevPath)
			}
			declaredIn[key] = path
			if _, ok := dst[key]; ok {
				log.Warnf("extra relation %q from %s is skipped: it is already a bundled relation", key, path)
				continue
			}
			dst[key] = relation
		}
	}
	return nil
}

// toModel validates an entry and converts it the way ./generator converts relations.json.
func (r relationJSON) toModel() (*model.Relation, error) {
	if !relationKeyRegexp.MatchString(r.Key) {
		return nil, fmt.Errorf("invalid key %q: expected to match %s", r.Key, relationKeyRegexp)
	}
	if r.Name == "" {
		return nil, fmt.Errorf("relation %q has no name", r.Key)
	}
	format, ok := model.RelationFormat_value[r.Format]
	if !ok {
		return nil, fmt.Errorf("relation %q has unknown format %q", r.Key, r.Format)
	}
	dataSource, ok := model.RelationDataSource_value[r.Source]
	if !ok {
		return nil, fmt.Errorf("relation %q has unknown source %q", r.Key, r.Source)
	}
	if r.MaxCount < 0 || r.MaxCount > math.MaxInt32 {
		return nil, fmt.Errorf("relation %q has maxCount %d out of range", r.Key, r.MaxCount)
	}
	var objectTypes []string
	for _, objectType := range r.ObjectTypes {
		if objectType == "" {
			return nil, fmt.Errorf("relation %q has an empty object type", r.Key)
		}
		if strings.HasPrefix(objectType, TypePrefix) {
			return nil, fmt.Errorf("relation %q object type %q must be a bare type key, without the %s prefix", r.Key, objectType, TypePrefix)
		}
		objectTypes = append(objectTypes, TypePrefix+objectType)
	}
	return &model.Relation{
		Id:               addr.BundledRelationURLPrefix + r.Key,
		Key:              r.Key,
		Name:             r.Name,
		Format:           model.RelationFormat(format),
		DataSource:       model.RelationDataSource(dataSource),
		Description:      r.Description,
		MaxCount:         int32(r.MaxCount),
		Hidden:           r.Hidden,
		ReadOnly:         r.Readonly,
		ReadOnlyRelation: true,
		Scope:            model.Relation_type,
		ObjectTypes:      objectTypes,
		Revision:         int64(r.Revision),
		IncludeTime:      r.IncludeTime,
	}, nil
}
