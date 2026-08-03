package events

import "log"

type EventType string

const (
	TypeDirtyRect            EventType = "DirtyRect"
	TypeDesktopResize        EventType = "DesktopResize"
	TypeReady                EventType = "Ready"
	TypeDisconnect           EventType = "Disconnect"
	TypeClipboard            EventType = "Clipboard"
	TypeClipboardFormatList  EventType = "ClipboardFormatList"
	TypeClipboardDataRequest EventType = "ClipboardDataRequest"
)

type Event struct {
	Type EventType
	Data interface{}
}

type DirtyRect struct {
	X, Y, W, H int
	Pixels      []byte
}

type Clipboard struct {
	Text string
}

type DesktopResize struct {
	Width, Height int
}

type Ready struct {
	Width, Height int
}

// Bus is a channel-based event bus for FreeRDP callbacks.
// Publish is non-blocking to prevent deadlocking CGO callback threads.
type Bus struct {
	C      chan Event
	closed bool
}

func NewBus(bufferSize int) *Bus {
	return &Bus{
		C: make(chan Event, bufferSize),
	}
}

// Publish sends an event to the bus. If the channel is full, the event
// is dropped and a warning is logged. This prevents CGO callback threads
// from blocking, which would deadlock the FreeRDP event loop.
func (b *Bus) Publish(t EventType, data interface{}) {
	if b.closed {
		return
	}
	select {
	case b.C <- Event{Type: t, Data: data}:
	default:
		log.Printf("[WARN] Event bus full, dropping %s event", t)
	}
}

func (b *Bus) Close() {
	if !b.closed {
		b.closed = true
		close(b.C)
	}
}
