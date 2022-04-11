package multids

import (
	"context"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-datastore/query"
	"github.com/jbenet/goprocess"
	"sync"
)

// newResultsCombiner returns combined results. order not guranteed. in race conds it can returns +1 entries
func newResultsCombiner(results ...query.Results) query.Results {
	r := &resultsCombiner{results: results}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	return r
}

type resultsCombiner struct {
	ctx     context.Context
	cancel  context.CancelFunc
	results []query.Results
}

func (r resultsCombiner) Query() query.Query {
	return r.results[0].Query()
}

func (r resultsCombiner) nextUnordered() <-chan query.Result {
	out := make(chan query.Result)
	var (
		total          int
		m              sync.Mutex
		limitExhausted = make(chan struct{})
	)

	limit := r.results[0].Query().Limit
	for _, results := range r.results {
		go func(res query.Results) {
			var more bool
			var v query.Result
			select {
			case v, more = <-res.Next():
				if !more {
					break
				}
				m.Lock()
				if total >= limit {
					m.Unlock()
					break
				}
				total++
				if total+1 == limit {
					close(limitExhausted)
				}

				m.Unlock()
				out <- v
			case <-r.ctx.Done():
				break
			case <-limitExhausted:
				break
			}
		}(results)
	}

	return out
}

func (r resultsCombiner) Next() <-chan query.Result {
	if len(r.results[0].Query().Orders) == 0 {
		return r.nextUnordered()
	}

	out := make(chan query.Result)
	entries, err := r.Rest()
	if err != nil {
		go func() {
			out <- query.Result{
				Entry: query.Entry{},
				Error: err,
			}
		}()
		return out
	}

	go func() {
		for _, entry := range entries {
			select {
			case out <- query.Result{
				Entry: entry,
			}:
			case <-r.ctx.Done():
			}
		}
	}()

	return out
}

func (r resultsCombiner) NextSync() (query.Result, bool) {
	for _, res := range r.results {
		v, more := res.NextSync()
		if more {
			return v, more
		}
	}
	return query.Result{}, false
}

func (r resultsCombiner) Rest() ([]query.Entry, error) {
	var entries = make([]query.Entry, 0)
	for _, res := range r.results {
		entriesPart, err := res.Rest()
		if err != nil {
			return nil, err
		}
		entries = append(entries, entriesPart...)
	}

	query.Sort(r.results[0].Query().Orders, entries)

	if len(entries) > r.results[0].Query().Limit {
		entries = entries[0:r.results[0].Query().Limit]
	}
	return entries, nil
}

func (r resultsCombiner) Close() error {
	r.cancel()
	var err error
	merr := multierror.Error{}
	for _, res := range r.results {
		err = res.Close()
		if err != nil {
			merr.Errors = append(merr.Errors, err)
		}
	}

	return merr.ErrorOrNil()
}

func (r resultsCombiner) Process() goprocess.Process {
	// todo
	return nil
}
