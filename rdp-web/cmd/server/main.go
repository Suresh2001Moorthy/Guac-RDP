package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"rdp-web/internal/freerdp"
)

type ConnectRequest struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	// Serve static files from the "web" directory
	fs := http.FileServer(http.Dir(`D:\products\Guac-RDP\rdp-web\web`))
	http.Handle("/", fs)

	// API Endpoint for testing the connection
	http.HandleFunc("/api/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ConnectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("Received connect request for %s", req.Address)

		// Milestone 1: Create session and connect
		session := freerdp.New()
		err := session.Connect(req.Address, req.Username, req.Password)

		if err != nil {
			log.Printf("Connection failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("Connection successful!")
		fmt.Fprintf(w, `{"status": "success"}`)
	})

	port := ":8080"
	log.Printf("Server listening on http://localhost%s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
