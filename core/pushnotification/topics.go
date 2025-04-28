package pushnotification

type TopicSet struct {
	topics map[string]struct{}
}

func NewTopicSet() TopicSet {
	return TopicSet{topics: make(map[string]struct{})}
}

func (ts *TopicSet) Add(topic string) (added bool) {
	if _, ok := ts.topics[topic]; ok {
		return false
	}
	ts.topics[topic] = struct{}{}
	return true
}

func (ts *TopicSet) Slice() []string {
	out := make([]string, 0, len(ts.topics))
	for t := range ts.topics {
		out = append(out, t)
	}
	return out
}
