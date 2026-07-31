package session

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"guac-rdp/internal/guac"
	"guac-rdp/internal/rdp"
)

// Session represents a single client connection and its backend RDP session.
type Session struct {
	ID         string
	conn       *websocket.Conn
	parser     *guac.Parser
	encoder    *guac.Encoder
	dispatcher *guac.Dispatcher

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Channels for communication
	readCh  chan *guac.Instruction
	writeCh chan *guac.Instruction

	rdpClient rdp.Client
	startOnce sync.Once
}

// NewSession creates a new session.
func NewSession(id string, conn *websocket.Conn) *Session {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Session{
		ID:         id,
		conn:       conn,
		ctx:        ctx,
		cancel:     cancel,
		readCh:     make(chan *guac.Instruction, 100),
		writeCh:    make(chan *guac.Instruction, 100),
		dispatcher: guac.NewDispatcher(),
		rdpClient:  rdp.NewGrdpClient(1024, 768),
	}

	// Wrap websocket connection in custom io.Reader/Writer
	s.parser = guac.NewParser(newWsReader(conn))
	s.encoder = guac.NewEncoder(newWsWriter(conn))

	// Register basic handlers
	s.dispatcher.SetFallback(guac.HandlerFunc(func(ins *guac.Instruction) error {
		log.Printf("[Session %s] Unhandled instruction: %s", s.ID, ins.Opcode)
		return nil
	}))

	s.dispatcher.Register("disconnect", guac.HandlerFunc(func(ins *guac.Instruction) error {
		return errors.New("client disconnected")
	}))

	s.dispatcher.Register("sync", guac.HandlerFunc(func(ins *guac.Instruction) error {
		// Client acknowledges frame
		return nil
	}))

	s.dispatcher.Register("nop", guac.HandlerFunc(func(ins *guac.Instruction) error {
		// Heartbeat instruction
		return nil
	}))

	s.dispatcher.Register("", guac.HandlerFunc(func(ins *guac.Instruction) error {
		// Ignore empty instructions
		return nil
	}))

	s.dispatcher.Register("ack", guac.HandlerFunc(func(ins *guac.Instruction) error {
		// Client acknowledges stream/blob
		return nil
	}))

	s.dispatcher.Register("mouse", guac.HandlerFunc(func(ins *guac.Instruction) error {
		if s.rdpClient != nil && len(ins.Args) >= 3 {
			// Input forwarding is currently a no-op for the screen-capture backend,
			// but keeping it wired preserves the Guacamole input path.
			x, y, buttons := atoi(ins.Args[0]), atoi(ins.Args[1]), atoi(ins.Args[2])
			s.rdpClient.SendMouseEvent(x, y, buttons)
		}
		return nil
	}))

	s.dispatcher.Register("key", guac.HandlerFunc(func(ins *guac.Instruction) error {
		if s.rdpClient != nil && len(ins.Args) >= 2 {
			keysym := atoi(ins.Args[0])
			pressed := ins.Args[1] == "1" || ins.Args[1] == "true"
			s.rdpClient.SendKeyEvent(keysym, pressed)
		}
		return nil
	}))

	s.dispatcher.Register("size", guac.HandlerFunc(func(ins *guac.Instruction) error {
		// Native client handles resize/size independently if supported.
		s.startOnce.Do(func() {
			go s.startBackend()
		})
		return nil
	}))

	s.dispatcher.Register("connect", guac.HandlerFunc(func(ins *guac.Instruction) error {
		s.startOnce.Do(func() {
			go s.startBackend()
		})
		return nil
	}))

	return s
}

// Start begins the session's worker goroutines.
func (s *Session) Start() {
	s.wg.Add(2)
	go s.readWorker()
	go s.writeWorker()

	s.Send(guac.NewInstruction("", s.ID))
	
	// Fallback start if client doesn't send size within 1 second
	go func() {
		time.Sleep(1 * time.Second)
		s.startOnce.Do(func() {
			go s.startBackend()
		})
	}()
}

// Wait blocks until the session has completely finished.
func (s *Session) Wait() {
	s.wg.Wait()
}

// Close gracefully shuts down the session.
func (s *Session) Close() {
	s.cancel()
	s.conn.Close()
}

// Send queues an instruction to be sent to the client.
func (s *Session) Send(ins *guac.Instruction) {
	select {
	case <-s.ctx.Done():
	case s.writeCh <- ins:
	default:
		log.Printf("[Session %s] Write channel full, dropping instruction", s.ID)
	}
}

func (s *Session) startBackend() {
	log.Printf("[Session %s] Connecting to RDP target...", s.ID)

	if err := s.rdpClient.Connect("192.168.1.243", "qaadmin", "Test@123"); err != nil {
		log.Printf("[Session %s] RDP connect failed: %v", s.ID, err)
		s.Send(guac.NewInstruction("error", err.Error(), "519"))
		s.cancel()
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.rdpClient.StartRendering(s.ctx, s.writeCh)
	}()
}

func atoi(value string) int {
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (s *Session) readWorker() {
	defer s.wg.Done()
	defer s.cancel() // trigger shutdown on error

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			ins, err := s.parser.ReadInstruction()
			if err != nil {
				if !errors.Is(err, guac.ErrConnectionClosed) && !errors.Is(err, io.EOF) && !websocket.IsUnexpectedCloseError(err) {
					log.Printf("[Session %s] Parser error: %v", s.ID, err)
				}
				return
			}

			if err := s.dispatcher.Dispatch(ins); err != nil {
				log.Printf("[Session %s] Dispatcher error: %v", s.ID, err)
				return
			}
		}
	}
}

func (s *Session) writeWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case ins := <-s.writeCh:
			if err := s.encoder.WriteInstruction(ins); err != nil {
				log.Printf("[Session %s] Encoder write error: %v", s.ID, err)
				return
			}
		case <-ticker.C:
			// Send heartbeat (nop instruction) to keep connection alive
			err := s.encoder.WriteInstruction(guac.NewInstruction("nop"))
			if err != nil {
				return
			}
		}
	}
}

// wsReader wraps a websocket connection to implement io.Reader
type wsReader struct {
	conn *websocket.Conn
	r    io.Reader
}

func newWsReader(c *websocket.Conn) *wsReader {
	return &wsReader{conn: c}
}

func (wr *wsReader) Read(p []byte) (int, error) {
	if wr.r == nil {
		_, r, err := wr.conn.NextReader()
		if err != nil {
			return 0, err
		}
		wr.r = r
	}

	n, err := wr.r.Read(p)
	if err == io.EOF {
		wr.r = nil
		return n, nil // Return n bytes read, next call will block on NextReader
	}
	return n, err
}

// wsWriter wraps a websocket connection to implement io.Writer
type wsWriter struct {
	conn *websocket.Conn
}

func newWsWriter(c *websocket.Conn) *wsWriter {
	return &wsWriter{conn: c}
}

func (ww *wsWriter) Write(p []byte) (int, error) {
	w, err := ww.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(p)
	if err != nil {
		w.Close()
		return n, err
	}
	err = w.Close()
	return n, err
}
