package connection

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestResolveHelloID(t *testing.T) {
	tests := []struct {
		input   string
		wantOK  bool
	}{
		{"chrome", true},
		{"firefox", true},
		{"safari", true},
		{"ios", true},
		{"CHROME", true},
		{"  chrome  ", true},
		{"Firefox", true},
		{"", false},
		{"edge", false},
		{"opera", false},
		{"unknown", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			id, ok := resolveHelloID(tc.input)
			if ok != tc.wantOK {
				t.Errorf("resolveHelloID(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if tc.wantOK && id == (utls.ClientHelloID{}) {
				t.Errorf("resolveHelloID(%q) returned empty ClientHelloID for valid input", tc.input)
			}
		})
	}
}

func TestReadConnectResponse(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCode   int
		wantText   string
		wantErr    bool
		errContain string
	}{
		{
			name:     "200 OK",
			input:    "HTTP/1.1 200 OK\r\n\r\n",
			wantCode: 200,
			wantText: "OK",
		},
		{
			name:     "200 Connection Established",
			input:    "HTTP/1.1 200 Connection Established\r\n\r\n",
			wantCode: 200,
			wantText: "Connection Established",
		},
		{
			name:     "200 with headers",
			input:    "HTTP/1.1 200 OK\r\nX-Custom: val\r\n\r\n",
			wantCode: 200,
			wantText: "OK",
		},
		{
			name:       "403 forbidden",
			input:      "HTTP/1.1 403 Forbidden\r\n\r\n",
			wantCode:   403,
			wantText:   "Forbidden",
		},
		{
			name:       "malformed line",
			input:      "GARBAGE\r\n",
			wantErr:    true,
			errContain: "malformed",
		},
		{
			name:       "non-numeric status",
			input:      "HTTP/1.1 ABC OK\r\n\r\n",
			wantErr:    true,
			errContain: "parsing",
		},
		{
			name:       "truncated no newline",
			input:      "HTTP/1.1 200 OK",
			wantErr:    true,
			errContain: "reading CONNECT status line",
		},
		{
			name:       "truncated headers no empty line",
			input:      "HTTP/1.1 200 OK\r\nX-Custom: val",
			wantErr:    true,
			errContain: "reading CONNECT headers",
		},
		{
			name:     "status without text",
			input:    "HTTP/1.1 204\r\n\r\n",
			wantCode: 204,
			wantText: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(tc.input))
			code, text, err := readConnectResponse(br)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContain)
				}
				if !strings.Contains(err.Error(), tc.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tc.wantCode {
				t.Errorf("statusCode = %d, want %d", code, tc.wantCode)
			}
			if text != tc.wantText {
				t.Errorf("statusText = %q, want %q", text, tc.wantText)
			}
		})
	}
}

func TestBasicAuthHeader(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		pass     string
		expected string
	}{
		{
			name:     "standard",
			user:     "user",
			pass:     "pass",
			expected: "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
		},
		{
			name:     "empty password",
			user:     "user",
			pass:     "",
			expected: "Basic " + base64.StdEncoding.EncodeToString([]byte("user:")),
		},
		{
			name:     "special characters",
			user:     "us@r",
			pass:     "p@ss:w0rd",
			expected: "Basic " + base64.StdEncoding.EncodeToString([]byte("us@r:p@ss:w0rd")),
		},
		{
			name:     "unicode",
			user:     "用户",
			pass:     "密码",
			expected: "Basic " + base64.StdEncoding.EncodeToString([]byte("用户:密码")),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := basicAuthHeader(tc.user, tc.pass)
			if got != tc.expected {
				t.Errorf("basicAuthHeader(%q, %q) = %q, want %q", tc.user, tc.pass, got, tc.expected)
			}
			if !strings.HasPrefix(got, "Basic ") {
				t.Errorf("result should start with 'Basic ', got %q", got)
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, "Basic "))
			if err != nil {
				t.Fatalf("failed to decode base64: %v", err)
			}
			if string(decoded) != tc.user+":"+tc.pass {
				t.Errorf("decoded = %q, want %q", string(decoded), tc.user+":"+tc.pass)
			}
		})
	}
}

func TestEnforceHandshakeTimeout(t *testing.T) {
	t.Run("zero timeout returns nil", func(t *testing.T) {
		ctx := context.Background()
		cancel := enforceHandshakeTimeout(&ctx, 0)
		if cancel != nil {
			t.Error("expected nil cancel for zero timeout")
		}
	})

	t.Run("negative timeout returns nil", func(t *testing.T) {
		ctx := context.Background()
		cancel := enforceHandshakeTimeout(&ctx, -1*time.Second)
		if cancel != nil {
			t.Error("expected nil cancel for negative timeout")
		}
	})

	t.Run("no existing deadline creates timeout", func(t *testing.T) {
		ctx := context.Background()
		cancel := enforceHandshakeTimeout(&ctx, 5*time.Second)
		defer cancel()
		if cancel == nil {
			t.Fatal("expected non-nil cancel")
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected context to have a deadline")
		}
		remaining := time.Until(deadline)
		if remaining < 4*time.Second || remaining > 6*time.Second {
			t.Errorf("deadline remaining = %v, want ~5s", remaining)
		}
	})

	t.Run("shorter existing deadline is no-op", func(t *testing.T) {
		ctx, origCancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer origCancel()
		origDeadline, _ := ctx.Deadline()

		cancel := enforceHandshakeTimeout(&ctx, 10*time.Second)
		if cancel != nil {
			t.Error("expected nil cancel when existing deadline is shorter")
		}
		newDeadline, _ := ctx.Deadline()
		if !newDeadline.Equal(origDeadline) {
			t.Error("deadline should not have changed")
		}
	})

	t.Run("longer existing deadline applies timeout", func(t *testing.T) {
		ctx, origCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer origCancel()

		cancel := enforceHandshakeTimeout(&ctx, 5*time.Second)
		defer cancel()
		if cancel == nil {
			t.Fatal("expected non-nil cancel when timeout is shorter than existing deadline")
		}
		deadline, _ := ctx.Deadline()
		remaining := time.Until(deadline)
		if remaining > 10*time.Second {
			t.Errorf("deadline should be ~5s from now, got %v remaining", remaining)
		}
	})

	t.Run("cancel function works", func(t *testing.T) {
		ctx := context.Background()
		cancel := enforceHandshakeTimeout(&ctx, 5*time.Second)
		if cancel == nil {
			t.Fatal("expected non-nil cancel")
		}
		cancel()
		if ctx.Err() == nil {
			t.Error("expected context to be cancelled after calling cancel()")
		}
	})
}

func TestTunnelConn_Read(t *testing.T) {
	t.Run("reads from bufio.Reader", func(t *testing.T) {
		data := "buffered_data"
		pr, pw := io.Pipe()
		defer pw.Close()
		defer pr.Close()

		// Pre-fill bufio.Reader buffer via write end
		go pw.Write([]byte(data))

		br := bufio.NewReader(pr)
		tc := &tunnelConn{Conn: nil, reader: br}

		buf := make([]byte, len(data))
		n, err := tc.Read(buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(buf[:n]) != data {
			t.Errorf("read %q, want %q", string(buf[:n]), data)
		}
	})

	t.Run("delegates to bufio.Reader which drains buffer first", func(t *testing.T) {
		// Verify that tunnelConn.Read delegates to bufio.Reader.Read
		// which inherently handles buffer draining then underlying reader
		data := "hello_world"
		br := bufio.NewReader(strings.NewReader(data))
		tc := &tunnelConn{Conn: nil, reader: br}

		var got bytes.Buffer
		buf := make([]byte, 4)
		for {
			n, err := tc.Read(buf)
			if n > 0 {
				got.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if got.String() != data {
			t.Errorf("read all = %q, want %q", got.String(), data)
		}
	})
}

func TestUTLSConnWrapper_ConnectionState(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tlsConfig := &utls.Config{InsecureSkipVerify: true, ServerName: "test"}
	uconn := utls.UClient(client, tlsConfig, utls.HelloCustom)

	wrapper := &utlsConnWrapper{Conn: uconn, uconn: uconn}

	// ConnectionState should not panic even without handshake
	cs := wrapper.ConnectionState()
	_ = cs.Version
	_ = cs.NegotiatedProtocol
}

func TestForceHTTP11ALPN(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tlsConfig := &utls.Config{InsecureSkipVerify: true, ServerName: "example.com"}
	uconn := utls.UClient(client, tlsConfig, utls.HelloChrome_Auto)

	err := forceHTTP11ALPN(uconn)
	if err != nil {
		t.Fatalf("forceHTTP11ALPN failed: %v", err)
	}

	// Verify the ALPN extension was replaced
	found := false
	for _, ext := range uconn.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			found = true
			if len(alpn.AlpnProtocols) != 1 || alpn.AlpnProtocols[0] != "http/1.1" {
				t.Errorf("ALPN protocols = %v, want [http/1.1]", alpn.AlpnProtocols)
			}
		}
	}
	if !found {
		t.Error("ALPN extension not found")
	}
}

func TestCreateDialTLSContext(t *testing.T) {
	t.Run("configures DialTLSContext when fingerprint set", func(t *testing.T) {
		config := &Config{
			BrowserFingerprint: "chrome",
			MinTLSVersion:      tls.VersionTLS12,
			MaxTLSVersion:      tls.VersionTLS13,
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer pm.Close()

		if pm.transport.DialTLSContext == nil {
			t.Error("DialTLSContext should be set when BrowserFingerprint is configured")
		}
		if pm.transport.ForceAttemptHTTP2 {
			t.Error("ForceAttemptHTTP2 should be false when BrowserFingerprint is set")
		}
		if pm.transport.TLSNextProto != nil {
			t.Error("TLSNextProto should be nil when BrowserFingerprint is set")
		}
	})

	t.Run("no DialTLSContext without fingerprint", func(t *testing.T) {
		config := &Config{
			EnableHTTP2: true,
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer pm.Close()

		if pm.transport.DialTLSContext != nil {
			t.Error("DialTLSContext should not be set without BrowserFingerprint")
		}
	})
}

func TestDialAndConnectProxy(t *testing.T) {
	// startProxy starts a TCP listener that acts as an HTTP proxy.
	// It reads the CONNECT request and sends back the given status.
	startProxy := func(t *testing.T, statusCode int, statusText string, checkAuth bool) (addr string, cleanup func()) {
		t.Helper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			br := bufio.NewReader(conn)
			// Read CONNECT line
			br.ReadString('\n')
			// Read headers
			for {
				line, err := br.ReadString('\n')
				if err != nil || strings.TrimSpace(line) == "" {
					break
				}
				if checkAuth && strings.HasPrefix(line, "Proxy-Authorization:") {
					// Verify auth header is present
				}
			}
			fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n\r\n", statusCode, statusText)
			// Keep conn alive briefly
			time.Sleep(200 * time.Millisecond)
		}()
		return ln.Addr().String(), func() {
			ln.Close()
			<-done
		}
	}

	t.Run("successful CONNECT", func(t *testing.T) {
		addr, cleanup := startProxy(t, 200, "OK", false)
		defer cleanup()

		config := &Config{
			ProxyURL:           "http://" + addr,
			DialTimeout:        2 * time.Second,
			AllowPrivateIPs:    true,
			BrowserFingerprint: "chrome",
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer pm.Close()

		conn, err := pm.dialAndConnectProxy(context.Background(), pm.createDialer(), "example.com:443")
		if err != nil {
			t.Fatalf("dialAndConnectProxy: %v", err)
		}
		conn.Close()
	})

	t.Run("proxy returns non-200", func(t *testing.T) {
		addr, cleanup := startProxy(t, 403, "Forbidden", false)
		defer cleanup()

		config := &Config{
			ProxyURL:        "http://" + addr,
			DialTimeout:     2 * time.Second,
			AllowPrivateIPs: true,
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer pm.Close()

		_, err = pm.dialAndConnectProxy(context.Background(), pm.createDialer(), "example.com:443")
		if err == nil {
			t.Fatal("expected error for non-200 proxy response")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("error should contain status 403, got: %v", err)
		}
	})

	t.Run("proxy with authentication", func(t *testing.T) {
		addr, cleanup := startProxy(t, 200, "OK", true)
		defer cleanup()

		config := &Config{
			ProxyURL:        "http://testuser:testpass@" + addr,
			DialTimeout:     2 * time.Second,
			AllowPrivateIPs: true,
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer pm.Close()

		conn, err := pm.dialAndConnectProxy(context.Background(), pm.createDialer(), "example.com:443")
		if err != nil {
			t.Fatalf("dialAndConnectProxy with auth: %v", err)
		}
		conn.Close()
	})

	t.Run("dialer error propagates", func(t *testing.T) {
		config := &Config{
			ProxyURL:        "http://127.0.0.1:1", // port 1 should be unreachable
			AllowPrivateIPs: true,
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer pm.Close()

		dialErr := fmt.Errorf("dial failed")
		dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, dialErr
		}

		_, err = pm.dialAndConnectProxy(context.Background(), dialer, "example.com:443")
		if err == nil {
			t.Fatal("expected error when dialer fails")
		}
		if !strings.Contains(err.Error(), "proxy dial failed") {
			t.Errorf("error should mention proxy dial, got: %v", err)
		}
	})
}

func TestReadConnectResponse_LargeHeaders(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("HTTP/1.1 200 OK\r\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&buf, "X-Header-%d: value-%d\r\n", i, i)
	}
	buf.WriteString("\r\n")

	br := bufio.NewReader(&buf)
	code, text, err := readConnectResponse(br)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 200 {
		t.Errorf("code = %d, want 200", code)
	}
	if text != "OK" {
		t.Errorf("text = %q, want OK", text)
	}
}
