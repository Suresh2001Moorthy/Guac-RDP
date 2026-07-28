//go:build freerdp
// +build freerdp

package rdp

/*
#cgo pkg-config: libfreerdp
#include "c_helpers.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// cCtx is the opaque C context pointer stored inside NativeClient.
type cCtx = unsafe.Pointer

// newCContext creates a new C-side context by calling rdpf_client_new.
func newCContext() cCtx {
	return C.rdpf_client_new()
}

// cConnect calls the C helper to perform connect. Returns nil on success.
func cConnect(ctx cCtx, host, user, pass string) error {
	chost := C.CString(host)
	defer C.free(unsafe.Pointer(chost))
	cuser := C.CString(user)
	defer C.free(unsafe.Pointer(cuser))
	cpass := C.CString(pass)
	defer C.free(unsafe.Pointer(cpass))

	res := C.rdpf_client_connect(ctx, chost, cuser, cpass)
	if int(res) != 0 {
		return fmt.Errorf("native connect failed: code=%d", int(res))
	}
	return nil
}

func cDisconnect(ctx cCtx) {
	C.rdpf_client_disconnect(ctx)
}

func cFree(ctx cCtx) {
	C.rdpf_client_free(ctx)
}

// nativeClientImpl embeds NativeClient and stores the C context pointer.
// This adapts the skeleton NativeClient defined in freerdp_native.go to the
// cgo-backed implementation.

type nativeClientImpl struct {
	*NativeClient
	cctx cCtx
}

func newNativeClientImpl() *nativeClientImpl {
	return &nativeClientImpl{
		NativeClient: &NativeClient{},
		cctx:         newCContext(),
	}
}

// Connect uses the c helper to connect.
func (n *nativeClientImpl) Connect(hostname, username, password string) error {
	if n.cctx == nil {
		return fmt.Errorf("native context is nil")
	}
	return cConnect(n.cctx, hostname, username, password)
}

// Disconnect tears down the native context.
func (n *nativeClientImpl) Disconnect() {
	if n.cctx != nil {
		cDisconnect(n.cctx)
		cFree(n.cctx)
		n.cctx = nil
	}
}

// NewNativeClient returns the cgo-backed implementation (overrides the skeleton).
func NewNativeClient() Client {
	return newNativeClientImpl()
}
