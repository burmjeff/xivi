package security

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	proxyHostnameCacheTTL      = 15 * time.Second
	proxyHostnameLookupTimeout = 2 * time.Second
)

type proxyHostnameLookup func(context.Context, string) ([]net.IPAddr, error)

// proxyHostnameResolver keeps request handling independent from Docker's
// ephemeral container addresses without trusting reverse DNS or any hostname
// supplied by a request. Only administrator-configured names are resolved.
type proxyHostnameResolver struct {
	mu        sync.RWMutex
	refreshMu sync.Mutex
	lookup    proxyHostnameLookup
	ttl       time.Duration
	key       string
	expires   time.Time
	addresses map[string]struct{}
}

var trustedProxyHostnameResolver = newProxyHostnameResolver(
	proxyHostnameCacheTTL,
	net.DefaultResolver.LookupIPAddr,
)

func newProxyHostnameResolver(ttl time.Duration, lookup proxyHostnameLookup) *proxyHostnameResolver {
	return &proxyHostnameResolver{
		lookup:    lookup,
		ttl:       ttl,
		addresses: make(map[string]struct{}),
	}
}

func (resolver *proxyHostnameResolver) contains(ip net.IP, configured []string) bool {
	if ip == nil || len(configured) == 0 {
		return false
	}
	hosts, key := normalizedProxyHostnames(configured)
	if len(hosts) == 0 {
		return false
	}
	if fresh, found := resolver.cachedResult(ip, key, time.Now()); fresh {
		return found
	}

	resolver.refreshMu.Lock()
	defer resolver.refreshMu.Unlock()
	now := time.Now()
	fresh, found := resolver.cachedResult(ip, key, now)
	if fresh {
		return found
	}

	ctx, cancel := context.WithTimeout(context.Background(), proxyHostnameLookupTimeout)
	defer cancel()
	addresses := make(map[string]struct{})
	for _, host := range hosts {
		results, err := resolver.lookup(ctx, host)
		if err != nil {
			continue
		}
		for _, result := range results {
			candidate := result.IP
			// DNS aliases are intended for same-host and private container
			// networks. Public proxy peers must use an explicit CIDR so a
			// compromised public DNS record cannot become a trusted hop.
			if candidate != nil && (candidate.IsLoopback() || candidate.IsPrivate()) {
				addresses[candidate.String()] = struct{}{}
			}
		}
	}

	resolver.mu.Lock()
	resolver.key = key
	resolver.addresses = addresses
	resolver.expires = now.Add(resolver.ttl)
	_, found = resolver.addresses[ip.String()]
	resolver.mu.Unlock()
	return found
}

func (resolver *proxyHostnameResolver) cachedResult(ip net.IP, key string, now time.Time) (bool, bool) {
	resolver.mu.RLock()
	defer resolver.mu.RUnlock()
	if resolver.key != key || !now.Before(resolver.expires) {
		return false, false
	}
	_, found := resolver.addresses[ip.String()]
	return true, found
}

func normalizedProxyHostnames(values []string) ([]string, string) {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	hosts := make([]string, 0, len(unique))
	for host := range unique {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts, strings.Join(hosts, "\x00")
}
