package block

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
)

func Test_GetBlocksAndChildrenSuccessParagraph(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "a80ae792-b87e-48d2-b24c-c32c1e14d509",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:52:00.000Z",
					"last_edited_time": "2022-11-14T12:18:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "paragraph",
					"paragraph": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "dsadasd sdasd\n",
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
								"plain_text": "dsadasd sdasd\n",
								"href": null
							},
							{
								"type": "text",
								"text": {
									"content": "asd ",
									"link": null
								},
								"annotations": {
									"bold": true,
									"italic": false,
									"strikethrough": false,
									"underline": false,
									"code": false,
									"color": "default"
								},
								"plain_text": "asd ",
								"href": null
							},
							{
								"type": "text",
								"text": {
									"content": "asdasd.  \n",
									"link": null
								},
								"annotations": {
									"bold": true,
									"italic": true,
									"strikethrough": false,
									"underline": true,
									"code": false,
									"color": "default"
								},
								"plain_text": "asdasd.  \n",
								"href": null
							},
							{
								"type": "text",
								"text": {
									"content": "asdasd",
									"link": null
								},
								"annotations": {
									"bold": true,
									"italic": true,
									"strikethrough": false,
									"underline": true,
									"code": false,
									"color": "orange_background"
								},
								"plain_text": "asdasd",
								"href": null
							}
						],
						"color": "green_background"
					}
				}
				
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*ParagraphBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessHeading3(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "968c10fd-39f5-4a31-8a47-719778d9cb22",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:54:00.000Z",
					"last_edited_time": "2022-11-14T11:54:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "heading_3",
					"heading_3": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "Heading 3",
									"link": null
								},
								"annotations": {
									"bold": false,
									"italic": false,
									"strikethrough": true,
									"underline": true,
									"code": false,
									"color": "blue_background"
								},
								"plain_text": "Heading 3",
								"href": null
							}
						],
						"is_toggleable": false,
						"color": "default"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*Heading3Block)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessTodo(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "c0c29ebb-3064-466c-b058-54f07128a1e9",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:53:00.000Z",
					"last_edited_time": "2022-11-14T11:54:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "to_do",
					"to_do": {
						"rich_text": [],
						"checked": false,
						"color": "default"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*ToDoBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessHeading2(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "1d5f7d59-32aa-46dc-aec7-e275fdd56752",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:54:00.000Z",
					"last_edited_time": "2022-11-14T11:54:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "heading_2",
					"heading_2": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "Heading 2",
									"link": null
								},
								"annotations": {
									"bold": true,
									"italic": true,
									"strikethrough": false,
									"underline": false,
									"code": false,
									"color": "default"
								},
								"plain_text": "Heading 2",
								"href": null
							}
						],
						"is_toggleable": false,
						"color": "default"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*Heading2Block)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessBulletList(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "152b978d-ee32-498c-a9db-985677a6dce6",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:55:00.000Z",
					"last_edited_time": "2022-11-14T11:55:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "bulleted_list_item",
					"bulleted_list_item": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "buller",
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
								"plain_text": "buller",
								"href": null
							}
						],
						"color": "default"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*BulletedListBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessNumberedList(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "38fc4773-8b28-445c-83bf-cb8c06badf10",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:55:00.000Z",
					"last_edited_time": "2022-11-14T12:17:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "numbered_list_item",
					"numbered_list_item": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "Number",
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
								"plain_text": "Number",
								"href": null
							}
						],
						"color": "default"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*NumberedListBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessToggle(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "ac80ca02-f09c-49f9-bc6f-2079058f1923",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:55:00.000Z",
					"last_edited_time": "2022-11-14T11:55:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "toggle",
					"toggle": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "Toggle",
									"link": null
								},
								"annotations": {
									"bold": true,
									"italic": false,
									"strikethrough": false,
									"underline": false,
									"code": true,
									"color": "default"
								},
								"plain_text": "Toggle",
								"href": null
							}
						],
						"color": "default"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*ToggleBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessQuote(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "659d5d72-2a8d-4df2-9475-9bd2ac816ddd",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:55:00.000Z",
					"last_edited_time": "2022-11-14T11:56:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "quote",
					"quote": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "Quote",
									"link": {
										"url": "ref"
									}
								},
								"annotations": {
									"bold": true,
									"italic": false,
									"strikethrough": true,
									"underline": false,
									"code": false,
									"color": "yellow_background"
								},
								"plain_text": "Quote",
								"href": "ref"
							}
						],
						"color": "default"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*QuoteBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessCallout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "b17fb388-715c-4f3d-841c-7492d4c29e39",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T11:56:00.000Z",
					"last_edited_time": "2022-11-14T12:17:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "callout",
					"callout": {
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "BBBBBBB",
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
								"plain_text": "BBBBBBB",
								"href": null
							}
						],
						"icon": {
							"type": "file",
							"file": {
								"url": "url",
								"expiry_time": "2022-11-14T14:38:56.733Z"
							}
						},
						"color": "gray_background"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*CalloutBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessCode(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "b4b694d7-aed7-4ceb-aa22-f48026b07f5d",
					"parent": {
						"type": "page_id",
						"page_id": "088b08d5-b692-4805-8338-1b147a3bff4a"
					},
					"created_time": "2022-11-14T12:22:00.000Z",
					"last_edited_time": "2022-11-14T12:22:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "code",
					"code": {
						"caption": [],
						"rich_text": [
							{
								"type": "text",
								"text": {
									"content": "Code",
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
								"plain_text": "Code",
								"href": null
							}
						],
						"language": "html"
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.Len(t, bl, 1)
	_, ok := bl[0].(*CodeBlock)
	assert.True(t, ok)
}

func Test_GetBlocksAndChildrenSuccessError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"Could not find block with ID: d6917e78-3212-444d-ae46-97499c021f2d. Make sure the relevant pages and databases are shared with your integration."}`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.NotNil(t, err)
	assert.Empty(t, bl)
}

func TestTableBlocks(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		{
			"object": "list",
			"results": [
				{
					"object": "block",
					"id": "25377f65-71cf-4779-9829-de3717767148",
					"parent": {
						"type": "block_id",
						"block_id": "049ab49c-17a5-4c03-bdbf-71811b4524b7"
					},
					"created_time": "2022-12-09T08:39:00.000Z",
					"last_edited_time": "2022-12-09T08:40:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "table_row",
					"table_row": {
						"cells": [
							[
								{
									"type": "text",
									"text": {
										"content": "1",
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
									"plain_text": "1",
									"href": null
								}
							],
							[],
							[
								{
									"type": "text",
									"text": {
										"content": "dddd",
										"link": null
									},
									"annotations": {
										"bold": false,
										"italic": false,
										"strikethrough": false,
										"underline": false,
										"code": false,
										"color": "pink"
									},
									"plain_text": "dddd",
									"href": null
								}
							],
							[]
						]
					}
				},
				{
					"object": "block",
					"id": "f9e3bf51-eb64-45c7-bf68-2776d808e503",
					"parent": {
						"type": "block_id",
						"block_id": "049ab49c-17a5-4c03-bdbf-71811b4524b7"
					},
					"created_time": "2022-12-09T08:39:00.000Z",
					"last_edited_time": "2022-12-09T08:40:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "table_row",
					"table_row": {
						"cells": [
							[],
							[
								{
									"type": "text",
									"text": {
										"content": "fsdf",
										"link": null
									},
									"annotations": {
										"bold": false,
										"italic": true,
										"strikethrough": false,
										"underline": false,
										"code": false,
										"color": "default"
									},
									"plain_text": "fsdf",
									"href": null
								}
							],
							[],
							[]
						]
					}
				},
				{
					"object": "block",
					"id": "1f68a81c-ba09-4f1a-ae99-a999dee96b07",
					"parent": {
						"type": "block_id",
						"block_id": "049ab49c-17a5-4c03-bdbf-71811b4524b7"
					},
					"created_time": "2022-12-09T08:39:00.000Z",
					"last_edited_time": "2022-12-09T08:40:00.000Z",
					"created_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"last_edited_by": {
						"object": "user",
						"id": "60faafc6-0c5c-4479-a3f7-67d77cd8a56d"
					},
					"has_children": false,
					"archived": false,
					"type": "table_row",
					"table_row": {
						"cells": [
							[
								{
									"type": "text",
									"text": {
										"content": "fdsdf",
										"link": null
									},
									"annotations": {
										"bold": true,
										"italic": false,
										"strikethrough": false,
										"underline": false,
										"code": false,
										"color": "default"
									},
									"plain_text": "fdsdf",
									"href": null
								}
							],
							[],
							[
								{
									"type": "text",
									"text": {
										"content": "sdf",
										"link": null
									},
									"annotations": {
										"bold": false,
										"italic": false,
										"strikethrough": false,
										"underline": false,
										"code": false,
										"color": "gray_background"
									},
									"plain_text": "sdf",
									"href": null
								}
							],
							[]
						]
					}
				}
			],
			"next_cursor": null,
			"has_more": false,
			"type": "block",
			"block": {}
		}
		`))
	}))

	defer s.Close()
	pageSize := int64(100)
	c := client.NewClient()
	c.BasePath = s.URL

	blockService := New(c)
	bl, err := blockService.GetBlocksAndChildren(context.TODO(), "id", "key", pageSize, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	assert.Nil(t, err)
	assert.NotNil(t, bl)
	assert.Len(t, bl, 3)
}