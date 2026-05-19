package servicekit

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func TestNewClientsRequiresDiscovery(t *testing.T) {
	clients, err := NewClients(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if clients != nil {
		t.Fatalf("expected nil clients, got %#v", clients)
	}
}

func TestClientRequiresConstructor(t *testing.T) {
	_, err := Client[any](nil, context.Background(), "user", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientsConnRequiresConfiguredManager(t *testing.T) {
	clients := &Clients{}
	conn, err := clients.Conn(context.Background(), "user")
	if err == nil {
		t.Fatal("expected error")
	}
	if conn != nil {
		t.Fatalf("expected nil conn, got %#v", conn)
	}
}

func TestClientReturnsConnError(t *testing.T) {
	_, err := Client(&Clients{}, context.Background(), "user", func(conn grpc.ClientConnInterface) any {
		return conn
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
