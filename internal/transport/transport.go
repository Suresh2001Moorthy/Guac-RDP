package transport

import "rdp-web/internal/renderer"

type InputType byte

const (
    InputMouse     InputType = 0x03
    InputKeyboard  InputType = 0x04
    InputClipboard InputType = 0x06
)

// InputEvent is a decoded client input.
type InputEvent struct {
    Type     InputType
    Flags    uint16
    X, Y     uint16   // mouse only
    Scancode uint16   // key only
    Data     []byte   // clipboard only
}

// Transport abstracts the client-facing connection.
type Transport interface {
    // SendFrame writes a rendered frame to the client.
    // The transport MUST return the frame's buffer to its pool when done.
    SendFrame(frame *renderer.Frame) error
    
    // SendClipboard sends clipboard text to the client.
    SendClipboard(text string) error

    // RecvInput blocks until the next input event arrives.
    RecvInput() (*InputEvent, error)

    // Close terminates the client connection.
    Close() error
}
