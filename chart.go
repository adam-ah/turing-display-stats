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
// DirtyRegion — a rectangular patch sent to the display.
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
	prevValues []float64
	linesDrawn bool
	noLines    bool // when true, never draw bounding lines; use full height for dots
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

func newChart(cfg GraphConfig, dotSize int, thresholds ThresholdConfig, noLines bool) *Chart {
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
		noLines:    noLines,
	}
}

// ---------------------------------------------------------------------------
// Update — append a value, diff, batch into blocks, return dirty regions.
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

	// DEBUG: send entire graph as one block every time.
	drawLines := !c.noLines && !c.linesDrawn
	dirty := []*DirtyRegion{c.renderFullBlock(currColors, currValues, drawLines)}

	if !c.linesDrawn {
		c.linesDrawn = true
	}

	return dirty
}

// ---------------------------------------------------------------------------
// markChanged — returns a bool slice: true for columns that changed.
// ---------------------------------------------------------------------------

func (c *Chart) markChanged(prevColors, currColors []color.RGBA, prevValues, currValues []float64) []bool {
	changed := make([]bool, c.capacity)
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
			changed[i] = true
			continue
		}
		if !wasDot && isDot {
			changed[i] = true
			continue
		}
		if prevColors[i] != currColors[i] || prevValues[i] != currValues[i] {
			changed[i] = true
		}
	}
	return changed
}

// ---------------------------------------------------------------------------
// batchBlocks — group contiguous changed columns into block bitmaps.
//
// Each block spans the full graph height. Only columns that changed are
// included; white columns are skipped entirely (no block created).
//
// Contiguous changed columns are merged into one block to minimise serial
// transfers. Non-contiguous changed columns become separate blocks.
// ---------------------------------------------------------------------------

func (c *Chart) batchBlocks(changed []bool, colors []color.RGBA, values []float64) []*DirtyRegion {
	var regions []*DirtyRegion
	white := color.RGBA{255, 255, 255, 255}

	// Walk columns, grouping contiguous changed ones.
	runStart := -1
	for i := 0; i < c.capacity; i++ {
		if !changed[i] {
			continue
		}
		if runStart == -1 {
			runStart = i
		}

		// Check if next column is also changed — if not, emit block.
		if i+1 >= c.capacity || !changed[i+1] {
			// Build bitmap for columns [runStart .. i].
			blockWidth := (i - runStart + 1) * c.dotSize
			img := image.NewRGBA(image.Rect(0, 0, blockWidth, c.cfg.Height))
			// Fill with white background.
			for y := 0; y < c.cfg.Height; y++ {
				for x := 0; x < blockWidth; x++ {
					img.Set(x, y, white)
				}
			}
			// Draw dots.
			for col := runStart; col <= i; col++ {
				clr := colors[col]
				if clr == white {
					continue // white-out: already white in background
				}
				dotX := (col - runStart) * c.dotSize
				dotY := c.valueToY(values[col])
				for dy := 0; dy < c.dotSize; dy++ {
					for dx := 0; dx < c.dotSize; dx++ {
						py := dotY + dy
						if py >= 0 && py < c.cfg.Height {
							img.Set(dotX+dx, py, clr)
						}
					}
				}
			}
			regions = append(regions, &DirtyRegion{
				X:     c.cfg.X + runStart*c.dotSize,
				Y:     c.cfg.Y,
				Image: img,
			})
			runStart = -1
		}
	}

	return regions
}

// ---------------------------------------------------------------------------
// renderFrame — colour + value of every dot column for the current state.
// Does not mutate history. Returns colours and values in parallel slices.
// ---------------------------------------------------------------------------

func (c *Chart) renderFrame(value float64) ([]color.RGBA, []float64) {
	colors := make([]color.RGBA, c.capacity)
	values := make([]float64, c.capacity)
	for i := range colors {
		colors[i] = color.RGBA{255, 255, 255, 255}
		values[i] = 0
	}

	var dots []float64
	if !c.filled {
		dots = append(append([]float64(nil), c.history...), value)
	} else {
		dots = make([]float64, c.capacity)
		copy(dots, c.history[1:])
		dots[c.capacity-1] = value
	}

	// Always render from the right edge.
	n := len(dots)
	for i := 0; i < n && i < c.capacity; i++ {
		col := c.capacity - n + i
		colors[col] = c.dotColor(dots[i])
		values[col] = dots[i]
	}
	return colors, values
}

// ---------------------------------------------------------------------------
// renderFullBlock — render entire graph area as one block (DEBUG).
// ---------------------------------------------------------------------------

func (c *Chart) renderFullBlock(colors []color.RGBA, values []float64, drawLines bool) *DirtyRegion {
	white := color.RGBA{255, 255, 255, 255}
	lineColor := color.RGBA{128, 128, 128, 255}

	var blockTop, blockHeight int
	if c.noLines {
		// No bounding lines — use full height for dots.
		blockTop = 0
		blockHeight = c.cfg.Height
	} else {
		blockTop = 0
		blockBot := c.cfg.Height - 1
		if !drawLines {
			blockTop = 1
			blockBot = c.cfg.Height - 2
		}
		blockHeight = blockBot - blockTop + 1
	}

	img := image.NewRGBA(image.Rect(0, 0, c.cfg.Width, blockHeight))

	// Fill white.
	for y := 0; y < blockHeight; y++ {
		for x := 0; x < c.cfg.Width; x++ {
			img.Set(x, y, white)
		}
	}

	// Bounding lines (only on first update, and only when not noLines).
	if drawLines && !c.noLines {
		for x := 0; x < c.cfg.Width; x++ {
			img.Set(x, 0, lineColor)            // top line at row 0
			img.Set(x, blockHeight-1, lineColor) // bottom line at row height-1
		}
	}

	// Dots.
	yOffset := blockTop
	for col := 0; col < c.capacity; col++ {
		if colors[col] == white {
			continue
		}
		clr := colors[col]
		dotX := col * c.dotSize
		dotY := c.valueToY(values[col]) - yOffset
		for dy := 0; dy < c.dotSize; dy++ {
			for dx := 0; dx < c.dotSize; dx++ {
				py := dotY + dy
				px := dotX + dx
				if py >= 0 && py < blockHeight && px >= 0 && px < c.cfg.Width {
					img.Set(px, py, clr)
				}
			}
		}
	}

	return &DirtyRegion{
		X:     c.cfg.X,
		Y:     c.cfg.Y + blockTop,
		Image: img,
	}
}

// drawBoundingLines — top (100%) and bottom (0%) lines, 1px high each.
// ---------------------------------------------------------------------------

func (c *Chart) drawBoundingLines() []*DirtyRegion {
	lineColor := color.RGBA{128, 128, 128, 255}
	regions := make([]*DirtyRegion, 0, 2)

	topImg := image.NewRGBA(image.Rect(0, 0, c.cfg.Width, 1))
	for x := 0; x < c.cfg.Width; x++ {
		topImg.Set(x, 0, lineColor)
	}
	regions = append(regions, &DirtyRegion{
		X:     c.cfg.X,
		Y:     c.cfg.Y,
		Image: topImg,
	})

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
// valueToY — maps percentage to Y pixel position.
// With lines: 0% → height-2, 100% → 1 (bounding lines at 0 and height-1).
// No lines: 0% → height-1, 100% → 0 (full height available).
// ---------------------------------------------------------------------------

func (c *Chart) valueToY(value float64) int {
	if c.noLines {
		// Dot top-left must fit in [0, height-dotSize] so the full dot is visible.
		maxDotY := c.cfg.Height - c.dotSize
		if maxDotY < 0 {
			maxDotY = 0
		}
		// 100% → row 0, 0% → row maxDotY
		return int((1.0 - value/100.0) * float64(maxDotY))
	}
	// With lines: dot must fit between row 1 and row height-2.
	maxDotY := c.cfg.Height - c.dotSize - 1
	if maxDotY < 1 {
		maxDotY = 1
	}
	// 100% → row 1, 0% → row maxDotY
	return 1 + int((1.0 - value/100.0) * float64(maxDotY-1))
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
