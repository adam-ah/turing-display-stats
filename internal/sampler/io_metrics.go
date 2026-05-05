package sampler

// NormalizeBytesPerSec converts a byte-per-second rate to a 0..100 percentage
// using the configured max. Values above the configured max clamp at 100.
func NormalizeBytesPerSec(bytesPerSec, maxBytesPerSec float64) float64 {
	if maxBytesPerSec <= 0 {
		return 0
	}
	pct := bytesPerSec * 100 / maxBytesPerSec
	return clampPct(pct)
}

// NormalizeDiskActiveTimePct clamps disk active-time values to 0..100.
func NormalizeDiskActiveTimePct(value float64) float64 {
	return clampPct(value)
}

func clampPct(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
