package scen

import (
	"fmt"
	"testing"

	"github.com/gonum/matrix/mat64"
)

func TestPeriodTimes(t *testing.T) {
	var tests = []struct {
		Dur    int
		Period int
		Offset int
		Want   []int
	}{
		{15, 3, 0, []int{1, 4, 7, 10, 13}},
		{13, 3, 0, []int{1, 4, 7, 10}},
		{2, 1, 0, []int{1}},
		{1, 1, 0, []int{}},
		{15, 3, 1, []int{2, 5, 8, 11, 14}},
		{15, 3, 2, []int{3, 6, 9, 12}},
		{16, 3, 2, []int{3, 6, 9, 12, 15}},
	}

	for i, test := range tests {
		s := &Scenario{
			SimDur:      test.Dur,
			BuildPeriod: test.Period,
			BuildOffset: test.Offset,
		}

		got := s.periodTimes()
		if len(got) != len(test.Want) {
			t.Errorf("case %v (dur=%v, per=%v, offset=%v): want %v, got %v", i, test.Dur, test.Period, test.Offset, test.Want, got)
		} else {
			for i := range got {
				if got[i] != test.Want[i] {
					t.Errorf("case %v (dur=%v, per=%v, offset=%v): want %v, got %v", i, test.Dur, test.Period, test.Offset, test.Want, got)
					break
				}
			}
		}
	}
}

func TestTransformVars(t *testing.T) {
	tests := []struct {
		Scen     *Scenario
		Vars     []float64
		PowerExp []float64
		BuildExp map[string][]int
	}{
		{
			Scen: &Scenario{
				SimDur:      10,
				BuildPeriod: 2,
				Facs: []Facility{
					{Proto: "Proto1", Cap: 1, Life: 0},
				},
				MaxPower: []float64{10, 20, 40, 60, 70},
				MinPower: []float64{10, 10, 10, 10, 70},
			},
			Vars:     []float64{.5, .5, .5, .5, .5},
			PowerExp: []float64{10, 15, 28, 44, 70},
			BuildExp: map[string][]int{
				"Proto1": {10, 5, 13, 16, 26},
			},
		}, {
			Scen: &Scenario{
				SimDur:      10,
				BuildPeriod: 2,
				Facs: []Facility{
					{Proto: "Proto1", Cap: 1, Life: 0},
				},
				MaxPower: []float64{10, 20, 40, 60, 70},
				MinPower: []float64{10, 10, 10, 10, 70},
			},
			Vars:     []float64{0, 0, 0, 0, 0},
			PowerExp: []float64{10, 10, 10, 10, 70},
		}, {
			Scen: &Scenario{
				SimDur:      10,
				BuildPeriod: 2,
				Facs: []Facility{
					{Proto: "Proto1", Cap: 3, Life: 0},
				},
				MaxPower: []float64{10, 20, 40, 60, 70},
				MinPower: []float64{10, 10, 10, 10, 70},
			},
			Vars:     []float64{.5, .5, .5, .5, .5},
			PowerExp: []float64{9, 15, 27, 45, 69},
		}, {
			Scen: &Scenario{
				SimDur:      10,
				BuildPeriod: 2,
				Facs: []Facility{
					{Proto: "Proto1", Cap: 4, Life: 0},
				},
				MaxPower: []float64{10, 20, 40, 60, 70},
				MinPower: []float64{10, 10, 10, 10, 70},
			},
			Vars:     []float64{.5, .5, .5, .5, .5},
			PowerExp: []float64{12, 16, 28, 44, 72},
			BuildExp: map[string][]int{
				"Proto1": {3, 1, 3, 4, 7},
			},
		}, {
			Scen: &Scenario{
				SimDur:      10,
				BuildPeriod: 2,
				Facs: []Facility{
					{Proto: "Proto1", Cap: 1, Life: 0},
					{Proto: "Proto2", Cap: 0, Life: 0, FracOfProtos: []string{"Proto1"}},
				},
				MaxPower: []float64{10, 20, 40, 60, 70},
				MinPower: []float64{10, 10, 10, 10, 70},
			},
			Vars:     []float64{.5, .5, .5, .5, .5, .5, .5, .5, .5, .5},
			PowerExp: []float64{10, 15, 28, 44, 70},
			BuildExp: map[string][]int{
				"Proto1": {10, 5, 13, 16, 26},
				"Proto2": {5, 8, 14, 22, 35},
			},
		},
	}

	for i, test := range tests {
		t.Logf("test %v", i)
		s := test.Scen
		vars := test.Vars

		builds, err := s.TransformVars(vars)
		if err != nil {
			t.Fatal(err)
		}

		timepowers := make([]float64, s.nperiods())
		for n, t := range s.periodTimes() {
			for _, buildsp := range builds {
				for _, b := range buildsp {
					if b.Alive(t) {
						timepowers[n] += b.fac.Cap * float64(b.N)
					}
				}
			}
		}

		t.Logf("  power cap want: %v", test.PowerExp)
		t.Logf("  power cap got: %v", timepowers)

		for proto, buildsp := range builds {
			nbuilt := make([]int, s.nperiods())
			for _, b := range buildsp {
				nbuilt[s.periodOf(b.Time)] += b.N
			}
			t.Logf("  proto %v nbuilt want: %v", proto, test.BuildExp[proto])
			t.Logf("  proto %v nbuilt got: %v", proto, nbuilt)
		}
	}
}

func TestVarNames(t *testing.T) {
	facs := []Facility{
		Facility{Proto: "Proto1"},
		Facility{Proto: "Proto2"},
		Facility{Proto: "Proto3"},
		Facility{Proto: "Proto4"},
	}
	s := &Scenario{
		SimDur:      10,
		BuildPeriod: 2,
		Facs:        facs,
		MinPower:    []float64{10, 20, 30, 50, 60},
		MaxPower:    []float64{150, 150, 150, 150, 150},
	}

	t.Logf("Scenario: %+v", s)
	for _, fac := range s.Facs {
		t.Logf("   %+v", fac)
	}
	t.Logf("nvars: %+v", s.nvars())
	t.Logf("nperiods: %+v", s.nperiods())

	t.Log("VarNames:")
	for i, name := range s.VarNames() {
		t.Logf("   %v| %v", i, name)
	}
	t.Logf("LowerBounds:\n%v", Mat{s.LowerBounds()})
	t.Logf("UpperBounds:\n%v", Mat{s.UpperBounds()})
}

type Mat struct {
	*mat64.Dense
}

func (m Mat) Format(f fmt.State, c rune) {
	mat64.Format(m, 0, 0, f, c)
}
