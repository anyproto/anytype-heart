package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockCreate(req *pb.BlockCreateRequest) *pb.BlockCreateResponse {
	response := func(code pb.BlockCreateResponse_Error_Code, err error) *pb.BlockCreateResponse {
		m := &pb.BlockCreateResponse{Block: block, Error: &pb.BlockCreateResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	block := &pb.Block{} // TODO

	m := &pb.Event{Message: &pb.Event_BlockCreateEvent{Block: block}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}

	return response(pb.BlockCreateResponse_Error_NULL, nil)
}

func (mw *Middleware) BlockRead(req *pb.BlockReadRequest) *pb.BlockReadResponse {
	response := func(code pb.BlockReadResponse_Error_Code, err error) *pb.BlockReadResponse {
		m := &pb.BlockReadResponse{Error: &pb.BlockReadResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	block := &pb.Block{} // TODO

	m := &pb.Event{Message: &pb.Event_BlockReadEvent{Block: block}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}

	return response(pb.BlockReadResponse_Error_NULL, nil)
}

func (mw *Middleware) BlockUpdate(req *pb.BlockUpdateRequest) *pb.BlockUpdateResponse {
	response := func(code pb.BlockUpdateResponse_Error_Code, err error) *pb.BlockUpdateResponse {
		m := &pb.BlockUpdateResponse{Error: &pb.BlockUpdateResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	changes := &pb.Changes{} // TODO

	m := &pb.Event{Message: &pb.Event_BlockUpdateEvent{Changes: changes}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}

	return response(pb.BlockUpdateResponse_Error_NULL, nil)
}
