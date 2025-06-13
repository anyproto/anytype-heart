package pushnotification

import (
	"slices"

	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
)

const ChatsTopicName = "chats"

var chatTopics = []string{ChatsTopicName}

func newSpaceTopicsCollection() *spaceTopicsCollection {
	return &spaceTopicsCollection{
		spaceTopicsBySpaceKey: map[string]*SpaceTopics{},
		spaceTopicsBySpaceId:  map[string]*SpaceTopics{},
	}
}

type spaceTopicsCollection struct {
	spaceTopicsBySpaceKey map[string]*SpaceTopics
	spaceTopicsBySpaceId  map[string]*SpaceTopics
}

func (c *spaceTopicsCollection) SetRemoteList(remoteTopics *pushapi.Topics) {
	for _, remoteTopic := range remoteTopics.Topics {
		topic := c.getSpaceTopic(string(remoteTopic.SpaceKey))
		topic.topics.Set(remoteTopic.Topic)
	}
}

func (c *spaceTopicsCollection) SetSpaceViewStatus(status *spaceViewStatus) {
	/*
		if status.spaceKey == nil || status.encKey == nil {
			return
		}

		spaceKey, err := status.spaceKey.GetPublic().Raw()
		if err != nil {
			log.Warn("failed to get space key raw", zap.Error(err))
			return
		}

		 topic := c.getSpaceTopic(string(spaceKey))
	*/
}

func (c *spaceTopicsCollection) getSpaceTopic(spaceKey string) *SpaceTopics {
	if topic, ok := c.spaceTopicsBySpaceKey[spaceKey]; ok {
		return topic
	} else {
		topic = &SpaceTopics{
			topics: newTopicSet(),
		}
		c.spaceTopicsBySpaceKey[spaceKey] = topic
		return topic
	}
}

type SpaceTopics struct {
	spaceId string

	spaceViewStatus *spaceViewStatus

	topics topicSet

	needUpdateTopics bool
	needCreateSpace  bool
	needUpdateEncKey bool
}

type topicSet struct {
	topics map[string]struct{}
}

func newTopicSet() topicSet {
	return topicSet{topics: make(map[string]struct{})}
}

func (ts *topicSet) Set(topics ...string) (changed bool) {
	for topic := range ts.topics {
		if !slices.Contains(topics, topic) {
			delete(ts.topics, topic)
			changed = true
		}
	}
	for _, topic := range topics {
		if _, ok := ts.topics[topic]; !ok {
			ts.topics[topic] = struct{}{}
			changed = true
		}
	}
	return
}

func (ts *topicSet) Slice() []string {
	out := make([]string, 0, len(ts.topics))
	for t := range ts.topics {
		out = append(out, t)
	}
	return out
}

func (ts *topicSet) Add(topic string) {
	ts.topics[topic] = struct{}{}
}

func (ts *topicSet) Len() int {
	return len(ts.topics)
}
