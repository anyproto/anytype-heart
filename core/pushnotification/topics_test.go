package pushnotification

import (
	"slices"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSpaceTopicsCollection(t *testing.T) {
	t.Run("empty - empty", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		tc.ResetLocal()
		assert.Nil(t, tc.MakeApiRequest())
	})
	t.Run("not empty - empty", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		tc.ResetLocal()
		statusS1 := newTestSpaceStatus("s1", 0, "my")
		tc.SetRemoteList(&pushapi.Topics{
			Topics: []*pushapi.Topic{
				topicFromStatus(statusS1, "t1"),
			},
		})
		res := tc.MakeApiRequest()
		require.NotNil(t, res)
		assert.Len(t, res.Topics, 0)
	})
	t.Run("empty remote - create space", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		tc.Flush()
		tc.SetSpaceViewStatus(newTestSpaceStatus("s1", 0, "my"), nil)
		tc.SetSpaceViewStatus(newTestSpaceStatus("s2", 0, "my"), nil)
		assert.Len(t, tc.SpaceKeysToCreate(), 2)
	})
	t.Run("remote exists - create space", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		tc.Flush()
		statusS1 := newTestSpaceStatus("s1", 0, "my")
		tc.SetRemoteList(&pushapi.Topics{
			Topics: []*pushapi.Topic{
				topicFromStatus(statusS1, "t1"),
			},
		})
		tc.SetSpaceViewStatus(statusS1, nil)
		tc.SetSpaceViewStatus(newTestSpaceStatus("s2", 0, "my"), nil)
		assert.Len(t, tc.SpaceKeysToCreate(), 1)
	})
	t.Run("make request", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		tc.Flush()
		statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
		statusS2 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
		tc.SetSpaceViewStatus(statusS1, nil)
		tc.SetSpaceViewStatus(statusS2, nil)
		req := tc.MakeApiRequest()
		require.NotNil(t, req)
		assert.Len(t, req.Topics, 4)

		// same list - no results
		tc.Flush()
		tc.SetSpaceViewStatus(statusS1, nil)
		tc.SetSpaceViewStatus(statusS2, nil)
		req = tc.MakeApiRequest()
		assert.Nil(t, req)

		// change mode
		tc.Flush()
		tc.SetSpaceViewStatus(statusS1, nil)
		statusS2.mode = pb.RpcPushNotification_Mentions
		tc.SetSpaceViewStatus(statusS2, nil)
		req = tc.MakeApiRequest()
		require.NotNil(t, req)
		assert.Len(t, req.Topics, 3)
	})
	t.Run("encrypt", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
		tc.SetSpaceViewStatus(statusS1, nil)
		keyId, res, err := tc.EncryptPayload("s1", []byte{1, 2, 3})
		require.NoError(t, err)
		assert.NotEmpty(t, res)
		assert.NotEmpty(t, keyId)

		keyId, res, err = tc.EncryptPayload("s2", []byte{1, 2, 3})
		require.Error(t, err)
		assert.Empty(t, res)
		assert.Empty(t, keyId)

	})
	t.Run("make topics", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
		tc.SetSpaceViewStatus(statusS1, nil)
		res, err := tc.MakeTopics("s1", []string{"1", "2"})
		require.NoError(t, err)
		assert.Len(t, res.Topics, 2)

		res, err = tc.MakeTopics("s2", []string{"1", "2"})
		require.Error(t, err)
	})
	t.Run("space delete", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		tc.Flush()
		statusS1 := newTestSpaceStatus("s1", 0, "my")
		tc.SetRemoteList(&pushapi.Topics{
			Topics: []*pushapi.Topic{
				topicFromStatus(statusS1, "t1"),
			},
		})
		tc.SetSpaceViewStatus(statusS1, nil)
		tc.SetSpaceViewStatus(newTestSpaceStatus("s2", 0, "my"), nil)
		statusS1.status = model.SpaceStatus_SpaceDeleted
		tc.SetSpaceViewStatus(statusS1, nil)
		req := tc.MakeApiRequest()
		require.NotNil(t, req)
		assert.Len(t, req.Topics, 2)
	})
	t.Run("custom status", func(t *testing.T) {
		t.Run("global=all", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
			statusS1.muteIds = []string{"ch1"}
			statusS1.mentionIds = []string{"ch2"}
			statusS1.allIds = []string{"ch3"}
			tc.SetSpaceViewStatus(statusS1, chatEntriesForIds("s1", "ch1", "ch2", "ch3", "ch4", "ch5"))
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasMuted(t, req.Topics, "ch1")
			assertHasMention(t, req.Topics, "ch2")
			assertHasAll(t, req.Topics, "ch3")
			assertHasAll(t, req.Topics, "ch4")
			assertHasAll(t, req.Topics, "ch5")
		})
		t.Run("global=mention", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_Mentions, "my")
			statusS1.muteIds = []string{"ch1"}
			statusS1.mentionIds = []string{"ch2"}
			statusS1.allIds = []string{"ch3"}
			tc.SetSpaceViewStatus(statusS1, chatEntriesForIds("s1", "ch1", "ch2", "ch3", "ch4", "ch5"))
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasMuted(t, req.Topics, "ch1")
			assertHasMention(t, req.Topics, "ch2")
			assertHasAll(t, req.Topics, "ch3")
			assertHasMention(t, req.Topics, "ch4")
			assertHasMention(t, req.Topics, "ch5")
		})
		t.Run("global=mute", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_Nothing, "my")
			statusS1.muteIds = []string{"ch1"}
			statusS1.mentionIds = []string{"ch2"}
			statusS1.allIds = []string{"ch3"}
			tc.SetSpaceViewStatus(statusS1, chatEntriesForIds("s1", "ch1", "ch2", "ch3", "ch4", "ch5"))
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasMuted(t, req.Topics, "ch1")
			assertHasMention(t, req.Topics, "ch2")
			assertHasAll(t, req.Topics, "ch3")
			assertHasMuted(t, req.Topics, "ch4")
			assertHasMuted(t, req.Topics, "ch5")
		})
	})
	t.Run("discussions", func(t *testing.T) {
		myParticipant := domain.NewParticipantId("s1", "my")
		t.Run("identity in subscribers - all messages", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_Mentions, "my")
			tc.SetSpaceViewStatus(statusS1, []chatEntry{
				{chatId: "d1", spaceId: "s1", isDiscussion: true, subscribers: []string{myParticipant}},
			})
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasAll(t, req.Topics, "d1")
		})
		t.Run("identity not in subscribers - mentions only", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
			tc.SetSpaceViewStatus(statusS1, []chatEntry{
				{chatId: "d1", spaceId: "s1", isDiscussion: true, subscribers: []string{domain.NewParticipantId("s1", "other")}},
			})
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasMention(t, req.Topics, "d1")
		})
		t.Run("empty subscribers - mentions only", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
			tc.SetSpaceViewStatus(statusS1, []chatEntry{
				{chatId: "d1", spaceId: "s1", isDiscussion: true},
			})
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasMention(t, req.Topics, "d1")
		})
		t.Run("space-level allIds beats subscribers absence", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_Mentions, "my")
			statusS1.allIds = []string{"d1"}
			tc.SetSpaceViewStatus(statusS1, []chatEntry{
				{chatId: "d1", spaceId: "s1", isDiscussion: true},
			})
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasAll(t, req.Topics, "d1")
		})
		t.Run("space-level muteIds beats subscriber membership", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_All, "my")
			statusS1.muteIds = []string{"d1"}
			tc.SetSpaceViewStatus(statusS1, []chatEntry{
				{chatId: "d1", spaceId: "s1", isDiscussion: true, subscribers: []string{myParticipant}},
				{chatId: "d2", spaceId: "s1", isDiscussion: true, subscribers: []string{myParticipant}},
			})
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			assertHasMuted(t, req.Topics, "d1")
			assertHasAll(t, req.Topics, "d2")
		})
		t.Run("non-discussion ignores subscribers", func(t *testing.T) {
			tc := newSpaceTopicsCollection("my")
			tc.Flush()
			// mode=Mentions + chat (not a discussion) where my identity IS in subscribers
			// → should still be mentions (subscribers ignored for non-discussion chats)
			statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotification_Mentions, "my")
			tc.SetSpaceViewStatus(statusS1, []chatEntry{
				{chatId: "c1", spaceId: "s1", isDiscussion: false, subscribers: []string{myParticipant}},
			})
			req := tc.MakeApiRequest()
			require.NotNil(t, req)
			// no discussion + no overrides → falls through to bulk topics (chats / identity)
			assert.Len(t, req.Topics, 1)
		})
	})
}

func chatEntriesForIds(spaceId string, ids ...string) []chatEntry {
	out := make([]chatEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, chatEntry{chatId: id, spaceId: spaceId})
	}
	return out
}

func assertHasMention(t *testing.T, topics []*pushapi.Topic, chId string) {
	require.True(t, slices.ContainsFunc(topics, func(t *pushapi.Topic) bool {
		return strings.HasSuffix(t.Topic, sha256hex(chId)+"/my")
	}), "mention not found")
}

func assertHasAll(t *testing.T, topics []*pushapi.Topic, chId string) {
	require.True(t, slices.ContainsFunc(topics, func(t *pushapi.Topic) bool {
		return strings.HasSuffix(t.Topic, sha256hex(chId))
	}), "all not found")
}

func assertHasMuted(t *testing.T, topics []*pushapi.Topic, chId string) {
	require.False(t, slices.ContainsFunc(topics, func(t *pushapi.Topic) bool {
		return strings.Contains(t.Topic, sha256hex(chId))
	}), "muted found")
}

func newTestSpaceStatus(spaceId string, mode pb.RpcPushNotificationMode, creator string) *spaceViewStatus {
	spaceKey, _, _ := crypto.GenerateRandomEd25519KeyPair()
	encKey, _ := crypto.NewRandomAES()
	return &spaceViewStatus{
		spaceId:     spaceId,
		spaceViewId: "sv_" + spaceId,
		mode:        mode,
		spaceKey:    spaceKey,
		encKey:      encKey,
		creator:     "cr_" + creator,
	}
}

func topicFromStatus(status *spaceViewStatus, topic string) *pushapi.Topic {
	pubKeyRaw, _ := status.spaceKey.GetPublic().Raw()
	return &pushapi.Topic{
		SpaceKey: pubKeyRaw,
		Topic:    topic,
	}
}
