package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"guac-rdp/internal/session"
	"guac-rdp/internal/websocket"
)

func main() {
	log.Println("Starting Guac-RDP Gateway...")

	// Initialize the session manager
	sessionManager := session.NewManager()

	// Initialize the WebSocket server
	wsServer := websocket.NewServer(sessionManager)

	// Set up HTTP routing
	mux := http.NewServeMux()

	// The standard Guacamole HTML5 client connects to an endpoint (usually /websocket-tunnel)
	mux.Handle("/websocket-tunnel", wsServer)

	// Serve static files if they exist (for the frontend client)
	mux.Handle("/", http.FileServer(http.Dir("C:/Guac-RDP/server/static")))

	// Start the HTTP server
	srv := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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

	log.Println("Shutting down server...")

	// TODO: Clean up active sessions

	if err := srv.Close(); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
