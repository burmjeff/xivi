package outbound

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Policy struct {
	AllowPrivate bool
	Timeout      time.Duration
	MaxRedirects int
}

var (
	ErrBlockedAddress   = errors.New("outbound destination is blocked")
	ErrResponseTooLarge = errors.New("outbound response exceeds its size limit")
	lookupIPAddr        = net.DefaultResolver.LookupIPAddr
	alwaysBlockedCIDRs  = mustNetworks(
		"0.0.0.0/8",         // current host and invalid historical encodings
		"100.64.0.0/10",     // carrier-grade NAT and provider metadata ranges
		"192.0.0.0/24",      // IETF protocol assignments
		"198.18.0.0/15",     // benchmarking networks
		"240.0.0.0/4",       // reserved IPv4
		"2001:db8::/32",     // IPv6 documentation range
		"fd00:ec2::254/128", // AWS IPv6 metadata endpoint
	)
)

func mustNetworks(values ...string) []*net.IPNet {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			panic(err)
		}
		result = append(result, network)
	}
	return result
}

func allowedIP(ip net.IP, allowPrivate bool) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, network := range alwaysBlockedCIDRs {
		if network.Contains(ip) {
			return false
		}
	}
	if !allowPrivate && ip.IsPrivate() {
		return false
	}
	return true
}

func parseHTTPURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("only absolute HTTP or HTTPS URLs are allowed")
	}
	if parsed.Fragment != "" {
		parsed.Fragment = ""
	}
	return parsed, nil
}

func resolveAllowed(ctx context.Context, hostname string, allowPrivate bool) ([]net.IP, error) {
	addresses, err := lookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("resolve outbound host: %w", err)
	}
	result := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if !allowedIP(address.IP, allowPrivate) {
			return nil, ErrBlockedAddress
		}
		result = append(result, address.IP)
	}
	if len(result) == 0 {
		return nil, ErrBlockedAddress
	}
	return result, nil
}

// Validate resolves every address and rejects the complete destination if any
// answer could reach a disallowed network. This prevents round-robin DNS from
// hiding a private or metadata target behind one public answer.
func Validate(ctx context.Context, raw string, allowPrivate bool) (*url.URL, error) {
	parsed, err := parseHTTPURL(raw)
	if err != nil {
		return nil, err
	}
	if _, err := resolveAllowed(ctx, parsed.Hostname(), allowPrivate); err != nil {
		return nil, err
	}
	return parsed, nil
}

func NewClient(policy Policy) *http.Client {
	if policy.Timeout <= 0 {
		policy.Timeout = 30 * time.Second
	}
	if policy.MaxRedirects <= 0 || policy.MaxRedirects > 5 {
		policy.MaxRedirects = 3
	}
	dialer := &net.Dialer{Timeout: min(policy.Timeout, 10*time.Second), KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := resolveAllowed(ctx, host, policy.AllowPrivate)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, ip := range addresses {
				connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return connection, nil
				}
				lastErr = dialErr
			}
			return nil, lastErr
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: min(policy.Timeout, 20*time.Second),
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   4,
	}
	client := &http.Client{Transport: transport, Timeout: policy.Timeout}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= policy.MaxRedirects {
			return errors.New("outbound redirect limit exceeded")
		}
		_, err := Validate(request.Context(), request.URL.String(), policy.AllowPrivate)
		return err
	}
	return client
}

func Open(ctx context.Context, raw string, policy Policy, headers http.Header) (*http.Response, error) {
	parsed, err := Validate(ctx, raw, policy.AllowPrivate)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := NewClient(policy).Do(request)
	if err != nil {
		var requestError *url.Error
		if errors.As(err, &requestError) {
			// url.Error includes the complete provider URL by default. Preserve
			// the useful network cause without allowing a path, query credential,
			// or userinfo value to flow into callers' logs and diagnostics.
			return nil, fmt.Errorf("outbound %s failed: %w", requestError.Op, requestError.Err)
		}
		return nil, errors.New("outbound request failed")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, fmt.Errorf("outbound source returned %s", response.Status)
	}
	return response, nil
}

func FetchBytes(ctx context.Context, raw string, policy Policy, limit int64, headers http.Header) ([]byte, http.Header, error) {
	response, err := Open(ctx, raw, policy, headers)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.ContentLength > limit {
		return nil, nil, ErrResponseTooLarge
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(content)) > limit {
		return nil, nil, ErrResponseTooLarge
	}
	return content, response.Header.Clone(), nil
}
