package outbound

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maximumRelayManifestBytes = 8 << 20
	maximumRelayResources     = 16_384
)

// Relay is a loopback-only, fixed-origin gateway used by native media
// pipelines. GStreamer receives only relay URLs; all provider DNS resolution,
// redirects, cookies, and HLS subresources remain inside the validated dialer.
type Relay struct {
	origin   *url.URL
	policy   Policy
	client   *http.Client
	server   *http.Server
	listener net.Listener

	mu        sync.Mutex
	nextID    uint64
	resources map[string]string
	reverse   map[string]string
	order     []string
	closeOnce sync.Once
}

func StartRelay(raw string, policy Policy) (*Relay, error) {
	validationTimeout := policy.Timeout
	if validationTimeout <= 0 || validationTimeout > 10*time.Second {
		validationTimeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()
	origin, err := Validate(ctx, raw, policy.AllowPrivate)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start protected stream relay: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	client := NewClient(policy)
	client.Timeout = 0 // live MPEG-TS responses intentionally have no body deadline
	client.Jar = jar
	relay := &Relay{
		origin: origin, policy: policy, client: client, listener: listener,
		resources: make(map[string]string), reverse: make(map[string]string),
	}
	relay.server = &http.Server{
		Handler:           http.HandlerFunc(relay.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	go func() {
		_ = relay.server.Serve(listener)
	}()
	return relay, nil
}

func (r *Relay) URL() string {
	if r == nil || r.listener == nil {
		return ""
	}
	return "http://" + r.listener.Addr().String() + "/root"
}

func (r *Relay) Close() error {
	if r == nil {
		return nil
	}
	var result error
	r.closeOnce.Do(func() {
		result = r.server.Close()
		if errors.Is(result, http.ErrServerClosed) {
			result = nil
		}
		if transport, ok := r.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	})
	return result
}

func (r *Relay) target(requestPath string) (string, bool) {
	if requestPath == "/root" {
		return r.origin.String(), true
	}
	const prefix = "/resource/"
	if !strings.HasPrefix(requestPath, prefix) {
		return "", false
	}
	r.mu.Lock()
	target, ok := r.resources[strings.TrimPrefix(requestPath, prefix)]
	r.mu.Unlock()
	return target, ok
}

func (r *Relay) register(target string) string {
	parsed, err := parseHTTPURL(target)
	if err != nil {
		return "/blocked"
	}
	target = parsed.String()
	r.mu.Lock()
	defer r.mu.Unlock()
	if id := r.reverse[target]; id != "" {
		return "/resource/" + id
	}
	r.nextID++
	id := strconv.FormatUint(r.nextID, 36)
	r.resources[id] = target
	r.reverse[target] = id
	r.order = append(r.order, id)
	if len(r.order) > maximumRelayResources {
		oldest := r.order[0]
		r.order = r.order[1:]
		delete(r.reverse, r.resources[oldest])
		delete(r.resources, oldest)
	}
	return "/resource/" + id
}

func (r *Relay) localURL(target string) string {
	return "http://" + r.listener.Addr().String() + r.register(target)
}

func copyRelayRequestHeaders(destination, source http.Header) {
	for _, name := range []string{"Accept", "Range", "If-Modified-Since", "If-None-Match", "User-Agent"} {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
}

func copyRelayResponseHeaders(destination, source http.Header) {
	for _, name := range []string{"Accept-Ranges", "Content-Range", "Content-Type", "ETag", "Last-Modified"} {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
	destination.Set("Cache-Control", "no-store")
	destination.Set("X-Content-Type-Options", "nosniff")
}

func (r *Relay) serveHTTP(writer http.ResponseWriter, downstream *http.Request) {
	if downstream.Method != http.MethodGet && downstream.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	target, ok := r.target(downstream.URL.Path)
	if !ok {
		http.NotFound(writer, downstream)
		return
	}
	parsedTarget, err := parseHTTPURL(target)
	if err != nil {
		http.Error(writer, "upstream unavailable", http.StatusBadGateway)
		return
	}
	upstream, err := http.NewRequestWithContext(downstream.Context(), downstream.Method, parsedTarget.String(), nil)
	if err != nil {
		http.Error(writer, "upstream unavailable", http.StatusBadGateway)
		return
	}
	copyRelayRequestHeaders(upstream.Header, downstream.Header)
	response, err := r.client.Do(upstream)
	if err != nil {
		http.Error(writer, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	copyRelayResponseHeaders(writer.Header(), response.Header)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writer.WriteHeader(response.StatusCode)
		return
	}
	if downstream.Method == http.MethodHead {
		writer.WriteHeader(response.StatusCode)
		return
	}
	body := bufio.NewReaderSize(response.Body, 4096)
	if isHLSManifest(response, body) {
		manifest, readErr := io.ReadAll(io.LimitReader(body, maximumRelayManifestBytes+1))
		if readErr != nil || len(manifest) > maximumRelayManifestBytes {
			http.Error(writer, "upstream manifest unavailable", http.StatusBadGateway)
			return
		}
		rewritten, rewriteErr := r.rewriteManifest(response.Request.URL, manifest)
		if rewriteErr != nil {
			http.Error(writer, "upstream manifest unavailable", http.StatusBadGateway)
			return
		}
		writer.Header().Del("Content-Length")
		writer.WriteHeader(response.StatusCode)
		_, _ = writer.Write(rewritten)
		return
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "xml") || strings.Contains(contentType, "html") || strings.Contains(contentType, "dash") {
		http.Error(writer, "unsupported upstream manifest", http.StatusUnsupportedMediaType)
		return
	}
	if response.ContentLength >= 0 {
		writer.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	writer.WriteHeader(response.StatusCode)
	_, _ = io.Copy(writer, body)
}

func isHLSManifest(response *http.Response, body *bufio.Reader) bool {
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "mpegurl") || strings.HasSuffix(strings.ToLower(response.Request.URL.Path), ".m3u8") {
		return true
	}
	prefix, _ := body.Peek(256)
	prefix = []byte(strings.TrimSpace(strings.TrimPrefix(string(prefix), "\ufeff")))
	return strings.HasPrefix(string(prefix), "#EXTM3U")
}

func (r *Relay) rewriteManifest(base *url.URL, body []byte) ([]byte, error) {
	if base == nil {
		return nil, errors.New("manifest base URL is missing")
	}
	var output strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 64*1024), maximumRelayManifestBytes)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			output.WriteString(line)
		case strings.HasPrefix(trimmed, "#"):
			output.WriteString(r.rewriteURIAttributes(base, line))
		default:
			output.WriteString(r.resolveManifestURI(base, trimmed))
		}
		output.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}

func (r *Relay) resolveManifestURI(base *url.URL, value string) string {
	reference, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return r.localURL("")
	}
	return r.localURL(base.ResolveReference(reference).String())
}

func (r *Relay) rewriteURIAttributes(base *url.URL, line string) string {
	upper := strings.ToUpper(line)
	searchFrom := 0
	for {
		position := strings.Index(upper[searchFrom:], `URI="`)
		if position < 0 {
			return line
		}
		start := searchFrom + position + len(`URI="`)
		endOffset := strings.IndexByte(line[start:], '"')
		if endOffset < 0 {
			return line
		}
		end := start + endOffset
		replacement := r.resolveManifestURI(base, line[start:end])
		line = line[:start] + replacement + line[end:]
		upper = strings.ToUpper(line)
		searchFrom = start + len(replacement) + 1
	}
}
