package sourceimpl

import (
	"math/rand"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var text = "-- Еh bien, mon prince. Gênes et Lucques ne sont plus que des apanages,\n  des поместья, de la famille Buonaparte.  Non, je  vous préviens, que si vous\n  ne  me dites pas, que nous avons la guerre, si vous vous permettez encore de\n  pallier  toutes les infamies, toutes les  atrocités  de cet  Antichrist  (ma\n  parole, j'y  crois) -- je  ne  vous  connais plus, vous n'êtes plus mon ami,\n  vous n'êtes  plus  мой  верный  раб,  comme  vous  dites.  [1]  Ну,\n  здравствуйте, здравствуйте.  Je vois  que  je  vous fais  peur, [2]\n  садитесь и рассказывайте.\n       Так говорила в июле 1805 года известная Анна Павловна Шерер, фрейлина и\n  приближенная  императрицы  Марии  Феодоровны,  встречая важного и  чиновного\n  князя  Василия,  первого  приехавшего  на  ее вечер. Анна  Павловна  кашляла\n  несколько  дней, у  нее был грипп, как она говорила (грипп  был тогда  новое\n  слово, употреблявшееся только  редкими).  В записочках, разосланных  утром с\n  красным лакеем, было написано без различия во всех:\n       \"Si vous n'avez rien de mieux à faire, M. le comte (или mon prince), et\n  si la perspective de passer la soirée chez une pauvre malade ne vous effraye\n  pas  trop,  je serai charmée de vous  voir chez moi  entre 7 et  10  heures.\n  Annette Scherer\".[3]"

func TestMarshallChange(t *testing.T) {
	t.Run("marshall small change", func(t *testing.T) {
		// given
		c := changeWithSmallTextUpdate()

		// when
		data, dt, err := MarshalChange(c)

		// then
		assert.NoError(t, err)
		assert.NotZero(t, len(data))
		assert.Empty(t, dt)
	})

	t.Run("marshall bigger change", func(t *testing.T) {
		// given
		c := changeWithSetBigDetail(snappyLowerLimit)

		// when
		data, dt, err := MarshalChange(c)

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, data)
		assert.Equal(t, dataTypeSnappy, dt)
	})
}

func TestNewUnmarshalTreeChange(t *testing.T) {
	ch1, _, _ := MarshalChange(changeWithBigSnapshot())
	ch2, _, _ := MarshalChange(changeWithBigSnapshot())
	unmarshalF := NewUnmarshalTreeChange()
	res1, err := unmarshalF(&objecttree.Change{DataType: dataTypeSnappy}, ch1)
	require.NoError(t, err)
	assert.NotNil(t, res1.(*pb.Change).Snapshot)
	res2, err := unmarshalF(&objecttree.Change{DataType: dataTypeSnappy}, ch2)
	require.NoError(t, err)
	assert.Nil(t, res2.(*pb.Change).Snapshot)
}

func TestUnmarshallChange(t *testing.T) {
	invalidDataType := "invalid"

	t.Run("unmarshall small change", func(t *testing.T) {
		// given
		c := changeWithSmallTextUpdate()
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)
		require.NotEmpty(t, data)
		require.Empty(t, dt)

		// when
		res, err := UnmarshalChange(&objecttree.Change{DataType: dt}, data)

		// then
		assert.NoError(t, err)
		assert.Equal(t, c, res)
	})

	t.Run("unmarshall bigger change", func(t *testing.T) {
		// given
		c := changeWithSetBigDetail(snappyLowerLimit)
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)
		require.NotEmpty(t, data)
		require.Equal(t, dataTypeSnappy, dt)

		// when
		res, err := UnmarshalChange(&objecttree.Change{DataType: dt}, data)

		// then
		assert.NoError(t, err)
		assert.Equal(t, c, res)
	})

	t.Run("unmarshall plain change with invalid data type", func(t *testing.T) {
		// given
		c := changeWithSmallTextUpdate()
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)
		require.NotEmpty(t, data)
		require.Empty(t, dt)

		// when
		res, err := UnmarshalChange(&objecttree.Change{DataType: invalidDataType}, data)

		// then
		assert.NoError(t, err)
		assert.Equal(t, c, res)
	})

	t.Run("unmarshall plain change with encoded data type", func(t *testing.T) {
		// given
		c := changeWithSmallTextUpdate()
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)
		require.NotEmpty(t, data)
		require.Empty(t, dt)

		// when
		res, err := UnmarshalChange(&objecttree.Change{DataType: dataTypeSnappy}, data)

		// then
		assert.NoError(t, err)
		assert.Equal(t, c, res)
	})

	t.Run("unmarshall bigger change with empty data type", func(t *testing.T) {
		// given
		c := changeWithSetBigDetail(snappyLowerLimit)
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)
		require.NotEmpty(t, data)
		require.Equal(t, dataTypeSnappy, dt)

		// when
		res, err := UnmarshalChange(&objecttree.Change{DataType: ""}, data)

		// then
		assert.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("unmarshall encoded change with invalid data type", func(t *testing.T) {
		// given
		c := changeWithSetBigDetail(snappyLowerLimit)
		data, dt, err := MarshalChange(c)
		require.NoError(t, err)
		require.NotEmpty(t, data)
		require.Equal(t, dataTypeSnappy, dt)

		// when
		res, err := UnmarshalChange(&objecttree.Change{DataType: invalidDataType}, data)

		// then
		assert.Error(t, err)
		assert.Nil(t, res)
	})
}

func BenchmarkMarshallChange_CreateBigBlock(b *testing.B) {
	benchmarkMarshallChange(changeWithCreateBigBlock(), b)
}

func BenchmarkMarshallChange_SetBigDetail(b *testing.B) {
	benchmarkMarshallChange(changeWithSetBigDetail(len(text)), b)
}

func BenchmarkMarshallChange_SetSmallDetail(b *testing.B) {
	benchmarkMarshallChange(changeWithSetSmallDetail(), b)
}

func BenchmarkMarshallChange_BigSnapshot(b *testing.B) {
	benchmarkMarshallChange(changeWithBigSnapshot(), b)
}

func BenchmarkMarshallChange_BlockUpdate(b *testing.B) {
	benchmarkMarshallChange(changeWithBlockUpdate(), b)
}

func BenchmarkMarshallChange_SmallTextUpdate(b *testing.B) {
	benchmarkMarshallChange(changeWithSmallTextUpdate(), b)
}

func BenchmarkUnmarshallChange_CreateBigBlock(b *testing.B) {
	data, dt, _ := MarshalChange(changeWithCreateBigBlock())
	benchmarkUnmarshallChange(&objecttree.Change{DataType: dt}, data, b)
}

func BenchmarkUnmarshallChange_SetBigDetail(b *testing.B) {
	data, dt, _ := MarshalChange(changeWithSetBigDetail(len(text)))
	benchmarkUnmarshallChange(&objecttree.Change{DataType: dt}, data, b)
}

func BenchmarkUnmarshallChange_SetSmallDetail(b *testing.B) {
	data, dt, _ := MarshalChange(changeWithSetSmallDetail())
	benchmarkUnmarshallChange(&objecttree.Change{DataType: dt}, data, b)
}

func BenchmarkUnmarshallChange_BigSnapshot(b *testing.B) {
	data, dt, _ := MarshalChange(changeWithBigSnapshot())
	benchmarkUnmarshallChange(&objecttree.Change{DataType: dt}, data, b)
}

func BenchmarkUnmarshallChange_BlockUpdate(b *testing.B) {
	data, dt, _ := MarshalChange(changeWithBlockUpdate())
	benchmarkUnmarshallChange(&objecttree.Change{DataType: dt}, data, b)
}

func BenchmarkUnmarshallChange_SmallTextUpdate(b *testing.B) {
	data, dt, _ := MarshalChange(changeWithSmallTextUpdate())
	benchmarkUnmarshallChange(&objecttree.Change{DataType: dt}, data, b)
}

func randStr(txt string) string {
	runes := []rune(txt)
	rand.Shuffle(len(runes), func(i, j int) {
		runes[i], runes[j] = runes[j], runes[i]
	})
	return string(runes)
}

func benchmarkMarshallChange(c *pb.Change, b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, _, _ = MarshalChange(c)
	}
}

func benchmarkUnmarshallChange(c *objecttree.Change, data []byte, b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = UnmarshalChange(c, data)
	}
}

func changeWithCreateBigBlock() *pb.Change {
	return &pb.Change{
		Content: []*pb.ChangeContent{{
			&pb.ChangeContentValueOfBlockCreate{BlockCreate: &pb.ChangeBlockCreate{
				TargetId: "bafyreibl7ni6vvsxorr4xd62p5hjntq2243wda6hkqnppcq7splxkguf12d",
				Position: 3,
				Blocks: []*model.Block{{
					Id: "bafyreig2hrroh5ik46cpnk2c5wbswpwpew7jguisxtnqr1dfydro1qxsf1",
					Content: &model.BlockContentOfText{Text: &model.BlockContentText{
						Text: randStr(text),
					}},
				}},
			}},
		}},
	}
}

func changeWithSetBigDetail(length int) *pb.Change {
	return &pb.Change{
		Content: []*pb.ChangeContent{{
			&pb.ChangeContentValueOfDetailsSet{DetailsSet: &pb.ChangeDetailsSet{
				Key:   bundle.RelationKeyDescription.String(),
				Value: pbtypes.String(randStr(text[:length])),
			}},
		}},
	}
}

func changeWithSetSmallDetail() *pb.Change {
	return &pb.Change{
		Content: []*pb.ChangeContent{{
			&pb.ChangeContentValueOfDetailsSet{DetailsSet: &pb.ChangeDetailsSet{
				Key: bundle.RelationKeyDescription.String(),
				Value: pbtypes.Struct(&types.Struct{
					Fields: map[string]*types.Value{
						"id":         pbtypes.String(randStr("bafyreign6gtm2fr4u6vo566eeht5wdiuurlek35twjjl4zrjqruhqrpm4s")),
						"isFavorite": pbtypes.Bool(true),
						"age":        pbtypes.Int64(rand.Int63()),
					},
				}),
			}},
		}},
	}
}

func changeWithBigSnapshot() *pb.Change {
	return &pb.Change{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{{
					Id: "bafyreig2hrroh5ik46cpnk2c5wbswpwpew7jguisxtnqr1dfydro1qxsf1",
					Content: &model.BlockContentOfText{Text: &model.BlockContentText{
						Text: randStr(text + text + text + text),
					}},
				}},
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"id":         pbtypes.String(randStr("bafyreign6gtm2fr4u6vkirillisthebestlek35twjjl4zrjqruhqrpm4s")),
						"isFavorite": pbtypes.Bool(false),
						"age":        pbtypes.Int64(rand.Int63()),
						"snippet":    pbtypes.String(randStr(text)),
					},
				},
				ObjectTypes: []string{},
				RelationLinks: []*model.RelationLink{
					{Key: "id", Format: model.RelationFormat(1)},
					{Key: "description", Format: model.RelationFormat(0)},
					{Key: "652802389239243", Format: model.RelationFormat(2)},
					{Key: "emoji", Format: model.RelationFormat(10)},
					{Key: "lastModifiedDate", Format: model.RelationFormat(4)},
					{Key: "assignee", Format: model.RelationFormat(100)},
				},
			},
		},
	}
}

func changeWithBlockUpdate() *pb.Change {
	return &pb.Change{
		Content: []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{BlockUpdate: &pb.ChangeBlockUpdate{
				Events: []*pb.EventMessage{{
					Value: &pb.EventMessageValueOfBlockSetText{
						BlockSetText: &pb.EventBlockSetText{
							Id:        "root",
							Text:      &pb.EventBlockSetTextText{Value: randStr(text)},
							Style:     &pb.EventBlockSetTextStyle{Value: model.BlockContentTextStyle(2)},
							Color:     &pb.EventBlockSetTextColor{Value: "purple"},
							IconEmoji: &pb.EventBlockSetTextIconEmoji{Value: "\U0001FAF6"},
						},
					},
				}},
			}},
		}},
	}
}

func changeWithSmallTextUpdate() *pb.Change {
	return &pb.Change{Content: []*pb.ChangeContent{{
		Value: &pb.ChangeContentValueOfBlockUpdate{BlockUpdate: &pb.ChangeBlockUpdate{
			Events: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfBlockSetText{BlockSetText: &pb.EventBlockSetText{
					Id:   "64df8194ccba0bb8cc51b7da",
					Text: &pb.EventBlockSetTextText{Value: "change"},
				}}}},
		}}},
	}}
}
