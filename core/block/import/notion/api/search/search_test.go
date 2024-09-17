package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/database"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/files/mock_files"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

func Test_GetDatabaseSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"database","id":"072a11cb-684f-4f2b-9490-79592700c67e","cover":{"type":"external","external":{"url":"https://www.notion.so/images/page-cover/webb1.jpg"}},"icon":{"type":"emoji","emoji":"👜"},"created_time":"2022-10-25T11:44:00.000Z","created_by":{"object":"user","id":"60faafc6-0c5c-4479-a3f7-67d77cd8a56d"},"last_edited_by":{"object":"user","id":"60faafc6-0c5c-4479-a3f7-67d77cd8a56d"},"last_edited_time":"2022-10-31T10:16:00.000Z","title":[{"type":"text","text":{"content":"fsdfsdf","link":null},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"fsdfsdf","href":null}],"description":[{"type":"text","text":{"content":"lkjlkjlkjklj","link":null},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":true,"code":false,"color":"default"},"plain_text":"lkjlkjlkjklj","href":null},{"type":"text","text":{"content":" lkhlkjl;lk’ ","link":null},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":" lkhlkjl;lk’ ","href":null},{"type":"text","text":{"content":"lkjkn ;oj;lj;lk’;l\\\n","link":null},"annotations":{"bold":true,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"lkjkn ;oj;lj;lk’;l\\\n","href":null},{"type":"text","text":{"content":"nb","link":{"url":"/43b4db4f23b846f99909c783b033fb7d"}},"annotations":{"bold":true,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"nb","href":"/43b4db4f23b846f99909c783b033fb7d"},{"type":"text","text":{"content":".    \n","link":null},"annotations":{"bold":true,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":".    \n","href":null},{"type":"equation","equation":{"expression":"m;lm;’,"},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"m;lm;’,","href":null}],"is_inline":true,"properties":{"Select":{"id":"C%5E%7DO","name":"Select","type":"select","select":{"options":[{"id":"f56c757b-eb58-4a15-b528-055a0e3e85b4","name":"dddd","color":"red"}]}},"Text":{"id":"LwEA","name":"Text","type":"relation","relation":{"database_id":"48f51ca6-f1e3-40ee-97a5-953c2e5d8dda","type":"single_property","single_property":{}}},"ssss":{"id":"MeQJ","name":"ssss","type":"last_edited_time","last_edited_time":{}},"Date":{"id":"VwL%5B","name":"Date","type":"date","date":{}},"Status":{"id":"VwSP","name":"Status","type":"status","status":{"options":[{"id":"d553e1cf-a835-4608-9740-01335bc43a33","name":"Not started","color":"default"},{"id":"e4927bd2-4580-4e37-9095-eb0af45923bc","name":"In progress","color":"blue"},{"id":"631ae48b-ccbd-47fc-83a0-64388237cb90","name":"Done","color":"green"}],"groups":[{"id":"95be5bb3-f557-4e5f-bf80-bc0ba078a5ad","name":"To-do","color":"gray","option_ids":["d553e1cf-a835-4608-9740-01335bc43a33"]},{"id":"c3e8b669-177f-4f6a-a58b-998020b47992","name":"In progress","color":"blue","option_ids":["e4927bd2-4580-4e37-9095-eb0af45923bc"]},{"id":"fdbcab62-2699-49b6-9002-eb10a89806ad","name":"Complete","color":"green","option_ids":["631ae48b-ccbd-47fc-83a0-64388237cb90"]}]}},"Number":{"id":"WxBc","name":"Number","type":"number","number":{"format":"ruble"}},"Last edited time":{"id":"XDl%3D","name":"Last edited time","type":"last_edited_time","last_edited_time":{}},"ww":{"id":"Y%3B%3Bz","name":"ww","type":"rich_text","rich_text":{}},"Multi-select":{"id":"%5D%60%3FX","name":"Multi-select","type":"multi_select","multi_select":{"options":[{"id":"EgfK","name":"ddd","color":"default"},{"id":"QO[c","name":"AAA","color":"purple"},{"id":"UsL>","name":"Option","color":"orange"}]}},"ww (1)":{"id":"%60%3C%3DZ","name":"ww (1)","type":"checkbox","checkbox":{}},"Email":{"id":"bQRa","name":"Email","type":"email","email":{}},"Tags":{"id":"gOGx","name":"Tags","type":"multi_select","multi_select":{"options":[{"id":"21e940af-b7a0-4aae-985b-d3bb38a6ebeb","name":"JJJJ","color":"pink"}]}},"Test test":{"id":"nWZg","name":"Test test","type":"people","people":{}},"Checkbox":{"id":"qVHX","name":"Checkbox","type":"checkbox","checkbox":{}},"Status 1":{"id":"tlUB","name":"Status 1","type":"status","status":{"options":[{"id":"bc2cb10d-92da-40d1-b043-d3c5b52c789e","name":"Not started","color":"default"},{"id":"648b1e10-c0ac-4886-84e0-42d501d36e45","name":"In progress","color":"blue"},{"id":"6f1e3ce8-97db-40b5-8538-13269de69b7f","name":"Done","color":"green"}],"groups":[{"id":"cd0c5f4a-de4d-4662-a2ee-1c78fc0385cd","name":"To-do","color":"gray","option_ids":["bc2cb10d-92da-40d1-b043-d3c5b52c789e"]},{"id":"a341ea69-3102-4d56-b74a-fa3f1c47fa85","name":"In progress","color":"blue","option_ids":["648b1e10-c0ac-4886-84e0-42d501d36e45"]},{"id":"061c2c1f-faa2-49be-996e-f52d74a5b86e","name":"Complete","color":"green","option_ids":["6f1e3ce8-97db-40b5-8538-13269de69b7f"]}]}},"Formula":{"id":"%7Do%40%7B","name":"Formula","type":"formula","formula":{"expression":"log2(prop(\"Number\"))"}},"Name":{"id":"title","name":"Name","type":"title","title":{}}},"parent":{"type":"page_id","page_id":"d6917e78-3212-444d-ae46-97499c021f2d"},"url":"https://www.notion.so/072a11cb684f4f2b949079592700c67e","archived":false}],"next_cursor":null,"has_more":false,"type":"page_or_database","page_or_database":{}}`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	searchService := New(c)
	db, _, err := searchService.Search(context.TODO(), "key", pageSize)
	assert.NotNil(t, db)
	assert.Len(t, db, 1)
	assert.Nil(t, err)

	ds := database.New(nil)
	progress := process.NewProgress(pb.ModelProcess_Import)
	downloader := mock_files.NewMockDownloader(t)
	downloader.EXPECT().QueueFileForDownload(mock.Anything).Return(nil, true)
	databases, _, ce := ds.GetDatabase(context.Background(), pb.RpcObjectImportRequest_ALL_OR_NOTHING, db, progress, api.NewNotionImportContext(), downloader)

	assert.NotNil(t, databases)
	assert.Len(t, databases.Snapshots, 16) // 1 database + 15 properties (name doesn't count)
	assert.Nil(t, ce)
}

func Test_GetPagesSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
    "object": "list",
    "results": [
        {
            "object": "page",
            "id": "48cfec01-2e79-4af1-aaec-c1a3a8a95855",
            "created_time": "2022-12-06T11:19:00.000Z",
            "last_edited_time": "2022-12-07T08:34:00.000Z",
            "created_by": {
                "object": "user",
                "id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
            },
            "last_edited_by": {
                "object": "user",
                "id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
            },
            "cover": null,
            "icon": null,
            "parent": {
                "type": "database_id",
                "database_id": "48f51ca6-f1e3-40ee-97a5-953c2e5d8dda"
            },
            "archived": false,
            "properties": {
                "Tags": {
                    "id": "!'(w",
                    "type": "multi_select",
                    "multi_select": [
                        {
                            "id": "00a58cba-c800-40cd-a8f1-6e42527b0a29",
                            "name": "Special Event",
                            "color": "yellow"
                        },
                        {
                            "id": "4322f3ac-635f-4d2f-808f-22d639bc393b",
                            "name": "Daily",
                            "color": "purple"
                        }
                    ]
                },
                "Rollup": {
                    "id": "%3Df%3E%7B",
                    "type": "rollup",
                    "rollup": {
                        "type": "number",
                        "number": 2,
                        "function": "count"
                    }
                },
                "Related Journal 1": {
                    "id": "%3D%7CO%7B",
                    "type": "relation",
                    "relation": [
                        {
                            "id": "088b08d5-b692-4805-8338-1b147a3bff4a"
                        }
                    ],
                    "has_more": false
                },
                "Files & media": {
                    "id": "%3FmtK",
                    "type": "files",
                    "files": [
                        {
                            "name": "2022-11-28 11.54.58.jpg",
                            "type": "file",
                            "file": {
                                "url": "",
                                "expiry_time": "2022-12-07T09:35:05.952Z"
                            }
                        }
                    ]
                },
                "Last edited time": {
                    "id": "%40x%3DJ",
                    "type": "last_edited_time",
                    "last_edited_time": "2022-12-07T08:34:00.000Z"
                },
                "Number": {
                    "id": "I%60O%7D",
                    "type": "number",
                    "number": null
                },
                "Multi-select": {
                    "id": "M%5Btn",
                    "type": "multi_select",
                    "multi_select": [
                        {
                            "id": "49d921f8-44b4-4175-8ae9-c0dc7dd70d76",
                            "name": "q",
                            "color": "blue"
                        },
                        {
                            "id": "55b166d6-7713-4628-b412-560013f6e0ad",
                            "name": "w",
                            "color": "brown"
                        },
                        {
                            "id": "fd5b1266-7c51-4208-83d4-ff3c01efc3b8",
                            "name": "r",
                            "color": "pink"
                        }
                    ]
                },
                "Checkbox": {
                    "id": "O%5DNd",
                    "type": "checkbox",
                    "checkbox": false
                },
                "Status": {
                    "id": "OdD%3A",
                    "type": "status",
                    "status": {
                        "id": "01648775-b1d6-4c21-b093-dab131155840",
                        "name": "In progress",
                        "color": "blue"
                    }
                },
                "Created by": {
                    "id": "WCk%3B",
                    "type": "created_by",
                    "created_by": {
                        "object": "user",
                        "id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d",
                        "name": "Anastasia Shemyakinskaya",
                        "avatar_url": "",
                        "type": "person",
                        "person": {}
                    }
                },
                "https://developers.notion.com/": {
                    "id": "%5BaNB",
                    "type": "url",
                    "url": "https://developers.notion.com/"
                },
                "Created time": {
                    "id": "%5C%3B_p",
                    "type": "created_time",
                    "created_time": "2022-12-06T11:19:00.000Z"
                },
                "Date": {
                    "id": "%5DIZz",
                    "type": "date",
                    "date": {
                        "start": "2022-12-16",
                        "end": "2022-12-16",
                        "time_zone": null
                    }
                },
                "Text": {
                    "id": "%5DS%3AW",
                    "type": "rich_text",
                    "rich_text": [
                        {
                            "type": "text",
                            "text": {
                                "content": "sdfsdfsdf",
                                "link": null
                            },
                            "annotations": {
                                "bold": false,
                                "italic": false,
                                "strikethrough": false,
                                "underline": false,
                                "code": false,
                                "color": "default"
                            },
                            "plain_text": "sdfsdfsdf",
                            "href": null
                        }
                    ]
                },
                "Related Journal": {
                    "id": "d%5DpH",
                    "type": "relation",
                    "relation": [
                        {
                            "id": "f90772d0-0155-4ba1-8086-5a9daa750308"
                        },
                        {
                            "id": "088b08d5-b692-4805-8338-1b147a3bff4a"
                        }
                    ],
                    "has_more": false
                },
                "email": {
                    "id": "ijvk",
                    "type": "email",
                    "email": null
                },
                "👜 Page": {
                    "id": "kZi%3D",
                    "type": "relation",
                    "relation": [],
                    "has_more": true
                },
                "Checkbox 1": {
                    "id": "n_gn",
                    "type": "checkbox",
                    "checkbox": true
                },
                "Last edited by": {
                    "id": "n%7Biq",
                    "type": "last_edited_by",
                    "last_edited_by": {
                        "object": "user",
                        "id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d",
                        "name": "Anastasia Shemyakinskaya",
                        "avatar_url": "",
                        "type": "person",
                        "person": {}
                    }
                },
                "URL": {
                    "id": "vj%5Dv",
                    "type": "url",
                    "url": null
                },
                "Phone": {
                    "id": "wtAo",
                    "type": "phone_number",
                    "phone_number": "phone_number"
                },
                "Created": {
                    "id": "%7D%25j%7B",
                    "type": "created_time",
                    "created_time": "2022-12-06T11:19:00.000Z"
                },
                "Formula": {
                    "id": "%7DdGa",
                    "type": "formula",
                    "formula": {
                        "type": "string",
                        "string": "Page"
                    }
                },
                "Name": {
                    "id": "title",
                    "type": "title",
                    "title": [
                        {
                            "type": "text",
                            "text": {
                                "content": "Test",
                                "link": null
                            },
                            "annotations": {
                                "bold": false,
                                "italic": false,
                                "strikethrough": false,
                                "underline": false,
                                "code": false,
                                "color": "default"
                            },
                            "plain_text": "Test",
                            "href": null
                        }
                    ]
                }
            },
            "url": "https://www.notion.so/"
        }
    ],
    "next_cursor": null,
    "has_more": false,
    "type": "page_or_database",
    "page_or_database": {}
}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	searchService := New(c)
	_, p, err := searchService.Search(context.TODO(), "key", pageSize)
	assert.NotNil(t, p)
	assert.Len(t, p, 1)
	assert.Nil(t, err)
}

func Test_SearchFailedRequest(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"object":"error","status":400,"code":"validation_error","message":"path failed validation: path.database_id should be a valid uuid"}`))
	}))
	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	searchService := New(c)
	db, p, err := searchService.Search(context.TODO(), "key", pageSize)
	assert.Nil(t, db)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "path failed validation: path.database_id should be a valid uuid")
}
