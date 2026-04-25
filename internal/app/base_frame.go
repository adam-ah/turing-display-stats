package app

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	screenWidth     = 320
	screenHeight    = 480
	divColor        = 0x80
	labelZoneHeight = 15
)

var sectionStart = [4]int{0, 120, 240, 360}

func renderBaseFrame(labels [4]string, fontColor, bg color.RGBA) *image.RGBA {
	gray := color.RGBA{divColor, divColor, divColor, 255}

	img := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	face := basicfont.Face7x13
	metrics := face.Metrics()

	for i, label := range labels {
		start := sectionStart[i]
		for x := 0; x < screenWidth; x++ {
			img.Set(x, start, gray)
		}

		labelTop := start + 1
		textWidth := font.MeasureString(face, label).Ceil()
		padding := 4
		rectW := textWidth + padding
		rectX := (screenWidth - rectW) / 2

		textH := metrics.Ascent.Ceil() + metrics.Descent.Ceil()
		textTop := labelTop + (labelZoneHeight-textH)/2
		baseline := textTop + metrics.Ascent.Ceil()

		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(fontColor),
			Face: face,
		}
		d.Dot = fixed.P(rectX+padding/2, baseline)
		d.DrawString(label)
	}

	return img
}
