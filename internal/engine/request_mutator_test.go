package engine

import (
	"testing"

	"github.com/cybergodev/httpc/internal/types"
)

// TestRequest_MutatorAdapter covers the four interface-facing callback bridges
// on Request (OnRequestMutator/OnResponseMutator/SetOnRequestMutator/
// SetOnResponseMutator). These methods are the sole path through which the
// public facade installs callbacks without naming *engine.Request (see
// types.RequestCallbackSetter and public_options.go), so they were previously
// 0%-covered despite being part of the exported contract.
func TestRequest_MutatorAdapter(t *testing.T) {
	t.Run("OnRequestMutator nil when unset", func(t *testing.T) {
		r := &Request{}
		if got := r.OnRequestMutator(); got != nil {
			t.Error("OnRequestMutator = non-nil, want nil when no callback set")
		}
	})

	t.Run("OnRequestMutator adapts concrete callback", func(t *testing.T) {
		r := &Request{}
		var seen *Request
		r.SetOnRequest(func(req *Request) error {
			seen = req
			return nil
		})

		adapted := r.OnRequestMutator()
		if adapted == nil {
			t.Fatal("OnRequestMutator = nil, want adapter for set callback")
		}

		// The adapter narrows a types.RequestMutator back to *Request.
		if err := adapted(r); err != nil {
			t.Errorf("adapted callback returned error: %v", err)
		}
		if seen != r {
			t.Errorf("adapted callback saw %v, want %v", seen, r)
		}
	})

	t.Run("OnResponseMutator nil when unset", func(t *testing.T) {
		r := &Request{}
		if got := r.OnResponseMutator(); got != nil {
			t.Error("OnResponseMutator = non-nil, want nil when no callback set")
		}
	})

	t.Run("OnResponseMutator adapts concrete callback", func(t *testing.T) {
		r := &Request{}
		var seen *Response
		r.SetOnResponse(func(resp *Response) error {
			seen = resp
			return nil
		})

		adapted := r.OnResponseMutator()
		if adapted == nil {
			t.Fatal("OnResponseMutator = nil, want adapter for set callback")
		}

		resp := &Response{}
		if err := adapted(resp); err != nil {
			t.Errorf("adapted callback returned error: %v", err)
		}
		if seen != resp {
			t.Errorf("adapted callback saw %v, want %v", seen, resp)
		}
	})

	t.Run("SetOnRequestMutator installs and clears", func(t *testing.T) {
		r := &Request{}

		// Install via the interface-facing setter.
		var called bool
		r.SetOnRequestMutator(func(req types.RequestMutator) error {
			called = true
			return nil
		})
		if r.OnRequestMutator() == nil {
			t.Fatal("OnRequestMutator = nil after SetOnRequestMutator")
		}
		if err := r.OnRequestMutator()(r); err != nil {
			t.Errorf("installed callback returned error: %v", err)
		}
		if !called {
			t.Error("installed callback was not invoked")
		}

		// nil clears the callback (the public WithOnRequest wraps SetOnRequest
		// in a nil-guard, but the contract here is that nil clears).
		r.SetOnRequestMutator(nil)
		if r.OnRequestMutator() != nil {
			t.Error("OnRequestMutator should be nil after SetOnRequestMutator(nil)")
		}
	})

	t.Run("SetOnResponseMutator installs and clears", func(t *testing.T) {
		r := &Request{}

		var called bool
		r.SetOnResponseMutator(func(resp types.ResponseMutator) error {
			called = true
			return nil
		})
		if r.OnResponseMutator() == nil {
			t.Fatal("OnResponseMutator = nil after SetOnResponseMutator")
		}
		if err := r.OnResponseMutator()(&Response{}); err != nil {
			t.Errorf("installed callback returned error: %v", err)
		}
		if !called {
			t.Error("installed callback was not invoked")
		}

		r.SetOnResponseMutator(nil)
		if r.OnResponseMutator() != nil {
			t.Error("OnResponseMutator should be nil after SetOnResponseMutator(nil)")
		}
	})
}
