package clickhouse

type TimeSeries struct {
	name  string
	value float64
}

func TimeSeriesEvent(name string, value float64) Event {
	return &TimeSeries{
		name:  name,
		value: value,
	}
}

func (ts *TimeSeries) table() string {
	return "test"
}

func (ts *TimeSeries) toRecord() []any {
	return []any{ts.name, ts.value}
}
