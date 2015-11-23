package scen

import "sort"

type smoothFn func(x float64) float64

type sample struct {
	X float64
	Y float64
}

type sampleSet []sample

func (s sampleSet) Len() int           { return len(s) }
func (s sampleSet) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sampleSet) Less(i, j int) bool { return s[i].X < s[j].X }

func interpolate(samples []sample) smoothFn {
	ss := make([]sample, len(samples))
	copy(ss, samples)
	sort.Sort(sampleSet(ss))
	return func(x float64) (y float64) {
		for i := range ss[:len(ss)-1] {
			left := ss[i].X
			right := ss[i+1].X
			if x <= right {
				lefty := ss[i].Y
				righty := ss[i+1].Y
				return lefty + (x-left)/(right-left)*(righty-lefty)
			}
		}
		panic("interpolate function's x value out of bounds")
	}
}

func extractKnownBests(disrups []Disruption) []sample {
	samples := []sample{}
	for _, d := range disrups {
		if d.Sample {
			samples = append(samples, sample{float64(d.Time), d.KnownBest})
		}
	}
	return samples
}

func extractProbs(disrups []Disruption) []sample {
	samples := []sample{}
	for _, d := range disrups {
		samples = append(samples, sample{float64(d.Time), d.Prob})
	}
	return samples
}
