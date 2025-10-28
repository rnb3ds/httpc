package httpc

import (
	"time"
)

// SecurityLevel defines the security level for client configuration
type SecurityLevel int

const (
	// SecurityLevelBalanced provides a balance between security and compatibility (default)
	SecurityLevelBalanced SecurityLevel = iota
	// SecurityLevelStrict enforces strict security settings
	SecurityLevelStrict
)

// ConfigPreset creates a configuration with predefined security settings
func ConfigPreset(level SecurityLevel) *Config {
	switch level {
	case SecurityLevelStrict:
		return strictConfig()
	default:
		return DefaultConfig()
	}
}

// strictConfig returns a configuration with strict security settings
func strictConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second, // 更短的超时
		MaxIdleConns:        50,
		MaxConnsPerHost:     10,
		InsecureSkipVerify:  false,            // 严格的TLS验证
		MaxResponseBodySize: 10 * 1024 * 1024, // 更小的响应体限制 (10MB)
		AllowPrivateIPs:     false,            // 禁止私有IP访问
		MaxRetries:          1,                // 更少的重试次数
		RetryDelay:          3 * time.Second,
		BackoffFactor:       2.0,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     false, // 禁用重定向以防止重定向攻击
		EnableHTTP2:         true,
		EnableCookies:       false, // 在严格模式下禁用Cookie
	}
}
