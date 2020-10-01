package smartblock

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/textileio/go-threads/core/thread"
)

type SmartBlockType uint64

const (
	SmartBlockTypePage        SmartBlockType = 0x10
	SmartBlockTypeProfilePage SmartBlockType = 0x11
	SmartBlockTypeHome        SmartBlockType = 0x20
	SmartBlockTypeArchive     SmartBlockType = 0x30
	SmartBlockTypeDatabase    SmartBlockType = 0x40
	SmartBlockTypeSet         SmartBlockType = 0x41
	SmartBlockTypeObjectType  SmartBlockType = 0x60
)

func SmartBlockTypeFromID(id string) (SmartBlockType, error) {
	tid, err := thread.Decode(id)
	if err != nil {
		return 0, err
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
	default:
		return model.ObjectInfo_Page
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
