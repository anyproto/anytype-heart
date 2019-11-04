package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockCreate(req *pb.Rpc_Block_Create_Request) *pb.Rpc_Block_Create_Response {
	response := func(code pb.Rpc_Block_Create_Response_Error_Code, err error) *pb.Rpc_Block_Create_Response {
		m := &pb.Rpc_Block_Create_Response{Error: &pb.Rpc_Block_Create_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	/*block := &pb.Model_Block{} // TODO

	m := &pb.Event{Message: &pb.Event_Block_Create{&pb.Rpc_Block_Create{Block: block}}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}*/

	return response(pb.Rpc_Block_Create_Response_Error_NULL, nil)
}

func (mw *Middleware) BlockOpen(req *pb.Rpc_Block_Open_Request) *pb.Rpc_Block_Open_Response {
	response := func(code pb.Rpc_Block_Open_Response_Error_Code, err error) *pb.Rpc_Block_Open_Response {
		m := &pb.Rpc_Block_Open_Response{Error: &pb.Rpc_Block_Open_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	/*block := &pb.Model_Block{} // TODO

	m := &pb.Event{Message: &pb.Event_Block_Show{&pb.Rpc_Block_Show{Block: block}}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}*/

	return response(pb.Rpc_Block_Open_Response_Error_NULL, nil)
}

func (mw *Middleware) BlockUpdate(req *pb.Rpc_Block_Update_Request) *pb.Rpc_Block_Update_Response {
	response := func(code pb.Rpc_Block_Update_Response_Error_Code, err error) *pb.Rpc_Block_Update_Response {
		m := &pb.Rpc_Block_Update_Response{Error: &pb.Rpc_Block_Update_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	/*
		 changes := &pb.Rpc_Block_Changes{} // TODO

		 m := &pb.Event{Message: &pb.Event_Block_Update{&pb.Rpc_Block_Update{changes}}}

		if mw.SendEvent != nil {
			mw.SendEvent(m)
		}*/

	return response(pb.Rpc_Block_Update_Response_Error_NULL, nil)
}
