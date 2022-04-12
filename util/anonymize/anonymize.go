package anonymize

import (
	"bytes"
	"math/rand"
	"unicode"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func State(s *state.State) (res *state.State) {
	// blocks
	res = s.Copy()
	s.Iterate(func(b simple.Block) (isContinue bool) {
		b.Model().Content = Block(b.Model()).Content
		return true
	})
	s.SetDetails(Struct(s.Details()))
	for i, er := range s.ExtraRelations() {
		s.ExtraRelations()[i] = Relation(er)
	}
	return
}

func Change(ch *pb.Change) (res *pb.Change) {
	resB, _ := ch.Marshal()
	res = &pb.Change{}
	res.Unmarshal(resB)
	if sh := res.Snapshot; sh != nil {
		sh.Data.Details = Struct(sh.Data.Details)
		for _, b := range sh.Data.Blocks {
			b.Content = Block(b).Content
		}
		for _, er := range sh.Data.ExtraRelations {
			if _, err := bundle.GetRelation(bundle.RelationKey(er.Key)); err != nil {
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
	resB, _ := chc.Marshal()
	res = &pb.ChangeContent{}
	res.Unmarshal(resB)
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
	}
	return
}

func Event(e *pb.EventMessage) (res *pb.EventMessage) {
	res = &pb.EventMessage{}
	resB, _ := e.Marshal()
	res.Unmarshal(resB)
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
	res = pbtypes.CopyStruct(in)
	if res != nil && res.Fields != nil {
		for k, v := range res.Fields {
			if k != "featuredRelations" {
				res.Fields[k] = StructValue(v)
			}
		}
	}
	return
}

func StructValue(in *types.Value) (res *types.Value) {
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
	if _, err := bundle.GetRelation(bundle.RelationKey(res.Key)); err != nil {
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
	buf := bytes.NewBuffer(make([]byte, 0, utf8.RuneCountInString(s)))

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
