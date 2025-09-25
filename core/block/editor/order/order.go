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
	SetOrder(orderId string) error
	SetNextOrder(previousOrderId string) (string, error)
	SetAfterOrder(orderId string) error
	SetBetweenOrders(previousOrderId, afterOrderId string) (string, error)
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

func (s *orderSettable) SetOrder(orderId string) error {
	st := s.NewState()
	st.SetDetail(s.orderKey, domain.String(orderId))
	return s.Apply(st)
}

func (s *orderSettable) SetNextOrder(previousOrderId string) (string, error) {
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

func (s *orderSettable) SetBetweenOrders(left, right string) (string, error) {
	var (
		between string
		err     error
	)
	if left == "" {
		// Insert before the first existing element
		between = lx.Prev(right)
	} else {
		// Insert between two existing elements
		between, err = lx.NextBefore(left, right)
	}

	if err != nil {
		return "", errors.Join(ErrLexidInsertionFailed, err)
	}

	st := s.NewState()
	st.SetDetail(s.orderKey, domain.String(between))
	return between, s.Apply(st)
}

func (s *orderSettable) UnsetOrder() error {
	st := s.NewState()
	st.RemoveDetail(s.orderKey)
	return s.Apply(st)
}

func (s *orderSettable) GetOrder() string {
	return s.Details().GetString(s.orderKey)
}

func GetSmallestOrder(currentSmallestOrder string) string {
	return lx.Prev(currentSmallestOrder)
}
