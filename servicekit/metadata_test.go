package servicekit

import (
	"context"
	"sync"
	"testing"

	"github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestMetadataPublisherSkipsUnchangedValues(t *testing.T) {
	store := newFakeMetadataStore()
	publisher := testMetadataPublisher(store)

	if err := publisher.reconcile(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if got := store.putCalls(); got != 2 {
		t.Fatalf("put calls after first reconcile = %d, want 2", got)
	}

	if err := publisher.reconcile(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if got := store.putCalls(); got != 2 {
		t.Fatalf("put calls after unchanged reconcile = %d, want 2", got)
	}
}

func TestMetadataPublisherPutsChangedValues(t *testing.T) {
	store := newFakeMetadataStore()
	publisher := testMetadataPublisher(store)

	if err := publisher.reconcile(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	publisher.descriptors["payment.v1"] = []byte("changed descriptor")

	if err := publisher.reconcile(context.Background()); err != nil {
		t.Fatalf("changed reconcile: %v", err)
	}
	if got := store.putCalls(); got != 3 {
		t.Fatalf("put calls after changed reconcile = %d, want 3", got)
	}
}

func testMetadataPublisher(store *fakeMetadataStore) *MetadataPublisher {
	publisher := NewMetadataPublisher(nil, MetadataPrefixes{
		RoutesPrefix:      "/runtime/gateway/routes",
		DescriptorsPrefix: "/runtime/gateway/descriptors",
	}, []gatewaymeta.RouteMeta{{
		ID:      "payment.get",
		Enabled: true,
		HTTP: gatewaymeta.HTTPMeta{
			Method: "GET",
			Path:   "/v1/payments/{id}",
		},
		GRPC: gatewaymeta.GRPCMeta{
			Service:      "payment",
			FullMethod:   "/payment.v1.PaymentService/GetPayment",
			RequestType:  "payment.v1.GetPaymentRequest",
			ResponseType: "payment.v1.PaymentResponse",
			DescriptorID: "payment.v1",
		},
	}}, map[string][]byte{
		"payment.v1": []byte("descriptor bytes"),
	})
	publisher.client = store
	return publisher
}

type fakeMetadataStore struct {
	mu     sync.Mutex
	values map[string][]byte
	puts   int
}

func newFakeMetadataStore() *fakeMetadataStore {
	return &fakeMetadataStore{values: make(map[string][]byte)}
}

func (s *fakeMetadataStore) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return &clientv3.GetResponse{}, nil
	}
	return &clientv3.GetResponse{
		Kvs: []*mvccpb.KeyValue{{Key: []byte(key), Value: append([]byte(nil), value...)}},
	}, nil
}

func (s *fakeMetadataStore) Put(ctx context.Context, key string, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = []byte(val)
	s.puts++
	return &clientv3.PutResponse{}, nil
}

func (s *fakeMetadataStore) putCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.puts
}
