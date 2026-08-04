package transport

import (
    "encoding/binary"
    "fmt"
    "sync"

    "github.com/gorilla/websocket"
    "rdp-web/internal/renderer"
    "rdp-web/internal/protocol"
)

// WSTransport implements Transport over gorilla/websocket using binary frames.
type WSTransport struct {
    conn    *websocket.Conn
    writeMu sync.Mutex // WebSocket writes are not concurrent-safe
}

func NewWSTransport(conn *websocket.Conn) *WSTransport {
    return &WSTransport{conn: conn}
}

func (t *WSTransport) SendFrame(frame *renderer.Frame) error {
    t.writeMu.Lock()
    err := t.conn.WriteMessage(websocket.BinaryMessage, frame.Data)
    t.writeMu.Unlock()

    // Return buffer to pool if available
    if frame.Pool != nil {
        frame.Pool.Put(&frame.Data)
    }

    return err
}

func (t *WSTransport) SendClipboard(text string) error {
    msg := protocol.EncodeClipboard(text)
    t.writeMu.Lock()
    err := t.conn.WriteMessage(websocket.BinaryMessage, msg)
    t.writeMu.Unlock()
    return err
}

func (t *WSTransport) RecvInput() (*InputEvent, error) {
    _, data, err := t.conn.ReadMessage()
    if err != nil {
        return nil, err
    }

    if len(data) < 1 {
        return nil, fmt.Errorf("empty message")
    }

    event := &InputEvent{
        Type: InputType(data[0]),
    }

    switch event.Type {
    case InputMouse:
        // [type:1][flags:2][x:2][y:2] = 7 bytes
        if len(data) < 7 {
            return nil, fmt.Errorf("mouse event too short: %d bytes", len(data))
        }
        event.Flags = binary.BigEndian.Uint16(data[1:3])
        event.X = binary.BigEndian.Uint16(data[3:5])
        event.Y = binary.BigEndian.Uint16(data[5:7])

    case InputKeyboard:
        // [type:1][flags:2][scancode:2] = 5 bytes
        if len(data) < 5 {
            return nil, fmt.Errorf("keyboard event too short: %d bytes", len(data))
        }
        event.Flags = binary.BigEndian.Uint16(data[1:3])
        event.Scancode = binary.BigEndian.Uint16(data[3:5])

    case InputClipboard:
        // [type:1][len:4][data:len]
        if len(data) < 5 {
            return nil, fmt.Errorf("clipboard event too short: %d bytes", len(data))
        }
        dataLen := binary.BigEndian.Uint32(data[1:5])
        if len(data) < 5+int(dataLen) {
            return nil, fmt.Errorf("clipboard data truncated")
        }
        event.Data = data[5 : 5+dataLen]

    default:
        return nil, fmt.Errorf("unknown input type: 0x%02x", data[0])
    }

    return event, nil
}

func (t *WSTransport) Close() error {
    return t.conn.Close()
}
