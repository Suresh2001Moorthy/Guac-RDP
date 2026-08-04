package renderer

import "sync"

// PixelFormat controls byte order in DirtyRect.Pixels
type PixelFormat int

const (
    FormatBGRA32 PixelFormat = iota // FreeRDP native — zero-cost from GDI
    FormatRGBA32                     // Pre-swapped for browser
)

// DirtyRect is a fully-owned, GC-safe rectangle of pixels.
type DirtyRect struct {
    X, Y   uint16
    W, H   uint16
    Pixels []byte // len = W * H * 4, owned by Go (from sync.Pool)
}

// CursorUpdate is a fully-owned cursor shape.
type CursorUpdate struct {
    HotX, HotY uint16
    W, H       uint16
    Pixels     []byte // len = W * H * 4
}

// Frame is the output of a Renderer — an opaque blob ready for transport.
type Frame struct {
    Data []byte     // complete binary message
    Pool *sync.Pool // return Data here when transport is done (may be nil)
}

// Renderer encodes session updates into transport-ready binary frames.
type Renderer interface {
    // RenderFrame encodes one or more dirty rects into a single Frame.
    RenderFrame(rects []DirtyRect) (*Frame, error)

    // RenderCursor encodes a cursor shape change.
    RenderCursor(cursor *CursorUpdate) (*Frame, error)

    // RenderResize encodes a desktop resize notification.
    RenderResize(width, height uint16) (*Frame, error)

    // PixelFormat returns what byte order this renderer expects.
    PixelFormat() PixelFormat

    // Close releases pooled resources.
    Close()
}

// Pixel buffer pool for DirtyRect.Pixels reuse
var pixelPool = sync.Pool{
    New: func() interface{} {
        buf := make([]byte, 64*64*4)
        return &buf
    },
}

// GetPixelBuffer returns a []byte of at least size bytes from the pool.
func GetPixelBuffer(size int) []byte {
    bp := pixelPool.Get().(*[]byte)
    buf := *bp
    if cap(buf) < size {
        buf = make([]byte, size)
    }
    return buf[:size]
}

// PutPixelBuffer returns a buffer to the pool.
func PutPixelBuffer(buf []byte) {
    pixelPool.Put(&buf)
}
