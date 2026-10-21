package fingerprint

import (
	"testing"
)

// TestProfile_HostLeadsTheHeaderBlock pins the Host slot: real browsers send
// Host first on HTTP/1.1, and the transport only places it at that position if
// the profile's order table names it. The placeholder carries no value — the
// transport writes Host itself — so it exists purely to reserve the slot.
func TestProfile_HostLeadsTheHeaderBlock(t *testing.T) {
	for _, p := range []*browserProfile{
		chrome152Desktop, chrome152Mobile,
		chrome155Desktop, chrome155Android, chrome155IOS,
		firefox148Desktop, firefox148Mobile,
		firefox158Desktop, firefox158Android, firefox158IOS,
	} {
		for _, method := range []string{"GET", "POST"} {
			order := p.order(method)
			if len(order) == 0 {
				t.Fatalf("%s/%s: empty order table", p.name, method)
			}

			// The first non-pseudo-header in the list must be "host".
			firstReal := ""
			for _, name := range order {
				if len(name) > 0 && name[0] != ':' {
					firstReal = name
					break
				}
			}
			if firstReal != "host" {
				t.Errorf("%s/%s: first real header = %q, want host first", p.name, method, firstReal)
			}
		}
	}
}

// TestProfile_HeaderCasingMatchesTheBrowser pins the per-profile wire casing:
// the Chromium client-hint family is lower case, everything else Title-Case,
// and Firefox emits no client hints at all.
func TestProfile_HeaderCasingMatchesTheBrowser(t *testing.T) {
	lowerCase := map[string]bool{
		"sec-ch-ua":          true,
		"sec-ch-ua-mobile":   true,
		"sec-ch-ua-platform": true,
	}

	profiles := []*browserProfile{
		chrome152Desktop, chrome152Mobile,
		chrome155Desktop, chrome155Android, chrome155IOS,
		firefox148Desktop, firefox148Mobile,
		firefox158Desktop, firefox158Android, firefox158IOS,
	}
	for _, p := range profiles {
		for _, method := range []string{"GET", "POST"} {
			for _, f := range p.mergedHeaders(osWindows, method) {
				if len(f.Name) == 0 || f.Name[0] == ':' {
					continue
				}
				if lowerCase[f.Name] {
					continue // the only names allowed in lower case
				}
				if f.Name != canonicalTestName(f.Name) {
					t.Errorf("%s/%s: %q is not Title-Case", p.name, method, f.Name)
				}
			}
		}
	}
}

// canonicalTestName upper-cases the first letter of each dash-separated token,
// the shape every browser uses for headers outside the client-hint family.
func canonicalTestName(name string) string {
	out := make([]byte, len(name))
	upper := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' {
			upper = true
			out[i] = c
			continue
		}
		if upper && c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		upper = false
		out[i] = c
	}
	return string(out)
}
