package app

import (
	"image/color"
	"testing"
)

func TestRenderBaseFrameUsesConfiguredBackground(t *testing.T) {
	labels := [4]string{"CPU", "RAM", "GPU", "VRAM"}
	bg := color.RGBA{12, 34, 56, 255}

	img := renderBaseFrame(labels, color.RGBA{0, 0, 0, 255}, bg)

	if got := img.At(0, 1); got != bg {
		t.Fatalf("pixel at (0,1) = %#v, want background %#v", got, bg)
	}
}
