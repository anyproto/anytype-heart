package details

import (
	"testing"
	"unsafe"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestPadding(t *testing.T) {
	v := Value{}
	t.Log("sizeof", unsafe.Sizeof(v))
	t.Log("kind", unsafe.Offsetof(v.kind), unsafe.Alignof(v.kind))
	t.Log("bool", unsafe.Offsetof(v.bool), unsafe.Alignof(v.bool))
	t.Log("string", unsafe.Offsetof(v.string), unsafe.Alignof(v.string))
	t.Log("float", unsafe.Offsetof(v.float), unsafe.Alignof(v.float))
	t.Log("strings", unsafe.Offsetof(v.strings), unsafe.Alignof(v.strings))
	t.Log("floats", unsafe.Offsetof(v.floats), unsafe.Alignof(v.floats))
}

func BenchmarkOwnValueType(b *testing.B) {
	for i := 0; i < b.N; i++ {
		d := &Details{
			data: map[domain.RelationKey]Value{},
		}
		d.data[bundle.RelationKeyId] = Value{string: "123124423849yufisduafhjsdklfhsdfjsdklf"}
		d.data[bundle.RelationKeyIsDeleted] = Value{bool: true}
		d.data[bundle.RelationKeyCreatedDate] = Value{float: 123124423849.0}
		d.data[bundle.RelationKeyFeaturedRelations] = Value{strings: []string{"123124423849yufisduafhjsdklfhsdfjsdklf", "123124423849yufisduafhjsdklfhsdfjsdklf", "123124423849yufisduafhjsdklfhsdfjsdklf"}}
		d.data[bundle.RelationKeyIconImage] = Value{floats: []float64{123124423849.0, 123124423849.0, 123124423849.0}}
	}
}

func BenchmarkProtobufValueType(b *testing.B) {
	for i := 0; i < b.N; i++ {
		d := &ODetails{
			data: map[domain.RelationKey]*types.Value{},
		}
		d.data[bundle.RelationKeyId] = &types.Value{Kind: &types.Value_StringValue{StringValue: "123124423849yufisduafhjsdklfhsdfjsdklf"}}
		d.data[bundle.RelationKeyIsDeleted] = &types.Value{Kind: &types.Value_BoolValue{BoolValue: true}}
		d.data[bundle.RelationKeyCreatedDate] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: 123124423849.0}}
		d.data[bundle.RelationKeyFeaturedRelations] = &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{
			{Kind: &types.Value_StringValue{StringValue: "123124423849yufisduafhjsdklfhsdfjsdklf"}},
			{Kind: &types.Value_StringValue{StringValue: "123124423849yufisduafhjsdklfhsdfjsdklf"}},
			{Kind: &types.Value_StringValue{StringValue: "123124423849yufisduafhjsdklfhsdfjsdklf"}},
		}}}}
		d.data[bundle.RelationKeyIconImage] = &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{
			{Kind: &types.Value_NumberValue{NumberValue: 123124423849.0}},
			{Kind: &types.Value_NumberValue{NumberValue: 123124423849.0}},
			{Kind: &types.Value_NumberValue{NumberValue: 123124423849.0}},
		}}}}
	}
}

func BenchmarkInterfaceValueType(b *testing.B) {
	for i := 0; i < b.N; i++ {
		d := &IDetails{
			data: map[domain.RelationKey]any{},
		}
		d.data[bundle.RelationKeyId] = "123124423849yufisduafhjsdklfhsdfjsdklf"
		d.data[bundle.RelationKeyIsDeleted] = true
		d.data[bundle.RelationKeyCreatedDate] = 123124423849.0
		d.data[bundle.RelationKeyFeaturedRelations] = []string{"123124423849yufisduafhjsdklfhsdfjsdklf", "123124423849yufisduafhjsdklfhsdfjsdklf", "123124423849yufisduafhjsdklfhsdfjsdklf"}
		d.data[bundle.RelationKeyIconImage] = []float64{123124423849.0, 123124423849.0, 123124423849.0}
	}
}
