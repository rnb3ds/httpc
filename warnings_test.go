package httpc

import (
	"io"
	"testing"
)

// TestSetSecurityWarnOutput_NormalizesNil pins the nil guard: storing a nil
// writer would make the next New() panic inside fmt.Fprintf, far from the
// misconfigured SetSecurityWarnOutput(nil) call site (and gated behind a
// sync.Once, so the panic fires at most once per process, at a distance).
func TestSetSecurityWarnOutput_NormalizesNil(t *testing.T) {
	orig := getSecurityWarnOutput()
	defer SetSecurityWarnOutput(orig)

	SetSecurityWarnOutput(nil)
	if got := getSecurityWarnOutput(); got != io.Discard {
		t.Errorf("getSecurityWarnOutput() = %v, want io.Discard", got)
	}
}
