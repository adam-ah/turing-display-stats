package app

import (
	"image/color"
	"testing"

	"turing-display-go/internal/chart"
)

func TestRenderBaseFrameUsesConfiguredBackground(t *testing.T) {
	screen := chart.ScreenConfig{Width: 320, Height: 480}
	blocks := []chart.BlockConfig{
		{Label: "CPU", X: 0, Y: 0, Width: 320, Height: 120},
		{Label: "RAM", X: 0, Y: 240, Width: 160, Height: 120},
		{Label: "VRAM", X: 160, Y: 240, Width: 160, Height: 120},
	}
	bg := color.RGBA{12, 34, 56, 255}

	img := renderBaseFrame(screen, blocks, color.RGBA{0, 0, 0, 255}, bg)

	if got := img.At(0, 1); got != bg {
		t.Fatalf("pixel at (0,1) = %#v, want background %#v", got, bg)
	}
	if got := img.At(0, 0); got != (color.RGBA{128, 128, 128, 255}) {
		t.Fatalf("divider pixel = %#v, want divider line", got)
	}
}
