// Translation of the transport's TLS state to the standard library type.
//
// The fingerprint transport handshakes with uTLS, whose ConnectionState is a
// distinct type from crypto/tls's even though the field set matches. The
// engine and every caller above it expect the stdlib type, so each response's
// state is converted field by field here rather than dropped: losing it would
// make the fingerprinted path the only one where TLS metadata is unavailable.

package fingerprint

import (
	"crypto/tls"

	utls "github.com/bogdanfinn/utls"
)

// toStdConnState converts a uTLS connection state to the crypto/tls
// equivalent. A nil input stays nil (plain-HTTP responses carry no TLS state).
func toStdConnState(in *utls.ConnectionState) *tls.ConnectionState {
	if in == nil {
		return nil
	}
	return &tls.ConnectionState{
		Version: in.Version,
		// NegotiatedProtocolIsMutual is omitted: deprecated and always true
		// since Go 1.16, so the zero value carries the same information.
		HandshakeComplete:           in.HandshakeComplete,
		DidResume:                   in.DidResume,
		CipherSuite:                 in.CipherSuite,
		NegotiatedProtocol:          in.NegotiatedProtocol,
		ServerName:                  in.ServerName,
		PeerCertificates:            in.PeerCertificates,
		VerifiedChains:              in.VerifiedChains,
		SignedCertificateTimestamps: in.SignedCertificateTimestamps,
		OCSPResponse:                in.OCSPResponse,
		TLSUnique:                   in.TLSUnique,
		ECHAccepted:                 in.ECHAccepted,
	}
}
