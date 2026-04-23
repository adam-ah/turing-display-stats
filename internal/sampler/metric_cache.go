package sampler

// metricCache controls when a metric should emit a chart update.
// The cadence is turn-based: each completed update cycle is one turn.
// Slower metrics only emit on their turn interval and advance by that many
// identical dots so the graph stays aligned.
type MetricCache struct {
	RefreshSec   int
	lastEmitTurn int
	value        float64
	valid        bool
}

func NewMetricCache(refreshSec int) MetricCache {
	if refreshSec <= 0 {
		refreshSec = 1
	}
	return MetricCache{RefreshSec: refreshSec}
}

func (c *MetricCache) Update(turn int, sample float64) (float64, int) {
	if !c.valid || turn-c.lastEmitTurn >= c.RefreshSec {
		c.value = sample
		c.lastEmitTurn = turn
		c.valid = true
		return c.value, c.RefreshSec
	}
	return c.value, 0
}
