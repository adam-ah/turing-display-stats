package chart

import (
	"image"
	"image/color"
	"time"
)

// SeriesChart renders multiple values on the same graph area.
// It is used for metrics such as network up/down and disk read/write activity.
type SeriesChart struct {
	cfg        GraphConfig
	dotSize    int
	bg         color.RGBA
	capacity   int
	series     []SeriesConfig
	history    [][]float64
	scratch    *image.RGBA
	filled     bool
	linesDrawn bool
	noLines    bool
}

func NewSeriesChart(cfg GraphConfig, dotSize int, bg color.RGBA, noLines bool) *SeriesChart {
	capacity := cfg.Width / dotSize
	if capacity <= 0 {
		capacity = 1
	}

	series := cfg.Series
	if len(series) == 0 {
		series = []SeriesConfig{
			{
				Name:      "value",
				Color:     RGBA{color.RGBA{0, 128, 0, 255}},
				FillColor: RGBA{color.RGBA{192, 224, 192, 255}},
			},
		}
	}

	return &SeriesChart{
		cfg:      cfg,
		dotSize:  dotSize,
		bg:       bg,
		capacity: capacity,
		series:   series,
		history:  make([][]float64, 0, capacity),
		noLines:  noLines,
	}
}

func newSeriesChart(cfg GraphConfig, dotSize int, bg color.RGBA, noLines bool) *SeriesChart {
	return NewSeriesChart(cfg, dotSize, bg, noLines)
}

func (c *SeriesChart) Capacity() int {
	return c.capacity
}

func (c *SeriesChart) Update(values []float64, now time.Time) []*DirtyRegion {
	return c.updateRepeated(values, 1)
}

func (c *SeriesChart) UpdateRepeated(values []float64, repeats int) []*DirtyRegion {
	return c.updateRepeated(values, repeats)
}

func (c *SeriesChart) UpdateSamples(samples [][]float64) []*DirtyRegion {
	if len(samples) == 0 {
		return nil
	}
	normalized := make([][]float64, 0, len(samples))
	for _, values := range samples {
		normalized = append(normalized, c.normalizeSample(values))
	}
	c.appendSamples(normalized)

	drawLines := !c.noLines && !c.linesDrawn
	dirty := []*DirtyRegion{c.renderFullBlock(drawLines)}
	if !c.linesDrawn {
		c.linesDrawn = true
	}
	return dirty
}

func (c *SeriesChart) updateRepeated(values []float64, repeats int) []*DirtyRegion {
	if repeats <= 0 {
		return nil
	}

	sample := c.normalizeSample(values)
	samples := make([][]float64, 0, repeats)
	for i := 0; i < repeats; i++ {
		samples = append(samples, append([]float64(nil), sample...))
	}
	c.appendSamples(samples)

	drawLines := !c.noLines && !c.linesDrawn
	dirty := []*DirtyRegion{c.renderFullBlock(drawLines)}
	if !c.linesDrawn {
		c.linesDrawn = true
	}
	return dirty
}

func (c *SeriesChart) appendSamples(samples [][]float64) {
	if len(samples) == 0 {
		return
	}
	if c.filled {
		if len(samples) >= c.capacity {
			c.history = c.history[:0]
			for _, sample := range samples[len(samples)-c.capacity:] {
				c.history = append(c.history, append([]float64(nil), sample...))
			}
		} else {
			copy(c.history, c.history[len(samples):])
			start := c.capacity - len(samples)
			for i, sample := range samples {
				c.history[start+i] = append([]float64(nil), sample...)
			}
		}
	} else {
		for _, sample := range samples {
			c.history = append(c.history, append([]float64(nil), sample...))
		}
		if len(c.history) >= c.capacity {
			if len(c.history) > c.capacity {
				c.history = c.history[len(c.history)-c.capacity:]
			}
			c.filled = true
		}
	}
}

func (c *SeriesChart) normalizeSample(values []float64) []float64 {
	sample := make([]float64, len(c.series))
	for i := range sample {
		if i < len(values) {
			v := values[i]
			if v < 0 {
				v = 0
			}
			if v > 100 {
				v = 100
			}
			sample[i] = v
		}
	}
	return sample
}

func (c *SeriesChart) renderFullBlock(drawLines bool) *DirtyRegion {
	bg := c.bg
	lineColor := color.RGBA{128, 128, 128, 255}

	var blockTop, blockHeight int
	if c.noLines {
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

	img := c.scratchImage(c.cfg.Width, blockHeight)
	for y := 0; y < blockHeight; y++ {
		for x := 0; x < c.cfg.Width; x++ {
			img.Set(x, y, bg)
		}
	}

	if drawLines && !c.noLines {
		for x := 0; x < c.cfg.Width; x++ {
			img.Set(x, 0, lineColor)
			img.Set(x, blockHeight-1, lineColor)
		}
	}

	for col := 0; col < c.capacity; col++ {
		historyIndex := len(c.history) - c.capacity + col
		if historyIndex < 0 || historyIndex >= len(c.history) {
			continue
		}
		sample := c.history[historyIndex]
		for seriesIdx := 0; seriesIdx < len(c.series) && seriesIdx < len(sample); seriesIdx++ {
			value := sample[seriesIdx]
			clr := c.series[seriesIdx].Color.RGBA
			if clr == bg {
				continue
			}
			fillClr := c.series[seriesIdx].FillColor.RGBA
			dotX := col * c.dotSize
			dotY := c.valueToY(value) - blockTop

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
		}
	}

	for col := 0; col < c.capacity; col++ {
		historyIndex := len(c.history) - c.capacity + col
		if historyIndex < 0 || historyIndex >= len(c.history) {
			continue
		}
		sample := c.history[historyIndex]
		for seriesIdx := 0; seriesIdx < len(c.series) && seriesIdx < len(sample); seriesIdx++ {
			value := sample[seriesIdx]
			clr := c.series[seriesIdx].Color.RGBA
			if clr == bg {
				continue
			}
			dotX := col * c.dotSize
			dotY := c.valueToY(value) - blockTop
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
	}

	return &DirtyRegion{
		X:     c.cfg.X,
		Y:     c.cfg.Y + blockTop,
		Image: img,
	}
}

func (c *SeriesChart) scratchImage(width, height int) *image.RGBA {
	if c.scratch == nil || c.scratch.Bounds().Dx() != width || c.scratch.Bounds().Dy() != height {
		c.scratch = image.NewRGBA(image.Rect(0, 0, width, height))
	}
	return c.scratch
}

func (c *SeriesChart) valueToY(value float64) int {
	if c.noLines {
		maxDotY := c.cfg.Height - c.dotSize
		if maxDotY < 0 {
			maxDotY = 0
		}
		return int((1.0 - value/100.0) * float64(maxDotY))
	}
	maxDotY := c.cfg.Height - c.dotSize - 1
	if maxDotY < 1 {
		maxDotY = 1
	}
	return 1 + int((1.0-value/100.0)*float64(maxDotY-1))
}
