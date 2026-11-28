package v2service

import (
	"encoding/json"
	"errors"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func pageWidthKind(kind model.SmartBlockType) bool {
	return kind == model.SmartBlockType_Page || kind == model.SmartBlockType_Template
}

func typeWidthKind(kind model.SmartBlockType) bool {
	return kind == model.SmartBlockType_STType || kind == model.SmartBlockType_BundledObjectType
}

func layoutWidthPropertyIssue(path string) v2model.Issue {
	return v2model.Issue{
		Path: path, Message: "This property is metadata on a page or template. Use layout_width_percent (100–200) to set its displayed width.",
	}.Hintf("set the field on create, or edit with %s", v2model.RefGetOpSchema("set_layout_width"))
}

func collectWidthWarnings(dst *[]v2model.Issue) func(anyblockjson.Issue) {
	return func(i anyblockjson.Issue) {
		if i.Code.IsLayoutWidthWarning() {
			*dst = append(*dst, v2model.Issue{Path: i.Path, Message: i.Message})
		}
	}
}

// An explicit null removes a width on PATCH. Create documents use omission.
func decodeWidthEdit(value json.RawMessage, path string, warnings *[]v2model.Issue) (domain.Value, error) {
	if string(value) == "null" {
		return domain.Null(), nil
	}
	width, err := anyblockjson.DecodeLayoutWidthPercent(value, func(i anyblockjson.Issue) {
		*warnings = append(*warnings, v2model.Issue{Path: path, Message: i.Message})
	})
	if err != nil {
		var invalid *anyblockjson.ValidationError
		if errors.As(err, &invalid) && len(invalid.Issues) > 0 {
			return domain.Null(), v2model.ValidationFailed("invalid layout width", v2model.Issue{Path: path, Message: invalid.Issues[0].Message})
		}
		return domain.Null(), err
	}
	return domain.Float64(width), nil
}

func (a *v2StateApplier) setLayoutWidth(value json.RawMessage, opPath string) error {
	path := opPath + ".layout_width_percent"
	isType := typeWidthKind(a.sbType)
	if !isType && !pageWidthKind(a.sbType) {
		return v2model.ValidationFailed("unsupported layout width", v2model.Issue{Path: opPath, Message: "set_layout_width takes a page, template, or object type"})
	}
	if value == nil {
		return v2model.ValidationFailed("layout_width_percent is required", v2model.Issue{Path: path, Message: "send a percentage from 100 to 200, or null to clear it"})
	}
	width, err := decodeWidthEdit(value, path, &a.warnings)
	if err != nil {
		return err
	}
	if isType {
		if width.IsNull() {
			a.st.RemoveDetail(bundle.RelationKeyLayoutWidth)
		} else {
			a.st.SetDetailAndBundledRelation(bundle.RelationKeyLayoutWidth, width)
		}
	} else if root := a.st.Get(a.st.RootId()); root == nil {
		return fmt.Errorf("object %s has no root block", a.objectId)
	} else if b := root.Model(); width.IsNull() {
		delete(b.Fields.GetFields(), "width")
	} else {
		b.Fields = pbtypes.EnsureStructInited(b.Fields)
		b.Fields.Fields["width"] = width.ToProto()
	}
	a.mutated()
	return nil
}
