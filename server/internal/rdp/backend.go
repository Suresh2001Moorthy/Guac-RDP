package rdp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Backend abstracts process/window/capture operations so the SDL-specific
// implementation can be swapped out (e.g., to xfreerdp) without touching the
// higher-level CaptureClient logic.
type Backend interface {
	// Launch starts the backend process and returns the *exec.Cmd and a channel
	// that will receive the process exit error (if any).
	Launch(processCtx context.Context, execPath string, args []string, env []string) (*exec.Cmd, chan error, error)

	// FindWindowByPID locates the window owned by the given PID. Returns 0 when
	// not found.
	FindWindowByPID(pid int) uintptr

	// HideWindow makes the window effectively invisible while still allowing
	// PrintWindow/GetDIBits to capture its contents.
	HideWindow(hwnd uintptr)

	// CaptureWindow captures the window contents into an *image.RGBA.
	CaptureWindow(hwnd uintptr) (*image.RGBA, error)
}

// SdlBackend is the current SDL-based implementation of Backend. All SDL and
// platform-specific logic is isolated here so it can be replaced later.
type SdlBackend struct{
	// defaultExecPath can be configured (or read from env) to avoid hardcoding
	// paths across the codebase.
	defaultExecPath string
}

// NewSdlBackend constructs an SdlBackend. If execPath is empty, a reasonable
// default is used. Prefer configuring this via environment instead of
// hardcoding in application code.
func NewSdlBackend(execPath string) *SdlBackend {
	if execPath == "" {
		// Attempt to read from environment first, fallback to previous default
		// path. This keeps existing behavior while allowing configuration.
		execPath = os.Getenv("GUAC_RDP_SDL_PATH")
		if execPath == "" {
			execPath = "D:\\products\\Guac-RDP\\server\\SDL3\\sdl-freerdp.exe"
		}
	}
	return &SdlBackend{defaultExecPath: execPath}
}

func (s *SdlBackend) Launch(processCtx context.Context, execPath string, args []string, env []string) (*exec.Cmd, chan error, error) {
	if execPath == "" {
		execPath = s.defaultExecPath
	}
	cmd := exec.CommandContext(processCtx, execPath, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start backend process: %w", err)
	}

	cmdWaitErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		cmdWaitErr <- err
		close(cmdWaitErr)
	}()

	return cmd, cmdWaitErr, nil
}

func (s *SdlBackend) FindWindowByPID(pid int) uintptr {
	return FindWindowByPID(pid)
}

func (s *SdlBackend) HideWindow(hwnd uintptr) {
	HideWindow(hwnd)
}

func (s *SdlBackend) CaptureWindow(hwnd uintptr) (*image.RGBA, error) {
	return CaptureWindow(hwnd)
}
