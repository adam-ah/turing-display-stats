package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// loadConfig
// ---------------------------------------------------------------------------

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{
		"screen": {"width": 480, "height": 320},
		"dot_size": 5,
		"thresholds": {"green": 0, "yellow": 40, "red": 70},
		"graphs": {"ram": {"x": 0, "y": 10, "width": 100, "height": 50}}
	}`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Screen.Width != 480 || cfg.Screen.Height != 320 {
		t.Errorf("screen = %+v, want 480x320", cfg.Screen)
	}
	if cfg.DotSize != 5 {
		t.Errorf("dot_size = %d, want 5", cfg.DotSize)
	}
	if cfg.Thresholds.Yellow != 40 || cfg.Thresholds.Red != 70 {
		t.Errorf("thresholds = %+v", cfg.Thresholds)
	}
	ram := cfg.Graphs["ram"]
	if ram.X != 0 || ram.Y != 10 || ram.Width != 100 || ram.Height != 50 {
		t.Errorf("ram graph = %+v", ram)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := loadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfigBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte("{not json}"), 0644)
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

// ---------------------------------------------------------------------------
// newChart
// ---------------------------------------------------------------------------

func TestNewChartCapacity(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 100, Height: 50}
	c := newChart(cfg, 5, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})
	if c.capacity != 20 {
		t.Errorf("capacity = %d, want 20", c.capacity)
	}
	if c.filled {
		t.Error("should start unfilled")
	}
	if len(c.history) != 0 {
		t.Error("history should start empty")
	}
	if c.linesDrawn {
		t.Error("lines should not be drawn yet")
	}
}

func TestNewChartZeroWidth(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 0, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{})
	if c.capacity != 1 {
		t.Errorf("capacity = %d, want 1 (minimum)", c.capacity)
	}
}

// ---------------------------------------------------------------------------
// dotColor
// ---------------------------------------------------------------------------

func TestDotColor(t *testing.T) {
	thresh := ThresholdConfig{Green: 0, Yellow: 50, Red: 80}
	cfg := GraphConfig{Width: 20, Height: 10}
	c := newChart(cfg, 4, thresh)

	tests := []struct {
		value float64
		want  string
	}{
		{0, "green"}, {25, "green"}, {49, "green"},
		{50, "yellow"}, {65, "yellow"}, {79, "yellow"},
		{80, "red"}, {95, "red"}, {100, "red"},
	}
	for _, tt := range tests {
		clr := c.dotColor(tt.value)
		got := colorName(clr)
		if got != tt.want {
			t.Errorf("dotColor(%v) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func colorName(clr color.RGBA) string {
	if clr.R == 0 && clr.G == 255 && clr.B == 0 {
		return "green"
	}
	if clr.R == 255 && clr.G == 255 && clr.B == 0 {
		return "yellow"
	}
	if clr.R == 255 && clr.G == 0 && clr.B == 0 {
		return "red"
	}
	if clr.R == 255 && clr.G == 255 && clr.B == 255 {
		return "white"
	}
	if clr.R == 128 && clr.G == 128 && clr.B == 128 {
		return "grey"
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// valueToY — bounding lines at row 0 and height-1, dots in [1..height-2]
// ---------------------------------------------------------------------------

func TestValueToY(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 100, Height: 100}
	c := newChart(cfg, 4, ThresholdConfig{})

	tests := []struct {
		value float64
		want  int
	}{
		{0, 98},    // just above bottom line (height-2)
		{100, 1},   // just below top line
		{50, 50},   // middle
	}
	for _, tt := range tests {
		got := c.valueToY(tt.value)
		if got != tt.want {
			t.Errorf("valueToY(%v) = %d, want %d", tt.value, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// makeDotImage
// ---------------------------------------------------------------------------

func TestMakeDotImage(t *testing.T) {
	cfg := GraphConfig{Width: 20, Height: 10}
	c := newChart(cfg, 4, ThresholdConfig{})
	clr := color.RGBA{255, 0, 0, 255}
	img := c.makeDotImage(clr)

	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 4 {
		t.Errorf("bounds = %v, want 4x4", img.Bounds())
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if img.At(x, y) != clr {
				t.Errorf("pixel(%d,%d) = %v, want %v", x, y, img.At(x, y), clr)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// dotRegion
// ---------------------------------------------------------------------------

func TestDotRegion(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 100, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{})
	clr := color.RGBA{0, 255, 0, 255}
	reg := c.dotRegion(3, clr, 50)

	if reg.X != 10+3*4 {
		t.Errorf("X = %d, want %d", reg.X, 10+3*4)
	}
	wantY := 20 + c.valueToY(50)
	if reg.Y != wantY {
		t.Errorf("Y = %d, want %d", reg.Y, wantY)
	}
	if reg.Image == nil {
		t.Fatal("Image is nil")
	}
}

// ---------------------------------------------------------------------------
// drawBoundingLines
// ---------------------------------------------------------------------------

func TestDrawBoundingLines(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 100, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{})
	regions := c.drawBoundingLines()
	if len(regions) != 2 {
		t.Fatalf("got %d regions, want 2", len(regions))
	}

	// Top line
	top := regions[0]
	if top.X != 10 || top.Y != 20 {
		t.Errorf("top line at (%d,%d), want (10,20)", top.X, top.Y)
	}
	if top.Image.Bounds().Dx() != 100 || top.Image.Bounds().Dy() != 1 {
		t.Errorf("top image size = %dx%d, want 100x1", top.Image.Bounds().Dx(), top.Image.Bounds().Dy())
	}

	// Bottom line
	bot := regions[1]
	if bot.X != 10 || bot.Y != 20+50-1 {
		t.Errorf("bottom line at (%d,%d), want (10,69)", bot.X, bot.Y)
	}
	if bot.Image.Bounds().Dx() != 100 || bot.Image.Bounds().Dy() != 1 {
		t.Errorf("bottom image size = %dx%d, want 100x1", bot.Image.Bounds().Dx(), bot.Image.Bounds().Dy())
	}
}

// ---------------------------------------------------------------------------
// renderFrame
// ---------------------------------------------------------------------------

func TestRenderFrame(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	// Empty chart — renderFrame(50) draws one yellow dot at rightmost col (4)
	colors, values := c.renderFrame(50)
	if colorName(colors[4]) != "yellow" {
		t.Errorf("col 4 = %s, want yellow", colorName(colors[4]))
	}
	if values[4] != 50 {
		t.Errorf("values[4] = %v, want 50", values[4])
	}
	for i := 0; i < 4; i++ {
		if colors[i] != (color.RGBA{255, 255, 255, 255}) {
			t.Errorf("col %d = %v, want white", i, colors[i])
		}
	}

	// Fill partially: history=[10, 60, 90], new=30 → dots at cols 1..4
	c.history = []float64{10, 60, 90}
	colors, values = c.renderFrame(30)
	if colorName(colors[1]) != "green" {
		t.Errorf("col 1 = %s, want green", colorName(colors[1]))
	}
	if colorName(colors[2]) != "yellow" {
		t.Errorf("col 2 = %s, want yellow", colorName(colors[2]))
	}
	if colorName(colors[3]) != "red" {
		t.Errorf("col 3 = %s, want red", colorName(colors[3]))
	}
	if colorName(colors[4]) != "green" {
		t.Errorf("col 4 = %s, want green", colorName(colors[4]))
	}
	if values[4] != 30 {
		t.Errorf("values[4] = %v, want 30", values[4])
	}
}

// ---------------------------------------------------------------------------
// diff
// ---------------------------------------------------------------------------

func TestDiff(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	prevC := []color.RGBA{
		{0, 255, 0, 255},  // green
		{255, 255, 0, 255}, // yellow
		{255, 0, 0, 255},  // red
	}
	prevV := []float64{10, 60, 90}
	currC := []color.RGBA{
		{0, 255, 0, 255},  // green (same color, same value)
		{255, 0, 0, 255},  // red (changed)
		{255, 0, 0, 255},  // red (same)
	}
	currV := []float64{10, 90, 90}

	regions := c.diff(prevC, currC, prevV, currV)
	// col 1: yellow→red (white-out + draw) = 2 regions
	if len(regions) != 2 {
		t.Fatalf("diff: got %d regions, want 2", len(regions))
	}
}

func TestDiffNoChanges(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{})

	prevC := []color.RGBA{{0, 255, 0, 255}, {0, 255, 0, 255}}
	currC := []color.RGBA{{0, 255, 0, 255}, {0, 255, 0, 255}}
	regions := c.diff(prevC, currC, []float64{10, 10}, []float64{10, 10})
	if len(regions) != 0 {
		t.Errorf("no-change diff: got %d regions, want 0", len(regions))
	}
}

func TestDiffNewDots(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	white := color.RGBA{255, 255, 255, 255}
	// prev: col 0 = green, col 1 = white
	prevC := []color.RGBA{{0, 255, 0, 255}, white}
	currC := []color.RGBA{{0, 255, 0, 255}, {255, 255, 0, 255}}
	regions := c.diff(prevC, currC, []float64{10, 0}, []float64{10, 60})
	// col 1: white→yellow = 1 region
	if len(regions) != 1 {
		t.Fatalf("new dot diff: got %d regions, want 1", len(regions))
	}
	if regions[0].X != 4 {
		t.Errorf("new dot X = %d, want 4", regions[0].X)
	}
}

func TestDiffWhiteOut(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	white := color.RGBA{255, 255, 255, 255}
	// prev: col 0 = green(30), col 1 = red(90)
	prevC := []color.RGBA{{0, 255, 0, 255}, {255, 0, 0, 255}}
	// curr: col 0 = green(30), col 1 = white (dot dropped)
	currC := []color.RGBA{{0, 255, 0, 255}, white}
	regions := c.diff(prevC, currC, []float64{30, 90}, []float64{30, 0})
	// col 1: red→white = 1 white-out region
	if len(regions) != 1 {
		t.Fatalf("white-out diff: got %d regions, want 1", len(regions))
	}
	// White-out should be at old Y (value 90 = top of graph)
	wantY := 0 + c.valueToY(90)
	if regions[0].Y != wantY {
		t.Errorf("white-out Y = %d, want %d (old dot position)", regions[0].Y, wantY)
	}
	// Image should be white
	if clr := regions[0].Image.At(0, 0); clr != white {
		t.Errorf("white-out colour = %v, want white", clr)
	}
}

// ---------------------------------------------------------------------------
// update — fill phase
// ---------------------------------------------------------------------------

func TestUpdateFillPhase(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})
	// capacity = 5, dots fill from right (col 4 → col 0)

	// First update — one dot at col 4 + 2 bounding lines = 3 regions
	regions := c.update(30)
	if len(regions) != 3 {
		t.Fatalf("first update: got %d regions, want 3 (dot + 2 lines)", len(regions))
	}
	if !c.linesDrawn {
		t.Error("lines should be drawn after first update")
	}

	// Second update — all dots shift left, every dot changes
	regions = c.update(60)
	if len(regions) < 1 {
		t.Fatal("second update: expected at least 1 region")
	}

	// Fill remaining
	c.update(90)  // 3 dots now
	c.update(10)  // 4 dots now
	c.update(40)  // 5 dots, now filled
	if !c.filled {
		t.Error("chart should be filled now")
	}
}

// ---------------------------------------------------------------------------
// update — shift phase
// ---------------------------------------------------------------------------

func TestUpdateShiftPhase(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	// Fill the chart with green dots
	for i := 0; i < 5; i++ {
		c.update(30)
	}
	if !c.filled {
		t.Fatal("should be filled")
	}

	// Next update — shift: col 4: green(30)→red(90) = white-out + draw = 2 regions
	regions := c.update(90)
	if len(regions) != 2 {
		t.Fatalf("shift update: got %d regions, want 2", len(regions))
	}
}

// ---------------------------------------------------------------------------
// update — diffing (no change = no regions)
// ---------------------------------------------------------------------------

func TestUpdateNoChange(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	// Fill with same value
	for i := 0; i < 5; i++ {
		c.update(30)
	}

	// Same value again — all dots shift but colours stay the same
	regions := c.update(30)
	if len(regions) != 0 {
		t.Errorf("same value update: got %d regions, want 0", len(regions))
	}
}

// ---------------------------------------------------------------------------
// update — colour change produces dirty regions
// ---------------------------------------------------------------------------

func TestUpdateColorChange(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	// Fill with green
	for i := 0; i < 5; i++ {
		c.update(10)
	}

	// Now push red — col 4: green(10)→red(90) = white-out + draw = 2
	regions := c.update(90)
	if len(regions) != 2 {
		t.Fatalf("colour change: got %d regions, want 2", len(regions))
	}
	// Last region should be the red dot
	lastClr := regions[len(regions)-1].Image.At(0, 0)
	if lastClr != (color.RGBA{R: 255, G: 0, B: 0, A: 255}) {
		t.Errorf("last region should be red, got %v", lastClr)
	}
}

// ---------------------------------------------------------------------------
// update — clamping
// ---------------------------------------------------------------------------

func TestUpdateClamping(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	c.update(-10) // clamped to 0, green at col 4
	regions := c.update(150) // clamped to 100, red at col 4, green at col 3
	// col 3: white→green (draw), col 4: green→red (white-out + draw) = 3
	if len(regions) != 3 {
		t.Fatalf("clamping: got %d regions, want 3", len(regions))
	}
	// Last region should be the red dot
	lastClr := regions[len(regions)-1].Image.At(0, 0)
	if lastClr != (color.RGBA{R: 255, G: 0, B: 0, A: 255}) {
		t.Errorf("last region should be red, got %v", lastClr)
	}
}

// ---------------------------------------------------------------------------
// update — shift with varying values (regression test)
// ---------------------------------------------------------------------------

func TestUpdateShiftVarying(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80})

	// Fill: [10, 10, 10, 10, 10]
	for i := 0; i < 5; i++ {
		c.update(10)
	}

	// Push 90: history becomes [10, 10, 10, 10, 90]
	c.update(90)

	// Push 50: history becomes [10, 10, 10, 90, 50]
	// col 0: green→white (white-out), col 3: green→red (white-out + draw),
	// col 4: red→yellow (white-out + draw) = 5 regions
	regions := c.update(50)
	if len(regions) < 1 {
		t.Fatal("varying shift: expected at least 1 region")
	}
}
