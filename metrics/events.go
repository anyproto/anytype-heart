package metrics

import (
	"fmt"
)

type RecordAcceptEventAggregated struct {
	IsNAT      bool
	RecordType string
	Count      int
}

func (r RecordAcceptEventAggregated) ToEvent() Event {
	return Event{
		EventType: "thread_record_accepted",
		EventData: map[string]interface{}{
			"record_type": r.RecordType,
			"is_nat":      r.IsNAT,
			"count":       r.Count,
		},
	}
}

func (r RecordAcceptEventAggregated) Key() string {
	return fmt.Sprintf("RecordAcceptEventAggregated%s%v", r.RecordType, r.IsNAT)
}

func (r RecordAcceptEventAggregated) Aggregate(other EventAggregatable) EventAggregatable {
	ev, ok := other.(RecordAcceptEventAggregated)
	// going here we already check the keys, so let's not do this another time
	if !ok {
		return r
	}
	r.Count += ev.Count
	return r
}

type ChangesetEvent struct {
	Diff int64
}

func (c ChangesetEvent) ToEvent() Event {
	return Event{
		EventType: "changeset_applied",
		EventData: map[string]interface{}{
			// we send diff, and not timestamps of records, because we cannot filter
			// them in Amplitude (unless we will have access to SQL there)
			"diff_current_time_vs_first": c.Diff,
		},
	}
}
