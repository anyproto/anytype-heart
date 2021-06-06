package metrics

type RecordAcceptEvent struct {
	IsNAT      bool
	recordType string
}

func (r RecordAcceptEvent) ToEvent() Event {
	return Event{
		EventType: "threads_record_accepted",
		EventData: map[string]interface{}{
			"record_type": r.recordType,
			"is_nat":      r.IsNAT,
		},
	}
}
