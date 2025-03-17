package anonymize

import (
	"bytes"
	"math/rand"
	"unicode"

	types "google.golang.org/protobuf/types/known/structpb"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

func Change(ch *pb.Change) (res *pb.Change) {
	resB, _ := ch.MarshalVT()
	res = &pb.Change{}
	res.UnmarshalVT(resB)
	if sh := res.Snapshot; sh != nil {
		sh.Data.Details = Struct(sh.Data.Details)
		for _, b := range sh.Data.Blocks {
			b.Content = Block(b).Content
		}
		for _, er := range sh.Data.ExtraRelations {
			if _, err := bundle.GetRelation(domain.RelationKey(er.Key)); err != nil {
				er.Name = Text(er.Name)
				er.Description = Text(er.Description)
			}
		}
		for _, fk := range sh.FileKeys {
			if fk.Keys != nil {
				for k, v := range fk.Keys {
					fk.Keys[k] = Text(v)
				}
			}
		}
	}
	for i := range res.Content {
		res.Content[i] = ChangeContent(res.Content[i])
	}
	return
}

func ChangeContent(chc *pb.ChangeContent) (res *pb.ChangeContent) {
	resB, _ := chc.MarshalVT()
	res = &pb.ChangeContent{}
	res.UnmarshalVT(resB)

	switch v := res.Value.(type) {
	case *pb.ChangeContentValueOfBlockCreate:
		for i, b := range v.BlockCreate.Blocks {
			v.BlockCreate.Blocks[i] = Block(b)
		}
	case *pb.ChangeContentValueOfBlockUpdate:
		for i, e := range v.BlockUpdate.Events {
			v.BlockUpdate.Events[i] = Event(e)
		}
	case *pb.ChangeContentValueOfBlockRemove:
	case *pb.ChangeContentValueOfBlockMove:
	case *pb.ChangeContentValueOfBlockDuplicate:
	case *pb.ChangeContentValueOfDetailsSet:
		v.DetailsSet.Value = StructValue(v.DetailsSet.Value)
	case *pb.ChangeContentValueOfDetailsUnset:
	case *pb.ChangeContentValueOfRelationAdd:
	case *pb.ChangeContentValueOfRelationRemove:
	case *pb.ChangeContentValueOfObjectTypeAdd:
	case *pb.ChangeContentValueOfObjectTypeRemove:
	case *pb.ChangeContentValueOfStoreKeySet:
		v.StoreKeySet.Value = StructValue(v.StoreKeySet.Value)
	case *pb.ChangeContentValueOfStoreKeyUnset:
	}
	return
}

func StringListValue(list []string) []string {
	anonymizeList := make([]string, 0, len(list))
	for _, s := range list {
		anonymizeList = append(anonymizeList, Text(s))
	}
	return anonymizeList
}

func Events(e []*pb.EventMessage) (res []*pb.EventMessage) {
	res = make([]*pb.EventMessage, len(e))
	for i, v := range e {
		res[i] = Event(v)

	}
	return
}

func Event(e *pb.EventMessage) (res *pb.EventMessage) {
	res = &pb.EventMessage{}
	resB, _ := e.MarshalVT()
	res.UnmarshalVT(resB)
	switch v := res.Value.(type) {
	case *pb.EventMessageValueOfObjectDetailsSet:
		v.ObjectDetailsSet.Details = Struct(v.ObjectDetailsSet.Details)
	case *pb.EventMessageValueOfObjectDetailsAmend:
		for i, d := range v.ObjectDetailsAmend.Details {
			v.ObjectDetailsAmend.Details[i].Value = StructValue(d.Value)
		}
	case *pb.EventMessageValueOfBlockAdd:
		if v.BlockAdd.Blocks != nil {
			for i, b := range v.BlockAdd.Blocks {
				v.BlockAdd.Blocks[i] = Block(b)
			}
		}
	case *pb.EventMessageValueOfBlockSetText:
		if v.BlockSetText.Text != nil {
			v.BlockSetText.Text.Value = Text(v.BlockSetText.Text.Value)
			if v.BlockSetText.Marks != nil && v.BlockSetText.Marks.Value != nil {
				for i, mark := range v.BlockSetText.Marks.Value.Marks {
					v.BlockSetText.Marks.Value.Marks[i].Param = Text(mark.Param)
				}
			}
		}
	case *pb.EventMessageValueOfBlockSetFile:
		if v.BlockSetFile.Name != nil {
			v.BlockSetFile.Name.Value = Text(v.BlockSetFile.Name.Value)
		}
	case *pb.EventMessageValueOfBlockSetLink:
		if v.BlockSetLink.Fields != nil {
			v.BlockSetLink.Fields.Value = Struct(v.BlockSetLink.Fields.Value)
		}
	case *pb.EventMessageValueOfBlockSetBookmark:
		if v.BlockSetBookmark.Title != nil {
			v.BlockSetBookmark.Title.Value = Text(v.BlockSetBookmark.Title.Value)
		}
		if v.BlockSetBookmark.Url != nil {
			v.BlockSetBookmark.Url.Value = Text(v.BlockSetBookmark.Url.Value)
		}
		if v.BlockSetBookmark.Description != nil {
			v.BlockSetBookmark.Description.Value = Text(v.BlockSetBookmark.Description.Value)
		}
	}
	return
}

func Block(b *model.Block) (res *model.Block) {
	res = pbtypes.CopyBlock(b)
	switch r := res.Content.(type) {
	case *model.BlockContentOfText:
		r.Text.Text = Text(r.Text.Text)
		if r.Text.Marks != nil {
			for _, m := range r.Text.Marks.Marks {
				m.Param = Text(m.Param)
			}
		}
	case *model.BlockContentOfLink:
		r.Link.TargetBlockId = Text(r.Link.TargetBlockId)
		r.Link.Fields = Struct(r.Link.Fields)
	case *model.BlockContentOfBookmark:
		r.Bookmark.Title = Text(r.Bookmark.Title)
		r.Bookmark.Url = Text(r.Bookmark.Url)
		r.Bookmark.Description = Text(r.Bookmark.Description)
	case *model.BlockContentOfFile:
		r.File.Name = Text(r.File.Name)
	}
	return
}

func Struct(in *types.Struct) (res *types.Struct) {
	res = pbtypes.CopyStruct(in, false)
	if res != nil && res.Fields != nil {
		for k, v := range res.Fields {
			if k != "featuredRelations" {
				res.Fields[k] = StructValue(v)
			}
		}
	}
	return
}

func Details(d *domain.Details) *domain.Details {
	str := d.ToProto()
	return domain.NewDetailsFromProto(Struct(str))
}

func StructValue(in *types.Value) (res *types.Value) {
	if in == nil {
		return
	}
	res = pbtypes.CopyVal(in)
	switch val := res.Kind.(type) {
	case *types.Value_StringValue:
		val.StringValue = Text(val.StringValue)
	case *types.Value_NumberValue:
		val.NumberValue = float64(rand.Intn(1000))
	case *types.Value_ListValue:
		for i, v2 := range val.ListValue.Values {
			val.ListValue.Values[i] = StructValue(v2)
		}
	case *types.Value_StructValue:
		if val.StructValue.Fields != nil {
			for k, v2 := range val.StructValue.Fields {
				val.StructValue.Fields[k] = StructValue(v2)
			}
		}
	}
	return
}

func Relation(r *model.Relation) (res *model.Relation) {
	res = pbtypes.CopyRelation(r)
	if _, err := bundle.GetRelation(domain.RelationKey(res.Key)); err != nil {
		res.Name = Text(res.Name)
		res.Description = Text(res.Description)
		for _, so := range res.SelectDict {
			so.Text = Text(so.Text)
		}
	}
	return
}

func Text(s string) (res string) {
	const digits = "1234567890"
	const letters = "abcdefghijklmnopqrstuvwxyz"
	if len(s) == 0 {
		return ""
	}
	buf := bytes.NewBuffer(make([]byte, 0, text.UTF16RuneCountString(s)))

	for _, r := range []rune(s) {
		switch {
		case unicode.IsDigit(r):
			buf.WriteRune(rune(digits[rand.Intn(len(digits))]))
		case unicode.IsLetter(r) && unicode.IsUpper(r):
			buf.WriteRune(unicode.ToUpper(rune(letters[rand.Intn(len(letters))])))
		case unicode.IsPunct(r) || unicode.IsSpace(r):
			buf.WriteRune(r)
		default:
			buf.WriteRune(rune(letters[rand.Intn(len(letters))]))
		}
	}

	return buf.String()
}
