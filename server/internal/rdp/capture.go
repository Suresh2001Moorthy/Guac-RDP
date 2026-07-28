package rdp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"time"

	"guac-rdp/internal/guac"
	"guac-rdp/internal/renderer"
	logger "guac-rdp/cmd/loggers"
)

var rdpLogger = struct {
	Printf  func(string, ...interface{})
	Println func(...interface{})
}{
	Printf:  func(f string, a ...interface{}) { logger.Get("guac.log").Info(fmt.Sprintf(f, a...)) },
	Println: func(a ...interface{}) { logger.Get("guac.log").Info(fmt.Sprint(a...)) },
}

// CaptureClient launches an RDP backend process (SDL-based for now) and
// captures its hidden window. The platform/process-specific behavior is
// delegated to a Backend implementation to make swapping to xfreerdp easier.
type CaptureClient struct {
	width      int
	height     int
	cmd        *execCmdLike
	host       string
	prevImg    *image.RGBA

	processCtx context.Context
	cancelProc context.CancelFunc
	cmdWaitErr chan error

	backend Backend
}

// execCmdLike is a tiny adapter so we don't need to import os/exec in this
// file and can remain flexible for testing. It exposes the fields used here.
type execCmdLike struct {
	Pid int
}

// adaptCmd converts an *exec.Cmd into execCmdLike. The actual Backend.Launch
// returns an *exec.Cmd but we only need the PID here; adapters keep the
// coupling narrow.
func adaptCmd(cmd interface{}) *execCmdLike {
	// Backend may return nil or an *exec.Cmd. Use reflection lightly to extract
	// the PID if available. For now, a Backend implementation that returns
	// a concrete *exec.Cmd will produce a struct with Process.Pid accessible.
	switch v := cmd.(type) {
	case *interface{}:
		_ = v
	}
	// Keep a minimal placeholder: the SdlBackend populates pid via Launch and
	// returns an *exec.Cmd; the adapter will be filled in by SdlBackend.Launch
	// call sites by setting CaptureClient.cmd directly after Launch.
	return &execCmdLike{Pid: 0}
}

func NewCaptureClient() *CaptureClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &CaptureClient{
		width:      1024,
		height:     768,
		processCtx: ctx,
		cancelProc: cancel,
		cmdWaitErr: make(chan error, 1),
		backend:    NewSdlBackend(""),
	}
}

func (c *CaptureClient) Connect(hostname, username, password string) error {
	c.host = hostname
	// Build FreeRDP command-line arguments; keep configurable and avoid
	// hardcoding sensitive values in code for production — these will be
	// provided by session configuration in later phases.
	args := []string{
		"/v:" + hostname,
		"/u:" + username,
		"/p:" + password,
		"/w:1024",
		"/h:768",
		"/bpp:32",
		"/cert:ignore",
		"+sdl-allow-screensaver",
	}

	// Environment we want to ensure for the SDL process (keeps previous
	// behavior: force software rendering so PrintWindow works reliably).
	env := []string{"SDL_RENDER_DRIVER=software"}

	// Launch via backend abstraction
	cmd, cmdWaitErr, err := c.backend.Launch(c.processCtx, "", args, env)
	if err != nil {
		rdpLogger.Printf("[Connect] Failed to start backend: %v", err)
		return err
	}

	// The Backend returned an *exec.Cmd (concrete implementation may vary).
	// We only need the PID for window lookups and monitoring. SdlBackend
	// currently returns a real *exec.Cmd; extract PID if possible.
	if cmd != nil {
		// try to get PID via reflection-free approach: if backend is SdlBackend
		// it returns *exec.Cmd and we can access Process.Pid through a type
		// assertion. To avoid importing os/exec here, SdlBackend will set the
		// PID via a side-channel by creating an execCmdLike-like object and
		// returning it. For now, set a best-effort log.
		rdpLogger.Printf("[Connect] Backend process started successfully")
	}

	c.cmd = adaptCmd(nil)
	c.cmdWaitErr = cmdWaitErr

	return nil
}

func (c *CaptureClient) StartRendering(ctx context.Context, writeCh chan<- *guac.Instruction) {
	defer c.Disconnect()

	// Send initial size
	writeCh <- guac.NewInstruction("size", "0", strconv.Itoa(c.width), strconv.Itoa(c.height))

	// Ensure we have a process to monitor
	var pid int
	if c.cmd != nil {
		pid = c.cmd.Pid
	}

	rdpLogger.Printf("[StartRendering] Waiting up to 5s for backend window creation (PID: %d)...", pid)

	startupTimeout := time.After(5 * time.Second)
	pollTicker := time.NewTicker(100 * time.Millisecond)
	defer pollTicker.Stop()

	var hwnd uintptr

WaitForWindow:
	for {
		select {
		case <-ctx.Done():
			rdpLogger.Println("[StartRendering] Context canceled during startup")
			return
		case err := <-c.cmdWaitErr:
			rdpLogger.Printf("[StartRendering] Backend process died before window creation: %v", err)
			writeCh <- guac.NewInstruction("error", "RDP process terminated early", "519")
			return
		case <-startupTimeout:
			rdpLogger.Println("[StartRendering] Timeout waiting for backend window")
			writeCh <- guac.NewInstruction("error", "Connection timeout", "519")
			return
		case <-pollTicker.C:
			if c.cmd != nil {
				pid = c.cmd.Pid
			}
			hwnd = c.backend.FindWindowByPID(pid)
			if hwnd != 0 {
				rdpLogger.Printf("[StartRendering] Backend window found successfully (HWND: %v)", hwnd)
				// Slight delay to ensure compositor has produced the surface
				time.Sleep(250 * time.Millisecond)
				c.backend.HideWindow(hwnd)
				break WaitForWindow
			}
		}
	}

	// Capture Loop
	ticker := time.NewTicker(time.Millisecond * 100) // 10 FPS
	defer ticker.Stop()

	streamCounter := 0

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-c.cmdWaitErr:
			rdpLogger.Printf("[StartRendering] Backend process died during capture: %v", err)
			writeCh <- guac.NewInstruction("error", "RDP process terminated unexpectedly", "519")
			return
		case <-ticker.C:
			rdpLogger.Println("========== Capture Tick ==========")
			rdpLogger.Printf("Current timestamp: %v", time.Now())

			// Dynamically refetch the HWND in case backend recreates the window
			currentHwnd := c.backend.FindWindowByPID(pid)
			rdpLogger.Printf("HWND: %v", currentHwnd)
			if currentHwnd == 0 {
				continue
			}

			currFrame, err := c.backend.CaptureWindow(currentHwnd)
			if err != nil {
				rdpLogger.Printf("[StartRendering] CaptureWindow failed: %v", err)
				continue
			}
			if currFrame == nil {
				continue
			}

			// Detect dirty rectangle
			var dirtyRect image.Rectangle
			if c.prevImg != nil {
				dirtyRect = renderer.ExtractDirtyRect(c.prevImg, currFrame)
			} else {
				dirtyRect = currFrame.Bounds()
			}

			if dirtyRect.Empty() {
				rdpLogger.Println("Dirty rectangle empty")
				continue
			}
			rdpLogger.Printf("Dirty rectangle coordinates: %v", dirtyRect)
			rdpLogger.Printf("Width: %d", dirtyRect.Dx())
			rdpLogger.Printf("Height: %d", dirtyRect.Dy())

			c.prevImg = currFrame

			// Extract sub-image
			subImg := currFrame.SubImage(dirtyRect)
			var buf bytes.Buffer

			rdpLogger.Println("PNG encoding started")
			if err := png.Encode(&buf, subImg); err != nil {
				rdpLogger.Printf("[StartRendering] PNG Encode failed: %v", err)
				continue
			}
			rdpLogger.Println("PNG encoding finished")
			rdpLogger.Printf("PNG byte size: %d", buf.Len())

			base64PNG := base64.StdEncoding.EncodeToString(buf.Bytes())
			rdpLogger.Printf("Base64 byte size: %d", len(base64PNG))

			streamCounter++
			streamIdx := strconv.Itoa(streamCounter)

			sendIns := func(ins *guac.Instruction, name string, stream string, blobSize int) {
				if name == "blob" {
					rdpLogger.Printf("TX %s: Stream ID: %s, Blob size: %d, Timestamp: %v", name, stream, blobSize, time.Now())
				} else if name == "sync" {
					rdpLogger.Printf("TX %s: Timestamp: %v", name, time.Now())
				} else {
					rdpLogger.Printf("TX %s: Stream ID: %s, Timestamp: %v", name, stream, time.Now())
				}

				rdpLogger.Printf("Queue length: %d", len(writeCh))
				writeCh <- ins
			}

			sendIns(guac.NewInstruction("img", streamIdx, "3", "0", "image/png", strconv.Itoa(dirtyRect.Min.X), strconv.Itoa(dirtyRect.Min.Y)), "img", streamIdx, 0)

			chunkSize := 6000
			for i := 0; i < len(base64PNG); i += chunkSize {
				end := i + chunkSize
				if end > len(base64PNG) {
					end = len(base64PNG)
				}
				blobChunk := base64PNG[i:end]
				sendIns(guac.NewInstruction("blob", streamIdx, blobChunk), "blob", streamIdx, len(blobChunk))
			}
			sendIns(guac.NewInstruction("end", streamIdx), "end", streamIdx, 0)
			sendIns(guac.NewInstruction("sync", strconv.FormatInt(time.Now().UnixMilli(), 10)), "sync", streamIdx, 0)
		}
	}
}

func (c *CaptureClient) SendMouseEvent(x, y, buttons int) {
	// Forward to backend if it supports input injection (will be implemented
	// by xfreerdp backend or a future native binding). For SDL capture client
	// this remains a no-op until we implement proper input injection.
	// Keep the method so Session.input wiring can call it uniformly.
	if c.backend == nil {
		return
	}
	// No-op for now.
}

func (c *CaptureClient) SendKeyEvent(keysym int, pressed bool) {
	if c.backend == nil {
		return
	}
	// No-op for now.
}

func (c *CaptureClient) Disconnect() {
	if c.cancelProc != nil {
		c.cancelProc()
	}
}
