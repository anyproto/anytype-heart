package subscription

import (
	"context"
	"errors"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

// Neutral transition ops produced by coreSubs under the space mutex and
// encoded into pb event messages off-mutex (see spaceState.deliver). Keeping
// pb out of the core keeps encoding costs off the hot path and the core
// testable without proto fixtures.

type opKind uint8

const (
	opSet opKind = iota
	opAdd
	opRemove
	opPosition
	opAmend
	opUnset
	opCounters
)

type amendKV struct {
	key   string
	value domain.Value
}

type subOp struct {
	sub     *coreSub
	kind    opKind
	id      string
	details *domain.Details // opSet: projection to the sub's keys
	amend   []amendKV       // opAmend
	unset   []string        // opUnset
	afterId string          // opAdd (ordered subs; "" for unordered)
	total   int64           // opCounters
}

// opBatch accumulates the ops of one logical batch; one batch becomes one
// pb.Event for broadcast delivery and one in-order message run per queue
type opBatch struct {
	ops []subOp
}

func (b *opBatch) append(op subOp) {
	b.ops = append(b.ops, op)
}

func encodeOp(op *subOp) *pb.EventMessage {
	subId := op.sub.subId
	spaceId := op.sub.spaceId
	switch op.kind {
	case opSet:
		return event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id:      op.id,
				Details: op.details.ToProto(),
				SubIds:  []string{subId},
			},
		})
	case opAdd:
		return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				Id:      op.id,
				AfterId: op.afterId,
				SubId:   subId,
			},
		})
	case opRemove:
		return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionRemove{
			SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
				Id:    op.id,
				SubId: subId,
			},
		})
	case opPosition:
		return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionPosition{
			SubscriptionPosition: &pb.EventObjectSubscriptionPosition{
				Id:      op.id,
				AfterId: op.afterId,
				SubId:   subId,
			},
		})
	case opAmend:
		kvs := make([]*pb.EventObjectDetailsAmendKeyValue, 0, len(op.amend))
		for _, kv := range op.amend {
			kvs = append(kvs, &pb.EventObjectDetailsAmendKeyValue{
				Key:   kv.key,
				Value: kv.value.ToProto(),
			})
		}
		return event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsAmend{
			ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
				Id:      op.id,
				Details: kvs,
				SubIds:  []string{subId},
			},
		})
	case opUnset:
		return event.NewMessage(spaceId, &pb.EventMessageValueOfObjectDetailsUnset{
			ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
				Id:     op.id,
				Keys:   op.unset,
				SubIds: []string{subId},
			},
		})
	case opCounters:
		return event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				Total: op.total,
				SubId: subId,
			},
		})
	}
	return nil
}

// deliverOps encodes one batch and routes it: broadcast ops of the batch go
// out as a single pb.Event (all events of one logical change in one payload),
// queue ops are appended per queue preserving the batch order. Must be called
// without holding any engine lock.
func deliverOps(ctx context.Context, sender eventSender, ops []subOp) {
	var broadcast []*pb.EventMessage
	var queues []*mb.MB[*pb.EventMessage]
	var queued map[*mb.MB[*pb.EventMessage]][]*pb.EventMessage // lazy: most batches are broadcast-only

	for i := range ops {
		op := &ops[i]
		if q := op.sub.queue; q != nil {
			if op.sub.queueOwned && q.Len() > maxInternalQueueLen {
				log.Errorf("subscription %s: internal queue overflow (>%d messages), closing — consumer stalled",
					op.sub.subId, maxInternalQueueLen)
				_ = q.Close()
				continue
			}
			msg := encodeOp(op)
			if msg == nil {
				continue
			}
			if queued == nil {
				queued = make(map[*mb.MB[*pb.EventMessage]][]*pb.EventMessage)
			}
			if _, ok := queued[q]; !ok {
				queues = append(queues, q)
			}
			queued[q] = append(queued[q], msg)
		} else if msg := encodeOp(op); msg != nil {
			broadcast = append(broadcast, msg)
		}
	}

	if len(broadcast) > 0 && sender != nil {
		sender.Broadcast(&pb.Event{Messages: broadcast})
	}
	for _, q := range queues {
		if err := q.Add(ctx, queued[q]...); err != nil &&
			!errors.Is(err, mb.ErrClosed) && !errors.Is(err, context.Canceled) {
			log.Warnf("subscription: enqueue events: %v", err)
		}
	}
}

// maxInternalQueueLen is the kill watermark for engine-owned internal queues:
// transition events cannot be coalesced after encoding, so a consumer this
// far behind is broken and its queue would otherwise grow without bound.
// Closing the queue terminates the consumer's read loop; caller-provided
// queues are never policed.
const maxInternalQueueLen = 50000

// eventSender is the broadcast half of event.Sender the engine needs
type eventSender interface {
	Broadcast(e *pb.Event)
}
