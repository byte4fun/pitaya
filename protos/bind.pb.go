// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0
// 	protoc        v3.21.12
// source: bind.proto

package protos

import (
	reflect "reflect"
	sync "sync"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type BindMsg struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Uid string `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	Fid string `protobuf:"bytes,2,opt,name=fid,proto3" json:"fid,omitempty"`
}

func (x *BindMsg) Reset() {
	*x = BindMsg{}
	if protoimpl.UnsafeEnabled {
		mi := &file_bind_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BindMsg) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BindMsg) ProtoMessage() {}

func (x *BindMsg) ProtoReflect() protoreflect.Message {
	mi := &file_bind_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BindMsg.ProtoReflect.Descriptor instead.
func (*BindMsg) Descriptor() ([]byte, []int) {
	return file_bind_proto_rawDescGZIP(), []int{0}
}

func (x *BindMsg) GetUid() string {
	if x != nil {
		return x.Uid
	}
	return ""
}

func (x *BindMsg) GetFid() string {
	if x != nil {
		return x.Fid
	}
	return ""
}

var File_bind_proto protoreflect.FileDescriptor

var file_bind_proto_rawDesc = []byte{
	0x0a, 0x0a, 0x62, 0x69, 0x6e, 0x64, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x73, 0x22, 0x2d, 0x0a, 0x07, 0x42, 0x69, 0x6e, 0x64, 0x4d, 0x73, 0x67, 0x12,
	0x10, 0x0a, 0x03, 0x75, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x75, 0x69,
	0x64, 0x12, 0x10, 0x0a, 0x03, 0x66, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03,
	0x66, 0x69, 0x64, 0x42, 0x3c, 0x5a, 0x29, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f,
	0x6d, 0x2f, 0x74, 0x6f, 0x70, 0x66, 0x72, 0x65, 0x65, 0x67, 0x61, 0x6d, 0x65, 0x73, 0x2f, 0x70,
	0x69, 0x74, 0x61, 0x79, 0x61, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x73,
	0xaa, 0x02, 0x0e, 0x4e, 0x50, 0x69, 0x74, 0x61, 0x79, 0x61, 0x2e, 0x50, 0x72, 0x6f, 0x74, 0x6f,
	0x73, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_bind_proto_rawDescOnce sync.Once
	file_bind_proto_rawDescData = file_bind_proto_rawDesc
)

func file_bind_proto_rawDescGZIP() []byte {
	file_bind_proto_rawDescOnce.Do(func() {
		file_bind_proto_rawDescData = protoimpl.X.CompressGZIP(file_bind_proto_rawDescData)
	})
	return file_bind_proto_rawDescData
}

var file_bind_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_bind_proto_goTypes = []interface{}{
	(*BindMsg)(nil), // 0: protos.BindMsg
}
var file_bind_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_bind_proto_init() }
func file_bind_proto_init() {
	if File_bind_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_bind_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BindMsg); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_bind_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_bind_proto_goTypes,
		DependencyIndexes: file_bind_proto_depIdxs,
		MessageInfos:      file_bind_proto_msgTypes,
	}.Build()
	File_bind_proto = out.File
	file_bind_proto_rawDesc = nil
	file_bind_proto_goTypes = nil
	file_bind_proto_depIdxs = nil
}
