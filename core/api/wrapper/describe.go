package wrapper

// describe.go — the interim degraded describe (§2 Phase 5 allows it
// explicitly): until the server-side GenerateSchema artifact ships
// (types/{type}/schema is a 501 stub), the wrapper assembles the
// prompt-ready property table itself from GET /types/{type} plus the live
// option lists — the same accuracy lever (A1), composed wrapper-side. When
// GenerateSchema lands, this file collapses to one GET.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

// describeProperty is one row of describe's machine shape.
type describeProperty struct {
	Key     string   `json:"key"`
	Name    string   `json:"name,omitempty"`
	Format  string   `json:"format"`
	Options []string `json:"options,omitempty"`
	// MoreOptions marks a truncated option list (C10 — tag-like properties
	// can hold thousands).
	MoreOptions bool `json:"moreOptions,omitempty"`
	// OptionsUnavailable marks a select whose option listing FAILED — which
	// must render differently from a select that genuinely has no options,
	// or the model is invited to invent a name the A2 guard then rejects.
	OptionsUnavailable bool `json:"optionsUnavailable,omitempty"`
}

// describeResult is describe's machine shape.
type describeResult struct {
	Type       string             `json:"type"`
	Name       string             `json:"name,omitempty"`
	Properties []describeProperty `json:"properties"`
}

// describeOptionsLimit bounds the per-property option listing.
const describeOptionsLimit = 25

func (r *Runner) runDescribe(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space := strArg(args, "space")
	typeKey := strArg(args, "type")
	doc, err := r.client.raw(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(space) + "/types/" + seg(typeKey),
	})
	if err != nil {
		// the server's repair hint names a REST route no tool backs — point
		// at the tool vocabulary instead
		var te *ToolError
		if errors.As(err, &te) {
			te.Text = strings.ReplaceAll(te.Text,
				fmt.Sprintf("list available keys with GET /v2/spaces/%s/types", space),
				"check the type key (find results show each object's type)")
		}
		return nil, err
	}
	var typeDoc struct {
		Key            string         `json:"key"`
		Properties     map[string]any `json:"properties"`
		TypeProperties []struct {
			Key    string `json:"key"`
			Name   string `json:"name"`
			Format string `json:"format"`
		} `json:"typeProperties"`
	}
	if err := json.Unmarshal(doc, &typeDoc); err != nil {
		return nil, fmt.Errorf("decode type document: %w", err)
	}

	result := describeResult{Type: typeKey}
	if name, ok := typeDoc.Properties["name"].(string); ok {
		result.Name = name
	}
	for _, tp := range typeDoc.TypeProperties {
		prop := describeProperty{Key: tp.Key, Name: tp.Name, Format: tp.Format}
		if selectFormats[tp.Format] {
			var resp apimodel.V2ListResponse[apimodel.V2OptionRow]
			if err := r.client.decode(ctx, apiRequest{
				method: "GET",
				path:   "/v2/spaces/" + seg(space) + "/properties/" + seg(tp.Key) + "/options",
				query:  url.Values{"limit": []string{fmt.Sprintf("%d", describeOptionsLimit)}},
			}, &resp); err == nil {
				for _, o := range resp.Data {
					prop.Options = append(prop.Options, o.Name)
				}
				prop.MoreOptions = resp.HasMore
			} else {
				prop.OptionsUnavailable = true
			}
		}
		result.Properties = append(result.Properties, prop)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "type %s", result.Type)
	if result.Name != "" && result.Name != result.Type {
		fmt.Fprintf(&b, " — %s", result.Name)
	}
	b.WriteString("\nproperties:")
	if len(result.Properties) == 0 {
		b.WriteString(" (none beyond name)")
	}
	for _, p := range result.Properties {
		fmt.Fprintf(&b, "\n  %s  %s", p.Key, p.Format)
		if len(p.Options) > 0 {
			fmt.Fprintf(&b, "  options: %s", strings.Join(p.Options, ", "))
			if p.MoreOptions {
				b.WriteString(", …")
			}
		}
		if p.OptionsUnavailable {
			b.WriteString("  options: (could not be listed — run describe again before using this property)")
		}
	}
	b.WriteString("\nuse these exact property keys and option names in create and set_properties")
	return &Result{Text: b.String(), JSON: result}, nil
}
