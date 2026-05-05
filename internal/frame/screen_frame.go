package frame

import (
	"image"
)

// NewScreenFrame returns the persistent in-memory full-screen buffer.
// It mirrors what should already be on the display and is updated only when
// a chart emits new pixels.
func NewScreenFrame(base *image.RGBA) *image.RGBA {
	return base
}
