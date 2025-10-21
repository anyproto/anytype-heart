package pushnotification

import (
	"slices"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotificationSetSpaceMode_All, "my")
		statusS2 := newTestSpaceStatus("s1", pb.RpcPushNotificationSetSpaceMode_All, "my")
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
		statusS2.mode = pb.RpcPushNotificationSetSpaceMode_Mentions
		tc.SetSpaceViewStatus(statusS2, nil)
		req = tc.MakeApiRequest()
		require.NotNil(t, req)
		assert.Len(t, req.Topics, 3)
	})
	t.Run("encrypt", func(t *testing.T) {
		tc := newSpaceTopicsCollection("my")
		statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotificationSetSpaceMode_All, "my")
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
		statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotificationSetSpaceMode_All, "my")
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
		tc := newSpaceTopicsCollection("my")
		tc.Flush()
		statusS1 := newTestSpaceStatus("s1", pb.RpcPushNotificationSetSpaceMode_Custom, "my")
		statusS1.muteIds = []string{"ch1"}
		statusS1.mentionIds = []string{"ch2"}
		tc.SetSpaceViewStatus(statusS1, []string{"ch1", "ch2", "ch3"})
		req := tc.MakeApiRequest()
		require.NotNil(t, req)
		assert.True(t, slices.ContainsFunc(req.Topics, func(t *pushapi.Topic) bool {
			return strings.HasSuffix(t.Topic, "ch2/my")
		}), "mention not found")
		assert.False(t, slices.ContainsFunc(req.Topics, func(t *pushapi.Topic) bool {
			return strings.Contains(t.Topic, "ch1")
		}), "muted found")
		assert.True(t, slices.ContainsFunc(req.Topics, func(t *pushapi.Topic) bool {
			return strings.HasSuffix(t.Topic, "ch3")
		}), "ch3 not found")

	})
}

func newTestSpaceStatus(spaceId string, mode pb.RpcPushNotificationSetSpaceModeMode, creator string) *spaceViewStatus {
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
