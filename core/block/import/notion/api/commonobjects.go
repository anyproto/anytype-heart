package api

import (
	"bytes"
	"encoding/json"
	"time"
)

type richTextType string

const (
	Text     richTextType = "text"
	Mention  richTextType = "mention"
	Equation richTextType = "equation"
)

type RichText struct {
	Type        richTextType `json:"type,omitempty"`
	Text        *TextObject  `json:"text,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
	PlainText   string       `json:"plain_text,omitempty"`
	Href        string       `json:"href,omitempty"`
}
type TextObject struct {
	Content string `json:"content"`
	Link    *Link  `json:"link,omitempty"`
}

type Link struct {
	Url string `json:"url,omitempty"`
}

type Color string

const (
	DefaultColor Color = "default"
	Gray         Color = "gray"
	Brown        Color = "brown"
	Orange       Color = "orange"
	Yellow       Color = "yellow"
	Green        Color = "green"
	Blue         Color = "blue"
	Purple       Color = "purple"
	Pink         Color = "pink"
	Red          Color = "red"
)

type Annotations struct {
	Bold          bool   `json:"bold"`
	Italic        bool   `json:"italic"`
	Strikethrough bool   `json:"strikethrough"`
	Underline     bool   `json:"underline"`
	Code          bool   `json:"code"`
	Color         string `json:"color"`
}

type FileType string

const (
	External FileType = "external"
	File     FileType = "file"
)

type FileObject struct {
	Name     string      `json:"name"`
	Type     FileType    `json:"type"`
	File     FileProperty `json:"file,omitempty"`
	External FileProperty `json:"external,omitempty"`
}

type FileProperty struct {
	URL        string     `json:"url,omitempty"`
	ExpiryTime *time.Time `json:"expiry_time,omitempty"`
}

func (o *FileProperty) UnmarshalJSON(data []byte) error {
	fp := make(map[string]interface{},0)
    if err := json.Unmarshal(data, &fp); err != nil {
        return err
    }
	if url, ok := fp["url"].(string); ok {
		o.URL = url
	}
	if t, ok := fp["expiry_time"].(*time.Time); ok {
		o.ExpiryTime = t
	}
    return nil
}

type Icon struct {
	Type     FileType    `json:"type"`
	Emoji    *string     `json:"emoji,omitempty"`
	File     *FileObject `json:"file,omitempty"`
	External *FileObject `json:"external,omitempty"`
}

type userType string

type User struct {
	Object    string     `json:"object,omitempty"`
	ID        string     `json:"id"`
	Type      userType   `json:"type,omitempty"`
	Name      string     `json:"name,omitempty"`
	AvatarURL string     `json:"avatar_url,omitempty"`
	Person    *Person    `json:"person,omitempty"`
	Bot       *struct{}  `json:"bot,omitempty"`
}

type Person struct {
	Email string `json:"email"`
}

type Parent struct {
	Type   string `json:"type,omitempty"`
	PageID string `json:"page_id"`
	DatabaseID string `json:"database_id"`
}

type Object interface {
	GetObjectType() string
}

func RichTextToDescription(rt []RichText) string {
	var description bytes.Buffer

	for _, text := range rt {
		description.WriteString(text.PlainText)
		description.WriteRune('\n')
	}
	return description.String()
}
