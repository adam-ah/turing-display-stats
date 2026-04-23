package chart

import (
	"image"
	"image/draw"
)

// ApplyRegions paints dirty regions into the persistent frame buffer.
func ApplyRegions(frame *image.RGBA, regions []*DirtyRegion) {
	for _, r := range regions {
		if r == nil || r.Image == nil {
			continue
		}
		dst := image.Rect(r.X, r.Y, r.X+r.Image.Bounds().Dx(), r.Y+r.Image.Bounds().Dy())
		draw.Draw(frame, dst, r.Image, image.Point{}, draw.Src)
	}
}
