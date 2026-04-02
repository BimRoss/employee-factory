package slackbot

import (
	"fmt"
	"math"
	"math/rand"
)

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// sampleHandoffProbability samples a per-reply probability within configured bounds,
// centered around the base probability when possible.
func sampleHandoffProbability(base, minP, maxP float64) float64 {
	base = clamp01(base)
	if base <= 0 {
		return 0
	}
	minP = clamp01(minP)
	maxP = clamp01(maxP)
	if maxP < minP {
		minP, maxP = maxP, minP
	}
	if maxP == minP {
		return minP
	}

	// Keep randomization bounded while preserving intent of base probability.
	halfSpan := (maxP - minP) / 2
	low := math.Max(minP, base-halfSpan)
	high := math.Min(maxP, base+halfSpan)
	if high < low {
		low, high = minP, maxP
	}
	if high == low {
		return low
	}
	return low + rand.Float64()*(high-low)
}

func shouldHandoff(base, minP, maxP float64) (bool, float64) {
	p := sampleHandoffProbability(base, minP, maxP)
	if p <= 0 {
		return false, p
	}
	return rand.Float64() < p, p
}

func recencyWeight(indexFromLatest int, decay float64, window int) float64 {
	if indexFromLatest < 0 {
		indexFromLatest = 0
	}
	if window < 1 {
		window = 1
	}
	if indexFromLatest >= window {
		indexFromLatest = window - 1
	}
	if decay <= 0 {
		decay = 0.5
	}
	if decay > 1 {
		decay = 1
	}
	return math.Pow(decay, float64(indexFromLatest))
}

func formatWeightedContext(role, text string, indexFromLatest int, decay float64, window int) string {
	w := recencyWeight(indexFromLatest, decay, window)
	return fmt.Sprintf("[w=%.2f][%s] %s", w, role, text)
}
