// Deep cloning of uTLS ClientHelloSpec values.
//
// The transport mutates the spec it hands to the handshake: this package
// rewrites the signature-algorithm list (GREASE substitution) and shuffles the
// extension order, and the transport library itself rewrites the ALPN list
// (forced HTTP/1.1) and the ALPS protocol list (h3 removal). Several of those
// writes go through extension structs by pointer.
//
// Because the profiles hold package-level spec values, every handshake must
// start from a copy: without one, a single EnableHTTP2=false request would
// leave the shared ALPN extension saying "http/1.1" and every later
// connection would silently negotiate HTTP/1.1 too.
//
// Every extension type used by the shipped profiles has an explicit case
// below; the reflection fallback covers any future profile data.

package fingerprint

import (
	"reflect"
	"unsafe"

	utls "github.com/bogdanfinn/utls"
)

// cloneSpec creates a deep copy of a utls.ClientHelloSpec that shares no
// mutable memory with the source. A nil input returns nil.
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
		return &utls.SNIExtension{ServerName: e.ServerName}
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
	case *utls.UtlsCompressCertExtension:
		return &utls.UtlsCompressCertExtension{
			Algorithms: append([]utls.CertCompressionAlgo{}, e.Algorithms...),
		}
	case *utls.SessionTicketExtension:
		// SessionState carries unexported fields (resumption secret, cipher
		// suite, certificates, ALPN, ...); hand-copying only the two exported
		// ones would clone a broken session, strictly worse than the
		// reflection fallback below, which copies unexported fields too.
		// Today's profiles hold zero-value tickets (Session == nil), so this
		// delegation is defensive.
		var session *utls.SessionState
		if e.Session != nil {
			session, _ = deepCloneInterface(e.Session).(*utls.SessionState)
		}
		return &utls.SessionTicketExtension{
			Session:     session,
			Ticket:      cloneBytes(e.Ticket),
			Initialized: e.Initialized,
		}
	case *utls.FakePreSharedKeyExtension:
		return &utls.FakePreSharedKeyExtension{
			Identities:   clonePskIdentities(e.Identities),
			Binders:      cloneByteSlices(e.Binders),
			OmitEmptyPsk: e.OmitEmptyPsk, //nolint:staticcheck // the fork still reads this field; no Config-level switch on this tls-client version //nolint:staticcheck // the fork still reads this field; no Config-level switch on this tls-client version
		}
	case *utls.UtlsPreSharedKeyExtension:
		// As above: delegate the SessionState clone to the reflection
		// fallback rather than hand-copying only the exported fields.
		var session *utls.SessionState
		if e.Session != nil {
			session, _ = deepCloneInterface(e.Session).(*utls.SessionState)
		}
		return &utls.UtlsPreSharedKeyExtension{
			PreSharedKeyCommon: utls.PreSharedKeyCommon{
				Identities:  clonePskIdentities(e.Identities),
				Binders:     cloneByteSlices(e.Binders),
				BinderKey:   cloneBytes(e.BinderKey),
				EarlySecret: cloneBytes(e.EarlySecret),
				Session:     session,
			},
			OmitEmptyPsk: e.OmitEmptyPsk, //nolint:staticcheck // the fork still reads this field; no Config-level switch on this tls-client version //nolint:staticcheck // the fork still reads this field; no Config-level switch on this tls-client version
		}
	case *utls.SupportedVersionsExtension:
		return &utls.SupportedVersionsExtension{
			Versions: append([]uint16{}, e.Versions...),
		}
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
			keyShares[i] = utls.KeyShare{Group: ks.Group, Data: cloneBytes(ks.Data)}
		}
		return &utls.KeyShareExtension{KeyShares: keyShares}
	case *utls.ApplicationSettingsExtension:
		return &utls.ApplicationSettingsExtension{
			SupportedProtocols: append([]string{}, e.SupportedProtocols...),
		}
	case *utls.ApplicationSettingsExtensionNew:
		return &utls.ApplicationSettingsExtensionNew{
			SupportedProtocols: append([]string{}, e.SupportedProtocols...),
		}
	case *utls.GREASEEncryptedClientHelloExtension:
		return &utls.GREASEEncryptedClientHelloExtension{
			CandidateCipherSuites: append([]utls.HPKESymmetricCipherSuite{}, e.CandidateCipherSuites...),
			CandidatePayloadLens:  append([]uint16(nil), e.CandidatePayloadLens...),
			CandidateConfigIds:    cloneBytes(e.CandidateConfigIds),
			EncapsulatedKey:       cloneBytes(e.EncapsulatedKey),
			// payload/initOnce are per-connection generated state and are
			// deliberately not cloned.
		}
	case *utls.RenegotiationInfoExtension:
		return &utls.RenegotiationInfoExtension{
			Renegotiation:          e.Renegotiation,
			RenegotiatedConnection: cloneBytes(e.RenegotiatedConnection),
		}
	case *utls.UtlsGREASEExtension:
		return &utls.UtlsGREASEExtension{
			Value: e.Value,
			Body:  cloneBytes(e.Body),
		}
	case *utls.GenericExtension:
		return &utls.GenericExtension{Id: e.Id, Data: cloneBytes(e.Data)}
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

// cloneBytes returns a copy of b, preserving nil-ness.
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

// deepCopyStruct performs deep copying of struct values using reflection,
// including unexported fields that cannot be set through normal reflection.
// Field addresses are derived with unsafe.Add from BOTH the source's and the
// destination's base pointers (both are always addressable here — built via
// reflect.New or sliced from one), the documented pattern for unexported-field
// access. Mirroring the source too is what makes the scalar/interface branches
// of deepCopyValue legal on unexported fields.
//
// The IsValid guard and default arm are defensive contracts for DIRECT
// callers, exercised by TestDeepCopyGuardArms — not dead code.
func deepCopyStruct(src, dst reflect.Value) {
	if !src.IsValid() {
		return
	}

	switch src.Kind() {
	case reflect.Struct:
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
