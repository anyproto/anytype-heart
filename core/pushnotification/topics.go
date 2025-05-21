package pushnotification

import "slices"

type TopicSet struct {
	topics map[string]struct{}
}

func NewTopicSet() TopicSet {
	return TopicSet{topics: make(map[string]struct{})}
}

func (ts *TopicSet) Set(topics ...string) (changed bool) {
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

func (ts *TopicSet) Slice() []string {
	out := make([]string, 0, len(ts.topics))
	for t := range ts.topics {
		out = append(out, t)
	}
	return out
}

func (ts *TopicSet) Add(topic string) {
	ts.topics[topic] = struct{}{}
}

func (ts *TopicSet) Len() int {
	return len(ts.topics)
}
