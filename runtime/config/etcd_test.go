package config

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdProviderKey(t *testing.T) {
	provider := NewEtcdProvider([]string{"127.0.0.1:2379"}, "/runtime/config/")

	if got := provider.key("gateway/runtime.yaml"); got != "/runtime/config/gateway/runtime.yaml" {
		t.Fatalf("key = %q", got)
	}
	if got := provider.key("/absolute/key"); got != "/absolute/key" {
		t.Fatalf("absolute key = %q", got)
	}
}

func TestEtcdProviderRequiresKey(t *testing.T) {
	provider := NewEtcdProviderWithClient(nil, "/runtime/config")

	if _, err := provider.Load(t.Context(), ""); err == nil {
		t.Fatal("expected load error")
	}
}

func TestEntryFromKV(t *testing.T) {
	entry := entryFromKV("runtime.yaml", &mvccpb.KeyValue{
		Value:       []byte(`{"ok":true}`),
		ModRevision: 42,
	})

	if entry.Key != "runtime.yaml" {
		t.Fatalf("key = %q", entry.Key)
	}
	if string(entry.Value) != `{"ok":true}` {
		t.Fatalf("value = %s", entry.Value)
	}
	if entry.Version != "42" {
		t.Fatalf("version = %q", entry.Version)
	}
}

func TestEtcdProviderIntegration(t *testing.T) {
	if os.Getenv("ETCD_INTEGRATION") != "1" {
		t.Skip("set ETCD_INTEGRATION=1 to run")
	}

	endpoint := os.Getenv("ETCD_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:2379"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new etcd client: %v", err)
	}
	defer client.Close()

	prefix := fmt.Sprintf("/runtime/test/config/%d", time.Now().UnixNano())
	provider := NewEtcdProviderWithClient(client, prefix)
	key := "runtime.yaml"
	fullKey := provider.key(key)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_, _ = client.Delete(cleanupCtx, prefix, clientv3.WithPrefix())
	}()

	if _, err := client.Put(ctx, fullKey, `{"gateway":{"proxy_timeout":"2s"}}`); err != nil {
		t.Fatalf("put initial config: %v", err)
	}

	data, err := provider.Load(ctx, key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(data) != `{"gateway":{"proxy_timeout":"2s"}}` {
		t.Fatalf("data = %s", data)
	}

	if _, err := client.Put(ctx, fullKey, `{"gateway":{"proxy_timeout":"3s"}}`); err != nil {
		t.Fatalf("put changed config: %v", err)
	}
	data, err = provider.Load(ctx, key)
	if err != nil {
		t.Fatalf("load changed: %v", err)
	}
	if string(data) != `{"gateway":{"proxy_timeout":"3s"}}` {
		t.Fatalf("changed data = %s", data)
	}

	created, err := provider.PutIfAbsent(ctx, "defaults.yaml", []byte(`{"created":true}`))
	if err != nil {
		t.Fatalf("put if absent new key: %v", err)
	}
	if !created {
		t.Fatal("expected put if absent to create new key")
	}
	created, err = provider.PutIfAbsent(ctx, "defaults.yaml", []byte(`{"created":false}`))
	if err != nil {
		t.Fatalf("put if absent existing key: %v", err)
	}
	if created {
		t.Fatal("expected put if absent to keep existing key")
	}
	entry, err := provider.Get(ctx, "defaults.yaml")
	if err != nil {
		t.Fatalf("get put if absent key: %v", err)
	}
	if string(entry.Value) != `{"created":true}` {
		t.Fatalf("put if absent value = %s", entry.Value)
	}
}
