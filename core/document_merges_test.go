package core

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func Test_mergeVersions(t *testing.T) {
	type args struct {
		ancestor *DocumentVersion
		version1 *DocumentVersion
		version2 *DocumentVersion
	}
	tests := []struct {
		name    string
		args    args
		want    *DocumentVersion
		wantErr bool
	}{
		/*{
			"no changes",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"one side block change",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1_changed"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"2 different blocks changed",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2_changed"},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1_changed"},
					{ID: "id2", Content: "2_changed"},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"add 2 blocks to the end of the same parent",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id5", Content: "5"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"},
					{ID: "id4", Content: "4"},
					{ID: "id5", Content: "5"}}},
			false,
		},
		{
			"add 2 blocks to the begin of the same parent",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id2", Content: "2"},
					{ID: "id1", Content: "1"},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"1 same block changed(version1 is predominant)",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed_1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed_2"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1_changed_1"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"1 same block changed(version2 is predominant)",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id4",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed_1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed_2"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id: "",
				Parents: []string{"ver_id3", "ver_id4"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1_changed_2"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"1 same block changed(version2 is local-only)",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed_1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1_changed_2"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1_changed_2"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"different blocks added by different peers",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id5", Content: "5"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"},
					{ID: "id4", Content: "4"},
					{ID: "id5", Content: "5"}}},
			false,
		},
		{
			"one same and one different block removed by different peers",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1"}}},
			false,
		},
		{
			"same block moved and changed from another peers",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id3", Content: "3"},
						{ID: "id2", Content: "2"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3_changed"}}},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1"},
					{ID: "id3", Content: "3_changed"},
					{ID: "id2", Content: "2"}}},
			false,
		},
		{
			"2 different blocks changed in the same children",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1"},
								{ID: "id1_2", Content: "1_2"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id2_1", Content: "2_1"},
							},
							},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1_changed"},
								{ID: "id1_2", Content: "1_2"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1"},
								{ID: "id1_2", Content: "1_2_changed"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id: "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1",
						ChildrenIds: []*DocumentBlock{
							{ID: "id1_1", Content: "1_1_changed"},
							{ID: "id1_2", Content: "1_2_changed"},
						},
					},
					{ID: "id2", Content: "2",
						ChildrenIds: []*DocumentBlock{
							{ID: "id2_1", Content: "2_1"},
						},
					},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"block moved from one parent to another and changed by another peer",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1"},
								{ID: "id1_2", Content: "1_2"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_2", Content: "1_2"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1"},
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id3", Content: "3"}}},
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1_changed"},
								{ID: "id1_2", Content: "1_2"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id: "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1",
						ChildrenIds: []*DocumentBlock{
							{ID: "id1_2", Content: "1_2"},
						},
					},
					{ID: "id2", Content: "2",
						ChildrenIds: []*DocumentBlock{
							{ID: "id1_1", Content: "1_1_changed"},
							{ID: "id2_1", Content: "2_1"},
						},
					},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"different blocks moved to different parents",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1"},
								{ID: "id1_2", Content: "1_2"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id3", Content: "3"}}},

				// this version move id1_1 into id2
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_2", Content: "1_2"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1"},
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id3", Content: "3"}}},

						// this version move id2_1 into id1 and change content of id1_1
				&DocumentVersion{
					Id: "ver_id3",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1_changed"},
								{ID: "id1_2", Content: "1_2"},
								{ID: "id2_1", Content: "2_1"},
							},
						},
						{ID: "id2", Content: "2",
							ChildrenIds: []*DocumentBlock{
							},
						},
						{ID: "id3", Content: "3"}}},
			},
			&DocumentVersion{
				Id: "",
				Parents: []string{"ver_id2", "ver_id3"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1",
						ChildrenIds: []*DocumentBlock{
							{ID: "id1_2", Content: "1_2"},
							{ID: "id2_1", Content: "2_1"},
						},
					},
					{ID: "id2", Content: "2",
						ChildrenIds: []*DocumentBlock{
							{ID: "id1_1", Content: "1_1_changed"},
						},
					},
					{ID: "id3", Content: "3"}}},
			false,
		},
		{
			"blocks opposite swapped ",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_1", Content: "1_1"},
								{ID: "id1_2", Content: "1_2"},
								{ID: "id1_3", Content: "1_3"},

							},
						},
					},
				},

				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_2", Content: "1_2"},
								{ID: "id1_1", Content: "1_1"},
								{ID: "id1_3", Content: "1_3"},
							},
						},
					},
				},

				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1",
							ChildrenIds: []*DocumentBlock{
								{ID: "id1_2", Content: "1_2"},
								{ID: "id1_3", Content: "1_3"},
								{ID: "id1_1", Content: "1_1"},
							},
						},
					},
				},
			},
			&DocumentVersion{
				Id: "",
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1",
						ChildrenIds: []*DocumentBlock{
							{ID: "id1_2", Content: "1_2"},
							{ID: "id1_3", Content: "1_3"},
							{ID: "id1_1", Content: "1_1"},
						},
					},
				},
			},
			false,
		},*/
		{
			"all blocks removed except one",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"},
					},
				},

				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"},
						{ID: "id5", Content: "5"},
						{ID: "id6_a", Content: "6_a"},
					},
				},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"},
						{ID: "id5", Content: "5"},
						{ID: "id6_b", Content: "6_b"},
					},
				},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id1", "ver_id2"},
				Blocks: []*DocumentBlock{
					{ID: "id1", Content: "1"},
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"},
					{ID: "id4", Content: "4"},
					{ID: "id5", Content: "5"},
					{ID: "id6_a", Content: "6_a"},
					{ID: "id6_b", Content: "6_b"},
				},
			},
			false,
		},
		{
			"block change and removed by other user",
			args{
				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"},
					},
				},

				&DocumentVersion{
					Id: "ver_id1",
					Blocks: []*DocumentBlock{
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"},
					},
				},
				&DocumentVersion{
					Id: "ver_id2",
					Blocks: []*DocumentBlock{
						{ID: "id1", Content: "1-"},
						{ID: "id2", Content: "2"},
						{ID: "id3", Content: "3"},
						{ID: "id4", Content: "4"},
					},
				},
			},
			&DocumentVersion{
				Id:      "",
				Parents: []string{"ver_id1", "ver_id2"},
				Blocks: []*DocumentBlock{
					{ID: "id2", Content: "2"},
					{ID: "id3", Content: "3"},
					{ID: "id4", Content: "4"},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name,
			func(t *testing.T) {
				got,
					err := mergeVersions(tt.args.ancestor,
					tt.args.version1,
					tt.args.version2)
				if (err != nil) != tt.wantErr {
					t.Errorf("mergeVersions() error = %v,wantErr %v",
						err,
						tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got,
					tt.want) {
					t.Errorf("mergeVersions() = %v,want %v",
						spew.Sdump(got),
						spew.Sdump(tt.want))
				}
			})
	}
}
