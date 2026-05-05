package app

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"turing-display-go/internal/chart"
)

const (
	divColor        = 0x80
	labelZoneHeight = 15
)

func defaultScreen() chart.ScreenConfig {
	return chart.ScreenConfig{Width: 320, Height: 480}
}

func defaultBlocks() []chart.BlockConfig {
	return []chart.BlockConfig{
		{Metric: "cpu", Label: "CPU", X: 0, Y: 0, Width: 320, Height: 120},
		{Metric: "gpu", Label: "GPU", X: 0, Y: 120, Width: 320, Height: 120},
		{Metric: "ram", Label: "RAM", X: 0, Y: 240, Width: 160, Height: 120},
		{Metric: "vram", Label: "VRAM", X: 160, Y: 240, Width: 160, Height: 120},
	}
}

func renderBaseFrame(screen chart.ScreenConfig, blocks []chart.BlockConfig, fontColor, bg color.RGBA) *image.RGBA {
	gray := color.RGBA{divColor, divColor, divColor, 255}

	img := image.NewRGBA(image.Rect(0, 0, screen.Width, screen.Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	face := basicfont.Face7x13
	metrics := face.Metrics()

	for _, block := range blocks {
		if block.Width <= 0 || block.Height <= 0 {
			continue
		}
		for x := block.X; x < block.X+block.Width && x < screen.Width; x++ {
			if block.Y >= 0 && block.Y < screen.Height {
				img.Set(x, block.Y, gray)
			}
		}

		labelTop := block.Y + 1
		if labelTop < 0 || labelTop >= screen.Height {
			continue
		}

		label := block.Label
		textWidth := font.MeasureString(face, label).Ceil()
		padding := 4
		rectW := textWidth + padding
		if rectW > block.Width {
			rectW = block.Width
		}
		rectX := block.X + (block.Width-rectW)/2

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
