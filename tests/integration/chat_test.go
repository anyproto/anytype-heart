package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/anytype/account"
	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	apiserver "github.com/anyproto/anytype-heart/core/api/server"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

// chatFixture stands up an in-process Anytype account, an HTTP API server, and
// a chat object so individual tests can talk to the chat REST endpoints over
// real HTTP with a real backing middleware.
type chatFixture struct {
	t          *testing.T
	mw         *core.Middleware
	spaceId    string
	identity   string
	appKey     string
	chatId     string
	baseURL    string
	httpClient *http.Client
}

func newChatFixture(t *testing.T) *chatFixture {
	ctx := context.Background()
	repoDir := t.TempDir()

	// initialparams stores its state in a process-global; reset so each test
	// run gets a fresh InitialSetParameters call that actually wires the
	// client version into the underlying application service.
	initialparams.ResetForTest()

	mw := core.New()

	initResp := mw.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
		Platform: "integration-test",
		Version:  "1.0.0",
	})
	require.Equal(t, pb.RpcInitialSetParametersResponseError_NULL, initResp.Error.Code, initResp.Error.Description)
	metrics.Service.InitWithKeys(metrics.DefaultInHouseKey)

	walletResp := mw.WalletCreate(ctx, &pb.RpcWalletCreateRequest{RootPath: repoDir})
	require.Equal(t, pb.RpcWalletCreateResponseError_NULL, walletResp.Error.Code, walletResp.Error.Description)

	eventQueue := mb.New[*pb.EventMessage](0)
	mw.SetEventSender(event.NewCallbackSender(func(e *pb.Event) {
		for _, m := range e.Messages {
			if err := eventQueue.Add(ctx, m); err != nil {
				log.Println("event queue:", err)
			}
		}
	}))

	accResp := mw.AccountCreate(ctx, &pb.RpcAccountCreateRequest{
		Name:                    "chat-test",
		StorePath:               repoDir,
		DisableLocalNetworkSync: true,
		NetworkMode:             pb.RpcAccount_LocalOnly,
	})
	require.Equal(t, pb.RpcAccountCreateResponseError_NULL, accResp.Error.Code, accResp.Error.Description)
	spaceId := accResp.Account.Info.AccountSpaceId

	linkResp := mw.AccountLocalLinkCreateApp(ctx, &pb.RpcAccountLocalLinkCreateAppRequest{
		App: &model.AccountAuthAppInfo{
			AppName: "integration-test",
			Scope:   model.AccountAuth_JsonAPI,
		},
	})
	require.Equal(t, pb.RpcAccountLocalLinkCreateAppResponseError_NULL, linkResp.Error.Code, linkResp.Error.Description)

	a := mw.GetApp()
	acctSvc := app.MustComponent[account.Service](a)
	evtSvc := app.MustComponent[apicore.EventService](a)
	crossSub := app.MustComponent[apicore.CrossSpaceSubscriptionService](a)
	chatSub := &chatSubAdapter{svc: app.MustComponent[chatsubscription.Service](a)}
	fileObjSvc := app.MustComponent[apicore.FileObjectService](a)

	// zero V2Deps: this suite exercises the v1 chat endpoints, so v2 stays
	// unregistered (NewServer skips it when Reader/Store are nil).
	srv := apiserver.NewServer(mw, acctSvc, evtSvc, crossSub, chatSub, fileObjSvc, apiserver.V2Deps{}, "127.0.0.1:0", nil, nil)
	ts := httptest.NewServer(srv.Engine())

	// Create a workspace-level chatDerived object. block.Service.ObjectAddChat
	// short-circuits on personal spaces, so we drive the creator directly to
	// produce the same chatDerived layout the API's ListChats filter targets.
	spaceSvc := app.MustComponent[space.Service](a)
	personalSpace, err := spaceSvc.Wait(ctx, spaceId)
	require.NoError(t, err)
	objCreator := app.MustComponent[objectcreator.Service](a)
	chatId, err := objCreator.AddChatDerivedObject(ctx, personalSpace, personalSpace.DerivedIDs().Workspace, false)
	require.NoError(t, err)
	require.NotEmpty(t, chatId)

	t.Cleanup(func() {
		ts.Close()
		mw.AppShutdown(ctx, &pb.RpcAppShutdownRequest{})
	})

	return &chatFixture{
		t:          t,
		mw:         mw,
		spaceId:    spaceId,
		identity:   acctSvc.AccountID(),
		appKey:     linkResp.AppKey,
		chatId:     chatId,
		baseURL:    ts.URL,
		httpClient: ts.Client(),
	}
}

// chatSubAdapter mirrors core/api/chatsubadapter.go, which is package-private.
type chatSubAdapter struct {
	svc chatsubscription.Service
}

func (a *chatSubAdapter) SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string, sink chan<- *pb.Event) ([]*model.ChatMessage, error) {
	resp, err := a.svc.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId: chatObjectId,
		SubId:        subId,
		Limit:        limit,
		SseSink:      sink,
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe last messages: %w", err)
	}
	msgs := make([]*model.ChatMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		msgs = append(msgs, m.ChatMessage)
	}
	return msgs, nil
}

func (a *chatSubAdapter) Unsubscribe(chatObjectId, subId string) error {
	return a.svc.Unsubscribe(chatObjectId, subId)
}

// doJSON issues an authenticated JSON request, decodes the response into out
// when provided, and returns the HTTP status code.
func (f *chatFixture) doJSON(method, path string, body, out any) int {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(f.t, err)
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, f.baseURL+path, reader)
	require.NoError(f.t, err)
	req.Header.Set("Authorization", "Bearer "+f.appKey)
	req.Header.Set("Anytype-Version", apiserver.ApiVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := f.httpClient.Do(req)
	require.NoError(f.t, err)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(f.t, err)

	if out != nil && len(raw) > 0 {
		require.NoError(f.t, json.Unmarshal(raw, out), "decode response body: %s", string(raw))
	}
	return resp.StatusCode
}

func (f *chatFixture) chatURL(suffix string) string {
	return fmt.Sprintf("/v1/spaces/%s/chats/%s%s", f.spaceId, f.chatId, suffix)
}

func TestChat(t *testing.T) {
	fx := newChatFixture(t)
	wantCreator := domain.NewParticipantId(fx.spaceId, fx.identity)

	var msgId string

	t.Run("list chats includes the new chat", func(t *testing.T) {
		var out struct {
			Data []apimodel.Object `json:"data"`
		}
		code := fx.doJSON(http.MethodGet, fmt.Sprintf("/v1/spaces/%s/chats", fx.spaceId), nil, &out)
		require.Equal(t, http.StatusOK, code)

		var found *apimodel.Object
		for i := range out.Data {
			if out.Data[i].Id == fx.chatId {
				found = &out.Data[i]
				break
			}
		}
		require.NotNil(t, found, "chat %s missing from list", fx.chatId)
		assert.Equal(t, apimodel.ObjectLayoutChat, found.Layout)
	})

	t.Run("add and get messages enriches creator", func(t *testing.T) {
		var added apimodel.AddChatMessageResponse
		code := fx.doJSON(http.MethodPost, fx.chatURL("/messages"),
			apimodel.AddChatMessageRequest{Text: "hello world"}, &added)
		require.Equal(t, http.StatusCreated, code)
		require.NotEmpty(t, added.MessageId)
		msgId = added.MessageId

		var listOut apimodel.ChatMessagesResponse
		code = fx.doJSON(http.MethodGet, fx.chatURL("/messages"), nil, &listOut)
		require.Equal(t, http.StatusOK, code)
		require.Len(t, listOut.Messages, 1)

		m := listOut.Messages[0]
		assert.Equal(t, msgId, m.Id)
		assert.Equal(t, "hello world", m.Content.Text)
		assert.Equal(t, wantCreator, m.Creator)
		assert.NotEmpty(t, m.CreatorName)
	})

	t.Run("get message by id", func(t *testing.T) {
		var out apimodel.ChatMessageResponse
		code := fx.doJSON(http.MethodGet, fx.chatURL("/messages/"+msgId), nil, &out)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, msgId, out.Message.Id)
		assert.Equal(t, "hello world", out.Message.Content.Text)
		assert.Equal(t, wantCreator, out.Message.Creator)
	})

	t.Run("edit message updates content", func(t *testing.T) {
		code := fx.doJSON(http.MethodPatch, fx.chatURL("/messages/"+msgId),
			apimodel.EditChatMessageRequest{Text: "edited text"}, nil)
		require.Equal(t, http.StatusOK, code)

		var out apimodel.ChatMessageResponse
		code = fx.doJSON(http.MethodGet, fx.chatURL("/messages/"+msgId), nil, &out)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, "edited text", out.Message.Content.Text)
	})

	t.Run("toggle reaction adds it", func(t *testing.T) {
		code := fx.doJSON(http.MethodPost, fx.chatURL("/messages/"+msgId+"/reactions"),
			apimodel.ToggleReactionRequest{Emoji: "👍"}, nil)
		require.Equal(t, http.StatusOK, code)

		var out apimodel.ChatMessageResponse
		code = fx.doJSON(http.MethodGet, fx.chatURL("/messages/"+msgId), nil, &out)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, out.Message.Reactions, "👍")
		assert.Contains(t, out.Message.Reactions["👍"], fx.identity)
	})

	t.Run("search finds posted message", func(t *testing.T) {
		var added apimodel.AddChatMessageResponse
		code := fx.doJSON(http.MethodPost, fx.chatURL("/messages"),
			apimodel.AddChatMessageRequest{Text: "uniqueneedle phrase"}, &added)
		require.Equal(t, http.StatusCreated, code)

		// Full-text indexing is asynchronous; poll briefly.
		require.Eventually(t, func() bool {
			var out struct {
				Data []apimodel.ChatMessageSearchResult `json:"data"`
			}
			if fx.doJSON(http.MethodGet, fx.chatURL("/messages/search?query=uniqueneedle"), nil, &out) != http.StatusOK {
				return false
			}
			for _, r := range out.Data {
				if r.Message.Id == added.MessageId {
					return true
				}
			}
			return false
		}, 5*time.Second, 100*time.Millisecond, "search did not return the message we just posted")
	})

	t.Run("read_all succeeds", func(t *testing.T) {
		code := fx.doJSON(http.MethodPost, fx.chatURL("/read_all"), nil, nil)
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("range-based read succeeds for messages and mentions", func(t *testing.T) {
		code := fx.doJSON(http.MethodPost, fx.chatURL("/messages/read"),
			apimodel.ReadChatMessagesRequest{}, nil)
		assert.Equal(t, http.StatusOK, code)

		code = fx.doJSON(http.MethodPost, fx.chatURL("/messages/read"),
			apimodel.ReadChatMessagesRequest{Type: "mentions"}, nil)
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("read reactions succeeds", func(t *testing.T) {
		code := fx.doJSON(http.MethodPost, fx.chatURL("/reactions/read"),
			apimodel.ReadChatReactionsRequest{}, nil)
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("sse stream delivers backlog, live events, and heartbeats", func(t *testing.T) {
		// Post a message so the stream has at least one initial event on connect.
		var initial apimodel.AddChatMessageResponse
		require.Equal(t, http.StatusCreated, fx.doJSON(http.MethodPost, fx.chatURL("/messages"),
			apimodel.AddChatMessageRequest{Text: "before-stream"}, &initial))

		streamCtx, cancelStream := context.WithCancel(context.Background())
		defer cancelStream()

		req, err := http.NewRequestWithContext(streamCtx, http.MethodGet,
			fx.baseURL+fx.chatURL("/messages/stream"), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+fx.appKey)
		req.Header.Set("Anytype-Version", apiserver.ApiVersion)
		req.Header.Set("Anytype-Heartbeat-Seconds", "1")

		resp, err := fx.httpClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

		items := make(chan sseItem, 32)
		go readSSEStream(resp.Body, items)

		// Initial backlog should include the message we just posted.
		ev := awaitMessageEvent(t, items, initial.MessageId, 5*time.Second)
		assert.Equal(t, "before-stream", ev.Message.Content.Text)

		// A live POST should produce a message_added event on the stream.
		var live apimodel.AddChatMessageResponse
		require.Equal(t, http.StatusCreated, fx.doJSON(http.MethodPost, fx.chatURL("/messages"),
			apimodel.AddChatMessageRequest{Text: "live-message"}, &live))

		ev = awaitMessageEvent(t, items, live.MessageId, 5*time.Second)
		assert.Equal(t, "live-message", ev.Message.Content.Text)
		assert.Equal(t, wantCreator, ev.Message.Creator)

		// Heartbeat is a `: ...` SSE comment; with the 1s header we should see one
		// within a couple of ticks.
		awaitHeartbeat(t, items, 3*time.Second)
	})

	t.Run("delete message then get returns 404", func(t *testing.T) {
		code := fx.doJSON(http.MethodDelete, fx.chatURL("/messages/"+msgId), nil, nil)
		require.Equal(t, http.StatusOK, code)

		code = fx.doJSON(http.MethodGet, fx.chatURL("/messages/"+msgId), nil, nil)
		assert.Equal(t, http.StatusNotFound, code)
	})
}

// sseItem is one decoded record from an SSE stream — either a typed event or
// an SSE comment line (the protocol-level keepalive shape we use for chat
// heartbeats).
type sseItem struct {
	kind  string // "event" or "heartbeat"
	event sseEvent
}

type sseEvent struct {
	Type string
	Data string
}

// readSSEStream parses the SSE protocol off r and pushes items onto out until
// the stream closes. Closes out on return.
func readSSEStream(r io.Reader, out chan<- sseItem) {
	defer close(out)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var cur sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if cur.Type != "" || cur.Data != "" {
				out <- sseItem{kind: "event", event: cur}
				cur = sseEvent{}
			}
		case strings.HasPrefix(line, ":"):
			out <- sseItem{kind: "heartbeat"}
		case strings.HasPrefix(line, "event: "):
			cur.Type = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// awaitMessageEvent drains items until a message_added event for messageId
// arrives, fails the test on timeout. Returns the decoded payload.
func awaitMessageEvent(t *testing.T, items <-chan sseItem, messageId string, timeout time.Duration) apimodel.ChatEventMessageAdded {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case item, ok := <-items:
			if !ok {
				t.Fatalf("stream closed before message %s arrived", messageId)
			}
			if item.kind != "event" || item.event.Type != "message_added" {
				continue
			}
			var envelope struct {
				Payload apimodel.ChatEventMessageAdded `json:"payload"`
			}
			require.NoError(t, json.Unmarshal([]byte(item.event.Data), &envelope), "decode SSE payload: %s", item.event.Data)
			if envelope.Payload.Message.Id == messageId {
				return envelope.Payload
			}
		case <-deadline:
			t.Fatalf("timed out waiting for message_added event for %s", messageId)
		}
	}
}

// awaitHeartbeat fails the test if no heartbeat comment arrives within timeout.
func awaitHeartbeat(t *testing.T, items <-chan sseItem, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case item, ok := <-items:
			if !ok {
				t.Fatal("stream closed before any heartbeat arrived")
			}
			if item.kind == "heartbeat" {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for a heartbeat comment")
		}
	}
}
