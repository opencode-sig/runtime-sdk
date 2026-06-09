package servicekit

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestResolveServiceAddressUsesAdvertiseAddress(t *testing.T) {
	resolver := serviceAddressResolver{}
	address, err := resolver.resolve(context.Background(), Config{
		Service: ServiceConfig{
			GRPCAddr:          ":9001",
			AdvertiseGRPCAddr: "payment:9001",
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if address != "payment:9001" {
		t.Fatalf("address = %q, want payment:9001", address)
	}
}

func TestResolveServiceAddressUsesConcreteListenAddress(t *testing.T) {
	resolver := serviceAddressResolver{}
	address, err := resolver.resolve(context.Background(), Config{
		Service: ServiceConfig{GRPCAddr: "10.0.1.8:9001"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if address != "10.0.1.8:9001" {
		t.Fatalf("address = %q, want 10.0.1.8:9001", address)
	}
}

func TestResolveServiceAddressesUsesAdvertiseHTTPAddress(t *testing.T) {
	resolver := serviceAddressResolver{}
	addresses, err := resolver.resolveAll(context.Background(), Config{
		Service: ServiceConfig{
			GRPCAddr:          "payment:9001",
			HTTPAddr:          ":9101",
			AdvertiseHTTPAddr: "payment-http:9101",
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if addresses.GRPC != "payment:9001" {
		t.Fatalf("grpc address = %q, want payment:9001", addresses.GRPC)
	}
	if addresses.HTTP != "payment-http:9101" {
		t.Fatalf("http address = %q, want payment-http:9101", addresses.HTTP)
	}
}

func TestResolveServiceAddressesAutoUsesSameLocalIPForGRPCAndHTTP(t *testing.T) {
	var dials int
	resolver := serviceAddressResolver{
		dialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			dials++
			return fakeProbeDialer("172.18.0.5")(ctx, network, address)
		},
		interfaceAddrs: func() ([]net.Addr, error) {
			t.Fatal("interface fallback should not be used")
			return nil, nil
		},
	}
	addresses, err := resolver.resolveAll(context.Background(), Config{
		Service: ServiceConfig{
			GRPCAddr: ":9001",
			HTTPAddr: ":9101",
		},
		Registry: RegistryConfig{Etcd: EtcdConfig{
			Endpoints: []string{"etcd:2379"},
		}},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if addresses.GRPC != "172.18.0.5:9001" {
		t.Fatalf("grpc address = %q, want 172.18.0.5:9001", addresses.GRPC)
	}
	if addresses.HTTP != "172.18.0.5:9101" {
		t.Fatalf("http address = %q, want 172.18.0.5:9101", addresses.HTTP)
	}
	if dials != 1 {
		t.Fatalf("dial count = %d, want 1", dials)
	}
}

func TestResolveServiceAddressesSkipsHTTPWhenNoHTTPListenAddress(t *testing.T) {
	resolver := serviceAddressResolver{}
	addresses, err := resolver.resolveAll(context.Background(), Config{
		Service: ServiceConfig{GRPCAddr: "10.0.1.8:9001"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if addresses.HTTP != "" {
		t.Fatalf("http address = %q, want empty", addresses.HTTP)
	}
}

func TestResolveServiceAddressAutoUsesProbeLocalIP(t *testing.T) {
	resolver := serviceAddressResolver{
		dialContext: fakeProbeDialer("172.18.0.5"),
		interfaceAddrs: func() ([]net.Addr, error) {
			t.Fatal("interface fallback should not be used")
			return nil, nil
		},
	}
	address, err := resolver.resolve(context.Background(), Config{
		Service: ServiceConfig{GRPCAddr: ":9001"},
		Registry: RegistryConfig{Etcd: EtcdConfig{
			Endpoints: []string{"etcd:2379"},
		}},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if address != "172.18.0.5:9001" {
		t.Fatalf("address = %q, want 172.18.0.5:9001", address)
	}
}

func TestResolveServiceAddressAutoFiltersInvalidProbeIPAndUsesInterface(t *testing.T) {
	resolver := serviceAddressResolver{
		dialContext: fakeProbeDialer("127.0.0.1"),
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{
				ipNet("169.254.1.10"),
				ipNet("127.0.0.1"),
				ipNet("172.18.0.6"),
			}, nil
		},
	}
	address, err := resolver.resolve(context.Background(), Config{
		Service: ServiceConfig{GRPCAddr: "0.0.0.0:9001"},
		Registry: RegistryConfig{Etcd: EtcdConfig{
			Endpoints: []string{"http://etcd:2379"},
		}},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if address != "172.18.0.6:9001" {
		t.Fatalf("address = %q, want 172.18.0.6:9001", address)
	}
}

func TestResolveServiceAddressAutoFailsWhenNoUsableIPExists(t *testing.T) {
	resolver := serviceAddressResolver{
		dialContext: fakeProbeDialer("::1"),
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{ipNet("127.0.0.1")}, nil
		},
	}
	_, err := resolver.resolve(context.Background(), Config{
		Service: ServiceConfig{GRPCAddr: "[::]:9001"},
		Registry: RegistryConfig{Etcd: EtcdConfig{
			Endpoints: []string{"etcd:2379"},
		}},
	})
	if err == nil {
		t.Fatal("resolve succeeded, want error")
	}
}

func fakeProbeDialer(ip string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		return fakeConn{local: &net.UDPAddr{IP: net.ParseIP(ip), Port: 4242}}, nil
	}
}

func ipNet(ip string) net.Addr {
	parsed := net.ParseIP(ip)
	if parsed.To4() != nil {
		return &net.IPNet{IP: parsed, Mask: net.CIDRMask(24, 32)}
	}
	return &net.IPNet{IP: parsed, Mask: net.CIDRMask(64, 128)}
}

type fakeConn struct {
	local net.Addr
}

func (c fakeConn) Read(b []byte) (int, error) {
	return 0, nil
}

func (c fakeConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (c fakeConn) Close() error {
	return nil
}

func (c fakeConn) LocalAddr() net.Addr {
	return c.local
}

func (c fakeConn) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 2379}
}

func (c fakeConn) SetDeadline(t time.Time) error {
	return nil
}

func (c fakeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c fakeConn) SetWriteDeadline(t time.Time) error {
	return nil
}
