package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// DoHResolver provides DNS-over-HTTPS resolution
type DoHResolver struct {
	client    *http.Client
	providers []*DoHProvider
	cache     sync.Map
	cacheTTL  time.Duration
}

// DoHProvider represents a DoH service provider
type DoHProvider struct {
	Name     string
	Template string // URL template with {name} placeholder
	Priority int
}

// CacheEntry holds cached DNS resolution results
type CacheEntry struct {
	IPs      []net.IPAddr
	Expires  time.Time
}

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
	if providers == nil || len(providers) == 0 {
		providers = DefaultDoHProviders()
	}
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}

	return &DoHResolver{
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  true,
				DisableKeepAlives:   false,
				ForceAttemptHTTP2:   true,
			},
		},
		providers: providers,
		cacheTTL:  cacheTTL,
	}
}

// LookupIPAddr resolves a host name to IP addresses using DoH
func (r *DoHResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	// Check cache first
	if cached, ok := r.cache.Load(host); ok {
		entry := cached.(*CacheEntry)
		if time.Now().Before(entry.Expires) {
			return entry.IPs, nil
		}
		r.cache.Delete(host)
	}

	// Try each provider until one succeeds
	var lastErr error
	for _, provider := range r.providers {
		ips, err := r.lookupWithProvider(ctx, provider, host)
		if err == nil && len(ips) > 0 {
			// Cache the result
			r.cache.Store(host, &CacheEntry{
				IPs:     ips,
				Expires: time.Now().Add(r.cacheTTL),
			})
			return ips, nil
		}
		lastErr = err
	}

	// Fallback to system resolver if all providers fail
	return r.fallbackLookup(ctx, host, lastErr)
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

// parseResponse parses DoH response
func (r *DoHResolver) parseResponse(resp *http.Response, provider *DoHProvider, host string) ([]net.IPAddr, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
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
		Status  int `json:"Status"`
		Answer  []struct {
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
		_, offset, err := parseDomain(body, offset)
		if err != nil {
			return nil, err
		}
		offset += 4 // QTYPE + QCLASS
	}

	// Parse answer section
	anCount, _ := getUint16(body[6:8])
	var ips []net.IPAddr

	for i := 0; i < int(anCount); i++ {
		_, newOffset, err := parseDomain(body, offset)
		if err != nil {
			break
		}
		offset = newOffset

		if offset+10 > len(body) {
			break
		}

		// TYPE, CLASS, TTL, RDLENGTH
		recordType := int(body[offset])<<8 | int(body[offset+1])
		// class := int(body[offset+2])<<8 | int(body[offset+3])
		// ttl := int(body[offset+4])<<24 | int(body[offset+5])<<16 | int(body[offset+6])<<8 | int(body[offset+7])
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

// parseDomain parses a DNS domain name (RFC 1035)
func parseDomain(msg []byte, offset int) (string, int, error) {
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

			// Parse the compressed domain
			compressedName, _, err := parseDomain(msg, pointer)
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

	domain := ""
	for i, label := range labels {
		if i > 0 {
			domain += "."
		}
		domain += label
	}

	return domain, offset, nil
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
func (r *DoHResolver) ClearCache() {
	r.cache.Range(func(key, value interface{}) bool {
		r.cache.Delete(key)
		return true
	})
}

// SetCacheTTL sets the cache TTL duration
func (r *DoHResolver) SetCacheTTL(ttl time.Duration) {
	r.cacheTTL = ttl
	r.ClearCache()
}
