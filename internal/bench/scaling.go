package bench

import (
	"math"
	"time"
)

// Slope fits log(time) against log(n) over the given sizes: ~1 is linear,
// ~2 quadratic. Each size takes the fastest of reps runs to shed noise.
func Slope(sizes []int, reps int, run func(n int)) (slope float64, times []time.Duration) {
	xs := make([]float64, len(sizes))
	ys := make([]float64, len(sizes))
	for i, n := range sizes {
		best := time.Duration(math.MaxInt64)
		for range reps {
			t0 := time.Now()
			run(n)
			best = min(best, time.Since(t0))
		}
		times = append(times, best)
		xs[i] = math.Log(float64(n))
		ys[i] = math.Log(float64(max(best, time.Microsecond)))
	}
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= float64(len(xs))
	my /= float64(len(ys))
	var num, den float64
	for i := range xs {
		num += (xs[i] - mx) * (ys[i] - my)
		den += (xs[i] - mx) * (xs[i] - mx)
	}
	return num / den, times
}
