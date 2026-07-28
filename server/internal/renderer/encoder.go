package renderer

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/draw"
	"image/png"
)

// ExtractDirtyRect compares two images and returns the bounding box of changed pixels.
// If no pixels changed, returns image.ZR
func ExtractDirtyRect(prev, curr *image.RGBA) image.Rectangle {
	bounds := curr.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := -1, -1

	// Fast path bounds check
	if prev == nil {
		return bounds
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			p1 := prev.RGBAAt(x, y)
			p2 := curr.RGBAAt(x, y)

			if p1 != p2 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}

	if maxX < 0 {
		return image.ZR
	}

	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// EncodeRegionToPNG takes an image and a bounding box, crops it, and returns the base64 PNG string.
func EncodeRegionToPNG(img *image.RGBA, rect image.Rectangle) (string, error) {
	subImg := img.SubImage(rect)

	// Ensure the subimage is an RGBA (png.Encode is faster with pure RGBA)
	newImg := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(newImg, newImg.Bounds(), subImg, rect.Min, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, newImg); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
