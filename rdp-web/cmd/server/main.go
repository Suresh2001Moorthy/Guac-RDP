package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"rdp-web/internal/freerdp"
	"rdp-web/internal/renderer"
	"rdp-web/internal/session"
	"rdp-web/internal/transport"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  16384,
	WriteBufferSize: 65536, // larger for binary pixel frames
	CheckOrigin: func(r *http.Request) bool {
		return true // TODO: production origin check
	},
}

var sessionManager = session.NewManager()

// Worker pool (Phase 2 will make this a proper multiplexed pool)
// For now, each session gets its own goroutine for the FreeRDP event loop.
func runEventLoop(rdpSession *freerdp.Session, sess *session.Session) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		select {
		case <-sess.Context().Done():
			return
		default:
		}

		if rdpSession.ShouldDisconnect() {
			return
		}

		handles := make([]unsafe.Pointer, 64)
		n := rdpSession.GetHandles(handles)
		if n == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// This is a simplified event check - the worker pool will improve this
		if rdpSession.CheckHandles() < 0 {
			return
		}
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	// Parse connection params from query string
	host := r.URL.Query().Get("host")
	user := r.URL.Query().Get("user")
	pass := r.URL.Query().Get("pass")
	width := queryInt(r, "w", 1920)
	height := queryInt(r, "h", 1080)

	if host == "" {
		http.Error(w, "missing host parameter", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Create components
	id := uuid.New().String()
	rend := renderer.NewRawRGBARenderer()
	trans := transport.NewWSTransport(conn)
	rdpSession := freerdp.New()

	// Connect to RDP
	pf := rend.PixelFormat() // Renderer decides pixel format
	if err := rdpSession.Connect(host, user, pass, width, height, freerdp.PixelFormat(pf)); err != nil {
		log.Printf("[%s] RDP connect failed: %v", id, err)
		conn.Close()
		return
	}

	// Create session orchestrator
	sess := session.NewSession(id, rdpSession, rend, trans)
	sessionManager.Add(sess)

	log.Printf("[%s] Session started: %s@%s %dx%d", id, user, host, width, height)

	// Start session workers
	sess.Start()

	// Run FreeRDP event loop (blocking, on locked OS thread)
	// Phase 2 will move this to a worker pool
	go runEventLoop(rdpSession, sess)

	// Wait for session to complete
	sess.Wait()

	// Cleanup
	sessionManager.Remove(id)
	log.Printf("[%s] Session ended", id)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultVal
	}
	n := 0
	for _, c := range val {
		if c < '0' || c > '9' {
			return defaultVal
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func main() {
	log.Println("Starting RDP-Web Gateway...")

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWS)

	// Serve static web files
	webDir := `C:\Guac-RDP\rdp-web\web`
	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	srv := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  0, // no timeout for WebSocket
		WriteTimeout: 0,
	}

	go func() {
		log.Println("Listening on http://localhost:8081")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	sessionManager.CloseAll()
	srv.Close()
	log.Println("Server exited")
}
