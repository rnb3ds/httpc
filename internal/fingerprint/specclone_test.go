package fingerprint

import (
	"reflect"
	"testing"

	utls "github.com/refraction-networking/utls"
)

// TestCloneSpec_Independence pins the deep-copy contract: mutating the clone
// (extension shuffle, GREASE substitution, ALPN filtering) must never touch
// the shared profile spec, or concurrent dials would race and corrupt the
// package-level data.
func TestCloneSpec_Independence(t *testing.T) {
	src := &chrome152Spec

	clone := cloneSpec(src)

	if clone == src {
		t.Fatal("clone returned the source pointer")
	}
	if len(clone.CipherSuites) == 0 || len(clone.Extensions) == 0 {
		t.Fatal("clone lost suites/extensions")
	}

	// Mutate every mutable surface the dial path touches.
	clone.CipherSuites[0] = 0xdead
	clone.Extensions[0], clone.Extensions[1] = clone.Extensions[1], clone.Extensions[0]
	if alpn := findALPN(clone); alpn != nil {
		alpn.AlpnProtocols = alpn.AlpnProtocols[:0]
	}
	if sigs := findSigAlgs(clone); sigs != nil {
		sigs.SupportedSignatureAlgorithms[0] = 0xbeef
	}

	if src.CipherSuites[0] == 0xdead {
		t.Error("cipher suite mutation leaked into source")
	}
	if src.Extensions[0] == clone.Extensions[0] && src.Extensions[1] == clone.Extensions[1] {
		// Identical adjacent extension pointers are possible in theory; with
		// the Chrome spec's distinct types at 0/1, equality means the swap
		// aliased the slices.
		if reflect.DeepEqual(src.Extensions[:2], clone.Extensions[:2]) &&
			src.Extensions[0] == clone.Extensions[1] {
			t.Error("extension slice mutation aliased source order")
		}
	}
	if alpn := findALPN(src); alpn != nil && len(alpn.AlpnProtocols) == 0 {
		t.Error("ALPN mutation leaked into source")
	}
	if sigs := findSigAlgs(src); sigs != nil && sigs.SupportedSignatureAlgorithms[0] == 0xbeef {
		t.Error("signature algorithm mutation leaked into source")
	}
}

// TestCloneSpec_CoversProfileExtensions pins that every STATEFUL extension
// in both shipped profiles is deep-cloned to a distinct allocation (a clone
// sharing an extension pointer with its source would share mutable state).
// Zero-size extensions (e.g. ExtendedMasterSecretExtension) are exempt: the
// runtime gives all zero-size allocations the same address (runtime.zerobase)
// and there is no state to share.
func TestCloneSpec_CoversProfileExtensions(t *testing.T) {
	for _, src := range []*utls.ClientHelloSpec{&chrome152Spec, &chrome152SpecMobile} {
		clone := cloneSpec(src)
		seen := make(map[utls.TLSExtension]bool, len(src.Extensions))
		for _, ext := range src.Extensions {
			seen[ext] = true
		}
		for _, ext := range clone.Extensions {
			if ext == nil {
				t.Fatal("nil extension in clone")
			}
			if t := reflect.TypeOf(ext); t.Kind() == reflect.Pointer && t.Elem().Size() == 0 {
				continue
			}
			if seen[ext] {
				t.Fatalf("clone shares stateful extension pointer %T with source", ext)
			}
		}
	}
}

// TestCloneSpec_Nil pins the nil contract.
func TestCloneSpec_Nil(t *testing.T) {
	if cloneSpec(nil) != nil {
		t.Fatal("cloneSpec(nil) must return nil")
	}
}

func findALPN(spec *utls.ClientHelloSpec) *utls.ALPNExtension {
	for _, ext := range spec.Extensions {
		if a, ok := ext.(*utls.ALPNExtension); ok {
			return a
		}
	}
	return nil
}

func findSigAlgs(spec *utls.ClientHelloSpec) *utls.SignatureAlgorithmsExtension {
	for _, ext := range spec.Extensions {
		if s, ok := ext.(*utls.SignatureAlgorithmsExtension); ok {
			return s
		}
	}
	return nil
}

// TestDeepCloneInterface_UnexportedScalarFields pins the reflection-fallback
// fix: the source is mirrored through unsafe addressing too, so unexported
// scalar/interface fields clone instead of panicking with "Set using value
// obtained using unexported field". (Currently unreachable via profile data —
// every shipped extension has an explicit case — this is the safety net for
// future extension types.)
func TestDeepCloneInterface_UnexportedScalarFields(t *testing.T) {
	type profileExt struct {
		Public  string
		secret  string
		flag    bool
		count   int
		hidden  any
		payload []byte
	}
	src := &profileExt{Public: "p", secret: "s", flag: true, count: 7, hidden: "if", payload: []byte{1, 2}}

	got, ok := deepCloneInterface(src).(*profileExt)
	if !ok {
		t.Fatalf("clone returned %T", deepCloneInterface(src))
	}
	if got.Public != "p" || got.secret != "s" || !got.flag || got.count != 7 || got.hidden != "if" {
		t.Errorf("scalar/interface fields not cloned: %+v", got)
	}
	if len(got.payload) != 2 || got.payload[0] != 1 {
		t.Errorf("payload not cloned: %v", got.payload)
	}

	src.secret = "changed"
	if got.secret != "s" {
		t.Error("clone aliases the source")
	}
}

// TestDeepCloneExtension_GREASEECHPreservesAllConfiguredFields pins the
// GREASE-ECH clone fix: CandidateConfigIds and EncapsulatedKey are cloned
// too. Harmless while BoringGREASEECH() fills only the first two fields, but
// a future real-ECH capture must not silently degrade to GREASE randomness
// on every connection.
func TestDeepCloneExtension_GREASEECHPreservesAllConfiguredFields(t *testing.T) {
	src := &utls.GREASEEncryptedClientHelloExtension{
		CandidateCipherSuites: []utls.HPKESymmetricCipherSuite{{}},
		CandidatePayloadLens:  []uint16{100, 200},
		CandidateConfigIds:    []uint8{0xab, 0xcd},
		EncapsulatedKey:       []byte{9, 8, 7},
	}

	got, ok := deepCloneExtension(src).(*utls.GREASEEncryptedClientHelloExtension)
	if !ok {
		t.Fatalf("clone returned %T", deepCloneExtension(src))
	}
	if len(got.CandidateConfigIds) != 2 || got.CandidateConfigIds[0] != 0xab {
		t.Errorf("CandidateConfigIds not cloned: %v", got.CandidateConfigIds)
	}
	if len(got.EncapsulatedKey) != 3 || got.EncapsulatedKey[2] != 7 {
		t.Errorf("EncapsulatedKey not cloned: %v", got.EncapsulatedKey)
	}

	src.EncapsulatedKey[0] = 0xff
	if got.EncapsulatedKey[0] == 0xff {
		t.Error("EncapsulatedKey aliases the source")
	}
}

// TestDeepCloneExtension_AllTypes walks every extension kind the switch
// handles: each clone is a distinct allocation that is field-for-field equal
// to its source (including nil sub-fields, which must stay nil, not empty).
func TestDeepCloneExtension_AllTypes(t *testing.T) {
	sessionState := &utls.SessionState{
		Extra:     [][]byte{{1, 2}, nil, {3}},
		EarlyData: true,
	}

	tests := []struct {
		name string
		ext  utls.TLSExtension
	}{
		{"SNI", &utls.SNIExtension{ServerName: "example.com"}},
		{"status request (OCSP)", &utls.StatusRequestExtension{}},
		{"supported curves", &utls.SupportedCurvesExtension{Curves: []utls.CurveID{utls.X25519, utls.CurveP256}}},
		{"supported points", &utls.SupportedPointsExtension{SupportedPoints: []uint8{0, 2}}},
		{"signature algorithms", &utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{utls.ECDSAWithP256AndSHA256}}},
		{"ALPN", &utls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}}},
		{"status request v2", &utls.StatusRequestV2Extension{}},
		{"SCT", &utls.SCTExtension{}},
		{"utls padding", &utls.UtlsPaddingExtension{PaddingLen: 16, WillPad: true}}, // functor covered separately (DeepEqual cannot compare funcs)
		{"extended master secret", &utls.ExtendedMasterSecretExtension{}},
		{"fake token binding nil params", &utls.FakeTokenBindingExtension{MajorVersion: 1, MinorVersion: 2}},
		{"fake token binding", &utls.FakeTokenBindingExtension{MajorVersion: 1, MinorVersion: 2, KeyParameters: []uint8{1, 2}}},
		{"compress cert", &utls.UtlsCompressCertExtension{Algorithms: []utls.CertCompressionAlgo{utls.CertCompressionBrotli}}},
		{"record size limit", &utls.FakeRecordSizeLimitExtension{Limit: 0x4001}},
		{"delegated credentials", &utls.FakeDelegatedCredentialsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{utls.PKCS1WithSHA256}}},
		{"session ticket empty", &utls.SessionTicketExtension{}},
		{"session ticket with session and ticket", &utls.SessionTicketExtension{Session: sessionState, Ticket: []byte{9, 9}, Initialized: true}},
		{"fake pre-shared key", &utls.FakePreSharedKeyExtension{
			OmitEmptyPsk: true,
			Identities:   []utls.PskIdentity{{Label: []byte{1, 2}, ObfuscatedTicketAge: 42}, {Label: nil}},
			Binders:      [][]byte{{7}, nil},
		}},
		{"utls pre-shared key", &utls.UtlsPreSharedKeyExtension{
			PreSharedKeyCommon: utls.PreSharedKeyCommon{
				Identities:  []utls.PskIdentity{{Label: []byte{3, 4}, ObfuscatedTicketAge: 7}},
				Binders:     [][]byte{{8, 8}},
				BinderKey:   []byte{0xAA},
				EarlySecret: []byte{0xBB},
				Session:     sessionState,
			},
			OmitEmptyPsk: true,
		}},
		{"supported versions", &utls.SupportedVersionsExtension{Versions: []uint16{utls.VersionTLS13, utls.VersionTLS12}}},
		{"cookie nil", &utls.CookieExtension{}},
		{"cookie", &utls.CookieExtension{Cookie: []byte{1, 2, 3}}},
		{"psk key exchange modes", &utls.PSKKeyExchangeModesExtension{Modes: []uint8{0, 1}}},
		{"signature algorithms cert", &utls.SignatureAlgorithmsCertExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{utls.PKCS1WithSHA512}}},
		{"key share nil data", &utls.KeyShareExtension{KeyShares: []utls.KeyShare{{Group: utls.X25519}}}},
		{"key share", &utls.KeyShareExtension{KeyShares: []utls.KeyShare{{Group: utls.X25519, Data: []byte{5, 6}}}}},
		{"quic transport parameters", &utls.QUICTransportParametersExtension{TransportParameters: utls.TransportParameters{
			&utls.GREASETransportParameter{IdOverride: 0x1a1a, Length: 8, ValueOverride: []byte{1, 2, 3}},
			&utls.FakeQUICTransportParameter{Id: 1, Val: []byte{4, 5}},
			&utls.VersionInformation{ChoosenVersion: utls.VERSION_1, AvailableVersions: []uint32{utls.VERSION_1, utls.VERSION_2}},
			utls.InitialSourceConnectionID{0xaa, 0xbb},
			utls.PaddingTransportParameter{0xcc},
			utls.MaxIdleTimeout(30),
		}}},
		{"NPN", &utls.NPNExtension{NextProtos: []string{"spdy/3.1"}}},
		{"application settings", &utls.ApplicationSettingsExtension{SupportedProtocols: []string{"h2"}}},
		{"application settings (new id)", &utls.ApplicationSettingsExtensionNew{SupportedProtocols: []string{"h3"}}},
		{"fake channel id", &utls.FakeChannelIDExtension{OldExtensionID: true}},
		{"renegotiation info nil", &utls.RenegotiationInfoExtension{}},
		{"renegotiation info", &utls.RenegotiationInfoExtension{RenegotiatedConnection: []byte{1}, Renegotiation: utls.RenegotiateOnceAsClient}},
		{"utls grease nil body", &utls.UtlsGREASEExtension{Value: 0x0a0a}},
		{"utls grease", &utls.UtlsGREASEExtension{Value: 0x1a1a, Body: []byte{0}}},
		{"generic nil data", &utls.GenericExtension{Id: 12345}},
		{"generic", &utls.GenericExtension{Id: 12345, Data: []byte{9, 9, 9}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := deepCloneExtension(tt.ext)
			if clone == nil {
				t.Fatal("clone is nil")
			}
			// Zero-size extensions get the runtime's shared zerobase address
			// for every allocation (no state to share — same exemption as
			// TestCloneSpec_CoversProfileExtensions); skip the identity check
			// for them.
			if typ := reflect.TypeOf(tt.ext); typ.Elem().Size() != 0 && clone == tt.ext {
				t.Fatal("clone returned the source pointer")
			}
			if reflect.TypeOf(clone) != reflect.TypeOf(tt.ext) {
				t.Fatalf("clone type = %T, want %T", clone, tt.ext)
			}
			if !reflect.DeepEqual(clone, tt.ext) {
				t.Errorf("clone not field-for-field equal:\n got %+v\nwant %+v", clone, tt.ext)
			}
		})
	}
}

// TestDeepCloneExtension_FuncFieldsSurvive pins that functor fields (the
// padding extension's GetPaddingLen) are carried to the clone by value.
// reflect.DeepEqual cannot compare funcs, so this is asserted behaviorally.
func TestDeepCloneExtension_FuncFieldsSurvive(t *testing.T) {
	src := &utls.UtlsPaddingExtension{
		PaddingLen:    16,
		WillPad:       true,
		GetPaddingLen: func(unpadded int) (int, bool) { return unpadded + 16, true },
	}

	got, ok := deepCloneExtension(src).(*utls.UtlsPaddingExtension)
	if !ok {
		t.Fatalf("clone type = %T", deepCloneExtension(src))
	}
	if got.GetPaddingLen == nil {
		t.Fatal("GetPaddingLen functor dropped by the clone")
	}
	if n, will := got.GetPaddingLen(100); n != 116 || !will {
		t.Errorf("cloned functor misbehaves: (%d, %v)", n, will)
	}
	if got.PaddingLen != 16 || !got.WillPad {
		t.Errorf("scalar fields lost: %+v", got)
	}
}

// thirdPartyPSK is a PreSharedKeyExtension implementation that is neither the
// Fake nor the Utls variant, to drive the interface case's reflection
// fallback. The embedded variant supplies the interface's unexported methods.
type thirdPartyPSK struct {
	utls.UtlsPreSharedKeyExtension
}

// TestDeepCloneExtension_ThirdPreSharedKeyImplementation pins the interface
// fallback inside the PreSharedKeyExtension case: an unknown PSK variant is
// cloned via reflection with its state intact.
func TestDeepCloneExtension_ThirdPreSharedKeyImplementation(t *testing.T) {
	src := &thirdPartyPSK{UtlsPreSharedKeyExtension: utls.UtlsPreSharedKeyExtension{
		PreSharedKeyCommon: utls.PreSharedKeyCommon{
			Identities: []utls.PskIdentity{{Label: []byte{1}, ObfuscatedTicketAge: 3}},
			Binders:    [][]byte{{2}},
		},
	}}

	// Interface dispatch: the switch must land in the PreSharedKeyExtension
	// case and take the default (reflection) branch.
	var asPSK utls.PreSharedKeyExtension = src
	cloneExt := deepCloneExtension(asPSK)

	got, ok := cloneExt.(*thirdPartyPSK)
	if !ok {
		t.Fatalf("clone type = %T, want *thirdPartyPSK", cloneExt)
	}
	if got == src {
		t.Fatal("clone returned the source pointer")
	}
	if !reflect.DeepEqual(got, src) {
		t.Errorf("reflection clone lost state:\n got %+v\nwant %+v", got, src)
	}

	// Independence: mutating the source's nested slice must not surface in
	// the clone.
	src.Identities[0].Label[0] = 0xFF
	if got.Identities[0].Label[0] == 0xFF {
		t.Error("reflection clone aliases the source identities")
	}
}

// TestCloneSpecHelpers pins the small clone helpers' nil-preservation
// contracts (len==0 vs nil must survive) and independence.
func TestCloneSpecHelpers(t *testing.T) {
	t.Run("cloneBytes", func(t *testing.T) {
		if cloneBytes(nil) != nil {
			t.Error("cloneBytes(nil) must stay nil")
		}
		b := []byte{1, 2, 3}
		got := cloneBytes(b)
		got[0] = 0xFF
		if b[0] == 0xFF {
			t.Error("cloneBytes aliases the source")
		}
	})

	t.Run("cloneByteSlices preserves per-element nil-ness", func(t *testing.T) {
		src := [][]byte{{1}, nil, {3, 4}}
		got := cloneByteSlices(src)
		if got[1] != nil {
			t.Error("nil element became non-nil")
		}
		got[0][0] = 0xFF
		if src[0][0] == 0xFF {
			t.Error("cloneByteSlices aliases the source")
		}
	})

	t.Run("clonePskIdentities", func(t *testing.T) {
		src := []utls.PskIdentity{{Label: []byte{1}, ObfuscatedTicketAge: 9}, {Label: nil}}
		got := clonePskIdentities(src)
		if len(got) != 2 || got[0].ObfuscatedTicketAge != 9 {
			t.Fatalf("clonePskIdentities lost data: %+v", got)
		}
		got[0].Label[0] = 0xFF
		if src[0].Label[0] == 0xFF {
			t.Error("clonePskIdentities aliases labels")
		}
	})
}

// TestCloneSpec_SessionTicketAndPSKIndependence pins that the ticketing /
// resumption state a future session-resumption knob would mutate cannot leak
// between a profile spec and its dial copies.
func TestCloneSpec_SessionTicketAndPSKIndependence(t *testing.T) {
	src := &utls.ClientHelloSpec{
		CipherSuites: []uint16{0x1301},
		Extensions: []utls.TLSExtension{
			&utls.SessionTicketExtension{
				Session: &utls.SessionState{Extra: [][]byte{{1}}, EarlyData: true},
				Ticket:  []byte{7, 7, 7},
			},
			&utls.UtlsPreSharedKeyExtension{
				PreSharedKeyCommon: utls.PreSharedKeyCommon{
					Identities:  []utls.PskIdentity{{Label: []byte{1, 2, 3}}},
					Binders:     [][]byte{{4}},
					BinderKey:   []byte{5},
					EarlySecret: []byte{6},
					Session:     &utls.SessionState{Extra: [][]byte{{2}}},
				},
			},
			&utls.FakePreSharedKeyExtension{
				Identities: []utls.PskIdentity{{Label: []byte{8}}},
				Binders:    [][]byte{{9}},
			},
			&utls.KeyShareExtension{KeyShares: []utls.KeyShare{{Group: utls.X25519, Data: []byte{1, 1}}}},
		},
	}

	clone := cloneSpec(src)

	// Mutate every nested byte surface on the source.
	src.Extensions[0].(*utls.SessionTicketExtension).Ticket[0] = 0xFF
	src.Extensions[0].(*utls.SessionTicketExtension).Session.Extra[0][0] = 0xFF
	psk := src.Extensions[1].(*utls.UtlsPreSharedKeyExtension)
	psk.Identities[0].Label[0] = 0xFF
	psk.Binders[0][0] = 0xFF
	psk.BinderKey[0] = 0xFF
	psk.EarlySecret[0] = 0xFF
	psk.Session.Extra[0][0] = 0xFF
	fake := src.Extensions[2].(*utls.FakePreSharedKeyExtension)
	fake.Identities[0].Label[0] = 0xFF
	fake.Binders[0][0] = 0xFF
	src.Extensions[3].(*utls.KeyShareExtension).KeyShares[0].Data[0] = 0xFF

	gotTicket := clone.Extensions[0].(*utls.SessionTicketExtension)
	if gotTicket.Ticket[0] == 0xFF || gotTicket.Session.Extra[0][0] == 0xFF {
		t.Error("session ticket state aliases the source")
	}
	gotPSK := clone.Extensions[1].(*utls.UtlsPreSharedKeyExtension)
	if gotPSK.Identities[0].Label[0] == 0xFF || gotPSK.Binders[0][0] == 0xFF ||
		gotPSK.BinderKey[0] == 0xFF || gotPSK.EarlySecret[0] == 0xFF ||
		gotPSK.Session.Extra[0][0] == 0xFF {
		t.Error("utls PSK state aliases the source")
	}
	gotFake := clone.Extensions[2].(*utls.FakePreSharedKeyExtension)
	if gotFake.Identities[0].Label[0] == 0xFF || gotFake.Binders[0][0] == 0xFF {
		t.Error("fake PSK state aliases the source")
	}
	if clone.Extensions[3].(*utls.KeyShareExtension).KeyShares[0].Data[0] == 0xFF {
		t.Error("key share data aliases the source")
	}
}

// TestCloneSpec_ScalarSurface pins the value-field contract: TLS versions,
// session-id hook, cipher suites and compression methods survive a clone,
// and a spec without extensions clones to a spec without extensions (nil,
// not an empty non-nil slice).
func TestCloneSpec_ScalarSurface(t *testing.T) {
	getSession := func(ticket []byte) [32]byte { return [32]byte{} }
	src := &utls.ClientHelloSpec{
		TLSVersMin:         utls.VersionTLS12,
		TLSVersMax:         utls.VersionTLS13,
		GetSessionID:       getSession,
		CipherSuites:       []uint16{0x1301, 0x1302},
		CompressionMethods: []uint8{0},
	}

	clone := cloneSpec(src)
	if clone.TLSVersMin != src.TLSVersMin || clone.TLSVersMax != src.TLSVersMax {
		t.Error("TLS version bounds lost")
	}
	if clone.GetSessionID == nil {
		t.Error("GetSessionID hook lost")
	}
	if len(clone.CipherSuites) != 2 || len(clone.CompressionMethods) != 1 {
		t.Errorf("suites/methods lost: %v %v", clone.CipherSuites, clone.CompressionMethods)
	}
	if clone.Extensions != nil {
		t.Errorf("nil Extensions must stay nil, got %v", clone.Extensions)
	}

	// Suite mutation independence.
	clone.CipherSuites[0] = 0xDEAD
	if src.CipherSuites[0] == 0xDEAD {
		t.Error("cipher suites alias the source")
	}
}

// dcNested and dcFixture exercise every reflect.Kind branch of deepCopyValue
// through the public reflection fallback (deepCloneInterface).
type dcNested struct {
	Value int
	Slice []byte
}

type dcFixture struct {
	Ptr      *dcNested
	NilPtr   *dcNested
	Arr      [3]int
	Slice    []dcNested
	NilSlice []int
	Map      map[string]*dcNested // pointer-valued (struct-valued maps are exercised by TestDeepCopyValue_UnaddressableStruct)
	NilMap   map[string]int
	Iface    any // holds *dcNested
	NilIface any
	Ch       chan int
	Fn       func(int) int
	Str      string
	Num      int

	nested dcNested // unexported struct field
	hidden []byte   // unexported slice field
}

// TestDeepCopyValue_KindCoverage drives every branch of deepCopyValue via the
// reflection fallback: structs, slices (nil and valued), arrays, maps, pointers,
// interfaces, chan/func-by-reference, and scalars — then proves independence
// by mutating every mutable surface of the source.
func TestDeepCopyValue_KindCoverage(t *testing.T) {
	ch := make(chan int, 1)
	fn := func(x int) int { return x + 1 }

	src := &dcFixture{
		Ptr:    &dcNested{Value: 1, Slice: []byte{1, 2}},
		Arr:    [3]int{1, 2, 3},
		Slice:  []dcNested{{Value: 3, Slice: []byte{3}}},
		Map:    map[string]*dcNested{"k": {Value: 4, Slice: []byte{4}}},
		Iface:  &dcNested{Value: 5, Slice: []byte{5}},
		Ch:     ch,
		Fn:     fn,
		Str:    "hello",
		Num:    7,
		nested: dcNested{Value: 8, Slice: []byte{8}},
		hidden: []byte{9, 9},
	}

	gotAny := deepCloneInterface(src)
	got, ok := gotAny.(*dcFixture)
	if !ok {
		t.Fatalf("deepCloneInterface returned %T, want *dcFixture", gotAny)
	}

	// chan/func copy by reference (documented contract).
	if got.Ch != ch || got.Fn == nil {
		t.Error("chan/func not carried by reference")
	}
	if got.Ptr.Value != 1 || got.Arr != [3]int{1, 2, 3} || got.Str != "hello" || got.Num != 7 {
		t.Errorf("value fields lost: %+v", got)
	}
	if got.Map["k"].Value != 4 || got.Slice[0].Value != 3 {
		t.Errorf("map/slice elements lost: %+v", got)
	}
	if gotIface, ok := got.Iface.(*dcNested); !ok || gotIface.Value != 5 {
		t.Errorf("interface element lost: %v", got.Iface)
	}
	if got.nested.Value != 8 || len(got.hidden) != 2 {
		t.Errorf("unexported fields lost: %+v %v", got.nested, got.hidden)
	}

	// Independence on every mutable surface.
	src.Ptr.Value = 100
	src.Arr[0] = 100
	src.Slice[0].Value = 100
	src.Slice[0].Slice[0] = 100
	src.Map["k"].Value = 100
	src.Map["k"].Slice[0] = 100
	src.Map["gone"] = &dcNested{Value: 100}
	src.Iface.(*dcNested).Value = 100
	src.nested.Value = 100
	src.hidden[0] = 100

	if got.Ptr.Value == 100 || got.Arr[0] == 100 || got.Slice[0].Value == 100 ||
		got.Slice[0].Slice[0] == 100 || got.Map["k"].Value == 100 || got.Map["k"].Slice[0] == 100 ||
		got.Iface.(*dcNested).Value == 100 || got.nested.Value == 100 || got.hidden[0] == 100 {
		t.Error("clone aliases the source on a mutable surface")
	}

	// Nil-ness survives.
	if got.NilPtr != nil || got.NilSlice != nil || got.NilMap != nil || got.NilIface != nil {
		t.Errorf("nil-ness not preserved: %+v", got)
	}
}

// TestDeepCopyValue_UnaddressableStruct pins the reflection-fallback fix: an
// unaddressable struct value — a non-pointer argument to deepCloneInterface,
// or an entry of a struct-valued map (reflect.Value.MapIndex hands out
// unaddressable copies) — no longer panics in deepCopyStruct's src.Addr();
// the source is mirrored through a fresh addressable copy first.
func TestDeepCopyValue_UnaddressableStruct(t *testing.T) {
	t.Run("non-pointer struct argument", func(t *testing.T) {
		src := dcNested{Value: 1, Slice: []byte{1, 2}}
		got, ok := deepCloneInterface(src).(dcNested)
		if !ok {
			t.Fatalf("deepCloneInterface returned %T, want dcNested", deepCloneInterface(src))
		}
		if got.Value != 1 || len(got.Slice) != 2 {
			t.Errorf("non-pointer struct not cloned: %+v", got)
		}
		src.Slice[0] = 0xFF
		if got.Slice[0] == 0xFF {
			t.Error("non-pointer struct clone aliases the source")
		}
	})

	t.Run("struct-valued map entry", func(t *testing.T) {
		src := &struct {
			M map[string]dcNested
		}{M: map[string]dcNested{"k": {Value: 7, Slice: []byte{3}}}}

		got, ok := deepCloneInterface(src).(*struct{ M map[string]dcNested })
		if !ok {
			t.Fatalf("deepCloneInterface returned %T", deepCloneInterface(src))
		}
		if got.M["k"].Value != 7 || len(got.M["k"].Slice) != 1 {
			t.Errorf("struct-valued map entry not cloned: %+v", got.M["k"])
		}
		src.M["k"].Slice[0] = 0xFF
		if got.M["k"].Slice[0] == 0xFF {
			t.Error("struct-valued map entry clone aliases the source")
		}
	})
}

// TestDeepCloneInterface_NonPointerAndScalars pins the non-pointer branch of
// the reflection fallback and the scalar default arm.
func TestDeepCloneInterface_NonPointerAndScalars(t *testing.T) {
	if got := deepCloneInterface(42); got != 42 {
		t.Errorf("deepCloneInterface(42) = %v", got)
	}
	if got := deepCloneInterface("str"); got != "str" {
		t.Errorf("deepCloneInterface(string) = %v", got)
	}
	// A map value (non-pointer, non-struct kind).
	m := map[string]int{"a": 1}
	gotMap, ok := deepCloneInterface(m).(map[string]int)
	if !ok || gotMap["a"] != 1 {
		t.Errorf("deepCloneInterface(map) = %v", deepCloneInterface(m))
	}
}

// TestDeepCloneInterface_NilPointer pins the typed-nil pointer branch.
func TestDeepCloneInterface_NilPointer(t *testing.T) {
	if got := deepCloneInterface((*dcFixture)(nil)); got != nil {
		t.Errorf("deepCloneInterface((*T)(nil)) = %v, want nil", got)
	}
	if got := deepCloneInterface(nil); got != nil {
		t.Errorf("deepCloneInterface(nil) = %v, want nil", got)
	}
}

// TestDeepCloneExtension_InterfaceTypedPSKVariants pins the inner dispatch
// of the PreSharedKeyExtension case: a value held in the interface type (as
// the extension list does) reaches the concrete Fake/Utls clones, not the
// reflection fallback.
func TestDeepCloneExtension_InterfaceTypedPSKVariants(t *testing.T) {
	t.Run("fake variant", func(t *testing.T) {
		var asPSK utls.PreSharedKeyExtension = &utls.FakePreSharedKeyExtension{
			Identities: []utls.PskIdentity{{Label: []byte{1}}},
			Binders:    [][]byte{{2}},
		}
		got, ok := deepCloneExtension(asPSK).(*utls.FakePreSharedKeyExtension)
		if !ok {
			t.Fatalf("clone type = %T, want *FakePreSharedKeyExtension", deepCloneExtension(asPSK))
		}
		if len(got.Identities) != 1 || len(got.Binders) != 1 {
			t.Errorf("state lost: %+v", got)
		}
	})

	t.Run("utls variant", func(t *testing.T) {
		var asPSK utls.PreSharedKeyExtension = &utls.UtlsPreSharedKeyExtension{
			PreSharedKeyCommon: utls.PreSharedKeyCommon{BinderKey: []byte{3}},
		}
		got, ok := deepCloneExtension(asPSK).(*utls.UtlsPreSharedKeyExtension)
		if !ok {
			t.Fatalf("clone type = %T, want *UtlsPreSharedKeyExtension", deepCloneExtension(asPSK))
		}
		if len(got.BinderKey) != 1 {
			t.Errorf("state lost: %+v", got)
		}
	})
}

// customExt is an extension type absent from deepCloneExtension's switch, to
// drive the outer default (reflection fallback). Embedding a concrete utls
// extension supplies the interface's unexported method.
type customExt struct {
	utls.SNIExtension
	Extra []byte
}

// TestDeepCloneExtension_UnknownExtensionFallsBackToReflection pins the outer
// default arm: future extension types clone via reflection with all fields,
// exported and unexported, intact.
func TestDeepCloneExtension_UnknownExtensionFallsBackToReflection(t *testing.T) {
	src := &customExt{SNIExtension: utls.SNIExtension{ServerName: "example.com"}, Extra: []byte{1, 2}}

	got, ok := deepCloneExtension(src).(*customExt)
	if !ok {
		t.Fatalf("clone type = %T, want *customExt", deepCloneExtension(src))
	}
	if got == src {
		t.Fatal("clone returned the source pointer")
	}
	if got.ServerName != "example.com" || len(got.Extra) != 2 {
		t.Errorf("reflection clone lost fields: %+v", got)
	}

	src.Extra[0] = 0xFF
	if got.Extra[0] == 0xFF {
		t.Error("reflection clone aliases the source")
	}
}

// TestDeepCopyGuardArms pins the defensive guard arms of the reflection
// helpers directly: invalid (zero) Values are no-ops, and deepCopyStruct's
// non-struct default delegates to deepCopyValue.
func TestDeepCopyGuardArms(t *testing.T) {
	dst := reflect.New(reflect.TypeOf(42)).Elem()

	// Invalid src: no-op for both helpers.
	deepCopyStruct(reflect.Value{}, dst)
	deepCopyValue(reflect.Value{}, dst)
	if dst.Interface() != 0 {
		t.Errorf("invalid src should leave dst untouched, got %v", dst.Interface())
	}

	// Non-struct value through deepCopyStruct's default arm.
	deepCopyStruct(reflect.ValueOf(7), dst)
	if dst.Interface() != 7 {
		t.Errorf("deepCopyStruct(non-struct) = %v, want 7", dst.Interface())
	}
}
