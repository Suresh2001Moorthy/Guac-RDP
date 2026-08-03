package freerdp

/*
#cgo CFLAGS: -D__STDC_NO_THREADS__
#cgo CFLAGS: -I.
#cgo CFLAGS: -IC:/msys64/ucrt64/include/freerdp3
#cgo CFLAGS: -IC:/msys64/ucrt64/include/winpr3

#cgo LDFLAGS: -LC:/msys64/ucrt64/lib
#cgo LDFLAGS: -lfreerdp-client3
#cgo LDFLAGS: -lfreerdp3
#cgo LDFLAGS: -lwinpr3

#include <stdlib.h>
#include "wrapper.h"
#ifdef _WIN32
#include <windows.h>
#endif
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"rdp-web/internal/events"
	"rdp-web/internal/renderer"
)

// PixelFormat matches renderer.PixelFormat
type PixelFormat int

const (
	FormatBGRA32 PixelFormat = 0
	FormatRGBA32 PixelFormat = 1
)

// Session represents a single FreeRDP connection.
type Session struct {
	id        uintptr
	ctx       *C.RDPContext
	Bus       *events.Bus
	mu        sync.Mutex
	connected bool
	clipCache []byte
}

// Global session registry for routing callbacks from C to Go
var (
	registryMu sync.RWMutex
	registry   = make(map[uintptr]*Session)
	nextID     uintptr = 1 // start at 1, 0 = invalid
)

func registerSession(s *Session) uintptr {
	registryMu.Lock()
	id := nextID
	nextID++
	registry[id] = s
	registryMu.Unlock()
	return id
}

func unregisterSession(id uintptr) {
	registryMu.Lock()
	delete(registry, id)
	registryMu.Unlock()
}

func lookupSession(id uintptr) *Session {
	registryMu.RLock()
	s := registry[id]
	registryMu.RUnlock()
	return s
}

// New creates a new FreeRDP session.
func New() *Session {
	s := &Session{
		Bus: events.NewBus(256),
	}
	s.id = registerSession(s)
	return s
}

// Connect establishes an RDP connection.
func (s *Session) Connect(addr, user, pass string, width, height int, pf PixelFormat) error {
	s.ctx = C.rdp_new(C.uintptr_t(s.id))
	if s.ctx == nil {
		return errors.New("failed to create FreeRDP instance")
	}

	cAddr := C.CString(addr)
	cUser := C.CString(user)
	cPass := C.CString(pass)
	defer C.free(unsafe.Pointer(cAddr))
	defer C.free(unsafe.Pointer(cUser))
	defer C.free(unsafe.Pointer(cPass))

	rc := C.rdp_connect(s.ctx, cAddr, cUser, cPass,
		C.int(width), C.int(height), C.int(pf))
	if rc == 0 {
		C.rdp_free(s.ctx)
		s.ctx = nil
		return fmt.Errorf("RDP connection failed to %s", addr)
	}

	s.connected = true
	return nil
}

// RunEventLoop blocks and processes FreeRDP events on a locked OS thread.
func (s *Session) RunEventLoop(ctx context.Context) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if s.ShouldDisconnect() {
			s.Bus.Publish(events.TypeDisconnect, nil)
			return
		}

		handles := make([]unsafe.Pointer, 64)
		n := s.getHandles(handles)
		if n == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		if s.checkHandles() < 0 {
			s.Bus.Publish(events.TypeDisconnect, nil)
			return
		}
	}
}

// getHandles returns FreeRDP wait handles for the worker pool.
func (s *Session) getHandles(handles []unsafe.Pointer) int {
	if s.ctx == nil {
		return 0
	}
	n := C.rdp_get_handles(s.ctx, (*unsafe.Pointer)(unsafe.Pointer(&handles[0])), C.int(len(handles)))
	return int(n)
}

// checkHandles processes pending FreeRDP events.
func (s *Session) checkHandles() int {
	if s.ctx == nil {
		return -1
	}
	return int(C.rdp_check_handles(s.ctx))
}

// ShouldDisconnect returns true if FreeRDP wants to disconnect.
func (s *Session) ShouldDisconnect() bool {
	if s.ctx == nil {
		return true
	}
	return C.rdp_should_disconnect(s.ctx) != 0
}

// SendMouse sends a mouse event to the remote desktop.
func (s *Session) SendMouse(flags, x, y uint16) {
	if s.ctx != nil {
		C.rdp_send_mouse(s.ctx, C.uint16_t(flags), C.uint16_t(x), C.uint16_t(y))
	}
}

// SendKey sends a keyboard event to the remote desktop.
func (s *Session) SendKey(flags, code uint16) {
	if s.ctx != nil {
		C.rdp_send_key(s.ctx, C.uint16_t(flags), C.uint16_t(code))
	}
}

// SendUnicodeText injects text into the remote session using RDP Unicode
// keyboard events. This is used as a low-latency paste path when full cliprdr
// synchronization is unavailable.
func (s *Session) SendUnicodeText(text string) {
	if s.ctx == nil || text == "" {
		return
	}

	for _, r := range text {
		for _, unit := range utf16.Encode([]rune{r}) {
			C.rdp_send_unicode(s.ctx, C.uint16_t(0), C.uint16_t(unit))
			C.rdp_send_unicode(s.ctx, C.uint16_t(0x8000), C.uint16_t(unit))
		}
	}
}

// SendClipboardText caches UTF-8 text and notifies the server via cliprdr.
func (s *Session) SendClipboardText(text string) {
	if s.ctx == nil {
		return
	}
	
	// Convert UTF-8 to UTF-16LE with null terminator
	utf16Data := utf16.Encode([]rune(text))
	utf16Data = append(utf16Data, 0)
	
	importBytes := make([]byte, len(utf16Data)*2)
	for i, v := range utf16Data {
		importBytes[i*2] = byte(v)
		importBytes[i*2+1] = byte(v >> 8)
	}
	
	s.mu.Lock()
	s.clipCache = importBytes
	s.mu.Unlock()
	
	C.rdp_cliprdr_send_client_format_list(s.ctx)
}

// RespondClipboardData replies to the server's data request with the cached text.
func (s *Session) RespondClipboardData() {
	if s.ctx == nil {
		return
	}
	
	s.mu.Lock()
	cache := s.clipCache
	s.mu.Unlock()
	
	if len(cache) == 0 {
		C.rdp_cliprdr_send_client_format_data_response(s.ctx, nil, 0)
		return
	}
	
	C.rdp_cliprdr_send_client_format_data_response(s.ctx, (*C.char)(unsafe.Pointer(&cache[0])), C.int(len(cache)))
}

// FetchClipboardText requests clipboard data from the server.
func (s *Session) FetchClipboardText() {
	if s.ctx != nil {
		C.rdp_cliprdr_send_client_format_data_request(s.ctx)
	}
}

// Disconnect closes the RDP connection.
func (s *Session) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		C.rdp_disconnect(s.ctx)
		C.rdp_free(s.ctx)
		s.ctx = nil
		s.connected = false
	}
	unregisterSession(s.id)
	if s.Bus != nil {
		s.Bus.Close()
	}
}

// ID returns the session's unique identifier.
func (s *Session) ID() uintptr {
	return s.id
}

//export goOnEndPaint
func goOnEndPaint(sessionID C.uintptr_t, cx, cy, cw, ch C.int,
	pixels unsafe.Pointer, stride C.int) {

	s := lookupSession(uintptr(sessionID))
	if s == nil || s.Bus == nil {
		return
	}

	x, y, w, h := int(cx), int(cy), int(cw), int(ch)
	if w <= 0 || h <= 0 {
		return
	}

	goStride := int(stride)
	pixelSize := w * h * 4
	dst := renderer.GetPixelBuffer(pixelSize)

	// Copy row-by-row from C framebuffer to Go-owned buffer.
	srcLen := (h-1)*goStride + w*4
	src := unsafe.Slice((*byte)(pixels), srcLen)

	rowBytes := w * 4
	for row := 0; row < h; row++ {
		srcOffset := row * goStride
		dstOffset := row * rowBytes
		copy(dst[dstOffset:dstOffset+rowBytes], src[srcOffset:srcOffset+rowBytes])
	}

	s.Bus.Publish(events.TypeDirtyRect, events.DirtyRect{
		X:      x,
		Y:      y,
		W:      w,
		H:      h,
		Pixels: dst,
	})
}

//export goOnDesktopResize
func goOnDesktopResize(sessionID C.uintptr_t, width, height C.int) {
	s := lookupSession(uintptr(sessionID))
	if s == nil || s.Bus == nil {
		return
	}
	log.Printf("[RDP] Desktop resize: %dx%d", int(width), int(height))
	s.Bus.Publish(events.TypeDesktopResize, events.DesktopResize{
		Width:  int(width),
		Height: int(height),
	})
}

//export goOnReady
func goOnReady(sessionID C.uintptr_t, width, height C.int) {
	s := lookupSession(uintptr(sessionID))
	if s == nil || s.Bus == nil {
		return
	}
	log.Printf("[RDP] Session ready: %dx%d", int(width), int(height))
	s.Bus.Publish(events.TypeReady, events.Ready{
		Width:  int(width),
		Height: int(height),
	})
}

//export goOnClipboardFormatList
func goOnClipboardFormatList(sessionID C.uintptr_t) {
	s := lookupSession(uintptr(sessionID))
	if s != nil && s.Bus != nil {
		s.Bus.Publish(events.TypeClipboardFormatList, nil)
	}
}

//export goOnClipboardDataRequest
func goOnClipboardDataRequest(sessionID C.uintptr_t) {
	s := lookupSession(uintptr(sessionID))
	if s != nil && s.Bus != nil {
		s.Bus.Publish(events.TypeClipboardDataRequest, nil)
	}
}

//export goOnClipboardDataResponse
func goOnClipboardDataResponse(sessionID C.uintptr_t, data *C.char, size C.int) {
	s := lookupSession(uintptr(sessionID))
	if s == nil || s.Bus == nil || size <= 0 || data == nil {
		return
	}
	
	bytesData := C.GoBytes(unsafe.Pointer(data), size)
	if len(bytesData) < 2 {
		return
	}
	
	utf16Data := make([]uint16, len(bytesData)/2)
	for i := 0; i < len(utf16Data); i++ {
		utf16Data[i] = uint16(bytesData[i*2]) | (uint16(bytesData[i*2+1]) << 8)
	}
	
	if len(utf16Data) > 0 && utf16Data[len(utf16Data)-1] == 0 {
		utf16Data = utf16Data[:len(utf16Data)-1]
	}
	
	utf8Text := string(utf16.Decode(utf16Data))
	s.Bus.Publish(events.TypeClipboard, events.Clipboard{
		Text: utf8Text,
	})
}
