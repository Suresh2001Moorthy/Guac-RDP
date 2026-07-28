package rdp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// XfReeRDPBackend launches xfreerdp as a managed process. It implements the
// Backend interface. Initially it behaves similarly to SdlBackend but uses the
// xfreerdp executable and arguments. The implementation delegates window
// location and capture to the existing Win32 helpers so it remains compatible
// with the current capture pipeline.

type XfReeRDPBackend struct{
	execPath string
}

// NewXfReeRDPBackend constructs a new backend. If execPath is empty the code
// will attempt to read GUAC_RDP_XFREERDP_PATH env var and fall back to
// "xfreerdp" which assumes the binary is on PATH.
func NewXfReeRDPBackend(execPath string) *XfReeRDPBackend {
	if execPath == "" {
		execPath = os.Getenv("GUAC_RDP_XFREERDP_PATH")
		if execPath == "" {
			execPath = "xfreerdp"
		}
	}
	return &XfReeRDPBackend{execPath: execPath}
}

func (x *XfReeRDPBackend) Launch(processCtx context.Context, execPath string, args []string, env []string) (*exec.Cmd, chan error, error) {
	if execPath == "" {
		execPath = x.execPath
	}
	// Build command
	cmd := exec.CommandContext(processCtx, execPath, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	// Start
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("xfreerdp: failed to start process: %w", err)
	}

	cmdWaitErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		cmdWaitErr <- err
		close(cmdWaitErr)
	}()

	rdpLogger.Printf("[XfReeRDPBackend] Started xfreerdp (pid=%d) exec=%s args=%v", cmd.Process.Pid, execPath, args)

	return cmd, cmdWaitErr, nil
}

func (x *XfReeRDPBackend) FindWindowByPID(pid int) uintptr {
	return FindWindowByPID(pid)
}

func (x *XfReeRDPBackend) HideWindow(hwnd uintptr) {
	HideWindow(hwnd)
}

func (x *XfReeRDPBackend) CaptureWindow(hwnd uintptr) (*image.RGBA, error) {
	return CaptureWindow(hwnd)
}
