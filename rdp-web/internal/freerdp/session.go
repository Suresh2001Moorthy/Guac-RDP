package freerdp

/*
#cgo CFLAGS: -I.
#cgo LDFLAGS: -lfreerdp-client3 -lfreerdp3 -lwinpr3
#include <stdlib.h>
#include "wrapper.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Session represents a remote desktop session.
// In Milestone 1, this just holds the Go API surface.
type Session struct {
}

// New creates a new RDP Session.
func New() *Session {
	return &Session{}
}

// Connect blocks and connects to the given RDP server.
// In this Milestone, it connects and then immediately disconnects to prove CGO works.
func (s *Session) Connect(address, username, password string) error {
	cAddress := C.CString(address)
	cUsername := C.CString(username)
	cPassword := C.CString(password)

	// Free C strings when we exit
	defer C.free(unsafe.Pointer(cAddress))
	defer C.free(unsafe.Pointer(cUsername))
	defer C.free(unsafe.Pointer(cPassword))

	// Call into the C wrapper
	success := C.freerdp_wrapper_connect(cAddress, cUsername, cPassword)
	if success == 0 {
		return errors.New("failed to connect to RDP server")
	}

	return nil
}
