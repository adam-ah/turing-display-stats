package frame

import (
	"image"
	"image/draw"
)

// newScreenFrame creates a persistent in-memory copy of the full screen.
// It mirrors what should already be on the display and is updated only when
// a chart emits new pixels.
func NewScreenFrame(base *image.RGBA) *image.RGBA {
	frame := image.NewRGBA(base.Bounds())
	draw.Draw(frame, base.Bounds(), base, image.Point{}, draw.Src)
	return frame
}
