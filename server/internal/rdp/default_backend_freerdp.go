//go:build freerdp
// +build freerdp

package rdp

import (
	"context"
)

// NativeBackend adapts the cgo-backed NativeClient to the Backend interface.
// It does not launch an external process; instead it drives rendering via the
// NativeClient.StartRendering path.
type NativeBackend struct {
	client *nativeClientImpl
}

func NewNativeBackend() *NativeBackend {
	return &NativeBackend{
		client: newNativeClientImpl(),
	}
}

// Launch is a no-op for the native backend because there is no external
// process to start. Returns nils to indicate no process was launched.
func (n *NativeBackend) Launch(processCtx context.Context, execPath string, args []string, env []string) (*exec.Cmd, chan error, error) {
	// The NativeBackend expects the caller to call Connect on the backend (via
	// the type-assertion used in CaptureClient.Connect) to establish the
	// session. We don't start a system process here.
	return nil, nil, nil
}

// FindWindowByPID is a no-op for native backend; return 0 to indicate no
// window-based capture is available.
func (n *NativeBackend) FindWindowByPID(pid int) uintptr {
	return 0
}

func (n *NativeBackend) HideWindow(hwnd uintptr) {
	// no-op
}

// CaptureWindow is not supported for native backend. Return nil so callers
// that still attempt process-based capture will skip.
func (n *NativeBackend) CaptureWindow(hwnd uintptr) (*image.RGBA, error) {
	return nil, nil
}

// Connect establishes the native FreeRDP session using the embedded client.
func (n *NativeBackend) Connect(host, user, pass string) error {
	return n.client.Connect(host, user, pass)
}

// StartRender lets the native backend take over rendering and stream frames
// directly to the writeCh. This method is detected by CaptureClient and
// used instead of the process/window capture loop.
func (n *NativeBackend) StartRender(ctx context.Context, writeCh chan<- *guac.Instruction) {
	n.client.StartRendering(ctx, writeCh)
}
