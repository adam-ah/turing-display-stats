package chart

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"turing-display-go/internal/sampler"
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

func TestLoadConfigRefreshSecondsCompat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{
		"screen": {"width": 480, "height": 320},
		"dot_size": 5,
		"thresholds": {"green": 0, "yellow": 40, "red": 70},
		"graphs": {
			"cpu": {"x": 0, "y": 0, "width": 100, "height": 50, "refresh_ms": 1500},
			"ram": {"x": 0, "y": 0, "width": 100, "height": 50, "refresh_sec": 2}
		}
	}`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.Graphs["cpu"].RefreshSec; got != 2 {
		t.Fatalf("cpu refresh = %d, want 2", got)
	}
	if got := cfg.Graphs["ram"].RefreshSec; got != 2 {
		t.Fatalf("ram refresh = %d, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// newChart
// ---------------------------------------------------------------------------

func TestNewChartCapacity(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 100, Height: 50}
	c := newChart(cfg, 5, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
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
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	if c.capacity != 1 {
		t.Errorf("capacity = %d, want 1 (minimum)", c.capacity)
	}
}

func TestNewChartNoLines(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 100, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, true)
	if !c.noLines {
		t.Error("noLines should be true")
	}
}

// ---------------------------------------------------------------------------
// dotColor
// ---------------------------------------------------------------------------

func TestDotColor(t *testing.T) {
	thresh := ThresholdConfig{Green: 0, Yellow: 50, Red: 80}
	cc := ColorConfig{
		Green:  RGBA{color.RGBA{0, 255, 0, 255}},
		Yellow: RGBA{color.RGBA{255, 255, 0, 255}},
		Red:    RGBA{color.RGBA{255, 0, 0, 255}},
	}
	cfg := GraphConfig{Width: 20, Height: 10}
	c := newChart(cfg, 4, thresh, cc, color.RGBA{255, 255, 255, 255}, 1000, false)

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
// valueToY (with lines)
// ---------------------------------------------------------------------------

func TestValueToY(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 100, Height: 100}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)

	// With lines: dot top fits in [1, height-dotSize-1] = [1, 95]
	tests := []struct {
		value float64
		want  int
	}{
		{0, 95},  // 0% → maxDotY (dot occupies 95..98, fits above bottom line at 98)
		{100, 1}, // 100% → row 1 (dot occupies 1..4, fits below top line at 0)
		{50, 48}, // 50% → middle
	}
	for _, tt := range tests {
		got := c.valueToY(tt.value)
		if got != tt.want {
			t.Errorf("valueToY(%v) = %d, want %d", tt.value, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// valueToY (no lines — full height)
// ---------------------------------------------------------------------------

func TestValueToYNoLines(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 100, Height: 100}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, true)

	// No lines: dot top fits in [0, height-dotSize] = [0, 96]
	tests := []struct {
		value float64
		want  int
	}{
		{0, 96},  // 0% → maxDotY (dot occupies 96..99, fits in height 100)
		{100, 0}, // 100% → row 0 (dot occupies 0..3)
		{50, 48}, // 50% → middle
	}
	for _, tt := range tests {
		got := c.valueToY(tt.value)
		if got != tt.want {
			t.Errorf("valueToY(%v) = %d, want %d", tt.value, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// drawBoundingLines
// ---------------------------------------------------------------------------

func TestDrawBoundingLines(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 100, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	regions := c.drawBoundingLines()
	if len(regions) != 2 {
		t.Fatalf("got %d regions, want 2", len(regions))
	}
	top := regions[0]
	if top.X != 10 || top.Y != 20 {
		t.Errorf("top line at (%d,%d), want (10,20)", top.X, top.Y)
	}
	if top.Image.Bounds().Dx() != 100 || top.Image.Bounds().Dy() != 1 {
		t.Errorf("top image size = %dx%d, want 100x1", top.Image.Bounds().Dx(), top.Image.Bounds().Dy())
	}
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
	cc := ColorConfig{
		Green:  RGBA{color.RGBA{0, 255, 0, 255}},
		Yellow: RGBA{color.RGBA{255, 255, 0, 255}},
		Red:    RGBA{color.RGBA{255, 0, 0, 255}},
	}
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, cc, color.RGBA{255, 255, 255, 255}, 1000, false)

	// One yellow dot at rightmost col (4)
	c.history = []float64{50}
	colors, values := c.renderFrame()
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

	// Partially filled: history=[10, 60, 90, 30] → cols 1..4
	c.history = []float64{10, 60, 90, 30}
	colors, values = c.renderFrame()
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
// markChanged
// ---------------------------------------------------------------------------

func TestMarkChanged(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	white := color.RGBA{255, 255, 255, 255}

	// col 0: same green, col 1: green→red, col 2: red→white, col 3: white→yellow, col 4: same
	prevC := []color.RGBA{{0, 255, 0, 255}, {0, 255, 0, 255}, {255, 0, 0, 255}, white, {0, 255, 0, 255}}
	currC := []color.RGBA{{0, 255, 0, 255}, {255, 0, 0, 255}, white, {255, 255, 0, 255}, {0, 255, 0, 255}}
	changed := c.markChanged(prevC, currC, []float64{10, 10, 90, 0, 10}, []float64{10, 90, 0, 60, 10})

	if changed[0] {
		t.Error("col 0 should not be changed (same green, same value)")
	}
	if !changed[1] {
		t.Error("col 1 should be changed (green→red)")
	}
	if !changed[2] {
		t.Error("col 2 should be changed (red→white)")
	}
	if !changed[3] {
		t.Error("col 3 should be changed (white→yellow)")
	}
	if changed[4] {
		t.Error("col 4 should not be changed (same green, same value)")
	}
}

// ---------------------------------------------------------------------------
// batchBlocks
// ---------------------------------------------------------------------------

func TestBatchBlocksSingleDot(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	white := color.RGBA{255, 255, 255, 255}

	changed := []bool{false, false, true, false, false}
	colors := []color.RGBA{white, white, {0, 255, 0, 255}, white, white}
	values := []float64{0, 0, 30, 0, 0}

	regions := c.batchBlocks(changed, colors, values)
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	r := regions[0]
	if r.X != 10+2*4 {
		t.Errorf("X = %d, want %d", r.X, 10+2*4)
	}
	if r.Y != 20 {
		t.Errorf("Y = %d, want 20", r.Y)
	}
	if r.Image.Bounds().Dx() != 4 {
		t.Errorf("block width = %d, want 4", r.Image.Bounds().Dx())
	}
	if r.Image.Bounds().Dy() != 50 {
		t.Errorf("block height = %d, want 50", r.Image.Bounds().Dy())
	}
	// Dot should be green at correct Y
	dotY := c.valueToY(30)
	if clr, ok := r.Image.At(0, dotY).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Errorf("dot colour = %v, want green", r.Image.At(0, dotY))
	}
}

func TestBatchBlocksContiguous(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	white := color.RGBA{255, 255, 255, 255}

	// cols 1,2,3 changed → one block
	changed := []bool{false, true, true, true, false}
	colors := []color.RGBA{white, {0, 255, 0, 255}, {255, 255, 0, 255}, {255, 0, 0, 255}, white}
	values := []float64{0, 20, 50, 80, 0}

	regions := c.batchBlocks(changed, colors, values)
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1 (contiguous)", len(regions))
	}
	r := regions[0]
	if r.X != 4 {
		t.Errorf("X = %d, want 4", r.X)
	}
	if r.Image.Bounds().Dx() != 12 {
		t.Errorf("block width = %d, want 12 (3 cols × 4px)", r.Image.Bounds().Dx())
	}
}

func TestBatchBlocksNonContiguous(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	white := color.RGBA{255, 255, 255, 255}

	// cols 0,1 changed, col 2 white, cols 3,4 changed → 2 blocks
	changed := []bool{true, true, false, true, true}
	colors := []color.RGBA{
		{0, 255, 0, 255}, {0, 255, 0, 255}, white, {255, 0, 0, 255}, {255, 0, 0, 255},
	}
	values := []float64{20, 20, 0, 80, 80}

	regions := c.batchBlocks(changed, colors, values)
	if len(regions) != 2 {
		t.Fatalf("got %d regions, want 2 (non-contiguous)", len(regions))
	}
	// Block 1: cols 0-1
	if regions[0].X != 0 {
		t.Errorf("block 1 X = %d, want 0", regions[0].X)
	}
	if regions[0].Image.Bounds().Dx() != 8 {
		t.Errorf("block 1 width = %d, want 8", regions[0].Image.Bounds().Dx())
	}
	// Block 2: cols 3-4
	if regions[1].X != 12 {
		t.Errorf("block 2 X = %d, want 12", regions[1].X)
	}
	if regions[1].Image.Bounds().Dx() != 8 {
		t.Errorf("block 2 width = %d, want 8", regions[1].Image.Bounds().Dx())
	}
}

func TestBatchBlocksWhiteOut(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	white := color.RGBA{255, 255, 255, 255}

	// col 0: green→white (white-out), col 1: white (unchanged)
	changed := []bool{true, false, false, false, false}
	colors := []color.RGBA{white, white, white, white, white}
	values := []float64{0, 0, 0, 0, 0}

	regions := c.batchBlocks(changed, colors, values)
	// White-out is white on white background → block is all white but still sent
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
}

// ---------------------------------------------------------------------------
// renderFullBlock (with lines)
// ---------------------------------------------------------------------------

func TestRenderFullBlockWithLines(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)

	white := color.RGBA{255, 255, 255, 255}
	colors := []color.RGBA{
		{0, 255, 0, 255}, {255, 255, 0, 255}, {255, 0, 0, 255}, white, white,
	}
	values := []float64{20, 50, 80, 0, 0}

	r := c.renderFullBlock(colors, values, true)
	if r.X != 10 || r.Y != 20 {
		t.Errorf("block at (%d,%d), want (10,20)", r.X, r.Y)
	}
	if r.Image.Bounds().Dx() != 20 || r.Image.Bounds().Dy() != 50 {
		t.Errorf("block size = %dx%d, want 20x50", r.Image.Bounds().Dx(), r.Image.Bounds().Dy())
	}
	// Top line should be grey
	if clr, ok := r.Image.At(0, 0).(color.RGBA); !ok || colorName(clr) != "grey" {
		t.Errorf("top line not grey")
	}
	// Bottom line should be grey
	if clr, ok := r.Image.At(0, 49).(color.RGBA); !ok || colorName(clr) != "grey" {
		t.Errorf("bottom line not grey")
	}
	// Dot at col 0 should be green (dot top at valueToY(20), full dot spans dotSize rows)
	dotY := c.valueToY(20)
	if clr, ok := r.Image.At(0, dotY).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Errorf("col 0 dot top not green at y=%d", dotY)
	}
	// Dot at col 2 should be red
	dotY = c.valueToY(80)
	if clr, ok := r.Image.At(8, dotY).(color.RGBA); !ok || colorName(clr) != "red" {
		t.Errorf("col 2 dot top not red at y=%d", dotY)
	}
}

func TestRenderFullBlockWithoutLines(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)

	white := color.RGBA{255, 255, 255, 255}
	colors := []color.RGBA{
		{0, 255, 0, 255}, white, white, white, white,
	}
	values := []float64{30, 0, 0, 0, 0}

	r := c.renderFullBlock(colors, values, false)
	// Block should skip line rows: Y starts at row 1, height = 48
	if r.X != 10 || r.Y != 21 {
		t.Errorf("block at (%d,%d), want (10,21)", r.X, r.Y)
	}
	if r.Image.Bounds().Dy() != 48 {
		t.Errorf("block height = %d, want 48 (no line rows)", r.Image.Bounds().Dy())
	}
	// Dot at col 0, value 30: valueToY(30) = some value, minus yOffset(1)
	dotY := c.valueToY(30) - 1
	if clr, ok := r.Image.At(0, dotY).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Errorf("col 0 dot not green at y=%d", dotY)
	}
}

// ---------------------------------------------------------------------------
// renderFullBlock (noLines mode)
// ---------------------------------------------------------------------------

func TestRenderFullBlockNoLines(t *testing.T) {
	cfg := GraphConfig{X: 10, Y: 20, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, true)

	white := color.RGBA{255, 255, 255, 255}
	colors := []color.RGBA{
		{0, 255, 0, 255}, {255, 255, 0, 255}, {255, 0, 0, 255}, white, white,
	}
	values := []float64{20, 50, 80, 0, 0}

	// Even with drawLines=true, noLines chart should NOT draw lines.
	r := c.renderFullBlock(colors, values, true)
	if r.X != 10 || r.Y != 20 {
		t.Errorf("block at (%d,%d), want (10,20)", r.X, r.Y)
	}
	if r.Image.Bounds().Dx() != 20 || r.Image.Bounds().Dy() != 50 {
		t.Errorf("block size = %dx%d, want 20x50", r.Image.Bounds().Dx(), r.Image.Bounds().Dy())
	}
	// No lines — top row should NOT be grey
	if clr, ok := r.Image.At(0, 0).(color.RGBA); !ok || colorName(clr) == "grey" {
		t.Errorf("top row should not be grey in noLines mode")
	}
	// Dot at col 0 should be green at valueToY(20) (full height mapping)
	dotY := c.valueToY(20)
	if clr, ok := r.Image.At(0, dotY).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Errorf("col 0 dot not green at y=%d", dotY)
	}
}

// tick returns a time base + count*1s, then increments count.
// Used in tests to step time forward one second at a time.
func tick(base time.Time, count *int) time.Time {
	t := base.Add(time.Duration(*count) * time.Second)
	*count++
	return t
}

// ---------------------------------------------------------------------------
// update — always advances on each call
// ---------------------------------------------------------------------------

func TestChartUpdateAlwaysAdvances(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1, false)
	base := time.Unix(0, 0)

	// First update always goes through.
	regions := c.update(50, base)
	if len(regions) != 1 {
		t.Fatalf("first update: got %d regions, want 1", len(regions))
	}

	// Update 500ms later — still advances because chart timing is handled elsewhere.
	regions = c.update(50, base.Add(500*time.Millisecond))
	if len(regions) != 1 {
		t.Errorf("too-soon update: got %d regions, want 1", len(regions))
	}

	// Update exactly 1s after first — also advances.
	regions = c.update(50, base.Add(1*time.Second))
	if len(regions) != 1 {
		t.Errorf("exact-interval update: got %d regions, want 1", len(regions))
	}

	// Update another second later — still advances and appends a new dot.
	regions = c.update(50, base.Add(2*time.Second))
	if len(regions) != 1 {
		t.Errorf("repeat update: got %d regions, want 1", len(regions))
	}

	if got := len(c.history); got != 4 {
		t.Errorf("history length = %d, want 4", got)
	}
}

func TestMetricCacheTurnCadence(t *testing.T) {
	cache := sampler.NewMetricCache(2)

	if got, repeats := cache.Update(0, 10); got != 10 || repeats != 2 {
		t.Fatalf("first update = (%v,%d), want (10,2)", got, repeats)
	}
	if got, repeats := cache.Update(1, 20); got != 10 || repeats != 0 {
		t.Fatalf("1s update = (%v,%d), want repeated (10,0)", got, repeats)
	}
	if got, repeats := cache.Update(2, 30); got != 30 || repeats != 2 {
		t.Fatalf("2s update = (%v,%d), want (30,2)", got, repeats)
	}
	if got, repeats := cache.Update(3, 40); got != 30 || repeats != 0 {
		t.Fatalf("3s update = (%v,%d), want repeated (30,0)", got, repeats)
	}
}

func TestMetricCacheDefaultsToOneSecond(t *testing.T) {
	cache := sampler.NewMetricCache(0)
	if cache.RefreshSec != 1 {
		t.Fatalf("refreshSec = %d, want 1", cache.RefreshSec)
	}
}

func TestSlowMetricSkipsIntermediateTurn(t *testing.T) {
	cache := sampler.NewMetricCache(2)

	value, repeats := cache.Update(0, 42)
	if value != 42 || repeats != 2 {
		t.Fatalf("initial update = (%v,%d), want (42,2)", value, repeats)
	}

	value, repeats = cache.Update(1, 99)
	if value != 42 || repeats != 0 {
		t.Fatalf("intermediate tick = (%v,%d), want repeated (42,0)", value, repeats)
	}

	value, repeats = cache.Update(2, 99)
	if value != 99 || repeats != 2 {
		t.Fatalf("refresh tick = (%v,%d), want (99,2)", value, repeats)
	}
}

func TestSlowMetricUsesTurnCountNotWallClock(t *testing.T) {
	cache := sampler.NewMetricCache(2)

	value, repeats := cache.Update(10, 55)
	if value != 55 || repeats != 2 {
		t.Fatalf("turn 10 = (%v,%d), want (55,2)", value, repeats)
	}

	// Large wall-clock gaps do not exist in this helper anymore; only turns matter.
	value, repeats = cache.Update(11, 66)
	if value != 55 || repeats != 0 {
		t.Fatalf("turn 11 = (%v,%d), want repeated (55,0)", value, repeats)
	}

	value, repeats = cache.Update(12, 77)
	if value != 77 || repeats != 2 {
		t.Fatalf("turn 12 = (%v,%d), want (77,2)", value, repeats)
	}
}

func TestApplyRegions(t *testing.T) {
	base := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			base.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}

	region := &DirtyRegion{
		X: 1,
		Y: 1,
		Image: func() *image.RGBA {
			img := image.NewRGBA(image.Rect(0, 0, 2, 2))
			for y := 0; y < 2; y++ {
				for x := 0; x < 2; x++ {
					img.Set(x, y, color.RGBA{255, 0, 0, 255})
				}
			}
			return img
		}(),
	}

	ApplyRegions(base, []*DirtyRegion{region})
	if clr, ok := base.At(1, 1).(color.RGBA); !ok || clr.R != 255 || clr.G != 0 || clr.B != 0 {
		t.Fatalf("composed pixel = %#v, want red", base.At(1, 1))
	}
	if clr, ok := base.At(0, 0).(color.RGBA); !ok || clr.R != 255 || clr.G != 255 || clr.B != 255 {
		t.Fatalf("base pixel = %#v, want white", base.At(0, 0))
	}
}

func TestChartUpdateRepeated(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1, false)

	regions := c.updateRepeated(25, 2)
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	if got := len(c.history); got != 2 {
		t.Fatalf("history length = %d, want 2", got)
	}
	if c.history[0] != 25 || c.history[1] != 25 {
		t.Fatalf("history = %#v, want two identical values", c.history)
	}
}

func TestSeriesChartUpdateRepeated(t *testing.T) {
	cfg := GraphConfig{
		X:      0,
		Y:      0,
		Width:  8,
		Height: 20,
		Series: []SeriesConfig{
			{Name: "down", Color: RGBA{color.RGBA{0, 255, 0, 255}}, FillColor: RGBA{color.RGBA{200, 255, 200, 255}}},
			{Name: "up", Color: RGBA{color.RGBA{255, 0, 0, 255}}, FillColor: RGBA{color.RGBA{255, 200, 200, 255}}},
		},
	}
	c := newSeriesChart(cfg, 4, color.RGBA{255, 255, 255, 255}, true)

	regions := c.UpdateRepeated([]float64{20, 80}, 1)
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	img := regions[0].Image
	if clr, ok := img.At(4, c.valueToY(20)).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Fatalf("download pixel = %#v, want green", img.At(4, c.valueToY(20)))
	}
	if clr, ok := img.At(4, c.valueToY(80)).(color.RGBA); !ok || colorName(clr) != "red" {
		t.Fatalf("upload pixel = %#v, want red", img.At(4, c.valueToY(80)))
	}
}

func TestSeriesChartUpdateSamplesKeepsFineGrainedValues(t *testing.T) {
	cfg := GraphConfig{
		X:      0,
		Y:      0,
		Width:  8,
		Height: 20,
		Series: []SeriesConfig{
			{Name: "value", Color: RGBA{color.RGBA{0, 255, 0, 255}}, FillColor: RGBA{color.RGBA{200, 255, 200, 255}}},
		},
	}
	c := newSeriesChart(cfg, 4, color.RGBA{255, 255, 255, 255}, true)

	regions := c.UpdateSamples([][]float64{{20}, {80}})
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	img := regions[0].Image
	if clr, ok := img.At(0, c.valueToY(20)).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Fatalf("first sample pixel = %#v, want green", img.At(0, c.valueToY(20)))
	}
	if clr, ok := img.At(4, c.valueToY(80)).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Fatalf("second sample pixel = %#v, want green", img.At(4, c.valueToY(80)))
	}
}

func TestSeriesChartWithoutLinesFillsLastContentRow(t *testing.T) {
	fill := color.RGBA{200, 255, 200, 255}
	cfg := GraphConfig{
		X:      0,
		Y:      0,
		Width:  8,
		Height: 20,
		Series: []SeriesConfig{
			{Name: "value", Color: RGBA{color.RGBA{0, 255, 0, 255}}, FillColor: RGBA{fill}},
		},
	}
	c := newSeriesChart(cfg, 4, color.RGBA{255, 255, 255, 255}, false)

	_ = c.UpdateRepeated([]float64{50}, 1)
	regions := c.UpdateRepeated([]float64{50}, 1)
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	img := regions[0].Image
	lastRow := img.Bounds().Dy() - 1
	if got := img.At(4, lastRow); got != fill {
		t.Fatalf("last content row = %#v, want fill %#v", got, fill)
	}
}

// ---------------------------------------------------------------------------
// update — fill phase (with lines)
// ---------------------------------------------------------------------------

func TestUpdateFillPhase(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	base := time.Unix(0, 0)
	n := 0

	// First update: lines drawn, block covers full height (50px)
	regions := c.update(30, tick(base, &n))
	if len(regions) != 1 {
		t.Fatalf("first update: got %d regions, want 1", len(regions))
	}
	if regions[0].Image.Bounds().Dy() != 50 {
		t.Errorf("first block height = %d, want 50 (with lines)", regions[0].Image.Bounds().Dy())
	}
	// Top line should be grey
	if clr, ok := regions[0].Image.At(0, 0).(color.RGBA); !ok || colorName(clr) != "grey" {
		t.Errorf("first update: top line not grey")
	}

	// Second update: no lines, block covers rows 1..48 only (48px)
	regions = c.update(30, tick(base, &n))
	if len(regions) != 1 {
		t.Fatalf("second update: got %d regions, want 1", len(regions))
	}
	if regions[0].Image.Bounds().Dy() != 48 {
		t.Errorf("second block height = %d, want 48 (no lines)", regions[0].Image.Bounds().Dy())
	}
	// Block Y should skip row 0 (the line row)
	if regions[0].Y != 1 {
		t.Errorf("second block Y = %d, want 1", regions[0].Y)
	}

	// Fill remaining
	for i := 0; i < 3; i++ {
		c.update(30, tick(base, &n))
	}
	if !c.filled {
		t.Error("should be filled")
	}
}

// ---------------------------------------------------------------------------
// update — fill phase (noLines mode)
// ---------------------------------------------------------------------------

func TestUpdateFillPhaseNoLines(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, true)
	base := time.Unix(0, 0)
	n := 0

	// First update: no lines, block covers full height (50px)
	regions := c.update(30, tick(base, &n))
	if len(regions) != 1 {
		t.Fatalf("first update: got %d regions, want 1", len(regions))
	}
	if regions[0].Image.Bounds().Dy() != 50 {
		t.Errorf("first block height = %d, want 50 (full height, no lines)", regions[0].Image.Bounds().Dy())
	}
	// No grey lines
	if clr, ok := regions[0].Image.At(0, 0).(color.RGBA); !ok || colorName(clr) == "grey" {
		t.Errorf("should not have grey lines in noLines mode")
	}

	// Second update: still full height
	regions = c.update(30, tick(base, &n))
	if len(regions) != 1 {
		t.Fatalf("second update: got %d regions, want 1", len(regions))
	}
	if regions[0].Image.Bounds().Dy() != 50 {
		t.Errorf("second block height = %d, want 50", regions[0].Image.Bounds().Dy())
	}
	if regions[0].Y != 0 {
		t.Errorf("second block Y = %d, want 0", regions[0].Y)
	}
}

// ---------------------------------------------------------------------------
// update — shift phase
// ---------------------------------------------------------------------------

func TestUpdateShiftPhase(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	base := time.Unix(0, 0)
	n := 0

	for i := 0; i < 5; i++ {
		c.update(30, tick(base, &n))
	}

	regions := c.update(90, tick(base, &n))
	if len(regions) != 1 {
		t.Fatalf("shift update: got %d regions, want 1", len(regions))
	}
}

// ---------------------------------------------------------------------------
// update — no change (DEBUG mode: still sends full block)
// ---------------------------------------------------------------------------

func TestUpdateNoChange(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	base := time.Unix(0, 0)
	n := 0

	for i := 0; i < 5; i++ {
		c.update(30, tick(base, &n))
	}
	regions := c.update(30, tick(base, &n))
	if len(regions) != 1 {
		t.Errorf("same value update: got %d regions, want 1 (DEBUG full block)", len(regions))
	}
}

// ---------------------------------------------------------------------------
// update — clamping
// ---------------------------------------------------------------------------

func TestUpdateClamping(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	base := time.Unix(0, 0)
	n := 0

	c.update(-10, tick(base, &n))
	regions := c.update(150, tick(base, &n))
	if len(regions) != 1 {
		t.Fatalf("clamping: got %d regions, want 1", len(regions))
	}
}

// ---------------------------------------------------------------------------
// update — varying values
// ---------------------------------------------------------------------------

func TestUpdateShiftVarying(t *testing.T) {
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, ColorConfig{}, color.RGBA{255, 255, 255, 255}, 1000, false)
	base := time.Unix(0, 0)
	n := 0

	for i := 0; i < 5; i++ {
		c.update(10, tick(base, &n))
	}
	c.update(90, tick(base, &n))
	regions := c.update(50, tick(base, &n))
	if len(regions) != 1 {
		t.Fatalf("varying shift: got %d regions, want 1", len(regions))
	}
}

// ---------------------------------------------------------------------------
// Regression: dots render right-aligned, shift seamlessly
// ---------------------------------------------------------------------------

func TestUpdateRightAlignedFill(t *testing.T) {
	cc := ColorConfig{
		Green:  RGBA{color.RGBA{0, 255, 0, 255}},
		Yellow: RGBA{color.RGBA{255, 255, 0, 255}},
		Red:    RGBA{color.RGBA{255, 0, 0, 255}},
	}
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, ThresholdConfig{Green: 0, Yellow: 50, Red: 80}, cc, color.RGBA{255, 255, 255, 255}, 1000, false)
	base := time.Unix(0, 0)
	n := 0

	// First dot — full block with lines
	regions := c.update(30, tick(base, &n))
	if len(regions) != 1 {
		t.Fatal("first update: expected 1 region")
	}
	if regions[0].X != 0 {
		t.Errorf("block X = %d, want 0", regions[0].X)
	}
	// Verify dot is at rightmost column (col 4)
	dotY := c.valueToY(30)
	if clr, ok := regions[0].Image.At(16, dotY).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Errorf("dot at col 4 not green")
	}

	// Second dot — block without lines (offset Y by 1)
	regions = c.update(30, tick(base, &n))
	if len(regions) != 1 {
		t.Fatal("second update: expected 1 region")
	}
	// Verify dots at cols 3 and 4 (yOffset = 1)
	dotY2 := c.valueToY(30) - 1
	if clr, ok := regions[0].Image.At(12, dotY2).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Errorf("dot at col 3 not green")
	}
	if clr, ok := regions[0].Image.At(16, dotY2).(color.RGBA); !ok || colorName(clr) != "green" {
		t.Errorf("dot at col 4 not green")
	}
}

// ---------------------------------------------------------------------------
// dotFillColor
// ---------------------------------------------------------------------------

func TestDotFillColor(t *testing.T) {
	thresh := ThresholdConfig{Green: 0, Yellow: 50, Red: 80}
	cc := ColorConfig{
		GreenFill:  RGBA{color.RGBA{144, 238, 144, 255}},
		YellowFill: RGBA{color.RGBA{255, 255, 224, 255}},
		RedFill:    RGBA{color.RGBA{255, 182, 193, 255}},
	}
	cfg := GraphConfig{Width: 20, Height: 10}
	c := newChart(cfg, 4, thresh, cc, color.RGBA{255, 255, 255, 255}, 1000, false)

	tests := []struct {
		value float64
		want  color.RGBA
	}{
		{0, cc.GreenFill.RGBA}, {25, cc.GreenFill.RGBA}, {49, cc.GreenFill.RGBA},
		{50, cc.YellowFill.RGBA}, {65, cc.YellowFill.RGBA}, {79, cc.YellowFill.RGBA},
		{80, cc.RedFill.RGBA}, {95, cc.RedFill.RGBA}, {100, cc.RedFill.RGBA},
	}
	for _, tt := range tests {
		got := c.dotFillColor(tt.value)
		if got != tt.want {
			t.Errorf("dotFillColor(%v) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// renderFullBlock — fill area below dots
// ---------------------------------------------------------------------------

func TestRenderFullBlockFillBelowDot(t *testing.T) {
	thresh := ThresholdConfig{Green: 0, Yellow: 50, Red: 80}
	cc := ColorConfig{
		GreenFill:  RGBA{color.RGBA{144, 238, 144, 255}},
		YellowFill: RGBA{color.RGBA{255, 255, 224, 255}},
		RedFill:    RGBA{color.RGBA{255, 182, 193, 255}},
	}
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, thresh, cc, color.RGBA{255, 255, 255, 255}, 1000, true) // noLines for simplicity

	white := color.RGBA{255, 255, 255, 255}
	colors := []color.RGBA{
		{0, 255, 0, 255}, {255, 255, 0, 255}, {255, 0, 0, 255}, white, white,
	}
	values := []float64{20, 50, 80, 0, 0}

	r := c.renderFullBlock(colors, values, false)

	// Col 0: green dot at value 20, fill below should be green_fill
	dotY := c.valueToY(20)
	// Dot pixel should be green
	if clr, ok := r.Image.At(0, dotY).(color.RGBA); !ok || clr != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("col 0 dot not green at y=%d", dotY)
	}
	// Pixel below dot should be green_fill
	fillY := dotY + c.dotSize
	if fillY < 50 {
		if clr, ok := r.Image.At(0, fillY).(color.RGBA); !ok || clr != cc.GreenFill.RGBA {
			t.Errorf("col 0 fill at y=%d = %v, want %v", fillY, clr, cc.GreenFill.RGBA)
		}
	}

	// Col 1: yellow dot at value 50, fill below should be yellow_fill
	dotY = c.valueToY(50)
	if clr, ok := r.Image.At(4, dotY).(color.RGBA); !ok || clr != (color.RGBA{255, 255, 0, 255}) {
		t.Errorf("col 1 dot not yellow at y=%d", dotY)
	}
	fillY = dotY + c.dotSize
	if fillY < 50 {
		if clr, ok := r.Image.At(4, fillY).(color.RGBA); !ok || clr != cc.YellowFill.RGBA {
			t.Errorf("col 1 fill at y=%d = %v, want %v", fillY, clr, cc.YellowFill.RGBA)
		}
	}

	// Col 2: red dot at value 80, fill below should be red_fill
	dotY = c.valueToY(80)
	if clr, ok := r.Image.At(8, dotY).(color.RGBA); !ok || clr != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("col 2 dot not red at y=%d", dotY)
	}
	fillY = dotY + c.dotSize
	if fillY < 50 {
		if clr, ok := r.Image.At(8, fillY).(color.RGBA); !ok || clr != cc.RedFill.RGBA {
			t.Errorf("col 2 fill at y=%d = %v, want %v", fillY, clr, cc.RedFill.RGBA)
		}
	}

	// Col 3: no dot (white), should remain white
	if clr, ok := r.Image.At(12, 25).(color.RGBA); !ok || clr != white {
		t.Errorf("col 3 should be white, got %v", clr)
	}
}

// ---------------------------------------------------------------------------
// batchBlocks — fill area below dots
// ---------------------------------------------------------------------------

func TestBatchBlocksFillBelowDot(t *testing.T) {
	thresh := ThresholdConfig{Green: 0, Yellow: 50, Red: 80}
	cc := ColorConfig{
		GreenFill:  RGBA{color.RGBA{144, 238, 144, 255}},
		YellowFill: RGBA{color.RGBA{255, 255, 224, 255}},
		RedFill:    RGBA{color.RGBA{255, 182, 193, 255}},
	}
	cfg := GraphConfig{X: 0, Y: 0, Width: 20, Height: 50}
	c := newChart(cfg, 4, thresh, cc, color.RGBA{255, 255, 255, 255}, 1000, false)
	white := color.RGBA{255, 255, 255, 255}

	changed := []bool{false, true, true, false, false}
	colors := []color.RGBA{white, {0, 255, 0, 255}, {255, 0, 0, 255}, white, white}
	values := []float64{0, 30, 90, 0, 0}

	regions := c.batchBlocks(changed, colors, values)
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	r := regions[0]

	// Col 1 (block x=0): green dot, fill below should be green_fill
	dotY := c.valueToY(30)
	if clr, ok := r.Image.At(0, dotY).(color.RGBA); !ok || clr != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("col 1 dot not green at y=%d", dotY)
	}
	fillY := dotY + c.dotSize
	if fillY < 50 {
		if clr, ok := r.Image.At(0, fillY).(color.RGBA); !ok || clr != cc.GreenFill.RGBA {
			t.Errorf("col 1 fill at y=%d = %v, want %v", fillY, clr, cc.GreenFill.RGBA)
		}
	}

	// Col 2 (block x=4): red dot, fill below should be red_fill
	dotY = c.valueToY(90)
	if clr, ok := r.Image.At(4, dotY).(color.RGBA); !ok || clr != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("col 2 dot not red at y=%d", dotY)
	}
	fillY = dotY + c.dotSize
	if fillY < 50 {
		if clr, ok := r.Image.At(4, fillY).(color.RGBA); !ok || clr != cc.RedFill.RGBA {
			t.Errorf("col 2 fill at y=%d = %v, want %v", fillY, clr, cc.RedFill.RGBA)
		}
	}
}

// ---------------------------------------------------------------------------
// Config loading with colors
// ---------------------------------------------------------------------------

func TestLoadConfigWithColors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{
		"screen": {"width": 320, "height": 480},
		"dot_size": 4,
		"thresholds": {"green": 0, "yellow": 50, "red": 80},
		"colors": {
			"green":       "#00FF00",
			"yellow":      "#FFFF00",
			"red":         "#FF0000",
			"green_fill":  "#90EE90",
			"yellow_fill": "#FFFFE0",
			"red_fill":    "#FFB6C1",
			"background":  "#FFFFFF",
			"font_color":  "#000000"
		},
		"graphs": {"cpu": {"x": 10, "y": 16, "width": 300, "height": 96}}
	}`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Colors.GreenFill.RGBA != (color.RGBA{144, 238, 144, 255}) {
		t.Errorf("green_fill = %v, want {144, 238, 144, 255}", cfg.Colors.GreenFill.RGBA)
	}
	if cfg.Colors.YellowFill.RGBA != (color.RGBA{255, 255, 224, 255}) {
		t.Errorf("yellow_fill = %v, want {255, 255, 224, 255}", cfg.Colors.YellowFill.RGBA)
	}
	if cfg.Colors.RedFill.RGBA != (color.RGBA{255, 182, 193, 255}) {
		t.Errorf("red_fill = %v, want {255, 182, 193, 255}", cfg.Colors.RedFill.RGBA)
	}
	if cfg.Colors.Green.RGBA != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("green = %v, want {0, 255, 0, 255}", cfg.Colors.Green.RGBA)
	}
	if cfg.Colors.Yellow.RGBA != (color.RGBA{255, 255, 0, 255}) {
		t.Errorf("yellow = %v, want {255, 255, 0, 255}", cfg.Colors.Yellow.RGBA)
	}
	if cfg.Colors.Red.RGBA != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("red = %v, want {255, 0, 0, 255}", cfg.Colors.Red.RGBA)
	}
	if cfg.Colors.Background.RGBA != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("background = %v, want {255, 255, 255, 255}", cfg.Colors.Background.RGBA)
	}
	if cfg.Colors.FontColor.RGBA != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("font_color = %v, want {0, 0, 0, 255}", cfg.Colors.FontColor.RGBA)
	}
}

func TestLoadConfigWithBlocksAndSeries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{
		"screen": {"width": 320, "height": 480},
		"dot_size": 4,
		"thresholds": {"green": 0, "yellow": 50, "red": 80},
		"colors": {
			"green":       "#117311",
			"yellow":      "#ffdd00",
			"red":         "#ab0202",
			"green_fill":  "#90EE90",
			"yellow_fill": "#faf0cd",
			"red_fill":    "#FFB6C1",
			"background":  "#FFFFFF",
			"font_color":  "#000000"
		},
		"graphs": {
			"network": {
				"x": 10, "y": 376, "width": 140, "height": 96, "refresh_sec": 2,
				"max_bytes_per_sec": 125000000,
				"series": [
					{"name": "download", "color": "#117311", "fill_color": "#90EE90"},
					{"name": "upload", "color": "#0057D9", "fill_color": "#BFD7FF"}
				]
			},
			"disk": {
				"x": 170, "y": 376, "width": 140, "height": 96, "refresh_sec": 2,
				"series": [
					{"name": "read", "color": "#ab0202", "fill_color": "#FFB6C1"},
					{"name": "write", "color": "#6A3D9A", "fill_color": "#D7C7F2"}
				]
			}
		},
		"blocks": [
			{"metric": "network", "label": "NET", "x": 0, "y": 360, "width": 160, "height": 120},
			{"metric": "disk", "label": "DISK", "x": 160, "y": 360, "width": 160, "height": 120}
		]
	}`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.Graphs["network"].MaxBytesPerSec; got != 125000000 {
		t.Fatalf("network max_bytes_per_sec = %v, want 125000000", got)
	}
	if got := cfg.Graphs["network"].RefreshSec; got != 2 {
		t.Fatalf("network refresh_sec = %d, want 2", got)
	}
	if got := cfg.Graphs["disk"].RefreshSec; got != 2 {
		t.Fatalf("disk refresh_sec = %d, want 2", got)
	}
	if got := len(cfg.Graphs["network"].Series); got != 2 {
		t.Fatalf("network series count = %d, want 2", got)
	}
	if got := cfg.Graphs["network"].Series[1].Name; got != "upload" {
		t.Fatalf("network series[1].name = %q, want upload", got)
	}
	if got := len(cfg.Blocks); got != 2 {
		t.Fatalf("blocks count = %d, want 2", got)
	}
	if cfg.Blocks[0].Metric != "network" || cfg.Blocks[1].Metric != "disk" {
		t.Fatalf("blocks = %+v, want network/disk", cfg.Blocks)
	}
}
