package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"rdp-web/internal/api"
	"rdp-web/internal/config"
	"rdp-web/internal/session"
)

func main() {
	log.Println("Starting RDP-Web Gateway...")

	cfg := config.Load()

	sessionManager := session.NewManager()

	srv := api.NewServer(cfg, sessionManager)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	
	if err := srv.Stop(); err != nil {
		log.Printf("Server stop error: %v", err)
	}
	sessionManager.CloseAll()
	
	log.Println("Server exited")
}
