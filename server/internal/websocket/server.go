package websocket

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"guac-rdp/internal/session"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	// In production, we should validate the Origin
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Subprotocols: []string{"guacamole"}, // Guacamole protocol specifies this subprotocol
}

// Server handles incoming WebSocket connections.
type Server struct {
	sessionManager *session.Manager
}

// NewServer creates a new WebSocket server.
func NewServer(sm *session.Manager) *Server {
	return &Server{
		sessionManager: sm,
	}
}

// ServeHTTP upgrades the HTTP request to a WebSocket connection and starts a session.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade websocket: %v", err)
		return
	}

	// Create a new session for this connection
	sess := s.sessionManager.CreateSession(conn)
	log.Printf("Created new session: %s from %s", sess.ID, r.RemoteAddr)

	// Start the session workers
	sess.Start()

	// Wait for the session to complete
	sess.Wait()

	// Clean up the session
	s.sessionManager.RemoveSession(sess.ID)
	log.Printf("Session %s ended", sess.ID)
}
