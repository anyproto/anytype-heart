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
// The file_upload variant carries only an upload id (no fetchable URL) and
// degrades with a warning at the call sites via fileUrl() == "".
type iconValue struct {
	Type     string `json:"type"` // emoji | external | file | custom_emoji | file_upload
	Emoji    string `json:"emoji"`
	External *struct {
		Url string `json:"url"`
	} `json:"external"`
	File *struct {
		Url string `json:"url"`
	} `json:"file"`
	CustomEmoji *customEmoji `json:"custom_emoji"`
}

// isExternal reports a user-provided external URL — its query string is
// part of the file's identity, unlike Notion-hosted signed URLs whose query
// is a per-response signature.
func (i *iconValue) isExternal() bool { return i != nil && i.Type == "external" }

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

func (f *fileValue) isExternal() bool { return f != nil && f.Type == "external" }

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
// image-backed icons and covers. refreshPath is the entity's GET path
// ("/pages/{id}" or "/data_sources/{id}") for expired-URL re-minting.
func (c *Converter) applyIcon(ctx context.Context, object *importv2.Object, icon *iconValue, cover *fileValue, refreshPath string, sink importv2.Sink) error {
	if icon != nil {
		if icon.Type == "emoji" && icon.Emoji != "" {
			object.Payload.Details.SetString(bundle.RelationKeyIconEmoji, icon.Emoji)
		} else if iconUrl := icon.fileUrl(); iconUrl != "" {
			refresh := c.entityUrlRefresher(refreshPath, func(fresh *iconValue, _ *fileValue) string {
				return fresh.fileUrl()
			})
			sourceKey, err := c.emitFileFromUrl(ctx, sink, iconUrl, "icon", icon.isExternal(), refresh)
			if err != nil {
				return err
			}
			object.Payload.Details.SetString(bundle.RelationKeyIconImage, sourceKey)
		}
	}
	if coverUrl := cover.url(); coverUrl != "" {
		refresh := c.entityUrlRefresher(refreshPath, func(_ *iconValue, fresh *fileValue) string {
			return fresh.url()
		})
		sourceKey, err := c.emitFileFromUrl(ctx, sink, coverUrl, "cover", cover.isExternal(), refresh)
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
// time zone. The API contract: when an IANA time_zone is set, start/end
// carry NO UTC offset (e.g. "2020-12-08T12:00:00") — the offset-less
// layouts interpret those in the given zone. Malformed dates return an
// error (v1 silently imported them as epoch 0) — the caller reports a
// warning and omits the detail.
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
	for _, layout := range []string{"2006-01-02T15:04:05.999", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed.Unix(), true, nil
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, location); err == nil {
		return parsed.Unix(), false, nil
	}
	return 0, false, fmt.Errorf("unparseable date %q", value)
}
