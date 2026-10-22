//go:build fingerprint_e2e

// Version-lag drift gate: compares the major versions the shipped profiles
// claim (chromeMajor / firefoxMajor) against the vendors' current stable
// channels. Runs with the network e2e gates in the weekly canary
// (.github/workflows/fingerprint-drift.yml), because "the fingerprint bytes
// still match" is only half of drift — a profile whose UA names a version
// nobody runs anymore is a coherence failure the byte gates cannot see.
//
// The gate fails when a profile lags the stable channel by maxLag majors or
// more (Chrome ships roughly every two weeks, Firefox every four, so four
// majors is about two months of staleness) and logs the lag otherwise: the
// supported window, not the exact major, is the contract.
package fingerprint_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/fingerprint"
)

const maxLag = 4

func TestFingerprintDriftStableVersionLag(t *testing.T) {
	client := &http.Client{Timeout: 15 * time.Second}

	// chromeStable tries the official version-history API first, then a
	// CI-refreshed capture of the latest stable browsers' User-Agents on
	// GitHub (the API is unreachable from some networks; the fallback keeps
	// the gate runnable everywhere, and a few days of capture staleness is
	// harmless against a four-major threshold). The primary gets a short
	// budget — it answers in well under a second when reachable, so a longer
	// wait only burns time on networks where it will never answer.
	chromeStable := func() (int, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
			"https://versionhistory.googleapis.com/v1/chrome/platforms/win/channels/stable/versions", nil)
		res, err := client.Do(req)
		if err == nil {
			defer func() { _ = res.Body.Close() }()
			var out struct {
				Versions []struct {
					Version string `json:"version"`
				} `json:"versions"`
			}
			if err := json.NewDecoder(res.Body).Decode(&out); err == nil {
				best := 0
				for _, v := range out.Versions {
					if major, ok := parseMajor(v.Version); ok && major > best {
						best = major
					}
				}
				if best > 0 {
					return best, nil
				}
			}
		}

		res, err = client.Get("https://raw.githubusercontent.com/fa0311/latest-user-agent/main/header.json")
		if err != nil {
			return 0, err
		}
		defer func() { _ = res.Body.Close() }()
		var out map[string]struct {
			UserAgent string `json:"user-agent"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			return 0, err
		}
		capture, ok := out["linux-chrome"]
		if !ok {
			return 0, fmt.Errorf("no linux-chrome capture in fallback source")
		}
		i := strings.Index(capture.UserAgent, "Chrome/")
		if i < 0 {
			return 0, fmt.Errorf("capture UA %q carries no Chrome/ token", capture.UserAgent)
		}
		major, ok := parseMajor(capture.UserAgent[i+len("Chrome/"):])
		if !ok {
			return 0, fmt.Errorf("capture UA %q Chrome major not parseable", capture.UserAgent)
		}
		return major, nil
	}

	firefoxStable := func() (int, error) {
		res, err := client.Get("https://product-details.mozilla.org/1.0/firefox_versions.json")
		if err != nil {
			return 0, err
		}
		defer func() { _ = res.Body.Close() }()
		var out map[string]string
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			return 0, err
		}
		major, ok := parseMajor(out["LATEST_FIREFOX_VERSION"])
		if !ok {
			return 0, fmt.Errorf("LATEST_FIREFOX_VERSION %q not parseable", out["LATEST_FIREFOX_VERSION"])
		}
		return major, nil
	}

	for _, tt := range []struct {
		browser string
		shipped int
		stable  func() (int, error)
	}{
		{"chrome", fingerprint.ChromeMajor, chromeStable},
		{"firefox", fingerprint.FirefoxMajor, firefoxStable},
	} {
		t.Run(tt.browser, func(t *testing.T) {
			stable, err := tt.stable()
			if err != nil {
				t.Fatalf("lookup stable %s: %v", tt.browser, err)
			}
			lag := stable - tt.shipped
			t.Logf("%s: shipped %d, stable %d, lag %d majors", tt.browser, tt.shipped, stable, lag)
			if lag < 0 {
				t.Logf("%s: shipped major is AHEAD of stable (fresh bump or API lag) — nothing to do", tt.browser)
				return
			}
			if lag >= maxLag {
				t.Errorf("%s profile lags stable by %d majors (shipped %d, stable %d): "+
					"cut a new profile (see the profile_chrome.go / profile_firefox.go headers)", tt.browser, lag, tt.shipped, stable)
			}
		})
	}
}

// parseMajor extracts the leading major from a version string ("156.0.8078.48" → 156).
func parseMajor(version string) (int, bool) {
	if i := strings.IndexByte(version, '.'); i > 0 {
		version = version[:i]
	}
	n, err := strconv.Atoi(version)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
