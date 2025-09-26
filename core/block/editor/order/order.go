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
