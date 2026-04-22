package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Compile-time interface check for io.Closer
var _ io.Closer = (*DoHResolver)(nil)

// DoHResolver provides DNS-over-HTTPS resolution
type DoHResolver struct {
	client    *http.Client
	providers []*DoHProvider
	cache     sync.Map
	cacheTTL  atomic.Int64 // Thread-safe cache TTL (stored as nanoseconds)
	cacheSize atomic.Int64 // O(1) cache size tracking
	closed    atomic.Bool  // Prevents double-close and operations after close
}

// Compile-time interface check
var _ Resolver = (*DoHResolver)(nil)

// DoHProvider represents a DoH service provider
type DoHProvider struct {
	Name     string
	Template string // URL template with {name} placeholder
	Priority int
}

// CacheEntry holds cached DNS resolution results
type CacheEntry struct {
	IPs     []net.IPAddr
	Expires time.Time
}

// Security constants for DoH
const (
	// maxDoHResponseSize limits the maximum size of a DoH response to prevent
	// memory exhaustion attacks from malicious DNS servers
	maxDoHResponseSize = 64 * 1024 // 64KB - DNS responses should never be this large

	// maxDoHCacheSize limits the number of cached DNS entries to prevent
	// unbounded memory growth
	maxDoHCacheSize = 1000
)

// DefaultDoHProviders returns common DoH providers
func DefaultDoHProviders() []*DoHProvider {
	return []*DoHProvider{
		{
			Name:     "cloudflare",
			Template: "https://1.1.1.1/dns-query?name={name}&type=A",
			Priority: 1,
		},
		{
			Name:     "google",
			Template: "https://dns.google/resolve?name={name}&type=A",
			Priority: 2,
		},
		{
			Name:     "ali",
			Template: "https://dns.alidns.com/resolve?name={name}&type=A",
			Priority: 3,
		},
	}
}

// NewDoHResolver creates a new DoH resolver
func NewDoHResolver(providers []*DoHProvider, cacheTTL time.Duration) *DoHResolver {
	if len(providers) == 0 {
		providers = DefaultDoHProviders()
	}
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}

	r := &DoHResolver{
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:       10,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: true,
				DisableKeepAlives:  false,
				ForceAttemptHTTP2:  true,
			},
		},
		providers: providers,
	}
	r.cacheTTL.Store(int64(cacheTTL))
	return r
}

// LookupIPAddr resolves a host name to IP addresses using DoH
func (r *DoHResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	// Check if resolver is closed
	if r.closed.Load() {
		return nil, fmt.Errorf("DoH resolver is closed")
	}

	// Check cache first using single LoadAndDelete to avoid TOCTOU race.
	// If the entry is valid, we re-store it; if expired or invalid, it stays deleted.
	if cached, loaded := r.cache.LoadAndDelete(host); loaded {
		entry, ok := cached.(*CacheEntry)
		if !ok || entry == nil {
			// Invalid entry type — decrement counter and fall through to lookup
			r.cacheSize.Add(-1)
		} else if time.Now().Before(entry.Expires) {
			// Valid and not expired — re-store the same entry and return a copy
			r.cache.Store(host, entry)
			ips := make([]net.IPAddr, len(entry.IPs))
			copy(ips, entry.IPs)
			return ips, nil
		} else {
			// Expired — decrement counter, entry stays deleted
			r.cacheSize.Add(-1)
		}
	}

	// Try each provider until one succeeds
	var lastErr error
	for _, provider := range r.providers {
		ips, err := r.lookupWithProvider(ctx, provider, host)
		if err == nil && len(ips) > 0 {
			// SECURITY: Use CAS to atomically reserve cache slot before storing
			// This prevents TOCTOU race where multiple goroutines could exceed cache limit
			for {
				current := r.cacheSize.Load()
				if current >= maxDoHCacheSize {
					break // Cache full, don't store but still return result
				}
				if r.cacheSize.CompareAndSwap(current, current+1) {
					// Successfully reserved slot, now store the entry
					cacheTTL := time.Duration(r.cacheTTL.Load())
					r.cache.Store(host, &CacheEntry{
						IPs:     ips,
						Expires: time.Now().Add(cacheTTL),
					})
					break
				}
				// CAS failed due to contention, retry
			}
			return ips, nil
		}
		lastErr = err
	}

	// Fallback to system resolver if all providers fail
	return r.fallbackLookup(ctx, host, lastErr)
}

// CacheSize returns the current number of entries in the cache (O(1) via atomic counter)
func (r *DoHResolver) CacheSize() int64 {
	return r.cacheSize.Load()
}

// lookupWithProvider performs DNS lookup using a specific DoH provider
func (r *DoHResolver) lookupWithProvider(ctx context.Context, provider *DoHProvider, host string) ([]net.IPAddr, error) {
	// Build request URL
	requestURL := fmt.Sprintf(provider.Template, url.PathEscape(host))

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Accept", "application/dns-json")

	// Execute request
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DoH request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH request returned status %d", resp.StatusCode)
	}

	// Parse response based on provider
	return r.parseResponse(resp, provider, host)
}

// parseResponse parses DoH response with size limits to prevent memory exhaustion
func (r *DoHResolver) parseResponse(resp *http.Response, provider *DoHProvider, host string) ([]net.IPAddr, error) {
	// SECURITY: Limit response body size to prevent memory exhaustion attacks
	limitedReader := io.LimitReader(resp.Body, maxDoHResponseSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	// SECURITY: Check if response exceeded size limit
	if len(body) > maxDoHResponseSize {
		return nil, fmt.Errorf("DoH response exceeds maximum size limit (%d bytes)", maxDoHResponseSize)
	}

	// Check response type
	contentType := resp.Header.Get("Content-Type")

	switch provider.Name {
	case "google", "ali":
		if contentType == "application/dns-json" || contentType == "application/json" {
			return r.parseJSONResponse(body, host)
		}
		return r.parseWireFormatResponse(body, host)
	case "cloudflare":
		return r.parseWireFormatResponse(body, host)
	default:
		// Try JSON first, then wire format
		if contentType == "application/dns-json" || contentType == "application/json" {
			return r.parseJSONResponse(body, host)
		}
		return r.parseWireFormatResponse(body, host)
	}
}

// parseJSONResponse parses JSON DoH response (Google and AliDNS format)
func (r *DoHResolver) parseJSONResponse(body []byte, host string) ([]net.IPAddr, error) {
	type DNSResponse struct {
		Status int `json:"Status"`
		Answer []struct {
			Name string `json:"name"`
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}

	var resp DNSResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse JSON failed: %w", err)
	}

	if resp.Status != 0 && resp.Status != 3 { // 0 = NOERROR, 3 = NXDOMAIN
		return nil, fmt.Errorf("DNS query returned status %d", resp.Status)
	}

	var ips []net.IPAddr
	for _, answer := range resp.Answer {
		// Type 1 = A record (IPv4), Type 28 = AAAA record (IPv6)
		if answer.Type == 1 || answer.Type == 28 {
			ip := net.ParseIP(answer.Data)
			if ip != nil {
				ips = append(ips, net.IPAddr{
					IP:   ip,
					Zone: "",
				})
			}
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found in response")
	}

	return ips, nil
}

// parseWireFormatResponse parses DNS wire format response (RFC 1035)
func (r *DoHResolver) parseWireFormatResponse(body []byte, host string) ([]net.IPAddr, error) {
	if len(body) < 12 {
		return nil, fmt.Errorf("response too short")
	}

	// Skip header (12 bytes)
	offset := 12

	// Parse question section
	qdCount, _ := getUint16(body[4:6])
	for i := 0; i < int(qdCount); i++ {
		_, offset, err := parseDomain(body, offset, 0)
		if err != nil {
			return nil, err
		}
		offset += 4 // QTYPE + QCLASS
	}

	// Parse answer section
	anCount, _ := getUint16(body[6:8])
	var ips []net.IPAddr

	for i := 0; i < int(anCount); i++ {
		_, newOffset, err := parseDomain(body, offset, 0)
		if err != nil {
			break
		}
		offset = newOffset

		if offset+10 > len(body) {
			break
		}

		// TYPE, CLASS, TTL, RDLENGTH
		recordType := int(body[offset])<<8 | int(body[offset+1])
		rdLength := int(body[offset+8])<<8 | int(body[offset+9])
		offset += 10

		if offset+rdLength > len(body) {
			break
		}

		// Type 1 = A record (IPv4), Type 28 = AAAA record (IPv6)
		if recordType == 1 && rdLength == 4 {
			ip := net.IP(body[offset : offset+4])
			ips = append(ips, net.IPAddr{IP: ip, Zone: ""})
		} else if recordType == 28 && rdLength == 16 {
			ip := net.IP(body[offset : offset+16])
			ips = append(ips, net.IPAddr{IP: ip, Zone: ""})
		}

		offset += rdLength
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found in response")
	}

	return ips, nil
}

// maxDomainRecursion limits the depth of DNS compression pointer recursion
// to prevent stack overflow from malicious responses with circular pointers.
// RFC 1035 allows compression, but malformed responses could exploit this.
// SECURITY: Increased from 10 to 16 to handle edge cases with legitimate but
// deeply nested compression while still preventing stack overflow.
const maxDomainRecursion = 16

// parseDomain parses a DNS domain name (RFC 1035) with recursion depth limit.
// The depth parameter tracks the current recursion depth to prevent stack overflow
// from malicious DNS responses with circular or deeply nested compression pointers.
func parseDomain(msg []byte, offset int, depth int) (string, int, error) {
	// SECURITY: Check recursion depth to prevent stack overflow from circular pointers
	if depth > maxDomainRecursion {
		return "", offset, fmt.Errorf("compression pointer depth exceeded (max %d)", maxDomainRecursion)
	}

	if offset >= len(msg) {
		return "", offset, fmt.Errorf("offset out of bounds")
	}

	var labels []string
	originalOffset := offset
	compressed := false

	for {
		if offset >= len(msg) {
			return "", originalOffset, fmt.Errorf("invalid domain name")
		}

		length := int(msg[offset])
		offset++

		if length == 0 {
			break
		}

		// Check for compression pointer
		if length >= 192 {
			if offset >= len(msg) {
				return "", originalOffset, fmt.Errorf("invalid compression pointer")
			}
			pointer := int(length&0x3F)<<8 | int(msg[offset])
			offset++

			if !compressed {
				compressed = true
			}

			// SECURITY: Pass incremented depth to recursive call
			compressedName, _, err := parseDomain(msg, pointer, depth+1)
			if err != nil {
				return "", originalOffset, err
			}
			labels = append(labels, compressedName)
			break
		}

		if offset+length > len(msg) {
			return "", originalOffset, fmt.Errorf("label exceeds message length")
		}

		labels = append(labels, string(msg[offset:offset+length]))
		offset += length
	}

	// Use strings.Join for O(n) allocation instead of O(n²) iterative concatenation
	return strings.Join(labels, "."), offset, nil
}

// getUint16 parses a big-endian 16-bit unsigned integer
func getUint16(b []byte) (uint16, error) {
	if len(b) < 2 {
		return 0, fmt.Errorf("buffer too short")
	}
	return uint16(b[0])<<8 | uint16(b[1]), nil
}

// fallbackLookup falls back to system DNS resolver
func (r *DoHResolver) fallbackLookup(ctx context.Context, host string, lastErr error) ([]net.IPAddr, error) {
	// Use system resolver as fallback
	systemResolver := net.DefaultResolver
	ips, err := systemResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DoH and system lookup both failed: %w (last DoH error: %w)", err, lastErr)
	}
	return ips, nil
}

// ClearCache clears the DNS cache
// SECURITY: Uses atomic counter reset to avoid race conditions with concurrent
// cache operations. The counter may be temporarily inaccurate during clear but
// will self-correct on subsequent operations.
func (r *DoHResolver) ClearCache() {
	// First, reset the counter to prevent new entries from being rejected
	// during the clear operation due to size limit checks
	r.cacheSize.Store(0)
	// Then delete all entries - any new entries added during this time
	// will correctly increment the counter from 0
	r.cache.Range(func(key, value interface{}) bool {
		r.cache.Delete(key)
		return true
	})
}

// SetCacheTTL sets the cache TTL duration.
// Thread-safe: can be called concurrently with other operations.
func (r *DoHResolver) SetCacheTTL(ttl time.Duration) {
	r.cacheTTL.Store(int64(ttl))
	r.ClearCache()
}

// GetCacheTTL returns the current cache TTL duration.
// Thread-safe: can be called concurrently with other operations.
func (r *DoHResolver) GetCacheTTL() time.Duration {
	return time.Duration(r.cacheTTL.Load())
}

// Close releases resources held by the DoHResolver.
// It closes idle connections in the internal HTTP client's transport.
// Safe to call multiple times - subsequent calls are no-ops.
// Implements io.Closer for consistent resource management.
func (r *DoHResolver) Close() error {
	// Use CompareAndSwap to ensure we only close once
	if !r.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Close idle connections in the transport
	if r.client != nil {
		if transport, ok := r.client.Transport.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
	}

	// Clear the cache to release memory
	r.ClearCache()

	return nil
}
