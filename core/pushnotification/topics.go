package pushnotification

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const ChatsTopicName = "chats"

func newSpaceTopicsCollection(identity string) *spaceTopicsCollection {
	return &spaceTopicsCollection{
		identity: identity,
		statuses: map[string]*spaceViewStatus{},
	}
}

type spaceTopicsCollection struct {
	identity          string
	remoteTopics      []*pushapi.Topic
	localTopics       []*pushapi.Topic
	spaceKeysToCreate []crypto.PrivKey
	statuses          map[string]*spaceViewStatus
	mu                sync.Mutex
}

func (c *spaceTopicsCollection) Flush() {
	c.remoteTopics, c.localTopics = c.localTopics, c.remoteTopics
	c.ResetLocal()
}

func (c *spaceTopicsCollection) ResetLocal() {
	c.localTopics = c.localTopics[:0]
	c.spaceKeysToCreate = c.spaceKeysToCreate[:0]
}

func (c *spaceTopicsCollection) SetRemoteList(remoteTopics *pushapi.Topics) {
	for _, remoteTopic := range remoteTopics.Topics {
		c.remoteTopics = append(c.remoteTopics, remoteTopic)
	}
}

func (c *spaceTopicsCollection) SetSpaceViewStatus(status *spaceViewStatus, chatIds []string) {
	if status.spaceKey == nil || status.encKey == nil {
		return
	}
	if status.status == model.SpaceStatus_SpaceDeleted {
		c.deleteSpace(*status)
		return
	}
	pubKey, _ := status.spaceKey.GetPublic().Raw()

	needCreate := false
	if isOwner := strings.HasSuffix(status.creator, c.identity); isOwner {
		needCreate = true
		for _, remoteTopic := range c.remoteTopics {
			if bytes.Equal(remoteTopic.SpaceKey, pubKey) {
				needCreate = false
				break
			}
		}
	}
	if needCreate {
		c.spaceKeysToCreate = append(c.spaceKeysToCreate, status.spaceKey)
	}

	makeTopic := func(topics ...string) *pushapi.Topic {
		topic := strings.Join(topics, "/")
		sign, _ := status.spaceKey.Sign([]byte(topic))
		return &pushapi.Topic{
			SpaceKey:  pubKey,
			Topic:     topic,
			Signature: sign,
		}
	}

	switch status.mode {
	case pb.RpcPushNotificationSetSpaceMode_All:
		c.localTopics = append(c.localTopics, makeTopic(ChatsTopicName), makeTopic(c.identity))
	case pb.RpcPushNotificationSetSpaceMode_Mentions:
		c.localTopics = append(c.localTopics, makeTopic(c.identity))
	case pb.RpcPushNotificationSetSpaceMode_Custom:
		for _, chatId := range chatIds {
			if slices.Contains(status.muteIds, chatId) {
				continue
			}
			if slices.Contains(status.mentionIds, chatId) {
				c.localTopics = append(c.localTopics, makeTopic(ChatsTopicName, sha256hex(chatId), c.identity))
			} else {
				c.localTopics = append(c.localTopics, makeTopic(ChatsTopicName, sha256hex(chatId)))
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses[status.spaceId] = status
}

func (c *spaceTopicsCollection) deleteSpace(status spaceViewStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.statuses, status.spaceId)
	rawSpaceKey, _ := status.spaceKey.GetPublic().Raw()
	c.remoteTopics = slices.DeleteFunc(c.remoteTopics, func(topic *pushapi.Topic) bool {
		return bytes.Equal(topic.SpaceKey, rawSpaceKey)
	})
	c.localTopics = slices.DeleteFunc(c.localTopics, func(topic *pushapi.Topic) bool {
		return bytes.Equal(topic.SpaceKey, rawSpaceKey)
	})
}

func (c *spaceTopicsCollection) SpaceKeysToCreate() []crypto.PrivKey {
	return c.spaceKeysToCreate
}

func compareTopics(a, b *pushapi.Topic) int {
	res := bytes.Compare(a.SpaceKey, b.SpaceKey)
	if res == 0 {
		res = strings.Compare(a.Topic, b.Topic)
	}
	return res
}

func (c *spaceTopicsCollection) MakeApiRequest() *pushapi.Topics {
	slices.SortFunc(c.remoteTopics, compareTopics)
	slices.SortFunc(c.localTopics, compareTopics)
	isEqual := slices.EqualFunc(c.remoteTopics, c.localTopics, func(a *pushapi.Topic, b *pushapi.Topic) bool {
		return bytes.Equal(a.SpaceKey, b.SpaceKey) && a.Topic == b.Topic
	})
	if isEqual {
		return nil
	}
	return &pushapi.Topics{
		Topics: c.localTopics,
	}
}

var errNoKey = errors.New("no key")

func (c *spaceTopicsCollection) EncryptPayload(spaceId string, payload []byte) (keyId string, result []byte, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	status, ok := c.statuses[spaceId]
	if !ok || status.encKey == nil {
		return "", nil, errNoKey
	}
	if result, err = status.encKey.Encrypt(payload); err != nil {
		return
	}
	encKeyRaw, _ := status.encKey.Raw()
	keyHash := sha256.Sum256(encKeyRaw)
	keyId = hex.EncodeToString(keyHash[:])
	return
}

func (c *spaceTopicsCollection) MakeTopics(spaceId string, topics []string) (*pushapi.Topics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	status, ok := c.statuses[spaceId]
	if !ok {
		return nil, errNoKey
	}
	res := &pushapi.Topics{
		Topics: make([]*pushapi.Topic, 0, len(topics)),
	}
	for _, topic := range topics {
		rawKey, err := status.spaceKey.GetPublic().Raw()
		if err != nil {
			return nil, err
		}
		sig, err := status.spaceKey.Sign([]byte(topic))
		if err != nil {
			return nil, err
		}
		res.Topics = append(res.Topics, &pushapi.Topic{
			SpaceKey:  rawKey,
			Topic:     topic,
			Signature: sig,
		})
	}
	return res, nil
}

func sha256hex(id string) string {
	hash := sha256.Sum256([]byte(id))
	return hex.EncodeToString(hash[:])
}
