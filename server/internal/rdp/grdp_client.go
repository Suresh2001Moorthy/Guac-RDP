package rdp

import (
	"bytes"
	"context"
	"encoding/base64"
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

			// RDP bitmaps are usually bottom-up.
			// Let's assume standard layout.
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					// RDP bottom-up indexing (often). If it's upside down, we invert y.
					// Let's just do top-down for now, if it's inverted we will flip later.
					idx := ((h - 1 - y) * w + x) * bytesPerPixel
					
					if idx >= 0 && idx+2 < len(data) {
						if bytesPerPixel >= 3 {
							// BGRA
							img.SetRGBA(x, y, color.RGBA{B: data[idx], G: data[idx+1], R: data[idx+2], A: 255})
						}
					}
				}
			}

			var buf bytes.Buffer
			png.Encode(&buf, img)
			base64PNG := base64.StdEncoding.EncodeToString(buf.Bytes())

			c.streamCounter++
			streamIdx := strconv.Itoa(c.streamCounter)

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
	p := &pdu.PointerEvent{}
	p.XPos = uint16(x)
	p.YPos = uint16(y)
	
	switch buttons {
	case 0:
		p.PointerFlags |= pdu.PTRFLAGS_MOVE
	case 1:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON1 | pdu.PTRFLAGS_DOWN
	case 2:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON2 | pdu.PTRFLAGS_DOWN
	}
	
	if c.pdu != nil {
		c.pdu.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
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
