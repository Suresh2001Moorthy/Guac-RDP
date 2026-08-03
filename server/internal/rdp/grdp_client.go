package rdp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/tomatome/grdp/core"
	"github.com/tomatome/grdp/glog"
	"github.com/tomatome/grdp/protocol/nla"
	"github.com/tomatome/grdp/protocol/pdu"
	"github.com/tomatome/grdp/protocol/sec"
	"github.com/tomatome/grdp/protocol/t125"
	"github.com/tomatome/grdp/protocol/tpkt"
	"github.com/tomatome/grdp/protocol/x224"

	"guac-rdp/internal/guac"
)

type GrdpClient struct {
	width    int
	height   int
	host     string
	username string
	password string

	tpkt *tpkt.TPKT
	x224 *x224.X224
	mcs  *t125.MCSClient
	sec  *sec.Client
	pdu  *pdu.Client

	streamCounter int
	prevButtons   int
}

func NewGrdpClient(width, height int) *GrdpClient {
	return &GrdpClient{
		width:  width,
		height: height,
	}
}

func bpp(b uint16) int {
	return int((b + 7) / 8)
}

func bitmapDecompress(bitmap *pdu.BitmapData) []byte {
	return core.Decompress(bitmap.BitmapDataStream, int(bitmap.Width), int(bitmap.Height), bpp(bitmap.BitsPerPixel))
}

func (c *GrdpClient) Connect(hostname, username, password string) error {
	c.host = hostname + ":3389"
	c.username = username
	c.password = password
	return nil
}

func (c *GrdpClient) StartRendering(ctx context.Context, writeCh chan<- *guac.Instruction) {
	glog.SetLogger(log.New(os.Stdout, "[grdp] ", log.LstdFlags))
	glog.SetLevel(glog.INFO)
	
	writeCh <- guac.NewInstruction("size", "0", strconv.Itoa(c.width), strconv.Itoa(c.height))

	log.Printf("Connecting grdp to %s as %s", c.host, c.username)
	conn, err := net.DialTimeout("tcp", c.host, 5*time.Second)
	if err != nil {
		log.Printf("Dial err: %v", err)
		return
	}

	c.tpkt = tpkt.New(core.NewSocketLayer(conn), nla.NewNTLMv2("", c.username, c.password))
	c.x224 = x224.New(c.tpkt)
	c.mcs = t125.NewMCSClient(c.x224)
	c.sec = sec.NewClient(c.mcs)
	c.pdu = pdu.NewClient(c.sec)

	c.mcs.SetClientCoreData(uint16(c.width), uint16(c.height))
	c.sec.SetUser(c.username)
	c.sec.SetPwd(c.password)
	c.sec.SetDomain("")

	c.tpkt.SetFastPathListener(c.sec)
	c.sec.SetFastPathListener(c.pdu)
	c.sec.SetChannelSender(c.mcs)

	c.pdu.On("error", func(e error) {
		log.Printf("grdp error: %v", e)
	}).On("close", func() {
		log.Println("grdp closed")
	}).On("update", func(rectangles []pdu.BitmapData) {
		log.Printf("grdp received %d rectangles", len(rectangles))
		for _, v := range rectangles {
			data := v.BitmapDataStream
			if v.IsCompress() {
				data = bitmapDecompress(&v)
			}

			w, h := int(v.Width), int(v.Height)
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			bytesPerPixel := bpp(v.BitsPerPixel)
			
			// 1. Row Stride Calculation
			// RDP decompresses into an exact buffer (width * bpp) with NO padding.
			// However, UNCOMPRESSED bitmaps retain the 4-byte boundary padding.
			stride := w * bytesPerPixel
			if !v.IsCompress() {
				stride = ((w * bytesPerPixel) + 3) &^ 3
			}

			// Phase 1: Comprehensive Rectangle Logging
			log.Printf("Rectangle %d: Compressed:%v BPP:%d Flags:0x%04x Width:%d Height:%d DestLeft:%d DestTop:%d DestRight:%d DestBottom:%d BitmapLength:%d Stride:%d bytesPerPixel:%d",
				c.streamCounter+1, v.IsCompress(), v.BitsPerPixel, v.Flags,
				w, h, v.DestLeft, v.DestTop, v.DestRight, v.DestBottom,
				v.BitmapLength, stride, bytesPerPixel)

			// 2. Pixel Indexing & 3. Bitmap Orientation
			for y := 0; y < h; y++ {
				// The RDP bitmap is already top-down in this context. 
				// We removed the (h - 1 - y) inversion which was causing the upside-down image.
				rowOffset := y * stride 
				
				for x := 0; x < w; x++ {
					idx := rowOffset + (x * bytesPerPixel)

					if idx >= 0 && idx+(bytesPerPixel-1) < len(data) {
						switch bytesPerPixel {
						case 2: // 16-bit BGR565 (Windows Default)
							// 4. Pixel Format & 5. Channel Order
							// RDP 16-bit is packed in little-endian.
							// The top 5 bits are Blue, middle 6 are Green, bottom 5 are Red.
							p := uint16(data[idx]) | (uint16(data[idx+1]) << 8)
							b := uint8((p >> 11) & 0x1F)
							g := uint8((p >> 5) & 0x3F)
							r := uint8(p & 0x1F)
							
							// Scale 5-bit and 6-bit values to 8-bit
							r = (r << 3) | (r >> 2)
							g = (g << 2) | (g >> 4)
							b = (b << 3) | (b >> 2)
							
							img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
						case 3: // 24-bit BGR
							img.SetRGBA(x, y, color.RGBA{B: data[idx], G: data[idx+1], R: data[idx+2], A: 255})
						case 4: // 32-bit BGRA
							img.SetRGBA(x, y, color.RGBA{B: data[idx], G: data[idx+1], R: data[idx+2], A: data[idx+3]})
						}
					}
				}
			}

			var buf bytes.Buffer
			png.Encode(&buf, img)
			
			// Phase 1: Image Statistics Calculation
			var uniqueColors = make(map[uint32]struct{})
			var transparentPixels int
			var sumR, sumG, sumB, sumA uint64
			
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					c := img.RGBAAt(x, y)
					c32 := uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A)
					uniqueColors[c32] = struct{}{}
					if c.A < 255 {
						transparentPixels++
					}
					sumR += uint64(c.R)
					sumG += uint64(c.G)
					sumB += uint64(c.B)
					sumA += uint64(c.A)
				}
			}
			
			pixelCount := uint64(w * h)
			var avgR, avgG, avgB, avgA uint8
			if pixelCount > 0 {
				avgR = uint8(sumR / pixelCount)
				avgG = uint8(sumG / pixelCount)
				avgB = uint8(sumB / pixelCount)
				avgA = uint8(sumA / pixelCount)
			}
			
			c.streamCounter++
			streamIdx := strconv.Itoa(c.streamCounter)

			// Phase 1: PNG Dumping & Stats Logging
			debugFile := fmt.Sprintf("frame_%04d.png", c.streamCounter)
			// os.WriteFile(debugFile, buf.Bytes(), 0644) // don't save screenshot
			log.Printf("PNG %s Stats: width:%d height:%d unique_colors:%d transparent:%d avg_rgb:(%d,%d,%d) avg_alpha:%d",
				debugFile, w, h, len(uniqueColors), transparentPixels, avgR, avgG, avgB, avgA)

			base64PNG := base64.StdEncoding.EncodeToString(buf.Bytes())

			writeCh <- guac.NewInstruction("img", streamIdx, "14", "0", "image/png", strconv.Itoa(int(v.DestLeft)), strconv.Itoa(int(v.DestTop)))

			chunkSize := 6000
			for i := 0; i < len(base64PNG); i += chunkSize {
				end := i + chunkSize
				if end > len(base64PNG) {
					end = len(base64PNG)
				}
				writeCh <- guac.NewInstruction("blob", streamIdx, base64PNG[i:end])
			}
			writeCh <- guac.NewInstruction("end", streamIdx)
		}
		writeCh <- guac.NewInstruction("sync", strconv.FormatInt(time.Now().UnixMilli(), 10))
	})

	err = c.x224.Connect()
	if err != nil {
		log.Printf("grdp connect err: %v", err)
		return
	}

	<-ctx.Done()
	c.Disconnect()
}

func (c *GrdpClient) SendMouseEvent(x, y, buttons int) {
	var events []pdu.InputEventsInterface

	// Always send a movement event so Windows knows exactly where the cursor is
	// before applying button clicks.
	pMove := &pdu.PointerEvent{
		XPos: uint16(x),
		YPos: uint16(y),
	}
	pMove.PointerFlags = pdu.PTRFLAGS_MOVE
	events = append(events, pMove)

	// Determine which buttons changed state since the last event
	changed := buttons ^ c.prevButtons

	// Helper to generate correct RDP button flag events
	addButtonEvent := func(rdpFlag uint16, isDown bool, buttonName string) {
		p := &pdu.PointerEvent{
			XPos: uint16(x),
			YPos: uint16(y),
			PointerFlags: rdpFlag,
		}
		
		stateStr := "Up"
		if isDown {
			p.PointerFlags |= pdu.PTRFLAGS_DOWN
			stateStr = "Down"
		}
		
		log.Printf("Server: %s %s flags=0x%04x", buttonName, stateStr, p.PointerFlags)
		events = append(events, p)
	}

	// 1. Browser: Left Button (Guacamole mask 1 -> RDP BUTTON1)
	if (changed & 1) != 0 {
		addButtonEvent(pdu.PTRFLAGS_BUTTON1, (buttons&1) != 0, "Left")
	}
	
	// 2. Browser: Middle Button (Guacamole mask 2 -> RDP BUTTON3)
	// Notice that Middle is Guacamole 2, but RDP BUTTON3
	if (changed & 2) != 0 {
		addButtonEvent(pdu.PTRFLAGS_BUTTON3, (buttons&2) != 0, "Middle")
	}
	
	// 3. Browser: Right Button (Guacamole mask 4 -> RDP BUTTON2)
	// Notice that Right is Guacamole 4, but RDP BUTTON2
	if (changed & 4) != 0 {
		addButtonEvent(pdu.PTRFLAGS_BUTTON2, (buttons&4) != 0, "Right")
	}

	log.Printf("Browser: mouse x=%d y=%d buttons=%d", x, y, buttons)

	// Update state
	c.prevButtons = buttons

	if c.pdu != nil && len(events) > 0 {
		c.pdu.SendInputEvents(pdu.INPUT_EVENT_MOUSE, events)
	}
}

func (c *GrdpClient) SendKeyEvent(keysym int, pressed bool) {
	p := &pdu.ScancodeKeyEvent{}
	// Convert X11 keysym to scancode (simplified for demo)
	p.KeyCode = uint16(keysym)
	if !pressed {
		p.KeyboardFlags |= pdu.KBDFLAGS_RELEASE
	}
	if c.pdu != nil {
		c.pdu.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	}
}

func (c *GrdpClient) Disconnect() {
	if c.tpkt != nil {
		c.tpkt.Close()
	}
}
