// Package dns provides DNS-over-HTTPS resolution with caching support
// for the httpc library.
package dns

import (
	"context"
	"encoding/json"
	"errors"
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
var _ resolver = (*DoHResolver)(nil)

// DoHProvider represents a DNS-over-HTTPS service provider with a URL template and priority.
type DoHProvider struct {
	Name     string
	Template string // URL template with {name} placeholder
	Priority int
}

// cacheEntry holds cached DNS resolution results
type cacheEntry struct {
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

// DefaultDoHProviders returns common DoH providers.
// Templates use {name} for hostname and {type} for record type (A/AAAA).
func DefaultDoHProviders() []*DoHProvider {
	return []*DoHProvider{
		{
			Name:     "cloudflare",
			Template: "https://1.1.1.1/dns-query?name={name}&type={type}",
			Priority: 1,
		},
		{
			Name:     "google",
			Template: "https://dns.google/resolve?name={name}&type={type}",
			Priority: 2,
		},
		{
			Name:     "ali",
			Template: "https://dns.alidns.com/resolve?name={name}&type={type}",
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

	// Check cache — Load is safe since cacheEntry is read-only after creation.
	if cached, ok := r.cache.Load(host); ok {
		if entry, typeOk := cached.(*cacheEntry); typeOk && entry != nil {
			if time.Now().Before(entry.Expires) {
				// Deep copy to prevent caller mutations from affecting cached data
				ips := make([]net.IPAddr, len(entry.IPs))
				for i, addr := range entry.IPs {
					ips[i] = net.IPAddr{IP: make(net.IP, len(addr.IP)), Zone: addr.Zone}
					copy(ips[i].IP, addr.IP)
				}
				return ips, nil
			}
			// Expired — atomically delete and decrement counter only if we win the race
			if _, deleted := r.cache.LoadAndDelete(host); deleted {
				r.cacheSize.Add(-1)
			}
		}
	}

	// Try each provider until one succeeds
	var lastErr error
	for _, provider := range r.providers {
		ips, err := r.lookupWithProvider(ctx, provider, host)
		if err == nil && len(ips) > 0 {
			// SECURITY: Use CAS to atomically reserve cache slot before storing
			for {
				current := r.cacheSize.Load()
				if current >= maxDoHCacheSize {
					// Cache full â evict expired entries to make room
					r.evictExpiredEntries()
					if r.cacheSize.Load() >= maxDoHCacheSize {
						break // Still full after eviction, skip caching
					}
					continue
				}
				if r.cacheSize.CompareAndSwap(current, current+1) {
					cacheTTL := time.Duration(r.cacheTTL.Load())
					// Deep copy IPs for cache isolation
					cachedIPs := make([]net.IPAddr, len(ips))
					for j, addr := range ips {
						cachedIPs[j] = net.IPAddr{IP: make(net.IP, len(addr.IP)), Zone: addr.Zone}
						copy(cachedIPs[j].IP, addr.IP)
					}
					newEntry := &cacheEntry{
						IPs:     cachedIPs,
						Expires: time.Now().Add(cacheTTL),
					}
					// Use LoadOrStore to detect concurrent stores and prevent counter drift.
					// If another goroutine already stored an entry for this host,
					// revert our counter increment since no new slot was consumed.
					if _, exists := r.cache.LoadOrStore(host, newEntry); exists {
						r.cacheSize.Add(-1)
					}
					break
				}
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

// lookupWithProvider performs DNS lookup using a specific DoH provider.
// It queries both A (IPv4) and AAAA (IPv6) records concurrently.
func (r *DoHResolver) lookupWithProvider(ctx context.Context, provider *DoHProvider, host string) ([]net.IPAddr, error) {
	escapedHost := url.PathEscape(host)

	type lookupResult struct {
		ips []net.IPAddr
		err error
	}

	chA := make(chan lookupResult, 1)
	chAAAA := make(chan lookupResult, 1)

	// Query A and AAAA records concurrently
	go func() {
		ips, err := r.lookupRecordType(ctx, provider, escapedHost, "A")
		chA <- lookupResult{ips: ips, err: err}
	}()
	go func() {
		ips, err := r.lookupRecordType(ctx, provider, escapedHost, "AAAA")
		chAAAA <- lookupResult{ips: ips, err: err}
	}()

	resA := <-chA
	resAAAA := <-chAAAA

	// Merge results: return any IPs found
	var ips []net.IPAddr
	if resA.err == nil {
		ips = append(ips, resA.ips...)
	}
	if resAAAA.err == nil {
		ips = append(ips, resAAAA.ips...)
	}
	if len(ips) > 0 {
		return ips, nil
	}

	// Both failed — return the first non-nil error, preferring A record error
	if resA.err != nil {
		return nil, resA.err
	}
	return nil, resAAAA.err
}

// lookupRecordType queries a specific DNS record type (A or AAAA) via DoH.
func (r *DoHResolver) lookupRecordType(ctx context.Context, provider *DoHProvider, escapedHost, recordType string) ([]net.IPAddr, error) {
	// Build request URL: replace {name} and {type} placeholders
	requestURL := strings.Replace(provider.Template, "{name}", escapedHost, 1)
	requestURL = strings.Replace(requestURL, "{type}", recordType, 1)
	// Fallback: append type parameter if template doesn't have {type}
	if !strings.Contains(provider.Template, "{type}") {
		sep := "&"
		if !strings.Contains(requestURL, "?") {
			sep = "?"
		}
		requestURL += sep + "type=" + recordType
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Accept", "application/dns-json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DoH request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDoHResponseSize))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH request returned status %d", resp.StatusCode)
	}

	return r.parseResponse(resp, provider, escapedHost)
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
	qdCount, err := getUint16(body[4:6])
	if err != nil {
		return nil, fmt.Errorf("invalid question count: %w", err)
	}
	for i := 0; i < int(qdCount); i++ {
		var err error
		_, offset, err = parseDomain(body, offset, 0)
		if err != nil {
			return nil, err
		}
		offset += 4 // QTYPE + QCLASS
	}

	// Parse answer section
	anCount, err := getUint16(body[6:8])
	if err != nil {
		return nil, fmt.Errorf("invalid answer count: %w", err)
	}
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
		// Copy IP bytes to avoid aliasing the response body slice,
		// which would corrupt IP data if the body is GC'd or reused.
		if recordType == 1 && rdLength == 4 {
			ip := make(net.IP, 4)
			copy(ip, body[offset:offset+4])
			ips = append(ips, net.IPAddr{IP: ip, Zone: ""})
		} else if recordType == 28 && rdLength == 16 {
			ip := make(net.IP, 16)
			copy(ip, body[offset:offset+16])
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
		return nil, fmt.Errorf("DoH and system lookup both failed: %w", errors.Join(err, lastErr))
	}
	return ips, nil
}

// ClearCache clears the DNS cache.
// Deletes all entries and atomically resets the counter to zero.
func (r *DoHResolver) ClearCache() {
	r.cache.Range(func(key, value any) bool {
		r.cache.Delete(key)
		return true
	})
	r.cacheSize.Store(0)
}

// evictExpiredEntries removes expired cache entries to free space.
// Called proactively when the cache is full to avoid discarding fresh results.
func (r *DoHResolver) evictExpiredEntries() {
	now := time.Now()
	r.cache.Range(func(key, value any) bool {
		if entry, ok := value.(*cacheEntry); ok && entry != nil {
			if now.After(entry.Expires) {
				if _, deleted := r.cache.LoadAndDelete(key); deleted {
					r.cacheSize.Add(-1)
				}
			}
		}
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
