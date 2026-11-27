//go:build darwin

package store

import (
	"encoding/binary"
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// getattrlist(2) and fgetattrlist(2) from libSystem. x/sys exports no
// wrapper for them, so they are bound here the way x/sys binds libc on
// darwin: a dynamic import and an assembly trampoline (acl_darwin.s),
// called through the runtime's libc call path. No cgo, and no raw syscall
// numbers, which Apple does not keep stable.

//go:cgo_import_dynamic libc_getattrlist getattrlist "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_fgetattrlist fgetattrlist "/usr/lib/libSystem.B.dylib"

var (
	libc_getattrlist_trampoline_addr  uintptr //nolint:revive,stylecheck // the linker's name
	libc_fgetattrlist_trampoline_addr uintptr //nolint:revive,stylecheck // the linker's name
)

//go:linkname syscall_syscall6 syscall.syscall6
func syscall_syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno) //nolint:revive,stylecheck

// macOS evaluates ACL entries before the mode bits, so a folder that reads
// 0700 can still be open to another user (Finder's "Sharing & Permissions"
// writes inheritable allow entries). The ACL is read with getattrlist and
// ATTR_CMN_EXTENDED_SECURITY, without cgo, and is not interpreted beyond
// counting its entries and how many of them allow.

const (
	// sizes in a kauth_filesec: magic, owner and group GUIDs, then the
	// ACL's entry count and flags, then 24-byte entries
	filesecHeader = 4 + 16 + 16
	aclHeader     = 4 + 4
	aceSize       = 16 + 4 + 4
	// KAUTH_FILESEC_NOACL
	noACL = 0xFFFFFFFF
	// ace_flags: the kind is in the low four bits
	aceKindMask = 0xF
	aceKindDeny = 2

	aclBuffer = 64 << 10
)

func attrList() unix.Attrlist {
	return unix.Attrlist{Bitmapcount: unix.ATTR_BIT_MAP_COUNT, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
}

func pathACL(path string) (aclInfo, error) {
	p, err := unix.BytePtrFromString(path)
	if err != nil {
		return aclInfo{}, err
	}
	list := attrList()
	buf := make([]byte, aclBuffer)
	_, _, errno := syscall_syscall6(libc_getattrlist_trampoline_addr, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&list)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), unix.FSOPT_NOFOLLOW, 0)
	if errno != 0 {
		return aclInfo{}, errno
	}
	return parseFilesec(buf)
}

func fileACL(fh *os.File) (aclInfo, error) {
	list := attrList()
	buf := make([]byte, aclBuffer)
	_, _, errno := syscall_syscall6(libc_fgetattrlist_trampoline_addr, fh.Fd(), uintptr(unsafe.Pointer(&list)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
	if errno != 0 {
		return aclInfo{}, errno
	}
	return parseFilesec(buf)
}

// parseFilesec reads getattrlist's reply: a length, then one attribute
// reference (an offset from the reference itself, and a length) to the
// kauth_filesec. A reply that can't be read counts as an ACL nobody
// understood, which is an error, never "no ACL".
func parseFilesec(buf []byte) (aclInfo, error) {
	bad := errors.New("unreadable extended security attribute")
	if len(buf) < 12 {
		return aclInfo{}, bad
	}
	total := int(binary.LittleEndian.Uint32(buf[0:4]))
	offset := int(int32(binary.LittleEndian.Uint32(buf[4:8]))) //nolint:gosec // attr_dataoffset is a signed 32-bit field
	length := int(binary.LittleEndian.Uint32(buf[8:12]))
	if total > len(buf) || total < 12 {
		return aclInfo{}, bad
	}
	if length == 0 {
		return aclInfo{}, nil // no extended security at all
	}
	start := 4 + offset
	if offset < 8 || start+length > total || length < filesecHeader+aclHeader {
		return aclInfo{}, bad
	}
	sec := buf[start : start+length]
	count := binary.LittleEndian.Uint32(sec[filesecHeader : filesecHeader+4])
	if count == noACL {
		return aclInfo{}, nil
	}
	entries := sec[filesecHeader+aclHeader:]
	if int(count) > len(entries)/aceSize {
		return aclInfo{}, bad
	}
	info := aclInfo{entries: int(count)}
	for i := 0; i < int(count); i++ {
		flags := binary.LittleEndian.Uint32(entries[i*aceSize+16 : i*aceSize+20])
		if flags&aceKindMask != aceKindDeny {
			info.allow++
		}
	}
	return info, nil
}
