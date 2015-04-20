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

func TestVarNames(t *testing.T) {
	facs := []Facility{
		Facility{Proto: "Proto1", OpCost: 10, CapitalCost: 3},
		Facility{Proto: "Proto2", OpCost: 5, CapitalCost: 3},
		Facility{Proto: "Proto3", OpCost: 5, CapitalCost: 5},
		Facility{Proto: "Proto4", OpCost: 0, CapitalCost: 3},
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
	t.Logf("Nvars: %+v", s.Nvars())
	t.Logf("nPeriods: %+v", s.nPeriods())

	t.Log("VarNames:")
	for i, name := range s.VarNames() {
		t.Logf("   %v| %v", i, name)
	}
	t.Logf("LowerBounds:\n%v", Mat{s.LowerBounds()})
	t.Logf("UpperBounds:\n%v", Mat{s.UpperBounds()})

	low, Ap, up := s.PowerConstr()
	t.Log("Power Constraints:")
	t.Logf("    LowerBounds:\n%v", Mat{low})
	t.Logf("    UpperBounds:\n%v", Mat{up})
	t.Logf("    A:\n%v", Mat{Ap})

	low, As, up := s.SupportConstr()
	t.Log("Support Constraints:")
	t.Logf("    LowerBounds:\n%v", Mat{low})
	t.Logf("    UpperBounds:\n%v", Mat{up})
	t.Logf("    A:\n%v", Mat{As})

	l, A, u := s.IneqConstr()
	t.Log("All Constraints:")
	t.Logf("    LowerBounds:\n%v", Mat{l})
	t.Logf("    UpperBounds:\n%v", Mat{u})
	t.Logf("    A:\n%v", Mat{A})
}

type Mat struct {
	*mat64.Dense
}

func (m Mat) Format(f fmt.State, c rune) {
	mat64.Format(m, 0, 0, f, c)
}
