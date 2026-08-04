package api

import (
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"rdp-web/internal/config"
	"rdp-web/internal/remote/rdp"
	"rdp-web/internal/renderer"
	"rdp-web/internal/session"
	"rdp-web/internal/transport"
)

type Server struct {
	cfg            *config.Config
	sessionManager *session.Manager
	httpServer     *http.Server
	upgrader       websocket.Upgrader
}

func NewServer(cfg *config.Config, sm *session.Manager) *Server {
	return &Server{
		cfg:            cfg,
		sessionManager: sm,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  16384,
			WriteBufferSize: 65536,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServer(http.Dir(s.cfg.WebRoot)))

	s.httpServer = &http.Server{
		Addr:    s.cfg.BindAddress,
		Handler: mux,
	}

	log.Printf("Listening on http://%s", s.cfg.BindAddress)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	user := r.URL.Query().Get("user")
	pass := r.URL.Query().Get("pass")
	width := queryInt(r, "w", 1920)
	height := queryInt(r, "h", 1080)

	if host == "" {
		http.Error(w, "missing host parameter", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	id := uuid.New().String()
	rend := renderer.NewRawRGBARenderer()
	trans := transport.NewWSTransport(conn)
	rdpSession := rdp.New()

	pf := rend.PixelFormat()
	if err := rdpSession.Connect(host, user, pass, width, height, rdp.PixelFormat(pf)); err != nil {
		log.Printf("[%s] RDP connect failed: %v", id, err)
		conn.Close()
		return
	}

	sess := session.NewSession(id, rdpSession, rend, trans)
	s.sessionManager.Add(sess)

	log.Printf("[%s] Session started: %s@%s %dx%d", id, user, host, width, height)

	sess.Start()

	// Wait for session to complete
	sess.Wait()

	s.sessionManager.Remove(id)
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
