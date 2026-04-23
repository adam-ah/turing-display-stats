package displayapp

// metricCache controls when a metric should emit a chart update.
// The cadence is turn-based: each completed update cycle is one turn.
// Slower metrics only emit on their turn interval and advance by that many
// identical dots so the graph stays aligned.
type metricCache struct {
	refreshSec   int
	lastEmitTurn int
	value        float64
	valid        bool
}

func newMetricCache(refreshSec int) metricCache {
	if refreshSec <= 0 {
		refreshSec = 1
	}
	return metricCache{refreshSec: refreshSec}
}

func (c *metricCache) update(turn int, sample float64) (float64, int) {
	if !c.valid || turn-c.lastEmitTurn >= c.refreshSec {
		c.value = sample
		c.lastEmitTurn = turn
		c.valid = true
		return c.value, c.refreshSec
	}
	return c.value, 0
}
