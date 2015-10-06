package scen

import (
	"fmt"
	"math"
	"sync"
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
	clone := s.Clone()
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

	// fire off concurrent sub-simulation objective evaluations
	var wg sync.WaitGroup
	wg.Add(len(disrup))
	subobjs := make([]float64, len(disrup))
	var errinner error
	for i, d := range disrup {
		// set separations plant to die disruption time.
		clone := s.Clone()
		if d.Time >= 0 {
			for i, b := range clone.Builds {
				if b.Proto == d.KillProto {
					clone.Builds[i].Life = d.Time - b.Time
				}
			}
		}

		go func(i int, s *Scenario) {
			defer wg.Done()
			val, err := obj(s)
			if err != nil {
				errinner = err
				val = math.Inf(1)
			}
			subobjs[i] = val
		}(i, clone)
	}

	wg.Wait()
	if errinner != nil {
		return math.Inf(1), fmt.Errorf("remote sub-simulation execution failed: %v", errinner)
	}

	// compute aggregate objective
	objval := 0.0
	for i := range subobjs {
		objval += disrup[i].Prob * subobjs[i]
	}
	return objval, nil
}
