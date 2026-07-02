package notion

import (
	"context"
	"fmt"
	"time"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// iconValue is a page/database/callout icon: emoji, custom emoji, or file.
type iconValue struct {
	Type     string `json:"type"` // emoji | external | file | custom_emoji
	Emoji    string `json:"emoji"`
	External *struct {
		Url string `json:"url"`
	} `json:"external"`
	File *struct {
		Url string `json:"url"`
	} `json:"file"`
	CustomEmoji *customEmoji `json:"custom_emoji"`
}

func (i *iconValue) fileUrl() string {
	switch {
	case i == nil:
		return ""
	case i.Type == "external" && i.External != nil:
		return i.External.Url
	case i.Type == "file" && i.File != nil:
		return i.File.Url
	case i.Type == "custom_emoji" && i.CustomEmoji != nil:
		// Approved decision: custom emoji import as their image (v1
		// dropped them).
		return i.CustomEmoji.Url
	default:
		return ""
	}
}

// fileValue is a file reference (cover, files property, file blocks).
type fileValue struct {
	Type     string `json:"type"` // external | file
	Name     string `json:"name"`
	External *struct {
		Url string `json:"url"`
	} `json:"external"`
	File *struct {
		Url string `json:"url"`
	} `json:"file"`
	Caption []richText `json:"caption"`
}

func (f *fileValue) url() string {
	switch {
	case f == nil:
		return ""
	case f.Type == "external" && f.External != nil:
		return f.External.Url
	case f.File != nil:
		return f.File.Url
	default:
		return ""
	}
}

// applyIcon writes icon/cover details, emitting file objects for
// image-backed icons and covers.
func (c *Converter) applyIcon(ctx context.Context, object *importv2.Object, icon *iconValue, cover *fileValue, sink importv2.Sink) error {
	if icon != nil {
		if icon.Type == "emoji" && icon.Emoji != "" {
			object.Payload.Details.SetString(bundle.RelationKeyIconEmoji, icon.Emoji)
		} else if iconUrl := icon.fileUrl(); iconUrl != "" {
			sourceKey, err := c.emitFileFromUrl(ctx, sink, iconUrl, "icon")
			if err != nil {
				return err
			}
			object.Payload.Details.SetString(bundle.RelationKeyIconImage, sourceKey)
		}
	}
	if coverUrl := cover.url(); coverUrl != "" {
		sourceKey, err := c.emitFileFromUrl(ctx, sink, coverUrl, "cover")
		if err != nil {
			return err
		}
		object.Payload.Details.SetString(bundle.RelationKeyCoverId, sourceKey)
		object.Payload.Details.SetInt64(bundle.RelationKeyCoverType, 1) // image cover
	}
	return nil
}

// setTimestamps parses the API's RFC3339 created/edited times.
func setTimestamps(details *domain.Details, createdTime, lastEditedTime string) {
	if created, err := time.Parse(time.RFC3339, createdTime); err == nil {
		details.SetInt64(bundle.RelationKeyCreatedDate, created.Unix())
	}
	if edited, err := time.Parse(time.RFC3339, lastEditedTime); err == nil {
		details.SetInt64(bundle.RelationKeyLastModifiedDate, edited.Unix())
	}
}

// parseNotionDate converts a date value to a unix timestamp, honoring the
// time zone. Malformed dates return an error (v1 silently imported them as
// epoch 0) — the caller reports a warning and omits the detail.
func parseNotionDate(value string, timeZone string) (int64, bool, error) {
	location := time.UTC
	if timeZone != "" {
		if loaded, err := time.LoadLocation(timeZone); err == nil {
			location = loaded
		}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Unix(), true, nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, location); err == nil {
		return parsed.Unix(), false, nil
	}
	return 0, false, fmt.Errorf("unparseable date %q", value)
}
