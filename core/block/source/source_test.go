package source

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var text = "-- Еh bien, mon prince. Gênes et Lucques ne sont plus que des apanages,\n  des поместья, de la famille Buonaparte.  Non, je  vous préviens, que si vous\n  ne  me dites pas, que nous avons la guerre, si vous vous permettez encore de\n  pallier  toutes les infamies, toutes les  atrocités  de cet  Antichrist  (ma\n  parole, j'y  crois) -- je  ne  vous  connais plus, vous n'êtes plus mon ami,\n  vous n'êtes  plus  мой  верный  раб,  comme  vous  dites.  [1]  Ну,\n  здравствуйте, здравствуйте.  Je vois  que  je  vous fais  peur, [2]\n  садитесь и рассказывайте.\n       Так говорила в июле 1805 года известная Анна Павловна Шерер, фрейлина и\n  приближенная  императрицы  Марии  Феодоровны,  встречая важного и  чиновного\n  князя  Василия,  первого  приехавшего  на  ее вечер. Анна  Павловна  кашляла\n  несколько  дней, у  нее был грипп, как она говорила (грипп  был тогда  новое\n  слово, употреблявшееся только  редкими).  В записочках, разосланных  утром с\n  красным лакеем, было написано без различия во всех:\n       \"Si vous n'avez rien de mieux à faire, M. le comte (или mon prince), et\n  si la perspective de passer la soirée chez une pauvre malade ne vous effraye\n  pas  trop,  je serai charmée de vous  voir chez moi  entre 7 et  10  heures.\n  Annette Scherer\".[3]"

func randStr(txt string) string {
	runes := []rune(txt)
	rand.Shuffle(len(runes), func(i, j int) {
		runes[i], runes[j] = runes[j], runes[i]
	})
	return string(runes)
}

func Test_snapshotChance(t *testing.T) {
	if os.Getenv("ANYTYPE_TEST_SNAPSHOT_CHANCE") == "" {
		t.Skip()
		return
	}
	for i := 0; i <= 500; i++ {
		for s := 0; s <= 10000; s++ {
			if snapshotChance(s) {
				fmt.Println(s)
				break
			}
		}
	}
	fmt.Println()
	// here is an example of distribution histogram
	// https://docs.google.com/spreadsheets/d/1xgH7fUxno5Rm-0VEaSD4LsTHeGeUXQFmHsOm29M6paI
}

func Test_snapshotChance2(t *testing.T) {
	if os.Getenv("ANYTYPE_TEST_SNAPSHOT_CHANCE") == "" {
		t.Skip()
		return
	}
	for s := 0; s <= 10000; s++ {
		total := 0
		for i := 0; i <= 50000; i++ {
			if snapshotChance(s) {
				total++
			}
		}
		fmt.Printf("%d\t%.5f\n", s, float64(total)/50000)
	}

	// here is an example of distribution histogram
	// https://docs.google.com/spreadsheets/d/1xgH7fUxno5Rm-0VEaSD4LsTHeGeUXQFmHsOm29M6paI
}

func benchmarkMarshallChange(c *pb.Change, b *testing.B) {
	for n := 0; n < b.N; n++ {
		res, dt, _ := MarshallChange(c)
		if n == 3 {
			fmt.Println(dt)
			fmt.Println(base64.StdEncoding.EncodeToString(res))
		}
	}
}

func BenchmarkMarshallChange_CreateBigBlock(b *testing.B) {
	c := &pb.Change{
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
	benchmarkMarshallChange(c, b)
}

func BenchmarkMarshallChange_SetBigDetail(b *testing.B) {
	c := &pb.Change{
		Content: []*pb.ChangeContent{{
			&pb.ChangeContentValueOfDetailsSet{DetailsSet: &pb.ChangeDetailsSet{
				Key:   bundle.RelationKeyDescription.String(),
				Value: pbtypes.String(randStr(text)),
			}},
		}},
	}
	benchmarkMarshallChange(c, b)
}

func BenchmarkMarshallChange_SetSmallDetail(b *testing.B) {
	c := &pb.Change{
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
	benchmarkMarshallChange(c, b)
}

func BenchmarkMarshallChange_BigSnapshot(b *testing.B) {
	c := &pb.Change{
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
	benchmarkMarshallChange(c, b)
}

func BenchmarkMarshallChange_BlockUpdate(b *testing.B) {
	c := &pb.Change{
		Content: []*pb.ChangeContent{{
			Value: &pb.ChangeContentValueOfBlockUpdate{BlockUpdate: &pb.ChangeBlockUpdate{
				[]*pb.EventMessage{{
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
	benchmarkMarshallChange(c, b)
}
