package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
	coresb "github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func NewService(c core.Service) Service {
	return &service{c}
}

type service struct {
	core.Service
}

func SmartBlockTypeToProto(t coresb.SmartBlockType) pb.SmartBlockType {
	switch t {
	case coresb.SmartBlockTypePage:
		return pb.SmartBlockType_Page
	case coresb.SmartBlockTypeArchive:
		return pb.SmartBlockType_Archive
	case coresb.SmartBlockTypeHome:
		return pb.SmartBlockType_Home
	case coresb.SmartBlockTypeProfilePage:
		return pb.SmartBlockType_ProfilePage
	case coresb.SmartBlockTypeSet:
		return pb.SmartBlockType_Set
	}
	return 0
}

func SmartBlockTypeToCore(t pb.SmartBlockType) coresb.SmartBlockType {
	switch t {
	case pb.SmartBlockType_Page:
		return coresb.SmartBlockTypePage
	case pb.SmartBlockType_Archive:
		return coresb.SmartBlockTypeArchive
	case pb.SmartBlockType_Home:
		return coresb.SmartBlockTypeHome
	case pb.SmartBlockType_ProfilePage:
		return coresb.SmartBlockTypeProfilePage
	case pb.SmartBlockType_Set:
		return coresb.SmartBlockTypeSet
	}
	return 0
}
