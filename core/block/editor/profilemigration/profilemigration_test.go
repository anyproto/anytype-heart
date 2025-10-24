package profilemigration

import (
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

var (
	stateOriginalRootId = "bafyreiew6ma6fkw3hyceivukjd2zelgwcsfdyr2mmbb34slf2575y6s63a"
	stateOriginal       = `{
 "sbType": "ProfilePage",
 "snapshot": {
  "data": {
   "blocks": [
    {
     "id": "bafyreiew6ma6fkw3hyceivukjd2zelgwcsfdyr2mmbb34slf2575y6s63a",
     "childrenIds": [
      "header",
      "65d7c13961fab283c2729ad5",
      "65d7c14061fab283c2729ad8",
      "65d7c14f61fab283c2729adb",
      "65d7c15a61fab283c2729add",
      "65d7c16f61fab283c2729ae0",
      "65d7c17a61fab283c2729ae2"
     ],
     "smartblock": {

     }
    },
    {
     "id": "header",
     "restrictions": {
      "edit": true,
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "childrenIds": [
      "title",
      "identity",
      "featuredRelations"
     ],
     "layout": {
      "style": "Header"
     }
    },
    {
     "id": "65d7c13961fab283c2729ad5",
     "text": {
      "text": "block1",
      "marks": {

      }
     }
    },
    {
     "id": "65d7c14061fab283c2729ad8",
     "text": {
      "text": "block2",
      "marks": {
       "marks": [
        {
         "range": {
          "to": 6
         },
         "type": "Bold"
        }
       ]
      }
     }
    },
    {
     "id": "65d7c14f61fab283c2729adb",
     "file": {
      "hash": "bafybeiarwjntnmjxc3wysufrqb6udvlm5s6exj6o5wzqjppvwpx4hvxx6m",
      "name": "image.png",
      "type": "Image",
      "mime": "image/png",
      "size": "42782",
      "targetObjectId": "bafyreibdezouiipznowdp7wi7hsvysakdavydjjzu7ny7hcstdnqhthpqi",
      "state": "Done",
      "style": "Embed"
     }
    },
    {
     "id": "65d7c15a61fab283c2729add",
     "relation": {
      "key": "lastModifiedBy"
     }
    },
    {
     "id": "65d7c16f61fab283c2729ae0",
     "relation": {
      "key": "tag"
     }
    },
    {
     "id": "65d7c17a61fab283c2729ae2",
     "dataview": {
      "views": [
       {
        "id": "63c59da2c54d2d6c985c2b6b",
        "name": "All",
        "sorts": [
         {
          "id": "63db97200e01101ff536a754",
          "RelationKey": "type"
         },
         {
          "id": "63db97200e01101ff536a755",
          "RelationKey": "lastModifiedDate",
          "type": "Desc"
         }
        ],
        "filters": [
         {
          "id": "63d24bef257d7bad62f58c7d",
          "RelationKey": "type",
          "condition": "NotIn",
          "value": [
             "bafyreihatd4tx3xsdgp7hr72amclva3x4x35webt5vuufxhxc5ikhp77hy",
             "bafyreih33gcr4t4fq3kgmsuq5dou42otzfrou6rn6eohkeidcd7aw7tme4",
             "bafyreiaparytg33b7ekqjfchni3axdrzrevbcw3uq7a5m5324slmtzq4ai",
             "bafyreiawofrlyhnlobrrjh2qkdfz5rzfa5nvz7salq62573v4ktqrcytey",
             "bafyreib2tcsejm5pvkj5b77ac7ezrt2kc5htlqlnlrqwa2vbh56byryvva",
             "bafyreifn7xdb6wzawml5ymf6i67raq2yxjgcgrfcyuxoiiiuglygdxyxni",
             "bafyreic6ncroguyctlvsepoaay67fve75xg3ongvc2yiz46r4tcxftzkva",
             "bafyreic7jleoay7jaeaonqdpaeqimivgzsdgsqnrklhgodh6to6aenze6q",
             "bafyreibyffj6c5surbofv3mjdpkwyozygmutqhn2pkz4g3gajr4bzqlv2e",
             "bafyreibz5s4pjua57dnukiglqzi63jskryfd4g7xppbzoqyjbevyb6lche",
             "bafyreieayrnuafcadst7x6emrsdc2wf4db2y72zbpikvciam2o7huc76iy"
            ],
          "format": "object"
         },
         {
          "id": "63d24bef257d7bad62f58c7e",
          "RelationKey": "name",
          "value": ""
         }
        ],
        "relations": [
         {
          "key": "name",
          "isVisible": true,
          "width": 500
         },
         {
          "key": "type",
          "isVisible": true,
          "width": 192
         },
         {
          "key": "lastModifiedDate",
          "width": 178
         },
         {
          "key": "description",
          "width": 192
         },
         {
          "key": "createdDate",
          "width": 192
         },
         {
          "key": "lastModifiedBy",
          "width": 192
         },
         {
          "key": "lastOpenedDate",
          "width": 192
         },
         {
          "key": "done"
         }
        ]
       },
       {
        "id": "e83e70ab-0601-4ab7-abd9-d4dc09b9e703",
        "type": "Gallery",
        "name": "Media",
        "sorts": [
         {
          "id": "63db97200e01101ff536a758",
          "RelationKey": "type"
         },
         {
          "id": "63db97200e01101ff536a759",
          "RelationKey": "name"
         }
        ],
        "filters": [
         {
          "id": "63d24bef257d7bad62f58c81",
          "RelationKey": "type",
          "condition": "In",
          "value": [
             "bafyreic7jleoay7jaeaonqdpaeqimivgzsdgsqnrklhgodh6to6aenze6q",
             "bafyreib2tcsejm5pvkj5b77ac7ezrt2kc5htlqlnlrqwa2vbh56byryvva",
             "bafyreibyffj6c5surbofv3mjdpkwyozygmutqhn2pkz4g3gajr4bzqlv2e",
             "bafyreieayrnuafcadst7x6emrsdc2wf4db2y72zbpikvciam2o7huc76iy"
            ]
         }
        ],
        "relations": [
         {
          "key": "name",
          "isVisible": true,
          "width": 500
         },
         {
          "key": "type",
          "width": 192
         },
         {
          "key": "createdDate",
          "width": 192
         },
         {
          "key": "description",
          "width": 192
         },
         {
          "key": "lastModifiedDate",
          "width": 192
         },
         {
          "key": "lastModifiedBy",
          "width": 192
         },
         {
          "key": "lastOpenedDate",
          "width": 192
         },
         {
          "key": "done"
         }
        ],
        "coverRelationKey": "iconImage",
        "groupRelationKey": "done"
       },
       {
        "id": "2589f7e2-aad5-43ed-8759-f0771a6a40c9",
        "name": "Pre-installed",
        "sorts": [
         {
          "id": "63db97200e01101ff536a754",
          "RelationKey": "type"
         },
         {
          "id": "63db97200e01101ff536a757",
          "RelationKey": "name"
         },
         {
          "id": "63db97200e01101ff536a755",
          "RelationKey": "lastModifiedDate",
          "type": "Desc"
         }
        ],
        "filters": [
         {
          "id": "63d24bef257d7bad62f58c7d",
          "RelationKey": "type",
          "condition": "NotIn",
          "value": [
             "bafyreihatd4tx3xsdgp7hr72amclva3x4x35webt5vuufxhxc5ikhp77hy",
             "bafyreih33gcr4t4fq3kgmsuq5dou42otzfrou6rn6eohkeidcd7aw7tme4",
             "bafyreiaparytg33b7ekqjfchni3axdrzrevbcw3uq7a5m5324slmtzq4ai",
             "bafyreiawofrlyhnlobrrjh2qkdfz5rzfa5nvz7salq62573v4ktqrcytey",
             "bafyreib2tcsejm5pvkj5b77ac7ezrt2kc5htlqlnlrqwa2vbh56byryvva",
             "bafyreifn7xdb6wzawml5ymf6i67raq2yxjgcgrfcyuxoiiiuglygdxyxni",
             "bafyreic6ncroguyctlvsepoaay67fve75xg3ongvc2yiz46r4tcxftzkva",
             "bafyreic7jleoay7jaeaonqdpaeqimivgzsdgsqnrklhgodh6to6aenze6q",
             "bafyreibyffj6c5surbofv3mjdpkwyozygmutqhn2pkz4g3gajr4bzqlv2e",
             "bafyreibz5s4pjua57dnukiglqzi63jskryfd4g7xppbzoqyjbevyb6lche",
             "bafyreieayrnuafcadst7x6emrsdc2wf4db2y72zbpikvciam2o7huc76iy"
            ]
         },
         {
          "id": "63d24bef257d7bad62f58c7e",
          "RelationKey": "tag",
          "condition": "In",
          "value": [
             "bafyreiawm24apxzqyhiz36a3aguwpywqyka7l7qpt2hxvasx3ivww7dawq"
            ],
          "format": "tag"
         }
        ],
        "relations": [
         {
          "key": "name",
          "isVisible": true,
          "width": 500
         },
         {
          "key": "type",
          "isVisible": true,
          "width": 192
         },
         {
          "key": "lastModifiedDate",
          "width": 178
         },
         {
          "key": "description",
          "width": 192
         },
         {
          "key": "createdDate",
          "width": 192
         },
         {
          "key": "lastModifiedBy",
          "width": 192
         },
         {
          "key": "lastOpenedDate",
          "width": 192
         },
         {
          "key": "done",
          "width": 192
         },
         {
          "key": "tag",
          "isVisible": true,
          "width": 192
         }
        ],
        "cardSize": "Medium",
        "groupRelationKey": "done"
       }
      ],
      "relationLinks": [
       {
        "key": "name",
        "format": "shorttext"
       },
       {
        "key": "type",
        "format": "object"
       },
       {
        "key": "lastModifiedDate",
        "format": "date"
       },
       {
        "key": "description"
       },
       {
        "key": "createdDate",
        "format": "date"
       },
       {
        "key": "lastModifiedBy",
        "format": "object"
       },
       {
        "key": "lastOpenedDate",
        "format": "date"
       },
       {
        "key": "done",
        "format": "checkbox"
       },
       {
        "key": "tag",
        "format": "tag"
       }
      ],
      "TargetObjectId": "bafyreibgdh7ka67etpdwwsckhnsez7k7qjqtxkxo2nkrx67w5otgsl4bsi"
     }
    },
    {
     "id": "title",
     "fields": {
       "_detailsKey": [
          "name",
          "done"
         ]
      },
     "restrictions": {
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "align": "AlignCenter",
     "text": {
      "style": "Title",
      "marks": {

      }
     }
    },
    {
     "id": "identity",
     "restrictions": {
      "edit": true,
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "relation": {
      "key": "profileOwnerIdentity"
     }
    },
    {
     "id": "featuredRelations",
     "restrictions": {
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "align": "AlignCenter",
     "featuredRelations": {

     }
    }
   ],
   "details": {
     "backlinks": [
       ],
     "createdDate": 1708638507,
     "creator": "_participant_bafyreifqavrshf5l2ovfexjzboibyvmweh6edx477mggj6lvpprp2tltgq_2msp3m2jb2vcd_A9YAB51C9MMWR5HLDzBR4A3BnkReMYHSR1wdmYEVfP4biKfr",
     "featuredRelations": [
        "backlinks"
       ],
     "iconEmoji": "",
     "iconImage": "bafyreifhebpqmp3kx52wyfhmje23j7du3kguziddj4ilri7m5jngogbs24",
     "iconOption": 5,
     "id": "bafyreiew6ma6fkw3hyceivukjd2zelgwcsfdyr2mmbb34slf2575y6s63a",
     "isHidden": true,
     "lastModifiedBy": "_participant_bafyreifqavrshf5l2ovfexjzboibyvmweh6edx477mggj6lvpprp2tltgq_2msp3m2jb2vcd_A9YAB51C9MMWR5HLDzBR4A3BnkReMYHSR1wdmYEVfP4biKfr",
     "lastModifiedDate": 1708694933,
     "layout": 1,
     "layoutAlign": 1,
     "links": [
        "bafyreibgdh7ka67etpdwwsckhnsez7k7qjqtxkxo2nkrx67w5otgsl4bsi"
       ],
     "name": "Roma",
     "profileOwnerIdentity": "_participant_bafyreifqavrshf5l2ovfexjzboibyvmweh6edx477mggj6lvpprp2tltgq_2msp3m2jb2vcd_A9YAB51C9MMWR5HLDzBR4A3BnkReMYHSR1wdmYEVfP4biKfr",
     "restrictions": [
        6,
        5,
        1,
        8
       ],
     "snippet": "block1\nblock2",
     "spaceId": "bafyreifqavrshf5l2ovfexjzboibyvmweh6edx477mggj6lvpprp2tltgq.2msp3m2jb2vcd",
     "tag": [
        "bafyreiayeo7sk3i56v5cqyenjgr73wuz2fn52mmcuirqqdefhjiy6qji24"
       ],
     "type": "bafyreifci7gcmxafbn6eikujw2elra4ynpxmdqs4fqzhql5n63gus7olpm"
    },
   "objectTypes": [
    "ot-profile"
   ],
   "relationLinks": [
    {
     "key": "backlinks",
     "format": "object"
    },
    {
     "key": "featuredRelations",
     "format": "object"
    },
    {
     "key": "id",
     "format": "object"
    },
    {
     "key": "spaceId",
     "format": "object"
    },
    {
     "key": "snippet"
    },
    {
     "key": "layout",
     "format": "number"
    },
    {
     "key": "layoutAlign",
     "format": "number"
    },
    {
     "key": "name",
     "format": "shorttext"
    },
    {
     "key": "description"
    },
    {
     "key": "iconEmoji",
     "format": "emoji"
    },
    {
     "key": "iconImage",
     "format": "file"
    },
    {
     "key": "type",
     "format": "object"
    },
    {
     "key": "coverId"
    },
    {
     "key": "coverScale",
     "format": "number"
    },
    {
     "key": "coverType",
     "format": "number"
    },
    {
     "key": "coverX",
     "format": "number"
    },
    {
     "key": "coverY",
     "format": "number"
    },
    {
     "key": "createdDate",
     "format": "date"
    },
    {
     "key": "creator",
     "format": "object"
    },
    {
     "key": "lastModifiedDate",
     "format": "date"
    },
    {
     "key": "lastModifiedBy",
     "format": "object"
    },
    {
     "key": "lastOpenedDate",
     "format": "date"
    },
    {
     "key": "isFavorite",
     "format": "checkbox"
    },
    {
     "key": "workspaceId",
     "format": "object"
    },
    {
     "key": "links",
     "format": "object"
    },
    {
     "key": "internalFlags",
     "format": "number"
    },
    {
     "key": "restrictions",
     "format": "number"
    },
    {
     "key": "iconOption",
     "format": "number"
    },
    {
     "key": "tag",
     "format": "tag"
    },
    {
     "key": "isHidden",
     "format": "checkbox"
    },
    {
     "key": "profileOwnerIdentity",
     "format": "shorttext"
    }
   ]
  }
 }
}`

	stateOriginalEmptyRootId = "bafyreif5rmdsvap7ieqnpvlm6vafizxw7igz37bmixtboopaiygpepyycm"
	stateOriginalEmpty       = `
{
 "sbType": "ProfilePage",
 "snapshot": {
  "data": {
   "blocks": [
    {
     "id": "bafyreif5rmdsvap7ieqnpvlm6vafizxw7igz37bmixtboopaiygpepyycm",
     "childrenIds": [
      "header",
      "65d8a97961fab22b735aa569",
      "65d8a98061fab22b735aa56b",
      "65d8a98061fab22b735aa56c"
     ],
     "smartblock": {

     }
    },
    {
     "id": "header",
     "restrictions": {
      "edit": true,
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "childrenIds": [
      "title",
      "identity",
      "featuredRelations"
     ],
     "layout": {
      "style": "Header"
     }
    },
    {
     "id": "65d8a97961fab22b735aa569",
     "text": {
      "marks": {

      }
     }
    },
    {
     "id": "65d8a98061fab22b735aa56b",
     "text": {
      "marks": {

      }
     }
    },
    {
     "id": "65d8a98061fab22b735aa56c",
     "text": {
      "marks": {

      }
     }
    },
    {
     "id": "title",
     "fields": {
       "_detailsKey": [
          "name",
          "done"
         ]
      },
     "restrictions": {
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "align": "AlignCenter",
     "text": {
      "style": "Title",
      "marks": {

      }
     }
    },
    {
     "id": "identity",
     "restrictions": {
      "edit": true,
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "relation": {
      "key": "profileOwnerIdentity"
     }
    },
    {
     "id": "featuredRelations",
     "restrictions": {
      "remove": true,
      "drag": true,
      "dropOn": true
     },
     "align": "AlignCenter",
     "featuredRelations": {

     }
    }
   ],
   "details": {
     "backlinks": [
       ],
     "createdDate": 1708697957,
     "creator": "_participant_bafyreicqzro4ea6ibejpswsq3ygdvl5yrswdhv3wcaeiya4zrhgxijorhi_25fl2y3kgedrp_ABG7CU4WFNtAtdYMbDoJArFguPJRHqq9Bag5un1iBoy2qqkn",
     "featuredRelations": [
        "backlinks"
       ],
     "iconOption": 15,
     "id": "bafyreif5rmdsvap7ieqnpvlm6vafizxw7igz37bmixtboopaiygpepyycm",
     "isHidden": true,
     "lastModifiedBy": "_participant_bafyreicqzro4ea6ibejpswsq3ygdvl5yrswdhv3wcaeiya4zrhgxijorhi_25fl2y3kgedrp_ABG7CU4WFNtAtdYMbDoJArFguPJRHqq9Bag5un1iBoy2qqkn",
     "lastModifiedDate": 1708698142,
     "layout": 1,
     "layoutAlign": 1,
     "links": [
       ],
     "name": "ooo",
     "profileOwnerIdentity": "_participant_bafyreicqzro4ea6ibejpswsq3ygdvl5yrswdhv3wcaeiya4zrhgxijorhi_25fl2y3kgedrp_ABG7CU4WFNtAtdYMbDoJArFguPJRHqq9Bag5un1iBoy2qqkn",
     "restrictions": [
        6,
        5,
        1,
        8
       ],
     "snippet": "",
     "spaceId": "bafyreicqzro4ea6ibejpswsq3ygdvl5yrswdhv3wcaeiya4zrhgxijorhi.25fl2y3kgedrp",
     "type": "bafyreiawi7jklmbhjqfux73ncl2t3ujytjahv2steorcqabydchkfze4iy"
    },
   "objectTypes": [
    "ot-profile"
   ],
   "relationLinks": [
    {
     "key": "backlinks",
     "format": "object"
    },
    {
     "key": "featuredRelations",
     "format": "object"
    },
    {
     "key": "id",
     "format": "object"
    },
    {
     "key": "spaceId",
     "format": "object"
    },
    {
     "key": "snippet"
    },
    {
     "key": "layout",
     "format": "number"
    },
    {
     "key": "layoutAlign",
     "format": "number"
    },
    {
     "key": "name",
     "format": "shorttext"
    },
    {
     "key": "description"
    },
    {
     "key": "iconEmoji",
     "format": "emoji"
    },
    {
     "key": "iconImage",
     "format": "file"
    },
    {
     "key": "type",
     "format": "object"
    },
    {
     "key": "coverId"
    },
    {
     "key": "coverScale",
     "format": "number"
    },
    {
     "key": "coverType",
     "format": "number"
    },
    {
     "key": "coverX",
     "format": "number"
    },
    {
     "key": "coverY",
     "format": "number"
    },
    {
     "key": "createdDate",
     "format": "date"
    },
    {
     "key": "creator",
     "format": "object"
    },
    {
     "key": "lastModifiedDate",
     "format": "date"
    },
    {
     "key": "lastModifiedBy",
     "format": "object"
    },
    {
     "key": "lastOpenedDate",
     "format": "date"
    },
    {
     "key": "isFavorite",
     "format": "checkbox"
    },
    {
     "key": "workspaceId",
     "format": "object"
    },
    {
     "key": "links",
     "format": "object"
    },
    {
     "key": "internalFlags",
     "format": "number"
    },
    {
     "key": "restrictions",
     "format": "number"
    },
    {
     "key": "iconOption",
     "format": "number"
    },
    {
     "key": "isHidden",
     "format": "checkbox"
    },
    {
     "key": "profileOwnerIdentity",
     "format": "shorttext"
    }
   ]
  }
 }
}`
)

func TestProfileMigrationExtractCustomState(t *testing.T) {
	sn := pb.SnapshotWithType{}
	err := jsonpb.UnmarshalString(stateOriginal, &sn)
	require.NoError(t, err)
	var identityBlockId = "identity"
	originalState, err := state.NewDocFromSnapshot(stateOriginalRootId, sn.Snapshot)
	require.NoError(t, err)
	originalStateCopy := originalState.Copy()
	extractedState, err := ExtractCustomState(originalState)
	require.NoError(t, err)
	for _, block := range originalState.Blocks() {
		// should contains only whitelisted blocks
		require.Containsf(t, []string{
			stateOriginalRootId,
			state.FeaturedRelationsID,
			state.TitleBlockID,
			state.HeaderLayoutID,
			state.FeaturedRelationsID,
			identityBlockId, // we do not remove this block
		}, block.Id, "state should not contain block %s", block.Id)
	}

	for _, block := range originalStateCopy.Blocks() {
		if block.Id == identityBlockId {
			require.Nilf(t, extractedState.Get(block.Id), "extractedState should not contain block %s", block.Id)
		} else {
			require.NotNilf(t, extractedState.Get(block.Id), "extractedState should contain block %s", block.Id)
		}
	}

	var whitelistedDetailKeys = []string{
		"iconEmoji",
		"name",
		"isHidden",
		"featuredRelations",
		"layout",
		"layoutAlign",
		"iconImage",
		"iconOption",
	}
	for k, v := range originalStateCopy.Details().Iterate() {
		if k == bundle.RelationKeyName {
			// should has suffix in the name
			v = domain.String(v.String() + " [migrated]")
		}
		if k == bundle.RelationKeyIsHidden {
			// extracted state should not be hidden
			v = domain.Bool(false)
		}
		require.Truef(t, v.Equal(extractedState.Details().Get(k)), "detail %s should be equal to original state", k)
	}

	for k, _ := range originalState.Details().Iterate() {
		require.Contains(t, whitelistedDetailKeys, k.String(), "old state should not contain %s", k)
	}
	require.Equal(t, bundle.TypeKeyPage, extractedState.ObjectTypeKey())

	_, err = ExtractCustomState(originalState.NewState())
	require.ErrorIsf(t, err, ErrNoCustomStateFound, "should return error on the second time call")
}

func TestProfileMigrationExtractCustomStateEmpty(t *testing.T) {
	sn := pb.SnapshotWithType{}
	err := jsonpb.UnmarshalString(stateOriginalEmpty, &sn)
	require.NoError(t, err)
	originalStateEmpty, err := state.NewDocFromSnapshot(stateOriginalEmptyRootId, sn.Snapshot)
	require.NoError(t, err)
	_, err = ExtractCustomState(originalStateEmpty)

	require.ErrorIsf(t, err, ErrNoCustomStateFound, "should return error because profile was not changed by user")
}
