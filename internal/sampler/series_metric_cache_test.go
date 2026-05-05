package sampler

import "testing"

func TestSeriesMetricCacheBuffersFineGrainedSamples(t *testing.T) {
	cache := NewSeriesMetricCache(2)

	samples := cache.Update(0, []float64{10, 20})
	if len(samples) != 1 {
		t.Fatalf("initial samples = %d, want 1", len(samples))
	}
	if samples[0][0] != 10 || samples[0][1] != 20 {
		t.Fatalf("initial sample = %#v, want first sample", samples[0])
	}

	samples = cache.Update(1, []float64{30, 40})
	if samples != nil {
		t.Fatalf("turn 1 samples = %#v, want nil", samples)
	}

	samples = cache.Update(2, []float64{50, 60})
	if len(samples) != 2 {
		t.Fatalf("turn 2 samples = %d, want 2", len(samples))
	}
	if samples[0][0] != 30 || samples[0][1] != 40 {
		t.Fatalf("first buffered sample = %#v, want turn 1 sample", samples[0])
	}
	if samples[1][0] != 50 || samples[1][1] != 60 {
		t.Fatalf("second buffered sample = %#v, want turn 2 sample", samples[1])
	}
}

func TestSeriesMetricCacheCopiesSamples(t *testing.T) {
	cache := NewSeriesMetricCache(2)
	sample := []float64{10, 20}

	_ = cache.Update(0, sample)
	sample[0] = 99
	sample[1] = 88
	_ = cache.Update(1, sample)
	sample[0] = 77
	sample[1] = 66
	samples := cache.Update(2, sample)

	if samples[0][0] != 99 || samples[0][1] != 88 {
		t.Fatalf("buffered sample changed with caller slice: %#v", samples[0])
	}
}
