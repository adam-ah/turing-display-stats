package sampler

// SeriesMetricCache buffers one sample per turn and emits the buffered samples
// only on the configured display cadence.
type SeriesMetricCache struct {
	RefreshSec   int
	lastEmitTurn int
	pending      [][]float64
	valid        bool
}

func NewSeriesMetricCache(refreshSec int) SeriesMetricCache {
	if refreshSec <= 0 {
		refreshSec = 1
	}
	return SeriesMetricCache{RefreshSec: refreshSec}
}

func (c *SeriesMetricCache) Update(turn int, sample []float64) [][]float64 {
	c.pending = append(c.pending, append([]float64(nil), sample...))
	if !c.valid || turn-c.lastEmitTurn >= c.RefreshSec {
		out := c.pending
		c.pending = nil
		c.lastEmitTurn = turn
		c.valid = true
		return out
	}
	return nil
}
