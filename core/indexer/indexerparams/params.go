package indexerparams

type IndexOptions struct {
	SkipIfHeadsNotChanged         bool
	SkipFullTextIfHeadsNotChanged bool
	Batch                         *IndexBatch
}

type IndexBatch struct {
	id   string
	done chan struct{}
}

func NewIndexBatch(id string) *IndexBatch {
	return &IndexBatch{
		id:   id,
		done: make(chan struct{}),
	}
}

func (b *IndexBatch) Wait() {
	<-b.done
}

func (b *IndexBatch) Done() {
	close(b.done)
}

type IndexOption func(*IndexOptions)

func SkipIfHeadsNotChanged(o *IndexOptions) {
	o.SkipIfHeadsNotChanged = true
}

func SkipFullTextIfHeadsNotChanged(o *IndexOptions) {
	o.SkipFullTextIfHeadsNotChanged = true
}

func WithIndexBatch(batch *IndexBatch) IndexOption {
	return func(o *IndexOptions) {
		o.Batch = batch
	}
}
