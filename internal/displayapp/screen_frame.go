package displayapp

import (
	"image"
	"image/draw"
)

// newScreenFrame creates a persistent in-memory copy of the full screen.
// It mirrors what should already be on the display and is updated only when
// a chart emits new pixels.
func newScreenFrame(base *image.RGBA) *image.RGBA {
	frame := image.NewRGBA(base.Bounds())
	draw.Draw(frame, base.Bounds(), base, image.Point{}, draw.Src)
	return frame
}

// applyRegions paints dirty regions into the persistent frame buffer.
func applyRegions(frame *image.RGBA, regions []*DirtyRegion) {
	for _, r := range regions {
		if r == nil || r.Image == nil {
			continue
		}
		dst := image.Rect(r.X, r.Y, r.X+r.Image.Bounds().Dx(), r.Y+r.Image.Bounds().Dy())
		draw.Draw(frame, dst, r.Image, image.Point{}, draw.Src)
	}
}
