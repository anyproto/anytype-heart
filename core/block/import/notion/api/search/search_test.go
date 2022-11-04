package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/database"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
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

	ds := database.New()
	databases := ds.GetDatabase(context.Background(), pb.RpcObjectImportRequest_ALL_OR_NOTHING, db)

	assert.NotNil(t, databases)
	assert.Len(t, databases.Snapshots, 1)
	assert.Nil(t, databases.Error)
}

func Test_GetDatabaseFailedRequest(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
        w.Write([]byte(`{"object":"error","status":400,"code":"validation_error","message":"path failed validation: path.database_id should be a valid uuid"}`))
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

	ds := database.New()
	databases := ds.GetDatabase(context.Background(), pb.RpcObjectImportRequest_ALL_OR_NOTHING, db)

	assert.NotNil(t, databases)
	assert.Nil(t, databases.Snapshots)
	assert.NotNil(t, databases.Error)
	assert.Contains(t, databases.Error.Error().Error(), "path failed validation: path.database_id should be a valid uuid")
}