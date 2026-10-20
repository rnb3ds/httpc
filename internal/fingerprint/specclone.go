// Deep cloning of uTLS ClientHelloSpec values.
//
// Ported from github.com/enetx/surf (internal/specclone), Copyright enetx/surf
// authors, MIT License. The fingerprint transport mutates the per-connection
// spec copy (extension shuffle, GREASE signature-algorithm substitution, ALPN
// rewrite for forced HTTP/1), so dialing from the shared package-level profile
// specs requires a copy that shares no memory with its source.
//
// Every extension type used by the Chrome 152 and Firefox 148 profiles has an
// explicit case below; the reflection fallback covers any future profile data.
// Note the fallback's latent limit: deepCopyStruct copies unexported fields
// verbatim, so an extension that lazily initializes through a sync.Once would
// carry the already-fired Once into the clone and freeze the first
// connection's random state. No current profile contains such an extension
// outside the explicit switch (GREASE-ECH, the one that does, is handled
// explicitly), but keep this in mind when adding new profile data.

package fingerprint

import (
	"reflect"
	"unsafe"

	utls "github.com/refraction-networking/utls"
)

// cloneSpec creates a deep copy of a utls.ClientHelloSpec. The result shares
// no memory with the source: value fields are copied, slices re-allocated,
// and every extension re-created with its nested data copied.
func cloneSpec(c *utls.ClientHelloSpec) *utls.ClientHelloSpec {
	if c == nil {
		return nil
	}

	clone := &utls.ClientHelloSpec{
		TLSVersMin:         c.TLSVersMin,
		TLSVersMax:         c.TLSVersMax,
		GetSessionID:       c.GetSessionID,
		CipherSuites:       append([]uint16(nil), c.CipherSuites...),
		CompressionMethods: append([]uint8(nil), c.CompressionMethods...),
	}

	if len(c.Extensions) > 0 {
		clone.Extensions = make([]utls.TLSExtension, len(c.Extensions))
		for i, ext := range c.Extensions {
			if ext != nil {
				clone.Extensions[i] = deepCloneExtension(ext)
			}
		}
	}

	return clone
}

// deepCloneExtension performs type-specific deep cloning of TLS extensions.
// Extension types without an explicit case fall back to reflection-based
// cloning via deepCloneInterface.
func deepCloneExtension(ext utls.TLSExtension) utls.TLSExtension {
	switch e := ext.(type) {
	case *utls.SNIExtension:
		return &utls.SNIExtension{
			ServerName: e.ServerName,
		}
	case *utls.StatusRequestExtension:
		return &utls.StatusRequestExtension{}
	case *utls.SupportedCurvesExtension:
		return &utls.SupportedCurvesExtension{
			Curves: append([]utls.CurveID(nil), e.Curves...),
		}
	case *utls.SupportedPointsExtension:
		return &utls.SupportedPointsExtension{
			SupportedPoints: append([]uint8(nil), e.SupportedPoints...),
		}
	case *utls.SignatureAlgorithmsExtension:
		return &utls.SignatureAlgorithmsExtension{
			SupportedSignatureAlgorithms: append([]utls.SignatureScheme{}, e.SupportedSignatureAlgorithms...),
		}
	case *utls.ALPNExtension:
		return &utls.ALPNExtension{
			AlpnProtocols: append([]string{}, e.AlpnProtocols...),
		}
	case *utls.StatusRequestV2Extension:
		return &utls.StatusRequestV2Extension{}
	case *utls.SCTExtension:
		return &utls.SCTExtension{}
	case *utls.UtlsPaddingExtension:
		return &utls.UtlsPaddingExtension{
			PaddingLen:    e.PaddingLen,
			WillPad:       e.WillPad,
			GetPaddingLen: e.GetPaddingLen,
		}
	case *utls.ExtendedMasterSecretExtension:
		return &utls.ExtendedMasterSecretExtension{}
	case *utls.FakeTokenBindingExtension:
		var params []uint8
		if e.KeyParameters != nil {
			params = append([]uint8(nil), e.KeyParameters...)
		}

		return &utls.FakeTokenBindingExtension{
			MajorVersion:  e.MajorVersion,
			MinorVersion:  e.MinorVersion,
			KeyParameters: params,
		}
	case *utls.UtlsCompressCertExtension:
		return &utls.UtlsCompressCertExtension{
			Algorithms: append([]utls.CertCompressionAlgo{}, e.Algorithms...),
		}
	case *utls.FakeRecordSizeLimitExtension:
		return &utls.FakeRecordSizeLimitExtension{
			Limit: e.Limit,
		}
	case *utls.FakeDelegatedCredentialsExtension:
		return &utls.FakeDelegatedCredentialsExtension{
			SupportedSignatureAlgorithms: append([]utls.SignatureScheme{}, e.SupportedSignatureAlgorithms...),
		}
	case *utls.SessionTicketExtension:
		var session *utls.SessionState
		if e.Session != nil {
			extra := make([][]byte, len(e.Session.Extra))
			for i, b := range e.Session.Extra {
				if b != nil {
					extra[i] = append([]byte(nil), b...)
				}
			}

			session = &utls.SessionState{
				Extra:     extra,
				EarlyData: e.Session.EarlyData,
			}
		}

		var ticket []byte
		if e.Ticket != nil {
			ticket = append([]byte(nil), e.Ticket...)
		}

		return &utls.SessionTicketExtension{
			Session:     session,
			Ticket:      ticket,
			Initialized: e.Initialized,
		}
	case *utls.FakePreSharedKeyExtension:
		return &utls.FakePreSharedKeyExtension{
			Identities:   clonePskIdentities(e.Identities),
			Binders:      cloneByteSlices(e.Binders),
			OmitEmptyPsk: e.OmitEmptyPsk,
		}
	case *utls.UtlsPreSharedKeyExtension:
		var clonedSession *utls.SessionState
		if e.Session != nil {
			clonedSession = &utls.SessionState{
				Extra:     cloneByteSlices(e.Session.Extra),
				EarlyData: e.Session.EarlyData,
			}
		}

		return &utls.UtlsPreSharedKeyExtension{
			PreSharedKeyCommon: utls.PreSharedKeyCommon{
				Identities:  clonePskIdentities(e.Identities),
				Binders:     cloneByteSlices(e.Binders),
				BinderKey:   cloneBytes(e.BinderKey),
				EarlySecret: cloneBytes(e.EarlySecret),
				Session:     clonedSession,
			},
			OmitEmptyPsk: e.OmitEmptyPsk,
		}
	case *utls.SupportedVersionsExtension:
		return &utls.SupportedVersionsExtension{
			Versions: append([]uint16{}, e.Versions...),
		}
	case *utls.CookieExtension:
		var cookie []byte
		if e.Cookie != nil {
			cookie = append([]byte(nil), e.Cookie...)
		}
		return &utls.CookieExtension{Cookie: cookie}
	case *utls.PSKKeyExchangeModesExtension:
		return &utls.PSKKeyExchangeModesExtension{
			Modes: append([]uint8{}, e.Modes...),
		}
	case *utls.SignatureAlgorithmsCertExtension:
		return &utls.SignatureAlgorithmsCertExtension{
			SupportedSignatureAlgorithms: append([]utls.SignatureScheme{}, e.SupportedSignatureAlgorithms...),
		}
	case *utls.KeyShareExtension:
		keyShares := make([]utls.KeyShare, len(e.KeyShares))

		for i, ks := range e.KeyShares {
			var data []byte
			if ks.Data != nil {
				data = append([]byte(nil), ks.Data...)
			}

			keyShares[i] = utls.KeyShare{
				Group: ks.Group,
				Data:  data,
			}
		}
		return &utls.KeyShareExtension{KeyShares: keyShares}
	case *utls.QUICTransportParametersExtension:
		return &utls.QUICTransportParametersExtension{
			TransportParameters: cloneTransportParameters(e.TransportParameters),
		}
	case *utls.NPNExtension:
		return &utls.NPNExtension{
			NextProtos: append([]string{}, e.NextProtos...),
		}
	case *utls.ApplicationSettingsExtension:
		return &utls.ApplicationSettingsExtension{
			SupportedProtocols: append([]string{}, e.SupportedProtocols...),
		}
	case *utls.ApplicationSettingsExtensionNew:
		return &utls.ApplicationSettingsExtensionNew{
			SupportedProtocols: append([]string{}, e.SupportedProtocols...),
		}
	case *utls.FakeChannelIDExtension:
		return &utls.FakeChannelIDExtension{
			OldExtensionID: e.OldExtensionID,
		}
	case *utls.GREASEEncryptedClientHelloExtension:
		var configIDs []uint8
		if e.CandidateConfigIds != nil {
			configIDs = append([]uint8(nil), e.CandidateConfigIds...)
		}
		var encapsulatedKey []byte
		if e.EncapsulatedKey != nil {
			encapsulatedKey = append([]byte(nil), e.EncapsulatedKey...)
		}
		return &utls.GREASEEncryptedClientHelloExtension{
			CandidateCipherSuites: append([]utls.HPKESymmetricCipherSuite{}, e.CandidateCipherSuites...),
			CandidatePayloadLens:  append([]uint16(nil), e.CandidatePayloadLens...),
			// CandidateConfigIds / EncapsulatedKey were previously dropped:
			// harmless while BoringGREASEECH() only fills the first two
			// fields, but a future real-ECH capture would silently degrade to
			// random GREASE values on every connection. payload/initOnce are
			// deliberately NOT cloned (per-connection generated state).
			CandidateConfigIds: configIDs,
			EncapsulatedKey:    encapsulatedKey,
		}
	case *utls.RenegotiationInfoExtension:
		var reneg []byte
		if e.RenegotiatedConnection != nil {
			reneg = append([]byte(nil), e.RenegotiatedConnection...)
		}

		return &utls.RenegotiationInfoExtension{
			Renegotiation:          e.Renegotiation,
			RenegotiatedConnection: reneg,
		}
	case *utls.UtlsGREASEExtension:
		var body []byte
		if e.Body != nil {
			body = append([]byte(nil), e.Body...)
		}

		return &utls.UtlsGREASEExtension{
			Value: e.Value,
			Body:  body,
		}
	case *utls.GenericExtension:
		var data []byte
		if e.Data != nil {
			data = append([]byte(nil), e.Data...)
		}

		return &utls.GenericExtension{
			Id:   e.Id,
			Data: data,
		}
	case utls.PreSharedKeyExtension:
		switch psk := e.(type) {
		case *utls.FakePreSharedKeyExtension:
			return deepCloneExtension(psk)
		case *utls.UtlsPreSharedKeyExtension:
			return deepCloneExtension(psk)
		default:
			return deepCloneInterface(e).(utls.TLSExtension)
		}
	default:
		return deepCloneInterface(ext).(utls.TLSExtension)
	}
}

// cloneBytes returns a copy of b, preserving nil-ness so len/non-nil
// distinctions survive the clone.
func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return append([]byte(nil), b...)
}

// cloneByteSlices deep-copies a [][]byte, preserving per-element nil-ness.
func cloneByteSlices(s [][]byte) [][]byte {
	out := make([][]byte, len(s))
	for i, b := range s {
		if b != nil {
			out[i] = append([]byte(nil), b...)
		}
	}
	return out
}

// clonePskIdentities deep-copies PSK identities (label bytes + ticket age).
// Shared by the Fake and Utls pre-shared-key clones, which previously each
// carried a verbatim copy of this loop.
func clonePskIdentities(ids []utls.PskIdentity) []utls.PskIdentity {
	out := make([]utls.PskIdentity, len(ids))
	for i, id := range ids {
		out[i] = utls.PskIdentity{
			Label:               cloneBytes(id.Label),
			ObfuscatedTicketAge: id.ObfuscatedTicketAge,
		}
	}
	return out
}

// cloneTransportParameters deep-copies a QUIC transport-parameter list so the
// clone shares no mutable memory with the source. The value-type scalars
// (MaxIdleTimeout, MaxUDPPayloadSize, …) are immutable copies already; the
// elements that carry mutable byte/uint32 slices — GREASE, Fake, VersionInformation,
// and the []byte-backed InitialSourceConnectionID/PaddingTransportParameter — are
// re-allocated here.
func cloneTransportParameters(params utls.TransportParameters) utls.TransportParameters {
	if params == nil {
		return nil
	}
	out := make(utls.TransportParameters, len(params))
	for i, p := range params {
		out[i] = cloneTransportParameter(p)
	}
	return out
}

// cloneTransportParameter deep-copies a single QUIC transport parameter.
func cloneTransportParameter(p utls.TransportParameter) utls.TransportParameter {
	switch tp := p.(type) {
	case *utls.GREASETransportParameter:
		return &utls.GREASETransportParameter{
			IdOverride:    tp.IdOverride,
			Length:        tp.Length,
			ValueOverride: cloneBytes(tp.ValueOverride),
		}
	case *utls.FakeQUICTransportParameter:
		return &utls.FakeQUICTransportParameter{
			Id:  tp.Id,
			Val: cloneBytes(tp.Val),
		}
	case *utls.VersionInformation:
		return &utls.VersionInformation{
			ChoosenVersion:    tp.ChoosenVersion,
			AvailableVersions: append([]uint32(nil), tp.AvailableVersions...),
			LegacyID:          tp.LegacyID,
		}
	case utls.InitialSourceConnectionID:
		return append(utls.InitialSourceConnectionID(nil), tp...)
	case utls.PaddingTransportParameter:
		return append(utls.PaddingTransportParameter(nil), tp...)
	default:
		// Immutable scalar value types (e.g. MaxIdleTimeout) and the empty
		// pointer types (DisableActiveMigration, GREASEQUICBit) share no
		// mutable state; return them unchanged.
		return p
	}
}

// deepCopyStruct performs deep copying of struct values using reflection,
// including unexported fields that cannot be set through normal reflection.
// Field addresses are derived with unsafe.Add from BOTH the source's and the
// destination's base pointers (both are always addressable here — built via
// reflect.New or sliced from one), the documented pattern for
// unexported-field access. Mirroring the source too is what makes the
// scalar/interface branches of deepCopyValue legal on unexported fields.
func deepCopyStruct(src, dst reflect.Value) {
	if !src.IsValid() {
		return
	}

	switch src.Kind() {
	case reflect.Struct:
		// src is normally addressable (built via reflect.New or sliced from
		// one), but the reflection fallback can hand deepCopyStruct an
		// unaddressable struct — a non-pointer argument to deepCloneInterface,
		// or an entry of a struct-valued map (reflect.Value.MapIndex yields an
		// unaddressable copy). src.Addr() panics on an unaddressable value, so
		// mirror the source through a fresh addressable copy first. Set on a
		// settable destination from an unaddressable-but-CanInterface source is
		// legal and copies unexported fields too (only a flagRO source panics,
		// which is handled by the per-field CanAddr mirror below).
		srcAddr := src
		if !srcAddr.CanAddr() {
			tmp := reflect.New(src.Type()).Elem()
			tmp.Set(src)
			srcAddr = tmp
		}
		srcBase := srcAddr.Addr().UnsafePointer()
		dstBase := dst.Addr().UnsafePointer()
		for i := 0; i < srcAddr.NumField(); i++ {
			srcField := srcAddr.Field(i)
			dstField := dst.Field(i)

			if !dstField.CanSet() {
				// Mirror BOTH sides through unsafe pointers: dst as before,
				// and src via its own address. Without the src mirror, the
				// value handed to deepCopyValue stays read-only (flagRO), so
				// the scalar/interface/chan/func branches panic with "Set
				// using value obtained using unexported field" — the
				// advertised "reflection fallback covers any future profile
				// data" did not hold for extensions carrying unexported
				// scalar or interface fields.
				if srcField.CanAddr() {
					offset := int(src.Type().Field(i).Offset)
					srcFieldRO := reflect.NewAt(srcField.Type(), unsafe.Add(srcBase, offset)).Elem()
					dstFieldPtr := reflect.NewAt(srcField.Type(), unsafe.Add(dstBase, offset))
					deepCopyValue(srcFieldRO, dstFieldPtr.Elem())
				}
			} else {
				deepCopyValue(srcField, dstField)
			}
		}
	default:
		deepCopyValue(src, dst)
	}
}

// deepCopyValue deep-copies values of the supported kinds: structs recurse
// field-wise, slices/arrays/maps are rebuilt element-wise, pointers are
// re-allocated, interfaces delegate to deepCloneInterface, and chan/func
// values are copied by reference (they cannot be deep copied).
func deepCopyValue(src, dst reflect.Value) {
	if !src.IsValid() {
		return
	}

	switch src.Kind() {
	case reflect.Struct:
		deepCopyStruct(src, dst)
	case reflect.Slice:
		if src.IsNil() {
			if dst.CanSet() {
				dst.Set(reflect.Zero(src.Type()))
			}
			return
		}

		newSlice := reflect.MakeSlice(src.Type(), src.Len(), src.Cap())

		for i := 0; i < src.Len(); i++ {
			deepCopyValue(src.Index(i), newSlice.Index(i))
		}

		if dst.CanSet() {
			dst.Set(newSlice)
		}
	case reflect.Array:
		for i := 0; i < src.Len(); i++ {
			deepCopyValue(src.Index(i), dst.Index(i))
		}
	case reflect.Map:
		if src.IsNil() {
			if dst.CanSet() {
				dst.Set(reflect.Zero(src.Type()))
			}
			return
		}

		newMap := reflect.MakeMap(src.Type())

		for _, key := range src.MapKeys() {
			newKey := reflect.New(key.Type()).Elem()
			newVal := reflect.New(src.MapIndex(key).Type()).Elem()

			deepCopyValue(key, newKey)
			deepCopyValue(src.MapIndex(key), newVal)

			newMap.SetMapIndex(newKey, newVal)
		}

		if dst.CanSet() {
			dst.Set(newMap)
		}
	case reflect.Pointer:
		if src.IsNil() {
			if dst.CanSet() {
				dst.Set(reflect.Zero(src.Type()))
			}
			return
		}

		newPtr := reflect.New(src.Type().Elem())

		deepCopyValue(src.Elem(), newPtr.Elem())

		if dst.CanSet() {
			dst.Set(newPtr)
		}
	case reflect.Interface:
		if src.IsNil() {
			if dst.CanSet() {
				dst.Set(reflect.Zero(src.Type()))
			}
			return
		}

		newVal := deepCloneInterface(src.Interface())

		if dst.CanSet() {
			dst.Set(reflect.ValueOf(newVal))
		}
	case reflect.Chan, reflect.Func:
		if dst.CanSet() {
			dst.Set(src)
		}
	default:
		if dst.CanSet() {
			dst.Set(src)
		}
	}
}

// deepCloneInterface creates a deep copy of any value using reflection; the
// fallback path for extension types without an explicit deepCloneExtension
// case.
func deepCloneInterface(src any) any {
	if src == nil {
		return nil
	}

	srcVal := reflect.ValueOf(src)
	srcType := srcVal.Type()

	if srcType.Kind() == reflect.Pointer {
		if srcVal.IsNil() {
			return nil
		}

		dstPtr := reflect.New(srcType.Elem())
		deepCopyValue(srcVal.Elem(), dstPtr.Elem())

		return dstPtr.Interface()
	}

	dst := reflect.New(srcType).Elem()
	deepCopyValue(srcVal, dst)

	return dst.Interface()
}
