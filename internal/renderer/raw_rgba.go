package renderer

import (
    "encoding/binary"
    "sync"
)

// Protocol message types
const (
    MsgFrameBatch    byte = 0x01
    MsgDesktopResize byte = 0x02
    MsgMouse         byte = 0x03
    MsgKeyboard      byte = 0x04
    MsgCursorShape   byte = 0x05
    MsgClipboard     byte = 0x06
)

// Frame header: 3 bytes (type + rect count)
// Per-rect header: 8 bytes (x, y, w, h as uint16 big-endian)
const (
    frameHeaderSize = 3
    rectHeaderSize  = 8
)

// RawRGBARenderer sends raw BGRA pixels with minimal binary headers.
// Browser does BGRA→RGBA swap via Uint32Array bitmask operations.
type RawRGBARenderer struct {
    framePool sync.Pool
}

func NewRawRGBARenderer() *RawRGBARenderer {
    return &RawRGBARenderer{
        framePool: sync.Pool{
            New: func() interface{} {
                buf := make([]byte, 0, 256*1024) // 256KB default
                return &buf
            },
        },
    }
}

func (r *RawRGBARenderer) PixelFormat() PixelFormat {
    return FormatBGRA32
}

func (r *RawRGBARenderer) RenderFrame(rects []DirtyRect) (*Frame, error) {
    // Calculate total size
    totalSize := frameHeaderSize
    for i := range rects {
        totalSize += rectHeaderSize + int(rects[i].W)*int(rects[i].H)*4
    }

    // Get buffer from pool
    bp := r.framePool.Get().(*[]byte)
    buf := *bp
    if cap(buf) < totalSize {
        buf = make([]byte, totalSize)
    }
    buf = buf[:totalSize]

    // Write frame header
    buf[0] = MsgFrameBatch
    binary.BigEndian.PutUint16(buf[1:3], uint16(len(rects)))

    // Write each rect
    offset := frameHeaderSize
    for i := range rects {
        binary.BigEndian.PutUint16(buf[offset:], rects[i].X)
        binary.BigEndian.PutUint16(buf[offset+2:], rects[i].Y)
        binary.BigEndian.PutUint16(buf[offset+4:], rects[i].W)
        binary.BigEndian.PutUint16(buf[offset+6:], rects[i].H)
        offset += rectHeaderSize

        pixelLen := int(rects[i].W) * int(rects[i].H) * 4
        copy(buf[offset:offset+pixelLen], rects[i].Pixels[:pixelLen])
        offset += pixelLen
    }

    return &Frame{
        Data: buf,
        Pool: &r.framePool,
    }, nil
}

func (r *RawRGBARenderer) RenderCursor(cursor *CursorUpdate) (*Frame, error) {
    // MSG_CURSOR_SHAPE: [type:1][hotX:2][hotY:2][w:2][h:2][pixels:w*h*4]
    pixelLen := int(cursor.W) * int(cursor.H) * 4
    totalSize := 1 + 2 + 2 + 2 + 2 + pixelLen  // = 9 + pixelLen

    buf := make([]byte, totalSize)
    buf[0] = MsgCursorShape
    binary.BigEndian.PutUint16(buf[1:], cursor.HotX)
    binary.BigEndian.PutUint16(buf[3:], cursor.HotY)
    binary.BigEndian.PutUint16(buf[5:], cursor.W)
    binary.BigEndian.PutUint16(buf[7:], cursor.H)
    copy(buf[9:], cursor.Pixels[:pixelLen])

    return &Frame{Data: buf}, nil
}

func (r *RawRGBARenderer) RenderResize(width, height uint16) (*Frame, error) {
    // MSG_DESKTOP_RESIZE: [type:1][width:2][height:2]
    buf := make([]byte, 5)
    buf[0] = MsgDesktopResize
    binary.BigEndian.PutUint16(buf[1:], width)
    binary.BigEndian.PutUint16(buf[3:], height)
    return &Frame{Data: buf}, nil
}

func (r *RawRGBARenderer) Close() {
    // Pool is GC'd automatically
}
