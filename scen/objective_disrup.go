package scen

import (
	"errors"
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
	// BuildProto is the prototype of which to build a single new instance at
	// the given time.
	BuildProto string
	// Prob holds the probability that the disruption will happen at a
	// particular time.  This is ignored in disrup-single mode.  An
	// unspecified probability for a disruption is assumed to be zero.  Note
	// that the integral of linear interpolation between probabilities over
	// the entire simulation duration must be less than or equal to 1; it must
	// be equal to the probability that the disruption happens over the entire
	// simulation.  So for four samples and a 2400 time step simulation, each
	// disruption should NOT have a 0.25 probability - they should have 1/2400
	// probability.
	Prob float64
	// Sample is true if this disruption time should be sampled for generation
	// of the Obj vs Disrup approximation.  KnownBests should generally be placed on
	// Sample=true disruption points only.
	Sample bool
	// KnownBest holds the objective value for the best known deployment
	// schedule for the scenario for with a priori knowledge of this
	// particular disruption always occuring.  This is only used in
	// disrup-multi-lin mode.  Linear interpolation is performed between the
	// KnownBests of disruptoin points with Sample=true.
	KnownBest float64
}

type disrupOpt int

const (
	optNone disrupOpt = 1 << iota
	optProb
	optKnownBest
)

func disrupSingleModeLin(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-single"].(map[string]interface{})
	disrup := Disruption{}
	disrup, err := parseDisrup(idisrup, optKnownBest)
	if err != nil {
		return math.Inf(1), fmt.Errorf("disrup-single-lin: %v", err)
	}

	// set separations plant to die disruption time.
	clone := s.Clone()
	clone.Builds = append(clone.Builds, buildsForDisrup(clone, disrup)...)
	if disrup.Time >= 0 {
		for i, b := range clone.Builds {
			clone.Builds[i] = modForDisrup(clone, disrup, b)
		}
	}

	subobj, err := obj(clone)
	if err != nil {
		return math.Inf(1), err
	}

	wPre := float64(disrup.Time) / float64(s.SimDur)
	if disrup.Time < 0 {
		wPre = 1.0
	}
	wPost := 1 - wPre
	return wPre*subobj + wPost*disrup.KnownBest, nil
}

func disrupSingleMode(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-single"].(map[string]interface{})
	disrup, err := parseDisrup(idisrup, optNone)
	if err != nil {
		return math.Inf(1), fmt.Errorf("disrup-single: %v", err)
	}

	// set separations plant to die disruption time.
	clone := s.Clone()
	clone.Builds = append(clone.Builds, buildsForDisrup(clone, disrup)...)
	if disrup.Time >= 0 {
		for i, b := range clone.Builds {
			clone.Builds[i] = modForDisrup(clone, disrup, b)
		}
	}

	return obj(clone)
}

func buildsForDisrup(s *Scenario, disrup Disruption) []Build {
	if disrup.Time < 0 || disrup.BuildProto == "" {
		return []Build{}
	}

	b := Build{
		Time:  disrup.Time,
		N:     1,
		Proto: disrup.BuildProto,
	}

	for _, fac := range s.Facs {
		if fac.Proto == b.Proto {
			b.fac = fac
			return []Build{b}
		}
	}
	panic("prototype " + b.Proto + " not found")
}

func modForDisrup(s *Scenario, disrup Disruption, b Build) Build {
	if disrup.Time < 0 {
		return b
	} else if b.Proto != disrup.KillProto {
		return b
	}

	b.Life = disrup.Time - b.Time
	return b
}

// disrupModeLin is the same as disrupMode except it performs the weighted
// linear combination of each sub objective with the know best for that
// disruption time to compute the final sub objectives that are then combined.
func disrupModeLin(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-multi"].([]interface{})
	disrups := make([]Disruption, len(idisrup))
	for i, td := range idisrup {
		m := td.(map[string]interface{})
		d, err := parseDisrup(m, optProb|optKnownBest)
		if err != nil {
			return math.Inf(1), fmt.Errorf("disrup-multi-lin: %v", err)
		}
		disrups[i] = d
	}

	subobjs, err := runDisrupSims(s, obj, disrups)
	if err != nil {
		return math.Inf(1), err
	}

	// compute aggregate objective using disruption times and corresponding
	// knownbests
	for i := range subobjs {
		wPre := float64(disrups[i].Time) / float64(s.SimDur)
		if wPre < 0 {
			wPre = 1
		}
		wPost := 1 - wPre
		subobjs[i] = wPre*subobjs[i] + wPost*disrups[i].KnownBest
	}

	objval := aggregateObj(s.SimDur, disrups, subobjs)
	return objval, nil
}

func disrupMode(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-multi"].([]interface{})
	disrups := make([]Disruption, len(idisrup))
	for i, td := range idisrup {
		m := td.(map[string]interface{})
		d, err := parseDisrup(m, optProb)
		if err != nil {
			return math.Inf(1), fmt.Errorf("disrup-multi: %v", err)
		}

		disrups[i] = d
	}

	subobjs, err := runDisrupSims(s, obj, disrups)
	if err != nil {
		return math.Inf(1), err
	}

	objval := aggregateObj(s.SimDur, disrups, subobjs)
	return objval, nil
}

// aggregateObj takes all disruption points (including unsampled) and their respective
// sub-objective values and generates interpolating functions for both the
// disruption probabilities vs time and sub-objectives vs time and integrates
// over their product and returns the mean outcome given the disruption
// probability distribution.
func aggregateObj(simdur int, disrups []Disruption, subobjs []float64) float64 {
	sampled := []Disruption{}
	for _, d := range disrups {
		if d.Sample {
			sampled = append(sampled, d)
		}
	}

	objVsTime := interpolate(zip(sampled, subobjs))
	probVsTime := interpolate(extractProbs(disrups))

	t0 := 0.0
	tend := float64(simdur)
	objval := integrateMid(productOf(objVsTime, probVsTime), t0, tend, 10000)
	// calculate probability of no disruption and assume objective for that
	// case is same as disruption occuring at t_end
	nodisruptail := (1 - integrateMid(probVsTime, t0, tend, 10000)) * objVsTime(tend)
	objval += nodisruptail

	return objval
}

func parseDisrup(disrup map[string]interface{}, opts disrupOpt) (Disruption, error) {
	d := Disruption{}
	if t, ok := disrup["Time"]; ok {
		d.Time = int(t.(float64))
	} else {
		return Disruption{}, errors.New("disruption config missing 'Time' param")
	}

	if proto, ok := disrup["KillProto"]; ok {
		d.KillProto = proto.(string)
	}

	if proto, ok := disrup["BuildProto"]; ok {
		d.BuildProto = proto.(string)
	}

	if (d.KillProto == "" && d.BuildProto == "") || (d.KillProto != "" && d.BuildProto != "") {
		return Disruption{}, errors.New("disruption config must have exactly one of 'BuildProto' or 'KillProto' params set")
	}

	if s, ok := disrup["Sample"]; ok {
		d.Sample = s.(float64) != 0
	}

	if prob, ok := disrup["Prob"]; ok {
		d.Prob = prob.(float64)
	} else if opts&optProb > 0 && d.Sample {
		return Disruption{}, errors.New("disruption config missing 'Prob' param")
	}

	if v, ok := disrup["KnownBest"]; ok {
		d.KnownBest = v.(float64)
	} else if opts&optKnownBest > 0 && d.Sample {
		return Disruption{}, errors.New("disruption config missing 'KnownBest' param")
	}
	return d, nil
}

// runDisrupSims takes all disruptions and only runs simulations for the
// sampled disruption points and returns their corresponding objective values
// (in order).
func runDisrupSims(s *Scenario, obj ObjExecFunc, disrups []Disruption) (objs []float64, err error) {
	sampled := []Disruption{}
	for _, d := range disrups {
		if d.Sample {
			sampled = append(sampled, d)
		}
	}

	// fire off concurrent sub-simulation objective evaluations
	var wg sync.WaitGroup
	wg.Add(len(sampled))
	objs = make([]float64, len(sampled))
	var errinner error
	for i, d := range sampled {
		// set separations plant to die disruption time.
		clone := s.Clone()
		clone.Builds = append(clone.Builds, buildsForDisrup(clone, d)...)
		if d.Time >= 0 {
			for i, b := range clone.Builds {
				clone.Builds[i] = modForDisrup(clone, d, b)
			}
		}

		go func(i int, scn *Scenario) {
			defer wg.Done()
			val, err := obj(scn)
			if err != nil {
				errinner = err
				val = math.Inf(1)
			}
			objs[i] = val
		}(i, clone)
	}

	wg.Wait()
	if errinner != nil {
		return nil, fmt.Errorf("remote sub-simulation execution failed: %v", errinner)
	}
	return objs, nil
}
