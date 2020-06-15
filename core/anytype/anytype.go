package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func NewService(c core.Service) Service {
	return &service{c}
}

type service struct {
	core.Service
}

func SmartBlockTypeToProto(t core.SmartBlockType) pb.SmartBlockType {
	switch t {
	case core.SmartBlockTypePage:
		return pb.SmartBlockType_Page
	case core.SmartBlockTypeArchive:
		return pb.SmartBlockType_Archive
	case core.SmartBlockTypeHome:
		return pb.SmartBlockType_Home
	case core.SmartBlockTypeProfilePage:
		return pb.SmartBlockType_ProfilePage
	}
	return 0
}

func SmartBlockTypeToCore(t pb.SmartBlockType) core.SmartBlockType {
	switch t {
	case pb.SmartBlockType_Page:
		return core.SmartBlockTypePage
	case pb.SmartBlockType_Archive:
		return core.SmartBlockTypeArchive
	case pb.SmartBlockType_Home:
		return core.SmartBlockTypeHome
	case pb.SmartBlockType_ProfilePage:
		return core.SmartBlockTypeProfilePage
	}
	return 0
}
