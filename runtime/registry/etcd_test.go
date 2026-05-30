package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdRegistryLeaseExpires(t *testing.T) {
	if os.Getenv("ETCD_INTEGRATION") != "1" {
		t.Skip("set ETCD_INTEGRATION=1 to run")
	}

	endpoint := os.Getenv("ETCD_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:2379"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new etcd client: %v", err)
	}
	defer client.Close()

	prefix := fmt.Sprintf("/runtime/test/registry/%d", time.Now().UnixNano())
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_, _ = client.Delete(cleanupCtx, prefix, clientv3.WithPrefix())
	}()

	registry := NewEtcdRegistry(client, prefix)
	registry.ttl = 1
	lease, err := registry.Register(ctx, ServiceInstance{
		ID:      "one",
		Name:    "user",
		Address: "127.0.0.1:19001",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	etcdLease, ok := lease.(*etcdRegistration)
	if !ok {
		t.Fatalf("registration type = %T", lease)
	}

	key := NewKeyspace(prefix).ServiceKey("user", "one")
	if count := etcdKeyCount(t, ctx, client, key); count != 1 {
		t.Fatalf("initial key count = %d", count)
	}

	etcdLease.cancel()
	select {
	case <-etcdLease.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for keepalive to stop")
	}

	waitForEtcdRegistry(t, func() bool {
		return etcdKeyCount(t, context.Background(), client, key) == 0
	})
}

func TestEtcdRegistrationRenewDetectsMissingKey(t *testing.T) {
	if os.Getenv("ETCD_INTEGRATION") != "1" {
		t.Skip("set ETCD_INTEGRATION=1 to run")
	}

	endpoint := os.Getenv("ETCD_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:2379"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new etcd client: %v", err)
	}
	defer client.Close()

	prefix := fmt.Sprintf("/runtime/test/registry/%d", time.Now().UnixNano())
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_, _ = client.Delete(cleanupCtx, prefix, clientv3.WithPrefix())
	}()

	registry := NewEtcdRegistry(client, prefix)
	lease, err := registry.Register(ctx, ServiceInstance{
		ID:      "one",
		Name:    "user",
		Address: "127.0.0.1:19001",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = lease.Deregister(cleanupCtx)
	})

	key := NewKeyspace(prefix).ServiceKey("user", "one")
	if _, err := client.Delete(ctx, key); err != nil {
		t.Fatalf("delete key: %v", err)
	}
	if err := lease.Renew(ctx); !errors.Is(err, ErrRegistrationExpired) {
		t.Fatalf("renew error = %v, want ErrRegistrationExpired", err)
	}
}

func etcdKeyCount(t *testing.T, ctx context.Context, client *clientv3.Client, key string) int64 {
	t.Helper()
	resp, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("get key %s: %v", key, err)
	}
	return resp.Count
}

func waitForEtcdRegistry(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for etcd registry condition")
}
