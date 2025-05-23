// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: pb/protos/snapshot.proto

package pb

import (
	fmt "fmt"
	model "github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	proto "github.com/gogo/protobuf/proto"
	io "io"
	math "math"
	math_bits "math/bits"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

type SnapshotWithType struct {
	SbType   model.SmartBlockType `protobuf:"varint,1,opt,name=sbType,proto3,enum=anytype.model.SmartBlockType" json:"sbType,omitempty"`
	Snapshot *ChangeSnapshot      `protobuf:"bytes,2,opt,name=snapshot,proto3" json:"snapshot,omitempty"`
}

func (m *SnapshotWithType) Reset()         { *m = SnapshotWithType{} }
func (m *SnapshotWithType) String() string { return proto.CompactTextString(m) }
func (*SnapshotWithType) ProtoMessage()    {}
func (*SnapshotWithType) Descriptor() ([]byte, []int) {
	return fileDescriptor_022f1596c727bff6, []int{0}
}
func (m *SnapshotWithType) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *SnapshotWithType) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_SnapshotWithType.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *SnapshotWithType) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SnapshotWithType.Merge(m, src)
}
func (m *SnapshotWithType) XXX_Size() int {
	return m.Size()
}
func (m *SnapshotWithType) XXX_DiscardUnknown() {
	xxx_messageInfo_SnapshotWithType.DiscardUnknown(m)
}

var xxx_messageInfo_SnapshotWithType proto.InternalMessageInfo

func (m *SnapshotWithType) GetSbType() model.SmartBlockType {
	if m != nil {
		return m.SbType
	}
	return model.SmartBlockType_AccountOld
}

func (m *SnapshotWithType) GetSnapshot() *ChangeSnapshot {
	if m != nil {
		return m.Snapshot
	}
	return nil
}

type Profile struct {
	Name             string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Avatar           string `protobuf:"bytes,2,opt,name=avatar,proto3" json:"avatar,omitempty"`
	Address          string `protobuf:"bytes,4,opt,name=address,proto3" json:"address,omitempty"`
	SpaceDashboardId string `protobuf:"bytes,5,opt,name=spaceDashboardId,proto3" json:"spaceDashboardId,omitempty"`
	ProfileId        string `protobuf:"bytes,6,opt,name=profileId,proto3" json:"profileId,omitempty"`
	AnalyticsId      string `protobuf:"bytes,7,opt,name=analyticsId,proto3" json:"analyticsId,omitempty"`
	StartingPage     string `protobuf:"bytes,8,opt,name=startingPage,proto3" json:"startingPage,omitempty"`
}

func (m *Profile) Reset()         { *m = Profile{} }
func (m *Profile) String() string { return proto.CompactTextString(m) }
func (*Profile) ProtoMessage()    {}
func (*Profile) Descriptor() ([]byte, []int) {
	return fileDescriptor_022f1596c727bff6, []int{1}
}
func (m *Profile) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Profile) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Profile.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Profile) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Profile.Merge(m, src)
}
func (m *Profile) XXX_Size() int {
	return m.Size()
}
func (m *Profile) XXX_DiscardUnknown() {
	xxx_messageInfo_Profile.DiscardUnknown(m)
}

var xxx_messageInfo_Profile proto.InternalMessageInfo

func (m *Profile) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Profile) GetAvatar() string {
	if m != nil {
		return m.Avatar
	}
	return ""
}

func (m *Profile) GetAddress() string {
	if m != nil {
		return m.Address
	}
	return ""
}

func (m *Profile) GetSpaceDashboardId() string {
	if m != nil {
		return m.SpaceDashboardId
	}
	return ""
}

func (m *Profile) GetProfileId() string {
	if m != nil {
		return m.ProfileId
	}
	return ""
}

func (m *Profile) GetAnalyticsId() string {
	if m != nil {
		return m.AnalyticsId
	}
	return ""
}

func (m *Profile) GetStartingPage() string {
	if m != nil {
		return m.StartingPage
	}
	return ""
}

func init() {
	proto.RegisterType((*SnapshotWithType)(nil), "anytype.SnapshotWithType")
	proto.RegisterType((*Profile)(nil), "anytype.Profile")
}

func init() { proto.RegisterFile("pb/protos/snapshot.proto", fileDescriptor_022f1596c727bff6) }

var fileDescriptor_022f1596c727bff6 = []byte{
	// 333 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x91, 0x41, 0x4b, 0xc3, 0x30,
	0x18, 0x86, 0xd7, 0x31, 0xdb, 0x2d, 0x13, 0x19, 0x39, 0x68, 0x18, 0xb3, 0x8c, 0xe1, 0x61, 0x78,
	0x68, 0x61, 0xea, 0x1f, 0x98, 0x5e, 0x76, 0x1b, 0x9d, 0x20, 0x78, 0xfb, 0xd2, 0xc4, 0xb6, 0xac,
	0x6b, 0x42, 0x12, 0x84, 0x9e, 0xfc, 0x0b, 0xfe, 0x2c, 0x8f, 0x3b, 0x7a, 0x53, 0xb6, 0x3f, 0x22,
	0xcb, 0xda, 0x4d, 0xf1, 0xf6, 0xbd, 0xef, 0xf7, 0x7c, 0xef, 0x1b, 0x08, 0x22, 0x92, 0x86, 0x52,
	0x09, 0x23, 0x74, 0xa8, 0x0b, 0x90, 0x3a, 0x15, 0x26, 0xb0, 0x1a, 0x7b, 0x50, 0x94, 0xa6, 0x94,
	0xbc, 0x7f, 0x25, 0x97, 0x49, 0x98, 0x67, 0x34, 0x94, 0x34, 0x5c, 0x09, 0xc6, 0xf3, 0xfa, 0xc0,
	0x0a, 0xbd, 0xc7, 0xfb, 0x17, 0xc7, 0xa0, 0x38, 0x85, 0x22, 0xe1, 0xd5, 0x62, 0xf4, 0x86, 0x7a,
	0x8b, 0x2a, 0xf9, 0x29, 0x33, 0xe9, 0x63, 0x29, 0x39, 0xbe, 0x43, 0xae, 0xa6, 0xbb, 0x89, 0x38,
	0x43, 0x67, 0x7c, 0x36, 0xb9, 0x0c, 0xaa, 0xb2, 0xc0, 0x66, 0x06, 0x8b, 0x15, 0x28, 0x33, 0xcd,
	0x45, 0xbc, 0xdc, 0x41, 0x51, 0x05, 0xe3, 0x5b, 0xd4, 0xae, 0x1f, 0x49, 0x9a, 0x43, 0x67, 0xdc,
	0x9d, 0x90, 0xc3, 0xe1, 0xbd, 0x2d, 0x0d, 0xea, 0xaa, 0xe8, 0x40, 0x8e, 0xbe, 0x1c, 0xe4, 0xcd,
	0x95, 0x78, 0xc9, 0x72, 0x8e, 0x31, 0x6a, 0x15, 0xb0, 0xda, 0xd7, 0x76, 0x22, 0x3b, 0xe3, 0x73,
	0xe4, 0xc2, 0x2b, 0x18, 0x50, 0x36, 0xb3, 0x13, 0x55, 0x0a, 0x13, 0xe4, 0x01, 0x63, 0x8a, 0x6b,
	0x4d, 0x5a, 0x76, 0x51, 0x4b, 0x7c, 0x8d, 0x7a, 0x5a, 0x42, 0xcc, 0x1f, 0x40, 0xa7, 0x54, 0x80,
	0x62, 0x33, 0x46, 0x4e, 0x2c, 0xf2, 0xcf, 0xc7, 0x03, 0xd4, 0x91, 0xfb, 0xf2, 0x19, 0x23, 0xae,
	0x85, 0x8e, 0x06, 0x1e, 0xa2, 0x2e, 0x14, 0x90, 0x97, 0x26, 0x8b, 0xf5, 0x8c, 0x11, 0xcf, 0xee,
	0x7f, 0x5b, 0x78, 0x84, 0x4e, 0xb5, 0x01, 0x65, 0xb2, 0x22, 0x99, 0x43, 0xc2, 0x49, 0xdb, 0x22,
	0x7f, 0xbc, 0xe9, 0xe0, 0x63, 0xe3, 0x3b, 0xeb, 0x8d, 0xef, 0x7c, 0x6f, 0x7c, 0xe7, 0x7d, 0xeb,
	0x37, 0xd6, 0x5b, 0xbf, 0xf1, 0xb9, 0xf5, 0x1b, 0xcf, 0x4d, 0x49, 0xa9, 0x6b, 0xff, 0xe1, 0xe6,
	0x27, 0x00, 0x00, 0xff, 0xff, 0x22, 0xe7, 0x30, 0xb8, 0xeb, 0x01, 0x00, 0x00,
}

func (m *SnapshotWithType) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *SnapshotWithType) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *SnapshotWithType) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Snapshot != nil {
		{
			size, err := m.Snapshot.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintSnapshot(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x12
	}
	if m.SbType != 0 {
		i = encodeVarintSnapshot(dAtA, i, uint64(m.SbType))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func (m *Profile) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Profile) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Profile) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.StartingPage) > 0 {
		i -= len(m.StartingPage)
		copy(dAtA[i:], m.StartingPage)
		i = encodeVarintSnapshot(dAtA, i, uint64(len(m.StartingPage)))
		i--
		dAtA[i] = 0x42
	}
	if len(m.AnalyticsId) > 0 {
		i -= len(m.AnalyticsId)
		copy(dAtA[i:], m.AnalyticsId)
		i = encodeVarintSnapshot(dAtA, i, uint64(len(m.AnalyticsId)))
		i--
		dAtA[i] = 0x3a
	}
	if len(m.ProfileId) > 0 {
		i -= len(m.ProfileId)
		copy(dAtA[i:], m.ProfileId)
		i = encodeVarintSnapshot(dAtA, i, uint64(len(m.ProfileId)))
		i--
		dAtA[i] = 0x32
	}
	if len(m.SpaceDashboardId) > 0 {
		i -= len(m.SpaceDashboardId)
		copy(dAtA[i:], m.SpaceDashboardId)
		i = encodeVarintSnapshot(dAtA, i, uint64(len(m.SpaceDashboardId)))
		i--
		dAtA[i] = 0x2a
	}
	if len(m.Address) > 0 {
		i -= len(m.Address)
		copy(dAtA[i:], m.Address)
		i = encodeVarintSnapshot(dAtA, i, uint64(len(m.Address)))
		i--
		dAtA[i] = 0x22
	}
	if len(m.Avatar) > 0 {
		i -= len(m.Avatar)
		copy(dAtA[i:], m.Avatar)
		i = encodeVarintSnapshot(dAtA, i, uint64(len(m.Avatar)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.Name) > 0 {
		i -= len(m.Name)
		copy(dAtA[i:], m.Name)
		i = encodeVarintSnapshot(dAtA, i, uint64(len(m.Name)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func encodeVarintSnapshot(dAtA []byte, offset int, v uint64) int {
	offset -= sovSnapshot(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *SnapshotWithType) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.SbType != 0 {
		n += 1 + sovSnapshot(uint64(m.SbType))
	}
	if m.Snapshot != nil {
		l = m.Snapshot.Size()
		n += 1 + l + sovSnapshot(uint64(l))
	}
	return n
}

func (m *Profile) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Name)
	if l > 0 {
		n += 1 + l + sovSnapshot(uint64(l))
	}
	l = len(m.Avatar)
	if l > 0 {
		n += 1 + l + sovSnapshot(uint64(l))
	}
	l = len(m.Address)
	if l > 0 {
		n += 1 + l + sovSnapshot(uint64(l))
	}
	l = len(m.SpaceDashboardId)
	if l > 0 {
		n += 1 + l + sovSnapshot(uint64(l))
	}
	l = len(m.ProfileId)
	if l > 0 {
		n += 1 + l + sovSnapshot(uint64(l))
	}
	l = len(m.AnalyticsId)
	if l > 0 {
		n += 1 + l + sovSnapshot(uint64(l))
	}
	l = len(m.StartingPage)
	if l > 0 {
		n += 1 + l + sovSnapshot(uint64(l))
	}
	return n
}

func sovSnapshot(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozSnapshot(x uint64) (n int) {
	return sovSnapshot(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *SnapshotWithType) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowSnapshot
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: SnapshotWithType: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: SnapshotWithType: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field SbType", wireType)
			}
			m.SbType = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.SbType |= model.SmartBlockType(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Snapshot", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Snapshot == nil {
				m.Snapshot = &ChangeSnapshot{}
			}
			if err := m.Snapshot.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipSnapshot(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthSnapshot
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Profile) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowSnapshot
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Profile: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Profile: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Name", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Name = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Avatar", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Avatar = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Address", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Address = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field SpaceDashboardId", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.SpaceDashboardId = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field ProfileId", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.ProfileId = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 7:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field AnalyticsId", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.AnalyticsId = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field StartingPage", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthSnapshot
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthSnapshot
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.StartingPage = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipSnapshot(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthSnapshot
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipSnapshot(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowSnapshot
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowSnapshot
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthSnapshot
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupSnapshot
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthSnapshot
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthSnapshot        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowSnapshot          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupSnapshot = fmt.Errorf("proto: unexpected end of group")
)
