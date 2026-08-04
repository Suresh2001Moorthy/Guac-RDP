package protocol

import "encoding/binary"

// Message types
const (
    MsgFrameBatch    byte = 0x01
    MsgDesktopResize byte = 0x02
    MsgMouse         byte = 0x03
    MsgKeyboard      byte = 0x04
    MsgCursorShape   byte = 0x05
    MsgClipboard     byte = 0x06
    MsgAudio         byte = 0x07
)

// EncodeMouse creates a 7-byte mouse event message.
func EncodeMouse(flags, x, y uint16) []byte {
    buf := make([]byte, 7)
    buf[0] = MsgMouse
    binary.BigEndian.PutUint16(buf[1:], flags)
    binary.BigEndian.PutUint16(buf[3:], x)
    binary.BigEndian.PutUint16(buf[5:], y)
    return buf
}

// EncodeKeyboard creates a 5-byte keyboard event message.
func EncodeKeyboard(flags, scancode uint16) []byte {
    buf := make([]byte, 5)
    buf[0] = MsgKeyboard
    binary.BigEndian.PutUint16(buf[1:], flags)
    binary.BigEndian.PutUint16(buf[3:], scancode)
    return buf
}

// EncodeDesktopResize creates a 5-byte resize message.
func EncodeDesktopResize(width, height uint16) []byte {
    buf := make([]byte, 5)
    buf[0] = MsgDesktopResize
    binary.BigEndian.PutUint16(buf[1:], width)
    binary.BigEndian.PutUint16(buf[3:], height)
    return buf
}

// DecodeMouse parses a mouse event from raw bytes.
func DecodeMouse(data []byte) (flags, x, y uint16, ok bool) {
    if len(data) < 7 || data[0] != MsgMouse {
        return 0, 0, 0, false
    }
    return binary.BigEndian.Uint16(data[1:3]),
        binary.BigEndian.Uint16(data[3:5]),
        binary.BigEndian.Uint16(data[5:7]),
        true
}

// DecodeKeyboard parses a keyboard event from raw bytes.
func DecodeKeyboard(data []byte) (flags, scancode uint16, ok bool) {
    if len(data) < 5 || data[0] != MsgKeyboard {
        return 0, 0, false
    }
    return binary.BigEndian.Uint16(data[1:3]),
        binary.BigEndian.Uint16(data[3:5]),
        true
}

// EncodeClipboard creates a clipboard event message.
// Payload format: [MsgClipboard (1)] [Length (4)] [UTF-8 Text...]
func EncodeClipboard(text string) []byte {
    textBytes := []byte(text)
    buf := make([]byte, 5+len(textBytes))
    buf[0] = MsgClipboard
    binary.BigEndian.PutUint32(buf[1:5], uint32(len(textBytes)))
    copy(buf[5:], textBytes)
    return buf
}

// DecodeClipboard parses a clipboard event from raw bytes.
func DecodeClipboard(data []byte) (text string, ok bool) {
    if len(data) < 5 || data[0] != MsgClipboard {
        return "", false
    }
    length := binary.BigEndian.Uint32(data[1:5])
    if uint32(len(data)) < 5+length {
        return "", false
    }
    return string(data[5 : 5+length]), true
}
