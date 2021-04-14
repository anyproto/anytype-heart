package smartblock

import (
	"fmt"
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
	SmartBlockTypeAnytypeProfile    SmartBlockType = 0x203 // temp

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

	// checks in order to detect invalid sb type
	_, err := SmartBlockType(blockType).toProto()
	if err != nil {
		return 0, err
	}

	return SmartBlockType(blockType), nil
}

// Panics in case of incorrect sb type!
func (sbt SmartBlockType) ToProto() model.ObjectInfoType {
	t, err := sbt.toProto()
	if err != nil {
		panic(err)
	}
	return t
}

func (sbt SmartBlockType) IsValid() bool {
	_, err := sbt.toProto()
	if err != nil {
		return false
	}
	return true
}

func (sbt SmartBlockType) toProto() (model.ObjectInfoType, error) {
	switch sbt {
	case SmartBlockTypePage:
		return model.ObjectInfo_Page, nil
	case SmartBlockTypeProfilePage:
		return model.ObjectInfo_ProfilePage, nil
	case SmartBlockTypeHome:
		return model.ObjectInfo_Home, nil
	case SmartBlockTypeArchive:
		return model.ObjectInfo_Archive, nil
	case SmartBlockTypeSet:
		return model.ObjectInfo_Set, nil
	case SmartblockTypeMarketplaceType:
		return model.ObjectInfo_Set, nil
	case SmartblockTypeMarketplaceTemplate:
		return model.ObjectInfo_Set, nil
	case SmartblockTypeMarketplaceRelation:
		return model.ObjectInfo_Set, nil
	case SmartBlockTypeFile:
		return model.ObjectInfo_File, nil
	case SmartBlockTypeObjectType:
		return model.ObjectInfo_ObjectType, nil
	case SmartBlockTypeBundledObjectType:
		return model.ObjectInfo_ObjectType, nil
	case SmartBlockTypeBundledRelation:
		return model.ObjectInfo_Relation, nil
	case SmartBlockTypeIndexedRelation:
		return model.ObjectInfo_Relation, nil
	case SmartBlockTypeTemplate:
		return model.ObjectInfo_Page, nil
	default:
		return 0, fmt.Errorf("unknown smartblock type")
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
		panic(fmt.Errorf("unknown smartblock type: %v", t))
	}
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
		panic(fmt.Errorf("unknown smartblock type: %v", t))
	}
}
