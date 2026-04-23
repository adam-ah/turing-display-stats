// Chart rendering for Turing Smart Screen.
// Pure logic — no Windows APIs — so it can be unit-tested anywhere.
package chart

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"time"
)

// ---------------------------------------------------------------------------
// JSON config types
// ---------------------------------------------------------------------------

// RGBA is a JSON-friendly wrapper around color.RGBA that accepts "#RRGGBB" strings.
type RGBA struct {
	color.RGBA
}

func (c *RGBA) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	var r, g, b uint64
	if _, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return fmt.Errorf("invalid color %q: %v", s, err)
	}
	c.R, c.G, c.B, c.A = uint8(r), uint8(g), uint8(b), 255
	return nil
}

type ChartConfig struct {
	Screen     ScreenConfig           `json:"screen"`
	DotSize    int                    `json:"dot_size"`
	FontSize   int                    `json:"font_size"`
	Thresholds ThresholdConfig        `json:"thresholds"`
	Colors     ColorConfig            `json:"colors"`
	Graphs     map[string]GraphConfig `json:"graphs"`
}

type ScreenConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type GraphConfig struct {
	X          int `json:"x"`
	Y          int `json:"y"`
	Width      int `json:"width"`
	Height     int `json:"height"`
	RefreshSec int `json:"refresh_sec"` // whole seconds between fresh samples; 0 = use default 1s
}

func (g *GraphConfig) UnmarshalJSON(data []byte) error {
	var raw struct {
		X          int  `json:"x"`
		Y          int  `json:"y"`
		Width      int  `json:"width"`
		Height     int  `json:"height"`
		RefreshSec *int `json:"refresh_sec"`
		RefreshMs  *int `json:"refresh_ms"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	g.X = raw.X
	g.Y = raw.Y
	g.Width = raw.Width
	g.Height = raw.Height

	switch {
	case raw.RefreshSec != nil:
		g.RefreshSec = *raw.RefreshSec
	case raw.RefreshMs != nil:
		g.RefreshSec = (*raw.RefreshMs + 999) / 1000
	default:
		g.RefreshSec = 0
	}

	return nil
}

type ThresholdConfig struct {
	Green  float64 `json:"green"`
	Yellow float64 `json:"yellow"`
	Red    float64 `json:"red"`
}

type ColorConfig struct {
	Green      RGBA `json:"green"`       // dot colour for green tier
	Yellow     RGBA `json:"yellow"`      // dot colour for yellow tier
	Red        RGBA `json:"red"`         // dot colour for red tier
	GreenFill  RGBA `json:"green_fill"`  // pastel fill below green dots
	YellowFill RGBA `json:"yellow_fill"` // pastel fill below yellow dots
	RedFill    RGBA `json:"red_fill"`    // pastel fill below red dots
	Background RGBA `json:"background"`  // chart background colour
	FontColor  RGBA `json:"font_color"`  // label text colour
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
	bg         color.RGBA
	capacity   int
	thresholds ThresholdConfig
	colors     ColorConfig
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

func LoadConfig(path string) (*ChartConfig, error) {
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

func loadConfig(path string) (*ChartConfig, error) {
	return LoadConfig(path)
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func NewChart(cfg GraphConfig, dotSize int, thresholds ThresholdConfig, cc ColorConfig, bg color.RGBA, refreshSec int, noLines bool) *Chart {
	_ = refreshSec // cadence is enforced by the sampling cache, not the chart renderer.
	capacity := cfg.Width / dotSize
	if capacity <= 0 {
		capacity = 1
	}
	prevColors := make([]color.RGBA, capacity)
	for i := range prevColors {
		prevColors[i] = bg
	}
	return &Chart{
		cfg:        cfg,
		dotSize:    dotSize,
		bg:         bg,
		capacity:   capacity,
		thresholds: thresholds,
		colors:     cc,
		history:    make([]float64, 0, capacity),
		prevColors: prevColors,
		prevValues: make([]float64, capacity),
		noLines:    noLines,
	}
}

func newChart(cfg GraphConfig, dotSize int, thresholds ThresholdConfig, cc ColorConfig, bg color.RGBA, refreshSec int, noLines bool) *Chart {
	return NewChart(cfg, dotSize, thresholds, cc, bg, refreshSec, noLines)
}

// ---------------------------------------------------------------------------
// Update — append one value, diff, batch into blocks, return dirty regions.
// value is a percentage 0..100.
// ---------------------------------------------------------------------------

func (c *Chart) Update(value float64, now time.Time) []*DirtyRegion {
	return c.updateRepeated(value, 1)
}

func (c *Chart) update(value float64, now time.Time) []*DirtyRegion {
	return c.Update(value, now)
}

func (c *Chart) UpdateRepeated(value float64, repeats int) []*DirtyRegion {
	return c.updateRepeated(value, repeats)
}

func (c *Chart) Capacity() int {
	return c.capacity
}

// updateRepeated appends the same value multiple times before rendering.
// This is used for slower charts so they advance by several identical dots at
// once instead of redrawing every second.
func (c *Chart) updateRepeated(value float64, repeats int) []*DirtyRegion {
	if repeats <= 0 {
		return nil
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}

	if c.filled {
		if repeats >= c.capacity {
			for i := range c.history {
				c.history[i] = value
			}
		} else {
			copy(c.history, c.history[repeats:])
			for i := c.capacity - repeats; i < c.capacity; i++ {
				c.history[i] = value
			}
		}
	} else {
		for i := 0; i < repeats; i++ {
			c.history = append(c.history, value)
		}
		if len(c.history) >= c.capacity {
			if len(c.history) > c.capacity {
				c.history = c.history[len(c.history)-c.capacity:]
			}
			c.filled = true
		}
	}

	currColors, currValues := c.renderFrame()

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
	bg := c.bg

	n := len(currColors)
	if len(prevColors) < n {
		n = len(prevColors)
	}
	if n > c.capacity {
		n = c.capacity
	}

	for i := 0; i < n; i++ {
		wasDot := prevColors[i] != bg
		isDot := currColors[i] != bg

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
	bg := c.bg

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
			// Fill with background.
			for y := 0; y < c.cfg.Height; y++ {
				for x := 0; x < blockWidth; x++ {
					img.Set(x, y, bg)
				}
			}
			// Draw dots + fill.
			for col := runStart; col <= i; col++ {
				clr := colors[col]
				if clr == bg {
					continue // white-out: already white in background
				}
				fillClr := c.dotFillColor(values[col])
				dotX := (col - runStart) * c.dotSize
				dotY := c.valueToY(values[col])

				// Fill the gap below the dot with pastel colour.
				for fy := dotY + c.dotSize; fy < c.cfg.Height; fy++ {
					for dx := 0; dx < c.dotSize; dx++ {
						img.Set(dotX+dx, fy, fillClr)
					}
				}

				// Draw the dot.
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

func (c *Chart) renderFrame() ([]color.RGBA, []float64) {
	colors := make([]color.RGBA, c.capacity)
	values := make([]float64, c.capacity)
	for i := range colors {
		colors[i] = c.bg
		values[i] = 0
	}

	dots := c.history

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
	bg := c.bg
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

	// Fill background.
	for y := 0; y < blockHeight; y++ {
		for x := 0; x < c.cfg.Width; x++ {
			img.Set(x, y, bg)
		}
	}

	// Bounding lines (only on first update, and only when not noLines).
	if drawLines && !c.noLines {
		for x := 0; x < c.cfg.Width; x++ {
			img.Set(x, 0, lineColor)             // top line at row 0
			img.Set(x, blockHeight-1, lineColor) // bottom line at row height-1
		}
	}

	// Dots + fill.
	yOffset := blockTop
	for col := 0; col < c.capacity; col++ {
		if colors[col] == bg {
			continue
		}
		clr := colors[col]
		fillClr := c.dotFillColor(values[col])
		dotX := col * c.dotSize
		dotY := c.valueToY(values[col]) - yOffset

		// Fill the gap below the dot with pastel colour.
		// Stop before bottom bounding line when lines are drawn.
		fillBot := blockHeight
		if drawLines && !c.noLines {
			fillBot = blockHeight - 1
		}
		for fy := dotY + c.dotSize; fy < fillBot; fy++ {
			for dx := 0; dx < c.dotSize; dx++ {
				px := dotX + dx
				if px >= 0 && px < c.cfg.Width {
					img.Set(px, fy, fillClr)
				}
			}
		}

		// Draw the dot.
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
	return 1 + int((1.0-value/100.0)*float64(maxDotY-1))
}

// ---------------------------------------------------------------------------
// dotColor — pick colour from thresholds.
// ---------------------------------------------------------------------------

func (c *Chart) dotColor(value float64) color.RGBA {
	switch {
	case value >= c.thresholds.Red:
		return c.colors.Red.RGBA
	case value >= c.thresholds.Yellow:
		return c.colors.Yellow.RGBA
	default:
		return c.colors.Green.RGBA
	}
}

// dotFillColor — pick the pastel fill colour from thresholds.
// ---------------------------------------------------------------------------

func (c *Chart) dotFillColor(value float64) color.RGBA {
	switch {
	case value >= c.thresholds.Red:
		return c.colors.RedFill.RGBA
	case value >= c.thresholds.Yellow:
		return c.colors.YellowFill.RGBA
	default:
		return c.colors.GreenFill.RGBA
	}
}
