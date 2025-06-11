package pushnotification

import (
	"slices"
)

const ChatsTopicName = "chats"

var chatTopics = []string{ChatsTopicName}

type SpaceTopics struct {
	spaceId   string
	spaceKeys *spaceKeys

	topics topicSet

	needUpdateTopics bool
	needCreateSpace  bool
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
