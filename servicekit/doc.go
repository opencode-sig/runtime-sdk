// Package servicekit provides the public runtime SDK for managed gRPC services.
//
// A service supplies its protobuf server registration, optional initialization
// hooks, and Gateway metadata publication. servicekit owns the runtime concerns:
// gRPC server, service HTTP listener, health, metrics, registry registration, metadata
// publication, graceful lifecycle, and control-command driven rebuilds.
//
// A rebuild creates a new DataPlane from the latest config, stops the old
// generation, and then starts the new one. Because most services reuse the same
// gRPC/HTTP listen addresses, this is a stop-start replacement and does not
// claim zero-downtime handoff on the same process.
package servicekit
