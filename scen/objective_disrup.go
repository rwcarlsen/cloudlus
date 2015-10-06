package scen

import (
	"fmt"
	"math"
)

type Disruption struct {
	// Time is the time step on which the facility shutdown disruption occurs.
	Time int
	// KillProto is the prototype for which all facilities will be shut down
	// by the given time.
	KillProto string
	// Prob holds the probability that the disruption will happen at a
	// particular time.  This is ignored in single-disrup mode.
	Prob float64
}

func disrupSingleMode(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-single"].(map[string]interface{})
	disrup := Disruption{}

	if t, ok := idisrup["Time"]; ok {
		disrup.Time = int(t.(float64))
	}

	if proto, ok := idisrup["KillProto"]; ok {
		disrup.KillProto = proto.(string)
	}

	if prob, ok := idisrup["Prob"]; ok {
		disrup.Prob = prob.(float64)
	}

	// set separations plant to die disruption time.
	cloneval := *s
	clone := &cloneval
	if disrup.Time >= 0 {
		for i, b := range clone.Builds {
			if b.Proto == disrup.KillProto {
				clone.Builds[i].Life = disrup.Time - b.Time
			}
		}
	}

	return obj(clone)
}

func disrupMode(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-multi"].([]interface{})
	disrup := make([]Disruption, len(idisrup))
	for i, td := range idisrup {
		m := td.(map[string]interface{})
		d := Disruption{}

		if t, ok := m["Time"]; ok {
			d.Time = int(t.(float64))
		}

		if proto, ok := m["KillProto"]; ok {
			d.KillProto = proto.(string)
		}

		if prob, ok := m["Prob"]; ok {
			d.Prob = prob.(float64)
		}

		disrup[i] = d
	}

	times := []int{1000, -1}
	probs := []float64{0.3, 0.7}

	type result struct {
		val float64
		err error
	}

	// make channel buffered so if we bail early on an error, we don't leak
	// goroutines waiting to send.
	ch := make(chan result, len(disrup))
	defer close(ch)

	// fire off concurrent sub-simulation objective evaluations
	for _, d := range disrup {
		// set separations plant to die disruption time.
		cloneval := *s
		clone := &cloneval
		if d.Time >= 0 {
			for i, b := range clone.Builds {
				if b.Proto == d.KillProto {
					clone.Builds[i].Life = d.Time - b.Time
				}
			}
		}

		go func() {
			val, err := obj(clone)
			ch <- result{val, err}
		}()
	}

	// collect all results
	subobjs := make([]float64, len(times))
	for i := range times {
		r := <-ch
		if r.err != nil {
			return math.Inf(1), fmt.Errorf("remote sub-simulation execution failed: %v", r.err)
		}
		subobjs[i] = r.val
	}

	// compute aggregate objective
	objval := 0.0
	for i := range subobjs {
		objval += probs[i] * subobjs[i]
	}
	return objval, nil
}
