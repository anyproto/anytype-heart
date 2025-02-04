package api

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api/files"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type RichTextType string

const (
	Text     RichTextType = "text"
	Mention  RichTextType = "mention"
	Equation RichTextType = "equation"
)

const NotionBackgroundColorSuffix = "background"

// RichText represent RichText object from Notion https://developers.notion.com/reference/rich-text
type RichText struct {
	Type        RichTextType    `json:"type,omitempty"`
	Text        *TextObject     `json:"text,omitempty"`
	Mention     *MentionObject  `json:"mention,omitempty"`
	Equation    *EquationObject `json:"equation,omitempty"`
	Annotations *Annotations    `json:"annotations,omitempty"`
	PlainText   string          `json:"plain_text,omitempty"`
	Href        string          `json:"href,omitempty"`
}
type TextObject struct {
	Content string `json:"content"`
	Link    *Link  `json:"link,omitempty"`
}
type EquationObject struct {
	Expression string `json:"expression"`
}

func (e *EquationObject) HandleEquation() *model.Block {
	id := bson.NewObjectId().Hex()
	return &model.Block{
		Id:          id,
		ChildrenIds: []string{},
		Content: &model.BlockContentOfLatex{
			Latex: &model.BlockContentLatex{
				Text: e.Expression,
			},
		},
	}
}

type Link struct {
	URL string `json:"url,omitempty"`
}

func (rt *RichText) BuildMarkdownFromAnnotations(from, to int32) []*model.BlockContentTextMark {
	var marks []*model.BlockContentTextMark
	if rt.Annotations == nil {
		return marks
	}
	if rt.Annotations.Bold {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: from,
				To:   to,
			},
			Type: model.BlockContentTextMark_Bold,
		})
	}
	if rt.Annotations.Italic {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: from,
				To:   to,
			},
			Type: model.BlockContentTextMark_Italic,
		})
	}
	if rt.Annotations.Strikethrough {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: from,
				To:   to,
			},
			Type: model.BlockContentTextMark_Strikethrough,
		})
	}
	if rt.Annotations.Underline {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: from,
				To:   to,
			},
			Type: model.BlockContentTextMark_Underscored,
		})
	}
	// not add marks for default color
	if rt.Annotations.Color != "" && rt.Annotations.Color != DefaultColor {
		markType := model.BlockContentTextMark_TextColor
		if strings.HasSuffix(rt.Annotations.Color, NotionBackgroundColorSuffix) {
			markType = model.BlockContentTextMark_BackgroundColor
		}
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: from,
				To:   to,
			},
			Type:  markType,
			Param: NotionColorToAnytype[rt.Annotations.Color],
		})
	}

	if rt.Annotations.Code {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: from,
				To:   to,
			},
			Type: model.BlockContentTextMark_Keyboard,
		})
	}

	return marks
}

type mentionType string

const (
	UserMention mentionType = "user"
	Page        mentionType = "page"
	Database    mentionType = "database"
	Date        mentionType = "date"
	LinkPreview mentionType = "link_preview"
)

type MentionObject struct {
	Type        mentionType      `json:"type,omitempty"`
	User        *User            `json:"user,omitempty"`
	Page        *PageMention     `json:"page,omitempty"`
	Database    *DatabaseMention `json:"database,omitempty"`
	Date        *DateObject      `json:"date,omitempty"`
	LinkPreview *Link            `json:"link_preview,omitempty"`
}

type PageMention struct {
	ID string `json:"id"`
}

type DatabaseMention struct {
	ID string `json:"id"`
}

type DateObject struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	TimeZone string `json:"time_zone"`
}

const (
	DefaultColor string = "default"
	Gray         string = "gray"
	Brown        string = "brown"
	Orange       string = "orange"
	Yellow       string = "yellow"
	Green        string = "green"
	Blue         string = "blue"
	Purple       string = "purple"
	Pink         string = "pink"
	Red          string = "red"

	GrayBackGround   string = "gray_background"
	BrownBackGround  string = "brown_background"
	OrangeBackGround string = "orange_background"
	YellowBackGround string = "yellow_background"
	GreenBackGround  string = "green_background"
	BlueBackGround   string = "blue_background"
	PurpleBackGround string = "purple_background"
	PinkBackGround   string = "pink_background"
	RedBackGround    string = "red_background"

	AnytypeGray    string = "grey"
	AnytypeOrange  string = "orange"
	AnytypeYellow  string = "yellow"
	AnytypeGreen   string = "lime"
	AnytypeBlue    string = "blue"
	AnytypePurple  string = "purple"
	AnytypePink    string = "pink"
	AnytypeRed     string = "red"
	AnytypeDefault string = ""
)

var NotionColorToAnytype = map[string]string{
	DefaultColor: AnytypeDefault,
	Gray:         AnytypeGray,
	Brown:        "",
	Orange:       AnytypeOrange,
	Yellow:       AnytypeYellow,
	Green:        AnytypeGreen,
	Blue:         AnytypeBlue,
	Purple:       AnytypePurple,
	Pink:         AnytypePink,
	Red:          AnytypeRed,

	GrayBackGround:   AnytypeGray,
	BrownBackGround:  "",
	OrangeBackGround: AnytypeOrange,
	YellowBackGround: AnytypeYellow,
	GreenBackGround:  AnytypeGreen,
	BlueBackGround:   AnytypeBlue,
	PurpleBackGround: AnytypePurple,
	PinkBackGround:   AnytypePink,
	RedBackGround:    AnytypeRed,
}

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

// FileObject represent File Object object from Notion https://developers.notion.com/reference/file-object
type FileObject struct {
	Name     string       `json:"name"`
	Type     FileType     `json:"type"`
	File     FileProperty `json:"file,omitempty"`
	External FileProperty `json:"external,omitempty"`
}

func (f *FileObject) GetFileBlock(fileType model.BlockContentFileType) (*model.Block, string) {
	id := bson.NewObjectId().Hex()
	name := f.External.URL
	if name == "" {
		name = f.File.URL
	}
	return &model.Block{
		Id: id,
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:    name,
				AddedAt: time.Now().Unix(),
				Type:    fileType,
			},
		},
	}, id
}

type FileProperty struct {
	URL        string     `json:"url,omitempty"`
	ExpiryTime *time.Time `json:"expiry_time,omitempty"`
}

func (o *FileProperty) UnmarshalJSON(data []byte) error {
	fp := make(map[string]interface{}, 0)
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
	Type     FileType      `json:"type"`
	Emoji    *string       `json:"emoji,omitempty"`
	File     *FileProperty `json:"file,omitempty"`
	External *FileProperty `json:"external,omitempty"`
}

func SetIcon(details *domain.Details, icon *Icon) *model.RelationLink {
	if icon.Emoji != nil {
		details.SetString(bundle.RelationKeyIconEmoji, *icon.Emoji)
	}
	var linkToIconImage string
	if icon.Type == External && icon.External != nil {
		linkToIconImage = icon.External.URL
	}
	if icon.Type == File && icon.File != nil {
		linkToIconImage = icon.File.URL
	}
	if linkToIconImage != "" {
		details.SetString(bundle.RelationKeyIconImage, linkToIconImage)
		return &model.RelationLink{
			Key:    bundle.RelationKeyIconImage.String(),
			Format: model.RelationFormat_file,
		}
	}
	return nil
}

func SetCover(details *domain.Details, cover *FileObject) {
	if cover == nil || details == nil {
		return
	}
	if cover.Type == External {
		details.Set(bundle.RelationKeyCoverId, domain.String(cover.External.URL))
		details.Set(bundle.RelationKeyCoverType, domain.Int64(1))
	}

	if cover.Type == File {
		details.Set(bundle.RelationKeyCoverId, domain.String(cover.File.URL))
		details.Set(bundle.RelationKeyCoverType, domain.Int64(1))
	}
}

type userType string

// User represent User Object object from Notion https://developers.notion.com/reference/user
type User struct {
	Object    string    `json:"object,omitempty"`
	ID        string    `json:"id"`
	Type      userType  `json:"type,omitempty"`
	Name      string    `json:"name,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	Person    *Person   `json:"person,omitempty"`
	Bot       *struct{} `json:"bot,omitempty"`
}

type Person struct {
	Email string `json:"email"`
}

type Parent struct {
	Type       string `json:"type,omitempty"`
	PageID     string `json:"page_id"`
	DatabaseID string `json:"database_id"`
	BlockID    string `json:"block_id"`
	Workspace  bool   `json:"workspace"`
}

func RichTextToDescription(rt []*RichText) string {
	var richText strings.Builder
	for i, title := range rt {
		richText.WriteString(title.PlainText)
		if i != len(rt)-1 {
			richText.WriteString("\n")
		}
	}
	return richText.String()
}

type relationUploadData struct {
	fileDownloader files.Downloader

	details *domain.Details
	mu      sync.Mutex

	tasks []func()
	wg    sync.WaitGroup
}

type relationFileMetaData struct {
	relationKey  domain.RelationKey
	url          string
	idxInDetails int
}

func UploadFileRelationLocally(fileDownloader files.Downloader, details *domain.Details, relationLinks []*model.RelationLink) {
	data := &relationUploadData{fileDownloader: fileDownloader, details: details}
	for _, relationLink := range relationLinks {
		if relationLink.Format == model.RelationFormat_file {
			fileUrl := details.GetString(domain.RelationKey(relationLink.Key))
			if fileUrl == "" {
				handleListValue(data, domain.RelationKey(relationLink.Key))
			}
			if fileUrl != "" {
				fileMetaData := &relationFileMetaData{relationKey: domain.RelationKey(relationLink.Key), url: fileUrl}
				stop := queueTaskWithFileDownload(data, fileMetaData)
				if stop {
					break
				}
			}
		}
	}
	for _, task := range data.tasks {
		go task()
	}
	data.wg.Wait()
}

func handleListValue(data *relationUploadData, relationKey domain.RelationKey) {
	fileUrls := data.details.GetStringList(relationKey)
	for i, url := range fileUrls {
		fileMetaData := &relationFileMetaData{relationKey, url, i}
		stop := queueTaskWithFileDownload(data, fileMetaData)
		if stop {
			break
		}
	}
}

func queueTaskWithFileDownload(data *relationUploadData, fileMetaData *relationFileMetaData) bool {
	file, stop := data.fileDownloader.QueueFileForDownload(fileMetaData.url)
	if stop {
		return true
	}
	data.wg.Add(1)
	data.tasks = append(data.tasks, func() {
		defer data.wg.Done()
		localPath, err := file.WaitForLocalPath()
		if err != nil {
			logging.Logger("notion").Errorf("failed to download file: %s", err)
		}
		data.mu.Lock()
		defer data.mu.Unlock()

		if _, ok := data.details.TryString(fileMetaData.relationKey); ok {
			data.details.SetString(fileMetaData.relationKey, localPath)
		}
		if fileUrlsList, ok := data.details.TryStringList(fileMetaData.relationKey); ok {
			fileUrlsList[fileMetaData.idxInDetails] = localPath
			data.details.SetStringList(fileMetaData.relationKey, fileUrlsList)
		}
	})
	return false
}
