package object

import (
	"errors"

	"github.com/anyproto/anytype-heart/cmd/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedGenerateChallenge = errors.New("failed to generate a new challenge")
	ErrInvalidInput            = errors.New("invalid input")
	ErrorFailedAuthenticate    = errors.New("failed to authenticate user")
)

type Service interface {
	ListObjects() ([]Object, error)
	GetObject(id string) (Object, error)
	CreateObject(obj Object) (Object, error)
	UpdateObject(obj Object) (Object, error)
	ListTypes() ([]ObjectType, error)
	ListTemplates() ([]ObjectTemplate, error)
}

type ObjectService struct {
	mw          service.ClientCommandsServer
	AccountInfo *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer) *ObjectService {
	return &ObjectService{mw: mw}
}

func (s *ObjectService) ListObjects() ([]Object, error) {
	// TODO
	return nil, nil
}

func (s *ObjectService) GetObject(id string) (Object, error) {
	// TODO
	return Object{}, nil
}

func (s *ObjectService) CreateObject(obj Object) (Object, error) {
	// TODO
	return Object{}, nil
}

func (s *ObjectService) UpdateObject(obj Object) (Object, error) {
	// TODO
	return Object{}, nil
}

func (s *ObjectService) ListTypes() ([]ObjectType, error) {
	// TODO
	return nil, nil
}

func (s *ObjectService) ListTemplates() ([]ObjectTemplate, error) {
	// TODO
	return nil, nil
}

// GetDetails returns the details of the object
func (s *ObjectService) GetDetails(resp *pb.RpcObjectShowResponse) []Detail {
	return []Detail{
		{
			Id: "lastModifiedDate",
			Details: map[string]interface{}{
				"lastModifiedDate": resp.ObjectView.Details[0].Details.Fields["lastModifiedDate"].GetNumberValue(),
			},
		},
		{
			Id: "createdDate",
			Details: map[string]interface{}{
				"createdDate": resp.ObjectView.Details[0].Details.Fields["createdDate"].GetNumberValue(),
			},
		},
		{
			Id: "tags",
			Details: map[string]interface{}{
				"tags": s.getTags(resp),
			},
		},
	}
}

// getTags returns the list of tags from the object details
func (s *ObjectService) getTags(resp *pb.RpcObjectShowResponse) []Tag {
	tags := []Tag{}

	tagField, ok := resp.ObjectView.Details[0].Details.Fields["tag"]
	if !ok {
		return tags
	}

	for _, tagId := range tagField.GetListValue().Values {
		id := tagId.GetStringValue()
		for _, detail := range resp.ObjectView.Details {
			if detail.Id == id {
				tags = append(tags, Tag{
					Id:    id,
					Name:  detail.Details.Fields["name"].GetStringValue(),
					Color: detail.Details.Fields["relationOptionColor"].GetStringValue(),
				})
				break
			}
		}
	}
	return tags
}

// GetBlocks returns the blocks of the object
func (s *ObjectService) GetBlocks(resp *pb.RpcObjectShowResponse) []Block {
	blocks := []Block{}

	for _, block := range resp.ObjectView.Blocks {
		var text *Text
		var file *File

		switch content := block.Content.(type) {
		case *model.BlockContentOfText:
			text = &Text{
				Text:    content.Text.Text,
				Style:   model.BlockContentTextStyle_name[int32(content.Text.Style)],
				Checked: content.Text.Checked,
				Color:   content.Text.Color,
				Icon:    util.GetIconFromEmojiOrImage(s.AccountInfo, content.Text.IconEmoji, content.Text.IconImage),
			}
		case *model.BlockContentOfFile:
			file = &File{
				Hash:           content.File.Hash,
				Name:           content.File.Name,
				Type:           model.BlockContentFileType_name[int32(content.File.Type)],
				Mime:           content.File.Mime,
				Size:           content.File.Size(),
				AddedAt:        int(content.File.AddedAt),
				TargetObjectId: content.File.TargetObjectId,
				State:          model.BlockContentFileState_name[int32(content.File.State)],
				Style:          model.BlockContentFileStyle_name[int32(content.File.Style)],
			}
			// TODO: other content types?
		}

		blocks = append(blocks, Block{
			Id:              block.Id,
			ChildrenIds:     block.ChildrenIds,
			BackgroundColor: block.BackgroundColor,
			Align:           mapAlign(block.Align),
			VerticalAlign:   mapVerticalAlign(block.VerticalAlign),
			Text:            text,
			File:            file,
		})
	}

	return blocks
}

func mapAlign(align model.BlockAlign) string {
	switch align {
	case model.Block_AlignLeft:
		return "left"
	case model.Block_AlignCenter:
		return "center"
	case model.Block_AlignRight:
		return "right"
	case model.Block_AlignJustify:
		return "justify"
	default:
		return "unknown"
	}
}

func mapVerticalAlign(align model.BlockVerticalAlign) string {
	switch align {
	case model.Block_VerticalAlignTop:
		return "top"
	case model.Block_VerticalAlignMiddle:
		return "middle"
	case model.Block_VerticalAlignBottom:
		return "bottom"
	default:
		return "unknown"
	}
}
