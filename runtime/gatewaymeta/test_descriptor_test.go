package gatewaymeta

import (
	"testing"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func testUserFile(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	fieldNumber := int32(1)
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    strptr("user/v1/user.proto"),
		Package: strptr("api.user.v1"),
		Syntax:  strptr("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strptr("GetUserRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name:     strptr("id"),
					JsonName: strptr("id"),
					Number:   &fieldNumber,
					Label:    labelptr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
					Type:     typeptr(descriptorpb.FieldDescriptorProto_TYPE_STRING),
				}},
			},
			{
				Name: strptr("UserResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name:     strptr("id"),
					JsonName: strptr("id"),
					Number:   &fieldNumber,
					Label:    labelptr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
					Type:     typeptr(descriptorpb.FieldDescriptorProto_TYPE_STRING),
				}},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: strptr("UserService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       strptr("GetUser"),
				InputType:  strptr(".api.user.v1.GetUserRequest"),
				OutputType: strptr(".api.user.v1.UserResponse"),
			}},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("build test descriptor: %v", err)
	}
	return file
}

func strptr(value string) *string {
	return &value
}

func labelptr(value descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto_Label {
	return &value
}

func typeptr(value descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto_Type {
	return &value
}
