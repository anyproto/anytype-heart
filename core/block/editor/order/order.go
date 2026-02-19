package order

import (
	"github.com/anyproto/lexid"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
)

var (
	LexId = lexid.Must(lexid.CharsBase64, 4, 4000)
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
	st.SetChangeType(domain.ChangeTypeOrderOperation)
	return s.Apply(st)
}

func (s *orderSettable) UnsetOrder() error {
	st := s.NewState()
	st.RemoveDetail(s.orderKey)
	st.SetChangeType(domain.ChangeTypeOrderOperation)
	return s.Apply(st)
}

func (s *orderSettable) GetOrder() string {
	return s.Details().GetString(s.orderKey)
}

func GetSmallestOrder(currentSmallestOrder string) string {
	if currentSmallestOrder == "" {
		return LexId.Middle()
	}
	return LexId.Prev(currentSmallestOrder)
}
