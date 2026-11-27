package chatobject

// Chat attribution (docs/ApiChangeAttribution.md "Chat"): a message says
// which local-API app CREATED it, from the creating change's envelope and
// from nothing else. Each test names the implementation change that turns
// it red.

import (
	"context"
	"testing"

	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// createPayload is the JSON document of the first create in a change set:
// the bytes that are signed and sent to every member.
func createPayload(t *testing.T, set storestate.ChangeSet) *anyenc.Value {
	t.Helper()
	require.NotEmpty(t, set.Changes)
	create := set.Changes[0].GetCreate()
	require.NotNil(t, create)
	value, err := anyenc.ParseJson(create.Value)
	require.NoError(t, err)
	return value
}

// storedHasKey reports whether the stored message document holds key.
func (fx *fixture) storedHasKey(t *testing.T, messageId, key string) bool {
	t.Helper()
	coll, err := fx.store.Collection(context.Background(), CollectionName)
	require.NoError(t, err)
	storageKey, err := chatrepository.FindMessageKey(context.Background(), coll, messageId)
	require.NoError(t, err)
	doc, err := coll.FindId(context.Background(), storageKey)
	require.NoError(t, err)
	return doc.Value().Get(key) != nil
}

func TestChatMessageAttribution(t *testing.T) {
	appCtx := context.Background()
	raycast := domain.CtxWithIntegrationName(appCtx, "Raycast")
	linear := domain.CtxWithIntegrationName(appCtx, "Linear")

	t.Run("a message sent through the API names the app, one sent from an app names nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		apiMessage, err := fx.AddMessage(raycast, nil, givenSimpleMessage("from the api"))
		require.NoError(t, err)
		appMessage, err := fx.AddMessage(appCtx, nil, givenSimpleMessage("from the app"))
		require.NoError(t, err)

		// then: red if BeforeCreate stops reading the change's attribution,
		// or toModel/MarshalAnyenc stop round-tripping the stored key
		assert.Equal(t, "Raycast", fx.getMessage(t, apiMessage).IntegrationName)
		assert.Equal(t, "", fx.getMessage(t, appMessage).IntegrationName)
		// only API messages carry the key at all
		assert.True(t, fx.storedHasKey(t, apiMessage, chatmodel.IntegrationNameKey))
		assert.False(t, fx.storedHasKey(t, appMessage, chatmodel.IntegrationNameKey))
	})

	t.Run("a name the client puts in the message never reaches the signed create", func(t *testing.T) {
		// given: ChatAddMessage accepts a whole model.ChatMessage from the
		// client, so the client can fill the field
		fx := newFixture(t)
		forged := givenSimpleMessage("hello")
		forged.IntegrationName = "Forged"

		// when: sent from an app, and sent through the API by another app
		appMessage, err := fx.AddMessage(appCtx, nil, forged)
		require.NoError(t, err)
		forgedAgain := givenSimpleMessage("hello again")
		forgedAgain.IntegrationName = "Forged"
		apiMessage, err := fx.AddMessage(raycast, nil, forgedAgain)
		require.NoError(t, err)

		// then: the serialized outgoing operations hold no such key — red if
		// AddMessage stops stripping it from the payload it signs
		require.Len(t, fx.appliedChangeSets, 2)
		for _, set := range fx.appliedChangeSets {
			assert.Nil(t, createPayload(t, set).Get(chatmodel.IntegrationNameKey))
		}
		// and the stored messages say what the envelope said
		assert.Equal(t, "", fx.getMessage(t, appMessage).IntegrationName)
		assert.Equal(t, "Raycast", fx.getMessage(t, apiMessage).IntegrationName)
	})

	t.Run("a name forged in a received payload is deleted before the insert", func(t *testing.T) {
		// given: a peer's create whose JSON carries the key, in a change
		// whose envelope carries no name (an old or a hostile client)
		fx := newFixture(t)
		arena := &anyenc.Arena{}
		obj := arena.NewObject()
		givenSimpleMessage("from a peer").MarshalAnyenc(obj, arena)
		obj.Set(chatmodel.IntegrationNameKey, arena.NewString("Forged"))
		builder := &storestate.Builder{}
		require.NoError(t, builder.Create(CollectionName, storestate.IdFromChange, obj))
		require.NotNil(t, createPayload(t, storestate.ChangeSet{Changes: builder.ChangeSet}).Get(chatmodel.IntegrationNameKey),
			"the fixture must really put the forged key on the wire")

		// when
		messageId := fx.pushAs(t, bob, builder)

		// then: red if BeforeCreate sets the field only when the envelope
		// has a name — storestate inserts the parsed payload, so a key the
		// handler does not delete is stored and served
		assert.Equal(t, "", fx.getMessage(t, messageId).IntegrationName)
		assert.False(t, fx.storedHasKey(t, messageId, chatmodel.IntegrationNameKey))
	})

	t.Run("an edit, a reaction and a pin through another app leave it as it was", func(t *testing.T) {
		// given: a message Raycast created
		fx := newFixture(t)
		messageId, err := fx.AddMessage(raycast, nil, givenSimpleMessage("original"))
		require.NoError(t, err)

		// when: Linear edits it, reacts and pins it, then an app edits it
		require.NoError(t, fx.EditMessage(linear, messageId, givenSimpleMessage("edited by linear")))
		_, err = fx.ToggleMessageReaction(linear, messageId, "👍")
		require.NoError(t, err)
		require.NoError(t, fx.SetMessagesPinned(linear, []string{messageId}, true))
		require.NoError(t, fx.EditMessage(appCtx, messageId, givenSimpleMessage("edited in the app")))

		// then: creation-only — red if any modify path reads the change's
		// attribution, or if a re-marshal of the edited document drops the
		// stored key (toModel not reading it)
		got := fx.getMessage(t, messageId)
		assert.Equal(t, "edited in the app", got.Message.Text)
		assert.True(t, got.Pinned, "the pin must really have been applied")
		assert.Equal(t, "Raycast", got.IntegrationName)
		// while every one of those changes is itself attributed
		want := []string{"Raycast", "Linear", "Linear", "Linear", ""}
		var gotNames []string
		for _, set := range fx.appliedChangeSets {
			gotNames = append(gotNames, set.IntegrationName)
		}
		assert.Equal(t, want, gotNames)
	})

	t.Run("a modify of the key itself is refused", func(t *testing.T) {
		// given
		fx := newFixture(t)
		messageId, err := fx.AddMessage(raycast, nil, givenSimpleMessage("original"))
		require.NoError(t, err)
		builder := &storestate.Builder{}
		require.NoError(t, builder.Modify(CollectionName, messageId, []string{chatmodel.IntegrationNameKey}, pb.ModifyOp_Set, (&anyenc.Arena{}).NewString("Forged")))

		// when: the message's own author tries
		_, err = fx.push(testCreator, "", builder)

		// then: red if UpgradeKeyModifier starts accepting this key path
		require.Error(t, err)
		assert.Equal(t, "Raycast", fx.getMessage(t, messageId).IntegrationName)
	})

	t.Run("a deleted discussion comment keeps it on its tombstone", func(t *testing.T) {
		// given
		message := givenSimpleMessage("comment")
		message.IntegrationName = "Raycast"

		// when
		message.MakeTombstone()

		// then: it says who made the comment, like Creator does. Red if
		// MakeTombstone starts clearing it.
		assert.Equal(t, "Raycast", message.IntegrationName)
	})

	t.Run("the live event, the cached last message and a new subscriber's first answer carry it", func(t *testing.T) {
		// given: a subscriber that gets events by broadcast
		fx := newFixture(t)
		fx.eventSender.EXPECT().BroadcastToOtherSessions(mock.Anything, mock.Anything).Return().Maybe()
		_, err := fx.chatSubscriptionService.SubscribeLastMessages(appCtx, chatsubscription.SubscribeLastMessagesRequest{
			ChatObjectId: fx.Id(),
			SubId:        "sub",
		})
		require.NoError(t, err)

		// and an observer, called synchronously with its own copy of the
		// message — what the space chat hub and the API stream are fed from
		var observed []chatsubscription.ChatChange
		fx.subscription.Lock()
		fx.subscription.AddObserver("test", func(change chatsubscription.ChatChange) {
			observed = append(observed, change)
		})
		fx.subscription.Unlock()

		// when
		messageId, err := fx.AddMessage(raycast, nil, givenSimpleMessage("live"))
		require.NoError(t, err)

		// then, the observer's cloned add payload: red if the clone is made
		// before the handler sets the field, or does not copy it
		var addedToObserver *chatmodel.Message
		for _, change := range observed {
			if change.Kind == chatsubscription.ChatChangeMessageAdded && change.MessageId == messageId {
				addedToObserver = change.Message
			}
		}
		require.NotNil(t, addedToObserver, "the observer must be told about the add")
		assert.Equal(t, "Raycast", addedToObserver.IntegrationName)

		// the live event: red if the field is set after
		// subscription.Add (the event is built from the message as it is at
		// that call)
		var added *pb.EventChatAdd
		for _, event := range fx.events {
			if add := event.GetChatAdd(); add != nil && add.Id == messageId {
				added = add
			}
		}
		require.NotNil(t, added, "the subscriber must receive the add event")
		assert.Equal(t, "Raycast", added.Message.IntegrationName)

		// the cached last message — what a chat preview is served from
		fx.subscription.Lock()
		last, ok, err := fx.subscription.GetLastMessage()
		fx.subscription.Unlock()
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, messageId, last.Id)
		assert.Equal(t, "Raycast", last.IntegrationName)

		// and the initial answer to a subscriber that arrives afterwards:
		// read back from the stored document. Red if the stored key is not
		// mapped into the served message.
		resp, err := fx.chatSubscriptionService.SubscribeLastMessages(appCtx, chatsubscription.SubscribeLastMessagesRequest{
			ChatObjectId: fx.Id(),
			SubId:        "late",
			Limit:        10,
		})
		require.NoError(t, err)
		require.Len(t, resp.Messages, 1)
		assert.Equal(t, "Raycast", resp.Messages[0].IntegrationName)
	})
}

// A chat's details are edited through smartblock.Apply like any object's,
// and reach the store through the push-change hook, on the component's own
// context. The apply's attribution must make that hop.
func TestChatDetailsChangeAttribution(t *testing.T) {
	nameChange := func(integrationName string) source.PushChangeParams {
		return source.PushChangeParams{
			IntegrationName: integrationName,
			Changes: []*pb.ChangeContent{{Value: &pb.ChangeContentValueOfDetailsSet{
				DetailsSet: &pb.ChangeDetailsSet{Key: bundle.RelationKeyName.String(), Value: pbtypes.String("Renamed")},
			}}},
		}
	}

	t.Run("an API rename of a chat pushes a stamped store change, an app rename does not", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when: what smartblock.Apply hands the hook for an API edit, then
		// for an app edit
		_, err := fx.detailsComponent.onPushOrdinaryChange(nameChange("Raycast"))
		require.NoError(t, err)
		_, err = fx.detailsComponent.onPushOrdinaryChange(nameChange(""))
		require.NoError(t, err)

		// then: red if the hook drops PushChangeParams.IntegrationName — the
		// first value — or keeps it on the component context — the second
		want := []string{"Raycast", ""}
		var got []string
		for _, set := range fx.appliedChangeSets {
			got = append(got, set.IntegrationName)
		}
		assert.Equal(t, want, got)
	})
}
