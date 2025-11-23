package engine

import (
	"github.com/cybergodev/httpc/internal/connection"
)

// testConnectionConfig returns a connection config suitable for testing
func testConnectionConfig() *connection.Config {
	config := connection.DefaultConfig()
	config.AllowPrivateIPs = true
	return config
}
