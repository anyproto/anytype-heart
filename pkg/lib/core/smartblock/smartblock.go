package smartblock

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/textileio/go-threads/core/thread"
)

type SmartBlockType uint64

const (
	SmartBlockTypePage                = SmartBlockType(model.SmartBlockType_Page)
	SmartBlockTypeProfilePage         = SmartBlockType(model.SmartBlockType_ProfilePage)
	SmartBlockTypeHome                = SmartBlockType(model.SmartBlockType_Home)
	SmartBlockTypeArchive             = SmartBlockType(model.SmartBlockType_Archive)
	SmartBlockTypeDatabase            = SmartBlockType(model.SmartBlockType_Database)
	SmartBlockTypeSet                 = SmartBlockType(model.SmartBlockType_Set)
	SmartBlockTypeObjectType          = SmartBlockType(model.SmartBlockType_ObjectType)
	SmartBlockTypeFile                = SmartBlockType(model.SmartBlockType_File)
	SmartblockTypeMarketplaceType     = SmartBlockType(model.SmartBlockType_MarketplaceType)
	SmartblockTypeMarketplaceRelation = SmartBlockType(model.SmartBlockType_MarketplaceRelation)
	SmartblockTypeMarketplaceTemplate = SmartBlockType(model.SmartBlockType_MarketplaceTemplate)
	SmartBlockTypeTemplate            = SmartBlockType(model.SmartBlockType_Template)
	SmartBlockTypeBundledRelation     = SmartBlockType(model.SmartBlockType_BundledRelation)
	SmartBlockTypeIndexedRelation     = SmartBlockType(model.SmartBlockType_IndexedRelation)
	SmartBlockTypeBundledObjectType   = SmartBlockType(model.SmartBlockType_BundledObjectType)
	SmartBlockTypeAnytypeProfile      = SmartBlockType(model.SmartBlockType_AnytypeProfile)
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
