package order

import (
	"errors"

	"github.com/anyproto/lexid"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
)

var (
	ErrLexidInsertionFailed = errors.New("lexid insertion failed")
	lx                      = lexid.Must(lexid.CharsBase64, 4, 4000)
)

type OrderSettable interface {
	GetOrder() string
	SetOrder(previousOrderId string) (string, error)
	SetAfterOrder(orderId string) error
	SetBetweenOrders(previousOrderId, afterOrderId string) error
	UnsetOrder() error
}

func NewOrderSettable(sb smartblock.SmartBlock, orderKey domain.RelationKey) OrderSettable {
	return &orderSettable{
		SmartBlock: sb,
		orderKey:   orderKey,
	}
}

type orderSettable struct {
	smartblock.SmartBlock
	orderKey domain.RelationKey
}

func (s *orderSettable) SetOrder(previousOrderId string) (string, error) {
	st := s.NewState()
	var newOrderId string
	if previousOrderId == "" {
		// For the first element, use a lexid with huge padding
		newOrderId = lx.Middle()
	} else {
		newOrderId = lx.Next(previousOrderId)
	}
	st.SetDetail(s.orderKey, domain.String(newOrderId))
	return newOrderId, s.Apply(st)
}

func (s *orderSettable) SetAfterOrder(orderId string) error {
	st := s.NewState()
	currentOrderId := st.Details().GetString(s.orderKey)
	if orderId > currentOrderId {
		currentOrderId = lx.Next(orderId)
		st.SetDetail(s.orderKey, domain.String(currentOrderId))
		return s.Apply(st)
	}
	return nil
}

func (s *orderSettable) SetBetweenOrders(previousOrderId, afterOrderId string) error {
	st := s.NewState()
	var before string
	var err error

	if previousOrderId == "" {
		// Insert before the first existing element
		before = lx.Prev(afterOrderId)
	} else {
		// Insert between two existing elements
		before, err = lx.NextBefore(previousOrderId, afterOrderId)
	}

	if err != nil {
		return errors.Join(ErrLexidInsertionFailed, err)
	}
	st.SetDetail(s.orderKey, domain.String(before))
	return s.Apply(st)
}

func (s *orderSettable) UnsetOrder() error {
	st := s.NewState()
	st.RemoveDetail(s.orderKey)
	return s.Apply(st)
}

func (s *orderSettable) GetOrder() string {
	return s.Details().GetString(s.orderKey)
}
