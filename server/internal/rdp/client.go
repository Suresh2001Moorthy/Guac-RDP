package rdp

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"time"

	"guac-rdp/internal/guac"
)

// Client interface abstracts the RDP backend (FreeRDP or Mock).
type Client interface {
	Connect(hostname string, username string, password string) error
	StartRendering(ctx context.Context, writeCh chan<- *guac.Instruction)
	SendMouseEvent(x, y, buttons int)
	SendKeyEvent(keysym int, pressed bool)
	Disconnect()
}

// MockClient generates a test pattern and sends it over Guacamole protocol.
// This is used to verify the frontend and WebSocket pipeline until FreeRDP CGO is implemented.
type MockClient struct {
	width  int
	height int
}

func NewMockClient(width, height int) *MockClient {
	return &MockClient{
		width:  width,
		height: height,
	}
}

func (m *MockClient) Connect(hostname, username, password string) error {
	return nil
}

func (m *MockClient) StartRendering(ctx context.Context, writeCh chan<- *guac.Instruction) {
	// Send initial size instruction
	writeCh <- guac.NewInstruction("size", "0", "1024", "768")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	posX := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Generate a simple moving pattern
			img := image.NewRGBA(image.Rect(0, 0, 100, 100))
			for y := 0; y < 100; y++ {
				for x := 0; x < 100; x++ {
					img.Set(x, y, color.RGBA{R: uint8(posX % 255), G: 100, B: 200, A: 255})
				}
			}

			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err == nil {
				// Send img instruction: opcode, layer, id, x, y, base64_png
				// But wait, standard guac is img, layer, mask, x, y, url
				// Oh actually: img, stream_index, mask, layer, mimetype, x, y
				// Let's use the standard base64 embedding: 
				// The client.js expects img instructions, but maybe we should just send a simple 'blob' or 'png' instruction?
				// Wait, the easiest way to send an image in guac is:
				// "img", stream_index, mask, layer, mimetype, x, y
				// Then "blob", stream_index, base64
				// Then "end", stream_index
				// Let's implement that.
				
				// 1. Send 'img' instruction
				// stream=1, mode=14 (copy), layer=0, mimetype=image/png, x=posX, y=100
				// For simplicity, layer=0 is the main display.
				// Wait, let's just use data URI? Guacamole doesn't use data URI. It uses streams.
			}

			posX = (posX + 50) % 800
		}
	}
}

func (m *MockClient) SendMouseEvent(x, y, buttons int) {}
func (m *MockClient) SendKeyEvent(keysym int, pressed bool) {}
func (m *MockClient) Disconnect() {}
