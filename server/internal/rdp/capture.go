package rdp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
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

// CaptureClient launches sdl-freerdp.exe and captures its hidden window.
type CaptureClient struct {
	width      int
	height     int
	cmd        *exec.Cmd
	host       string
	prevImg    *image.RGBA

	processCtx context.Context
	cancelProc context.CancelFunc
	cmdWaitErr chan error
}

func NewCaptureClient() *CaptureClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &CaptureClient{
		width:      1024,
		height:     768,
		processCtx: ctx,
		cancelProc: cancel,
		cmdWaitErr: make(chan error, 1),
	}
}

func (c *CaptureClient) Connect(hostname, username, password string) error {
	c.host = hostname
	// Launch FreeRDP
	args := []string{
		"/v:" + hostname,
		"/u:" + username,
		"/p:" + password,
		"/w:1024",
		"/h:768",
		"/bpp:32",
		"/cert:ignore", // CRITICAL FIX: Bypass self-signed certificate prompts
		"+sdl-allow-screensaver",
	}

	execPath := "D:\\products\\Guac-RDP\\server\\SDL3\\sdl-freerdp.exe"
	c.cmd = exec.CommandContext(c.processCtx, execPath, args...)

	// CRITICAL FIX: Force SDL3 to use software rendering so PrintWindow can capture it!
	c.cmd.Env = append(os.Environ(), "SDL_RENDER_DRIVER=software")

	// Redirect output to console for debugging
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr

	rdpLogger.Printf("[Connect] Starting FreeRDP: %s %v", execPath, args)

	if err := c.cmd.Start(); err != nil {
		rdpLogger.Printf("[Connect] Failed to start FreeRDP: %v", err)
		return err
	}

	rdpLogger.Printf("[Connect] FreeRDP process started successfully. PID: %d", c.cmd.Process.Pid)

	// Process monitor goroutine
	go func() {
		err := c.cmd.Wait()
		if err != nil {
			rdpLogger.Printf("[ProcessMonitor] FreeRDP (PID: %d) exited unexpectedly with error: %v", c.cmd.Process.Pid, err)
		} else {
			rdpLogger.Printf("[ProcessMonitor] FreeRDP (PID: %d) exited cleanly", c.cmd.Process.Pid)
		}
		c.cmdWaitErr <- err
		close(c.cmdWaitErr)
	}()

	return nil
}

func (c *CaptureClient) StartRendering(ctx context.Context, writeCh chan<- *guac.Instruction) {
	defer c.Disconnect()

	// Send initial size
	writeCh <- guac.NewInstruction("size", "0", strconv.Itoa(c.width), strconv.Itoa(c.height))

	rdpLogger.Printf("[StartRendering] Waiting up to 5s for SDL window creation (PID: %d)...", c.cmd.Process.Pid)
	
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
			rdpLogger.Printf("[StartRendering] Process died before window creation: %v", err)
			writeCh <- guac.NewInstruction("error", "RDP process terminated early", "519")
			return
		case <-startupTimeout:
			rdpLogger.Println("[StartRendering] Timeout waiting for SDL window")
			writeCh <- guac.NewInstruction("error", "Connection timeout", "519")
			return
		case <-pollTicker.C:
			hwnd = FindWindowByPID(c.cmd.Process.Pid)
			if hwnd != 0 {
				// Window found! Break out of startup loop.
				rdpLogger.Printf("[StartRendering] SDL window found successfully (HWND: %v)", hwnd)
				// Slight delay to ensure DWM has composited the window before hiding
				time.Sleep(250 * time.Millisecond)
				HideWindow(hwnd)
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
			rdpLogger.Printf("[StartRendering] FreeRDP process died during capture: %v", err)
			writeCh <- guac.NewInstruction("error", "RDP process terminated unexpectedly", "519")
			return
		case <-ticker.C:
			rdpLogger.Println("========== Capture Tick ==========")
			rdpLogger.Printf("Current timestamp: %v", time.Now())
			
			// Dynamically refetch the HWND in case SDL recreates the window
			currentHwnd := FindWindowByPID(c.cmd.Process.Pid)
			rdpLogger.Printf("HWND: %v", currentHwnd)
			rdpLogger.Printf("FindWindowByPID() succeeded: %v", currentHwnd != 0)
			
			// Check if process is still alive
			processAlive := true
			select {
			case <-c.cmdWaitErr:
				processAlive = false
			default:
			}
			rdpLogger.Printf("Process is still alive: %v", processAlive)
			
			if currentHwnd == 0 {
				continue
			}

			currFrame, err := CaptureWindow(currentHwnd)
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

			// img, stream_index, mode, layer, mimetype, x, y
			
			sendIns := func(ins *guac.Instruction, name string, stream string, blobSize int) {
				if name == "blob" {
					rdpLogger.Printf("TX %s: Stream ID: %s, Blob size: %d, Timestamp: %v", name, stream, blobSize, time.Now())
				} else if name == "sync" {
					rdpLogger.Printf("TX %s: Timestamp: %v", name, time.Now())
				} else {
					rdpLogger.Printf("TX %s: Stream ID: %s, Timestamp: %v", name, stream, time.Now())
				}
				
				rdpLogger.Printf("Queue length: %d", len(writeCh))
				rdpLogger.Println("Whether send blocks: yes, if queue is full")
				writeCh <- ins
				rdpLogger.Println("Whether send succeeds: true (queued)")
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
	// We can't easily inject mouse events into a hidden FreeRDP window without SendMessage/PostMessage.
	// We leave this empty for now.
}

func (c *CaptureClient) SendKeyEvent(keysym int, pressed bool) {
	// We can't easily inject key events into a hidden FreeRDP window without SendMessage/PostMessage.
}

func (c *CaptureClient) Disconnect() {
	if c.cancelProc != nil {
		c.cancelProc()
	}
}
