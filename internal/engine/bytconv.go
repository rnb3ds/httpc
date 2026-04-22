package engine

import (
	"unsafe"
)

// bytesToString converts a []byte to string without copying the underlying data.
//
// # Safety Contract
//
// This is safe ONLY when the caller guarantees:
//  1. The []byte is freshly allocated (not from a sync.Pool)
//  2. The []byte will not be modified after this call
//  3. The []byte is not retained by any other reference
//
// The response body path satisfies this contract because readBody()
// returns a newly allocated []byte that is never pooled or shared.
func bytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}
