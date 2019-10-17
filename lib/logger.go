package lib

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

func Log(b []byte) []byte {
	response := func(code pb.LogResponse_Error_Code, err error) []byte {
		m := &pb.LogResponse{}
		if code != pb.LogResponse_Error_NULL {
			m.Error = &pb.LogResponse_Error{Code: code}
			if err != nil {
				m.Error.Description = err.Error()
			}
		}

		return Marshal(m)
	}

	var q pb.LogRequest
	err := proto.Unmarshal(b, &q)

	if err != nil {
		return response(pb.LogResponse_Error_BAD_INPUT, err)
	}

	switch q.Level {
	case pb.LogRequest_FATAL:
		log.Fatal(q.Message)
	case pb.LogRequest_PANIC:
		log.Panic(q.Message)
	case pb.LogRequest_DEBUG:
		log.Debug(q.Message)
	case pb.LogRequest_INFO:
		log.Info(q.Message)
	case pb.LogRequest_WARNING:
		log.Warning(q.Message)
	case pb.LogRequest_ERROR:
		log.Error(q.Message)
	}

	return response(pb.LogResponse_Error_NULL, nil)
}
