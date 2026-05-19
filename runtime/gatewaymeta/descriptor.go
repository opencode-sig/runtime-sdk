package gatewaymeta

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// GatewayDescriptorSet builds FileDescriptorSet bytes required by Gateway dynamicpb.
//
// It recursively collects the proto file and imports so Gateway can construct
// request and response messages without compile-time access to service code.
func GatewayDescriptorSet(files ...protoreflect.FileDescriptor) ([]byte, error) {
	seen := make(map[string]bool)
	set := &descriptorpb.FileDescriptorSet{}
	for _, file := range files {
		collectFileDescriptors(file, seen, set)
	}
	return proto.Marshal(set)
}

// collectFileDescriptors recursively collects FileDescriptorSet entries.
//
// Imports are collected before the current file so descriptor dependencies are
// available when consumers load the set.
func collectFileDescriptors(file protoreflect.FileDescriptor, seen map[string]bool, set *descriptorpb.FileDescriptorSet) {
	if file == nil || seen[file.Path()] {
		return
	}
	imports := file.Imports()
	for i := 0; i < imports.Len(); i++ {
		collectFileDescriptors(imports.Get(i).FileDescriptor, seen, set)
	}
	seen[file.Path()] = true
	set.File = append(set.File, protodesc.ToFileDescriptorProto(file))
}
