//go:build freerdp
// +build freerdp

package rdp

/*
#cgo pkg-config: libfreerdp
#include <freerdp/freerdp.h>
#include <freerdp/client.h>
#include <stdlib.h>

// NOTE:
// - This file is a thin cgo entry point. Real helper C code and wrappers will
//   be added progressively (helper C source files can be placed alongside
//   this file and compiled only with the 'freerdp' tag).
// - The build requires libfreerdp development headers and linker libraries.
// - On Debian/Ubuntu this typically comes from packages like libfreerdp-dev.
*/
import "C"

import (
	"context"
	"fmt"
	"image"
	"log"
	"unsafe"
)

// NativeClient is the first-step skeleton for an embedded libfreerdp client.
// It implements the rdp.Client interface so Session can switch to using it.
// This file is compiled only when the 'freerdp' build tag is specified.
type NativeClient struct {
	// future fields:
	// ctx       context.Context
	// cancel    context.CancelFunc
	// freerdp   *C.freerdp
	// instance  unsafe.Pointer // placeholder for rdpContext/instance
}

// NewNativeClient returns a new NativeClient. The real implementation will be
// progressively implemented behind this type.
func NewNativeClient() Client {
	log.Println("[rdp] NewNativeClient: native FreeRDP client (build tag 'freerdp')")
	return &NativeClient{}
}

// Connect establishes an RDP connection using libfreerdp APIs.
// For now this is a placeholder that will be implemented in follow-up commits.
func (n *NativeClient) Connect(hostname, username, password string) error {
	// TODO: call into libfreerdp to create an instance, set callbacks, and connect.
	// Conceptual steps:
	//  - allocate freerdp instance
	//  - configure settings (hostname, credentials, resolution, etc.)
	//  - register update callbacks (bitmap, pointer, etc.)
	//  - call freerdp_connect()
	return fmt.Errorf("NativeClient Connect not implemented yet")
}

// StartRendering should hook into FreeRDP bitmap/framebuffer callbacks and
// forward encoded frames to writeCh. Placeholder for now.
func (n *NativeClient) StartRendering(ctx context.Context, writeCh chan<- *guac.Instruction) {
	// The real implementation will:
	//  - register a bitmap/framebuffer update callback with FreeRDP
	//  - when a bitmap arrives, convert to image.RGBA (or reuse buffer)
	//  - compute dirty rects, encode to PNG/other encoder, and send guac instructions
	// For now: return immediately to avoid blocking.
	return
}

// SendMouseEvent injects mouse events into the active FreeRDP session.
func (n *NativeClient) SendMouseEvent(x, y, buttons int) {
	// TODO: map guac mouse semantics to FreeRDP input events and call the C wrapper.
}

// SendKeyEvent injects keyboard events (including unicode) into FreeRDP.
func (n *NativeClient) SendKeyEvent(keysym int, pressed bool) {
	// TODO: translate keysym to scancode/virtual key and call into libfreerdp.
}

// Disconnect tears down the FreeRDP session and cleans up resources.
func (n *NativeClient) Disconnect() {
	// TODO: freerdp_disconnect and free resources
}
