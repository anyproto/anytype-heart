package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockCreate(req *pb.BlockCreateRequest) *pb.BlockCreateResponse {
	response := func(code pb.BlockCreateResponse_Error_Code, err error) *pb.BlockCreateResponse {
		m := &pb.BlockCreateResponse{Error: &pb.BlockCreateResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	block := &pb.Model_Block{} // TODO

	m := &pb.Event{Message: &pb.Event_BlockCreate{&pb.BlockCreate{Block: block}}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}

	return response(pb.BlockCreateResponse_Error_NULL, nil)
}

func (mw *Middleware) BlockOpen(req *pb.BlockOpenRequest) *pb.BlockOpenResponse {
	response := func(code pb.BlockOpenResponse_Error_Code, err error) *pb.BlockOpenResponse {
		m := &pb.BlockOpenResponse{Error: &pb.BlockOpenResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	block := &pb.Model_Block{} // TODO

	m := &pb.Event{Message: &pb.Event_BlockShow{&pb.BlockShow{Block: block}}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}

	return response(pb.BlockOpenResponse_Error_NULL, nil)
}

func (mw *Middleware) BlockUpdate(req *pb.BlockUpdateRequest) *pb.BlockUpdateResponse {
	response := func(code pb.BlockUpdateResponse_Error_Code, err error) *pb.BlockUpdateResponse {
		m := &pb.BlockUpdateResponse{Error: &pb.BlockUpdateResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	changes := &pb.BlockChanges{} // TODO

	m := &pb.Event{Message: &pb.Event_BlockUpdate{&pb.BlockUpdate{changes}}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}

	return response(pb.BlockUpdateResponse_Error_NULL, nil)
}
