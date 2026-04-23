// Chart rendering for Turing Smart Screen.
// Pure logic — no Windows APIs — so it can be unit-tested anywhere.
package main

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
)

// ---------------------------------------------------------------------------
// JSON config types
// ---------------------------------------------------------------------------

type ChartConfig struct {
	Screen     ScreenConfig       `json:"screen"`
	DotSize    int                `json:"dot_size"`
	Thresholds ThresholdConfig    `json:"thresholds"`
	Graphs     map[string]GraphConfig `json:"graphs"`
}

type ScreenConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type GraphConfig struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type ThresholdConfig struct {
	Green  float64 `json:"green"`
	Yellow float64 `json:"yellow"`
	Red    float64 `json:"red"`
}

// ---------------------------------------------------------------------------
// DirtyRegion — a small rectangular patch sent to the display.
// ---------------------------------------------------------------------------

type DirtyRegion struct {
	X     int
	Y     int
	Image *image.RGBA
}

// ---------------------------------------------------------------------------
// Chart — state for one graph overlay.
// ---------------------------------------------------------------------------

type Chart struct {
	cfg        GraphConfig
	dotSize    int
	capacity   int
	thresholds ThresholdConfig
	history    []float64
	filled     bool
	prevColors []color.RGBA
	prevValues []float64  // previous values for Y-position of each column
	linesDrawn bool       // bounding lines drawn once on first update
}

// ---------------------------------------------------------------------------
// Config loading
// ---------------------------------------------------------------------------

func loadConfig(path string) (*ChartConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ChartConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func newChart(cfg GraphConfig, dotSize int, thresholds ThresholdConfig) *Chart {
	capacity := cfg.Width / dotSize
	if capacity <= 0 {
		capacity = 1
	}
	prevColors := make([]color.RGBA, capacity)
	for i := range prevColors {
		prevColors[i] = color.RGBA{255, 255, 255, 255}
	}
	return &Chart{
		cfg:        cfg,
		dotSize:    dotSize,
		capacity:   capacity,
		thresholds: thresholds,
		history:    make([]float64, 0, capacity),
		prevColors: prevColors,
		prevValues: make([]float64, capacity),
	}
}

// ---------------------------------------------------------------------------
// Update — append a value, diff against previous frame, return dirty regions.
// value is a percentage 0..100.
// ---------------------------------------------------------------------------

func (c *Chart) update(value float64) []*DirtyRegion {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}

	currColors, currValues := c.renderFrame(value)
	dirty := c.diff(c.prevColors, currColors, c.prevValues, currValues)

	if !c.filled {
		c.history = append(c.history, value)
		if len(c.history) >= c.capacity {
			c.filled = true
		}
	} else {
		copy(c.history, c.history[1:])
		c.history[c.capacity-1] = value
	}

	copy(c.prevColors, currColors)
	copy(c.prevValues, currValues)

	// Draw bounding lines once on first update.
	if !c.linesDrawn {
		c.linesDrawn = true
		dirty = append(dirty, c.drawBoundingLines()...)
	}

	return dirty
}

// ---------------------------------------------------------------------------
// renderFrame — colour + value of every dot column for the current state.
// Does not mutate history. Returns colours and values in parallel slices.
// ---------------------------------------------------------------------------

func (c *Chart) renderFrame(value float64) ([]color.RGBA, []float64) {
	colors := make([]color.RGBA, c.capacity)
	values := make([]float64, c.capacity)
	for i := range colors {
		colors[i] = color.RGBA{255, 255, 255, 255} // white background
		values[i] = 0
	}

	// Build the full history including the new value.
	var dots []float64
	if !c.filled {
		dots = append(append([]float64(nil), c.history...), value)
	} else {
		dots = make([]float64, c.capacity)
		copy(dots, c.history[1:])
		dots[c.capacity-1] = value
	}

	// Always render from the right edge. The "camera" starts at the right
	// and slides left as more data accumulates. This means the transition
	// from fill→shift is seamless — dots always push left.
	n := len(dots)
	for i := 0; i < n && i < c.capacity; i++ {
		col := c.capacity - n + i // right-aligned
		colors[col] = c.dotColor(dots[i])
		values[col] = dots[i]
	}
	return colors, values
}

// ---------------------------------------------------------------------------
// diff — compare prev/curr colour+value arrays, return DirtyRegion for changes.
//
// For each column:
//   prev white → curr colored : draw colored dot at new Y
//   prev colored → curr white : white-out at OLD Y (clear ghost dot)
//   prev colored → curr colored, different : white-out old Y, draw new color
//   prev colored → curr colored, same : skip
//   prev white → curr white : skip
// ---------------------------------------------------------------------------

func (c *Chart) diff(prevColors, currColors []color.RGBA, prevValues, currValues []float64) []*DirtyRegion {
	var regions []*DirtyRegion
	white := color.RGBA{255, 255, 255, 255}

	n := len(currColors)
	if len(prevColors) < n {
		n = len(prevColors)
	}
	if n > c.capacity {
		n = c.capacity
	}

	for i := 0; i < n; i++ {
		wasDot := prevColors[i] != white
		isDot := currColors[i] != white

		if !wasDot && !isDot {
			continue
		}

		if wasDot && !isDot {
			regions = append(regions, c.dotRegion(i, white, prevValues[i]))
			continue
		}

		if !wasDot && isDot {
			regions = append(regions, c.dotRegion(i, currColors[i], currValues[i]))
			continue
		}

		if prevColors[i] != currColors[i] || prevValues[i] != currValues[i] {
			regions = append(regions, c.dotRegion(i, white, prevValues[i]))
			regions = append(regions, c.dotRegion(i, currColors[i], currValues[i]))
		}
	}

	return regions
}

// ---------------------------------------------------------------------------
// drawBoundingLines — top (100%) and bottom (0%) lines, 1px high each.
// Returns dirty regions to draw them on the white background.
// ---------------------------------------------------------------------------

func (c *Chart) drawBoundingLines() []*DirtyRegion {
	lineColor := color.RGBA{128, 128, 128, 255} // grey
	regions := make([]*DirtyRegion, 0, 2)

	// Top line at y=0 (100% boundary) — drawn as a 1px high strip.
	topImg := image.NewRGBA(image.Rect(0, 0, c.cfg.Width, 1))
	for x := 0; x < c.cfg.Width; x++ {
		topImg.Set(x, 0, lineColor)
	}
	regions = append(regions, &DirtyRegion{
		X:     c.cfg.X,
		Y:     c.cfg.Y,
		Image: topImg,
	})

	// Bottom line at y=height-1 (0% boundary).
	bottomImg := image.NewRGBA(image.Rect(0, 0, c.cfg.Width, 1))
	for x := 0; x < c.cfg.Width; x++ {
		bottomImg.Set(x, 0, lineColor)
	}
	regions = append(regions, &DirtyRegion{
		X:     c.cfg.X,
		Y:     c.cfg.Y + c.cfg.Height - 1,
		Image: bottomImg,
	})

	return regions
}

// ---------------------------------------------------------------------------
// dotRegion — build a DirtyRegion for one dot at the correct screen position.
// ---------------------------------------------------------------------------

func (c *Chart) dotRegion(col int, clr color.RGBA, value float64) *DirtyRegion {
	return &DirtyRegion{
		X:     c.cfg.X + col*c.dotSize,
		Y:     c.cfg.Y + c.valueToY(value),
		Image: c.makeDotImage(clr),
	}
}

// ---------------------------------------------------------------------------
// valueToY — map percentage to pixel offset within the graph area.
//
// The graph area is [0 .. height-1]. Top and bottom rows are reserved for
// bounding lines, so dots occupy [1 .. height-2].
//   0%   → height-2  (just above bottom line)
//  100%  → 1         (just below top line)
// ---------------------------------------------------------------------------

func (c *Chart) valueToY(value float64) int {
	// Usable range: 1 .. height-2  (height-2 rows)
	usable := c.cfg.Height - 2
	if usable <= 0 {
		usable = 1
	}
	// Normalise to 0..1, map to [0 .. usable-1], then offset by 1.
	pixel := int((value / 100.0) * float64(usable-1))
	return 1 + (usable - 1 - pixel)
}

// ---------------------------------------------------------------------------
// makeDotImage — dotSize×dotSize solid-colour image.
// ---------------------------------------------------------------------------

func (c *Chart) makeDotImage(clr color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.dotSize, c.dotSize))
	for y := 0; y < c.dotSize; y++ {
		for x := 0; x < c.dotSize; x++ {
			img.Set(x, y, clr)
		}
	}
	return img
}

// ---------------------------------------------------------------------------
// dotColor — pick colour from thresholds.
// ---------------------------------------------------------------------------

func (c *Chart) dotColor(value float64) color.RGBA {
	switch {
	case value >= c.thresholds.Red:
		return color.RGBA{255, 0, 0, 255}
	case value >= c.thresholds.Yellow:
		return color.RGBA{255, 255, 0, 255}
	default:
		return color.RGBA{0, 255, 0, 255}
	}
}
