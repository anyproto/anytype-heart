package smartblock

import (
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/textileio/go-threads/core/thread"
)

type SmartBlockType uint64

const (
	SmartBlockTypePage                SmartBlockType = 0x10
	SmartBlockTypeProfilePage         SmartBlockType = 0x11
	SmartBlockTypeHome                SmartBlockType = 0x20
	SmartBlockTypeArchive             SmartBlockType = 0x30
	SmartBlockTypeDatabase            SmartBlockType = 0x40
	SmartBlockTypeSet                 SmartBlockType = 0x41
	SmartBlockTypeObjectType          SmartBlockType = 0x60
	SmartBlockTypeFile                SmartBlockType = 0x100
	SmartblockTypeMarketplaceType     SmartBlockType = 0x110
	SmartblockTypeMarketplaceRelation SmartBlockType = 0x111
	SmartblockTypeMarketplaceTemplate SmartBlockType = 0x112
	SmartBlockTypeTemplate            SmartBlockType = 0x120

	SmartBlockTypeBundledRelation   SmartBlockType = 0x200 // temp
	SmartBlockTypeIndexedRelation   SmartBlockType = 0x201 // temp
	SmartBlockTypeBundledObjectType SmartBlockType = 0x202 // temp

)

func SmartBlockTypeFromID(id string) (SmartBlockType, error) {
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return SmartBlockTypeBundledRelation, nil
	}
	if strings.HasPrefix(id, addr.CustomRelationURLPrefix) {
		return SmartBlockTypeIndexedRelation, nil
	}
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return SmartBlockTypeBundledObjectType, nil
	}
	if strings.HasPrefix(id, addr.AnytypeProfileId) {
		return SmartBlockTypeProfilePage, nil
	}

	c, err := cid.Decode(id)
	// TODO: discard this fragile condition as soon as we will move to the multiaddr with prefix
	if err == nil && c.Prefix().Codec == 0x70 && c.Prefix().MhType == multihash.SHA2_256 {
		return SmartBlockTypeFile, nil
	}

	tid, err := thread.Decode(id)
	if err != nil {
		return SmartBlockTypePage, err
	}

	return SmartBlockTypeFromThreadID(tid)
}

func SmartBlockTypeFromThreadID(tid thread.ID) (SmartBlockType, error) {
	rawid := tid.KeyString()
	// skip version
	_, n := uvarint(rawid)
	// skip variant
	_, n2 := uvarint(rawid[n:])
	blockType, _ := uvarint(rawid[n+n2:])

	return SmartBlockType(blockType), nil
}

func (sbt SmartBlockType) ToProto() model.ObjectInfoType {
	switch sbt {
	case SmartBlockTypePage:
		return model.ObjectInfo_Page
	case SmartBlockTypeProfilePage:
		return model.ObjectInfo_ProfilePage
	case SmartBlockTypeHome:
		return model.ObjectInfo_Home
	case SmartBlockTypeArchive:
		return model.ObjectInfo_Archive
	case SmartBlockTypeSet:
		return model.ObjectInfo_Set
	case SmartblockTypeMarketplaceType:
		return model.ObjectInfo_Set
	case SmartblockTypeMarketplaceTemplate:
		return model.ObjectInfo_Set
	case SmartblockTypeMarketplaceRelation:
		return model.ObjectInfo_Set
	case SmartBlockTypeFile:
		return model.ObjectInfo_File
	case SmartBlockTypeObjectType:
		return model.ObjectInfo_ObjectType
	case SmartBlockTypeBundledObjectType:
		return model.ObjectInfo_ObjectType
	case SmartBlockTypeBundledRelation:
		return model.ObjectInfo_Relation
	case SmartBlockTypeIndexedRelation:
		return model.ObjectInfo_Relation
	case SmartBlockTypeTemplate:
		return model.ObjectInfo_Page
	default:
		panic("unknown smartblock type")
	}
}

// Snapshot of varint function that work with a string rather than
// []byte to avoid unnecessary allocation

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license as given at https://golang.org/LICENSE

// uvarint decodes a uint64 from buf and returns that value and the
// number of characters read (> 0). If an error occurred, the value is 0
// and the number of bytes n is <= 0 meaning:
//
// 	n == 0: buf too small
// 	n  < 0: value larger than 64 bits (overflow)
// 	        and -n is the number of bytes read
//
func uvarint(buf string) (uint64, int) {
	var x uint64
	var s uint
	// we have a binary string so we can't use a range loope
	for i := 0; i < len(buf); i++ {
		b := buf[i]
		if b < 0x80 {
			if i > 9 || i == 9 && b > 1 {
				return 0, -(i + 1) // overflow
			}
			return x | uint64(b)<<s, i + 1
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
	return 0, 0
}

func SmartBlockTypeToProto(t SmartBlockType) pb.SmartBlockType {
	switch t {
	case SmartBlockTypePage:
		return pb.SmartBlockType_Page
	case SmartBlockTypeArchive:
		return pb.SmartBlockType_Archive
	case SmartBlockTypeHome:
		return pb.SmartBlockType_Home
	case SmartBlockTypeProfilePage:
		return pb.SmartBlockType_ProfilePage
	case SmartBlockTypeSet:
		return pb.SmartBlockType_Set
	case SmartBlockTypeObjectType:
		return pb.SmartBlockType_ObjectType
	case SmartBlockTypeBundledObjectType:
		return pb.SmartBlockType_ObjectType
	case SmartBlockTypeBundledRelation:
		return pb.SmartBlockType_Relation
	case SmartBlockTypeIndexedRelation:
		return pb.SmartBlockType_Relation
	case SmartblockTypeMarketplaceRelation:
		return pb.SmartBlockType_MarketplaceRelation
	case SmartblockTypeMarketplaceType:
		return pb.SmartBlockType_MarketplaceType
	case SmartblockTypeMarketplaceTemplate:
		return pb.SmartBlockType_MarketplaceTemplate
	case SmartBlockTypeTemplate:
		return pb.SmartBlockType_Template
	default:
		panic("unknown smartblock type")
	}
	return 0
}

func SmartBlockTypeToCore(t pb.SmartBlockType) SmartBlockType {
	switch t {
	case pb.SmartBlockType_Page:
		return SmartBlockTypePage
	case pb.SmartBlockType_Archive:
		return SmartBlockTypeArchive
	case pb.SmartBlockType_Home:
		return SmartBlockTypeHome
	case pb.SmartBlockType_ProfilePage:
		return SmartBlockTypeProfilePage
	case pb.SmartBlockType_Set:
		return SmartBlockTypeSet
	case pb.SmartBlockType_ObjectType:
		return SmartBlockTypeObjectType
	case pb.SmartBlockType_MarketplaceType:
		return SmartblockTypeMarketplaceType
	case pb.SmartBlockType_MarketplaceRelation:
		return SmartblockTypeMarketplaceRelation
	case pb.SmartBlockType_Template:
		return SmartBlockTypeTemplate
	default:
		panic("unknown smartblock type")
	}
	return 0
}
