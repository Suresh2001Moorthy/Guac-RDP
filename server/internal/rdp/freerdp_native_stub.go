package rdp

import "log"

// NewNativeClient should return an implementation of Client that uses the
// libfreerdp/native FreeRDP bindings. This is a build-time stub used when the
// native "freerdp" build tag is not enabled. The real implementation will be
// provided in a file built with the 'freerdp' build tag and cgo.
func NewNativeClient() Client {
	log.Println("[rdp] libfreerdp native client not compiled; using MockClient fallback")
	// Return a MockClient so the rest of the system can run unchanged while
	// we implement the native binding incrementally.
	return NewMockClient(1024, 768)
}
