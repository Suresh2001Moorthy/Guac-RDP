package session

import (
	"context"
	"log"
	"sync"
	"time"

	"rdp-web/internal/events"
	"rdp-web/internal/remote/rdp"
	"rdp-web/internal/renderer"
	"rdp-web/internal/transport"
)

type State int

const (
	StateInitializing State = iota
	StateConnecting
	StateActive
	StateDisconnecting
	StateClosed
)

type Session struct {
	ID        string
	rdp       *rdp.Session
	renderer  renderer.Renderer
	transport transport.Transport

	state   State
	stateMu sync.RWMutex

	dirtyBatch  []renderer.DirtyRect
	batchMu     sync.Mutex
	flushSignal chan struct{}

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closeOnce sync.Once
}

func NewSession(id string, rdpSess *rdp.Session, rend renderer.Renderer, trans transport.Transport) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		ID:          id,
		rdp:         rdpSess,
		renderer:    rend,
		transport:   trans,
		state:       StateInitializing,
		dirtyBatch:  make([]renderer.DirtyRect, 0, 64),
		flushSignal: make(chan struct{}, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *Session) TransitionTo(newState State) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = newState
}

func (s *Session) GetState() State {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

func (s *Session) Start() {
	s.TransitionTo(StateConnecting)

	s.wg.Add(3)
	go s.eventWorker()
	go s.flushWorker()
	go s.inputWorker()

	// Start FreeRDP event loop
	go s.rdp.RunEventLoop(s.ctx)
}

func (s *Session) eventWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case e, ok := <-s.rdp.Bus.C:
			if !ok {
				return
			}
			switch e.Type {
			case events.TypeDirtyRect:
				data := e.Data.(events.DirtyRect)
				s.onDirtyRect(data.X, data.Y, data.W, data.H, data.Pixels)
			case events.TypeDesktopResize:
				data := e.Data.(events.DesktopResize)
				s.onResize(data.Width, data.Height)
			case events.TypeReady:
				s.TransitionTo(StateActive)
				data := e.Data.(events.Ready)
				log.Printf("[Session %s] RDP ready: %dx%d", s.ID, data.Width, data.Height)
				frame, err := s.renderer.RenderResize(uint16(data.Width), uint16(data.Height))
				if err == nil {
					s.transport.SendFrame(frame)
				}
			case events.TypeClipboardFormatList:
				s.rdp.FetchClipboardText()
			case events.TypeClipboardDataRequest:
				s.rdp.RespondClipboardData()
			case events.TypeClipboard:
				data := e.Data.(events.Clipboard)
				s.transport.SendClipboard(data.Text)
			case events.TypeDisconnect:
				s.Close()
			}
		}
	}
}

func (s *Session) onDirtyRect(x, y, w, h int, pixels []byte) {
	if s.GetState() != StateActive {
		renderer.PutPixelBuffer(pixels)
		return
	}

	rect := renderer.DirtyRect{
		X: uint16(x), Y: uint16(y),
		W: uint16(w), H: uint16(h),
		Pixels: pixels,
	}
	s.batchMu.Lock()
	s.dirtyBatch = append(s.dirtyBatch, rect)
	s.batchMu.Unlock()

	select {
	case s.flushSignal <- struct{}{}:
	default:
	}
}

func (s *Session) onResize(width, height int) {
	if s.GetState() != StateActive {
		return
	}
	log.Printf("[Session %s] Desktop resize: %dx%d", s.ID, width, height)
	frame, err := s.renderer.RenderResize(uint16(width), uint16(height))
	if err == nil {
		s.transport.SendFrame(frame)
	}
}

func (s *Session) flushWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.flushSignal:
			if s.GetState() != StateActive {
				continue
			}

			// Small coalesce window to batch more rects
			time.Sleep(time.Millisecond)

			s.batchMu.Lock()
			if len(s.dirtyBatch) == 0 {
				s.batchMu.Unlock()
				continue
			}
			batch := make([]renderer.DirtyRect, len(s.dirtyBatch))
			copy(batch, s.dirtyBatch)
			s.dirtyBatch = s.dirtyBatch[:0]
			s.batchMu.Unlock()

			frame, err := s.renderer.RenderFrame(batch)
			if err != nil {
				log.Printf("[Session %s] render error: %v", s.ID, err)
				continue
			}

			if err := s.transport.SendFrame(frame); err != nil {
				log.Printf("[Session %s] transport error: %v", s.ID, err)
				s.Close()
				return
			}

			// Return pixel buffers to pool
			for i := range batch {
				renderer.PutPixelBuffer(batch[i].Pixels)
			}
		}
	}
}

func (s *Session) inputWorker() {
	defer s.wg.Done()
	for {
		event, err := s.transport.RecvInput()
		if err != nil {
			s.Close()
			return
		}

		if s.GetState() != StateActive {
			continue
		}

		switch event.Type {
		case transport.InputMouse:
			s.rdp.SendMouse(event.Flags, event.X, event.Y)
		case transport.InputKeyboard:
			s.rdp.SendKey(event.Flags, event.Scancode)
		case transport.InputClipboard:
			s.rdp.SendClipboardText(string(event.Data))
		}
	}
}

func (s *Session) Wait() {
	s.wg.Wait()
}

// Close safely tears down the session. Uses sync.Once to guarantee
// single execution even if called from multiple goroutines (eventWorker,
// inputWorker, flushWorker) simultaneously.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.TransitionTo(StateDisconnecting)

		s.cancel()
		s.rdp.Disconnect()
		s.transport.Close()
		s.renderer.Close()

		s.TransitionTo(StateClosed)
		log.Printf("[Session %s] Closed", s.ID)
	})
}

func (s *Session) Context() context.Context {
	return s.ctx
}
