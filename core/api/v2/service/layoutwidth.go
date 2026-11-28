package v2service

import (
	"encoding/json"
	"errors"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func pageWidthKind(kind model.SmartBlockType) bool {
	return kind == model.SmartBlockType_Page || kind == model.SmartBlockType_Template
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

func (a *v2StateApplier) setLayoutWidth(value json.RawMessage, opPath string) error {
	path := opPath + ".layout_width_percent"
	if !pageWidthKind(a.sbType) {
		return v2model.ValidationFailed("set_layout_width requires a page or template", v2model.Issue{Path: opPath, Message: "on an object type, set the Layout width property (0–1) to change its default"})
	}
	if value == nil {
		return v2model.ValidationFailed("layout_width_percent is required", v2model.Issue{Path: path, Message: "send a percentage from 100 to 200, or null to inherit"})
	}
	root := a.st.Get(a.st.RootId())
	if root == nil {
		return fmt.Errorf("object %s has no root block", a.objectId)
	}
	b := root.Model()
	if string(value) == "null" {
		delete(b.Fields.GetFields(), "width")
	} else {
		width, err := anyblockjson.DecodeLayoutWidthPercent(value, func(i anyblockjson.Issue) {
			a.warnings = append(a.warnings, v2model.Issue{Path: path, Message: i.Message})
		})
		if err != nil {
			var invalid *anyblockjson.ValidationError
			if errors.As(err, &invalid) && len(invalid.Issues) > 0 {
				return v2model.ValidationFailed("invalid page width", v2model.Issue{Path: path, Message: invalid.Issues[0].Message})
			}
			return err
		}
		b.Fields = pbtypes.EnsureStructInited(b.Fields)
		b.Fields.Fields["width"] = pbtypes.Float64(width)
	}
	a.mutated()
	return nil
}
