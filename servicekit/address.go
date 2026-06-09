package servicekit

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const advertiseIPProbeTimeout = 500 * time.Millisecond

type serviceAddressResolver struct {
	dialContext    func(ctx context.Context, network string, address string) (net.Conn, error)
	interfaceAddrs func() ([]net.Addr, error)
}

type serviceAdvertiseAddresses struct {
	GRPC string
	HTTP string
}

var defaultServiceAddressResolver = serviceAddressResolver{
	dialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: advertiseIPProbeTimeout}
		return dialer.DialContext(ctx, network, address)
	},
	interfaceAddrs: net.InterfaceAddrs,
}

func resolveServiceAddress(ctx context.Context, cfg Config) (string, error) {
	return defaultServiceAddressResolver.resolve(ctx, cfg)
}

func resolveServiceAddresses(ctx context.Context, cfg Config) (serviceAdvertiseAddresses, error) {
	return defaultServiceAddressResolver.resolveAll(ctx, cfg)
}

func (r serviceAddressResolver) resolve(ctx context.Context, cfg Config) (string, error) {
	addresses, err := r.resolveAll(ctx, cfg)
	if err != nil {
		return "", err
	}
	return addresses.GRPC, nil
}

func (r serviceAddressResolver) resolveAll(ctx context.Context, cfg Config) (serviceAdvertiseAddresses, error) {
	var localIP net.IP
	resolveListen := func(kind string, listenAddr string) (string, error) {
		listenAddr = strings.TrimSpace(listenAddr)
		if listenAddr == "" {
			return "", nil
		}
		host, port, ok := splitServiceListenAddress(listenAddr)
		if !ok || !isWildcardListenHost(host) {
			return listenAddr, nil
		}
		if localIP == nil {
			ip, err := r.resolveLocalAdvertiseIP(ctx, cfg)
			if err != nil {
				return "", fmt.Errorf("resolve advertise %s addr for %q: %w", kind, listenAddr, err)
			}
			localIP = ip
		}
		return net.JoinHostPort(localIP.String(), port), nil
	}

	var addresses serviceAdvertiseAddresses
	if address := strings.TrimSpace(cfg.Service.AdvertiseGRPCAddr); address != "" {
		addresses.GRPC = address
	} else {
		address, err := resolveListen("grpc", cfg.Service.GRPCAddr)
		if err != nil {
			return serviceAdvertiseAddresses{}, err
		}
		addresses.GRPC = address
	}
	if address := strings.TrimSpace(cfg.Service.AdvertiseHTTPAddr); address != "" {
		addresses.HTTP = address
	} else {
		address, err := resolveListen("http", cfg.Service.HTTPAddr)
		if err != nil {
			return serviceAdvertiseAddresses{}, err
		}
		addresses.HTTP = address
	}
	return addresses, nil
}

func splitServiceListenAddress(address string) (string, string, bool) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return "", "", false
	}
	return strings.TrimSpace(host), strings.TrimSpace(port), port != ""
}

func isWildcardListenHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	return host == "" || host == "0.0.0.0" || host == "::"
}

func (r serviceAddressResolver) resolveLocalAdvertiseIP(ctx context.Context, cfg Config) (net.IP, error) {
	for _, target := range advertiseIPProbeTargets(cfg) {
		ip, err := r.probeLocalIP(ctx, target)
		if err == nil && isUsableAdvertiseIP(ip) {
			return ip, nil
		}
	}
	ips, err := r.interfaceAdvertiseIPs()
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip, nil
		}
	}
	if len(ips) > 0 {
		return ips[0], nil
	}
	return nil, fmt.Errorf("no usable non-loopback local IP found")
}

func advertiseIPProbeTargets(cfg Config) []string {
	var targets []string
	add := func(values []string) {
		for _, value := range values {
			if target := normalizeProbeTarget(value); target != "" {
				targets = append(targets, target)
			}
		}
	}
	add(cfg.Registry.Etcd.Endpoints)
	add(cfg.Runtime.Config.Etcd.Endpoints)
	add(cfg.Infra.Etcd.Endpoints)
	return uniqueStrings(targets)
}

func normalizeProbeTarget(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return ""
		}
		endpoint = parsed.Host
	}
	if endpoint == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(endpoint); err == nil {
		return endpoint
	}
	host := strings.Trim(endpoint, "[]")
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, "2379")
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (r serviceAddressResolver) probeLocalIP(ctx context.Context, target string) (net.IP, error) {
	if r.dialContext == nil {
		return nil, fmt.Errorf("dialer is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := r.dialContext(ctx, "udp", target)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr == nil {
		return nil, fmt.Errorf("local address %T is not UDP", conn.LocalAddr())
	}
	return addr.IP, nil
}

func (r serviceAddressResolver) interfaceAdvertiseIPs() ([]net.IP, error) {
	if r.interfaceAddrs == nil {
		return nil, fmt.Errorf("interface address resolver is not configured")
	}
	addrs, err := r.interfaceAddrs()
	if err != nil {
		return nil, err
	}
	var ipv4 []net.IP
	var ipv6 []net.IP
	for _, addr := range addrs {
		ip := ipFromAddr(addr)
		if !isUsableAdvertiseIP(ip) {
			continue
		}
		if ip.To4() != nil {
			ipv4 = append(ipv4, ip)
			continue
		}
		ipv6 = append(ipv6, ip)
	}
	return append(ipv4, ipv6...), nil
}

func ipFromAddr(addr net.Addr) net.IP {
	switch value := addr.(type) {
	case *net.IPNet:
		return value.IP
	case *net.IPAddr:
		return value.IP
	default:
		return nil
	}
}

func isUsableAdvertiseIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !ip.IsUnspecified() &&
		!ip.IsLoopback() &&
		!ip.IsMulticast() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast()
}
