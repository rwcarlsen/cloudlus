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

func TestSupportConstr(t *testing.T) {
	t.Fatalf("not implemented")
}
func TestAfterConstr(t *testing.T) {
	t.Fatalf("not implemented")
}

func TestPowerConstr(t *testing.T) {
	var tests = []struct {
		Dur      int
		Period   int
		Offset   int
		Facs     []Facility
		Want     [][]float64
		WantUp   []float64
		WantLow  []float64
		MinPower []float64
		MaxPower []float64
		Params   []Param
	}{
		{
			Dur: 15, Period: 3,
			Facs: []Facility{
				{Proto: "Proto1", Cap: 3},
				{Proto: "Proto2", Cap: 0},
				{Proto: "Proto3", Cap: 7},
			},
			MinPower: []float64{10, 20, 40, 70, 80},
			MaxPower: []float64{20, 30, 50, 80, 80},
			Want: [][]float64{
				{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 0},
				{3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 7, 7, 0, 0, 0},
				{3, 3, 3, 0, 0, 0, 0, 0, 0, 0, 7, 7, 7, 0, 0},
				{3, 3, 3, 3, 0, 0, 0, 0, 0, 0, 7, 7, 7, 7, 0},
				{3, 3, 3, 3, 3, 0, 0, 0, 0, 0, 7, 7, 7, 7, 7},
			},
			WantLow: []float64{10, 20, 40, 70, 80},
			WantUp:  []float64{20, 30, 50, 80, 80},
		},
		{
			Dur: 15, Period: 3,
			Facs: []Facility{
				{Proto: "Proto1", Cap: 3, Life: 5},
				{Proto: "Proto2", Cap: 0},
				{Proto: "Proto3", Cap: 7, Life: 1},
			},
			MinPower: []float64{10, 20, 40, 70, 80},
			MaxPower: []float64{20, 30, 50, 80, 80},
			Want: [][]float64{
				{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 0},
				{3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0},
				{0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0},
				{0, 0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0},
				{0, 0, 0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7},
			},
			WantLow: []float64{10, 20, 40, 70, 80},
			WantUp:  []float64{20, 30, 50, 80, 80},
		},
		{
			Dur: 15, Period: 3,
			Facs: []Facility{
				{Proto: "Proto1", Cap: 3, Life: 5},
				{Proto: "Proto2", Cap: 0},
				{Proto: "Proto3", Cap: 7, Life: 7},
			},
			MinPower: []float64{10, 20, 40, 70, 80},
			MaxPower: []float64{20, 30, 50, 80, 80},
			Want: [][]float64{
				{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 0},
				{3, 3, 0, 0, 0, 0, 0, 0, 0, 0, 7, 7, 0, 0, 0},
				{0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 7, 7, 7, 0, 0},
				{0, 0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 7, 7, 7, 0},
				{0, 0, 0, 3, 3, 0, 0, 0, 0, 0, 0, 0, 7, 7, 7},
			},
			WantLow: []float64{1, 4, 33, 63, 80},
			WantUp:  []float64{11, 14, 43, 73, 80},
			Params: []Param{
				{Time: 1, Proto: "Proto1", N: 3},
				{Time: 3, Proto: "Proto3", N: 1},
			},
		},
	}

	for i, test := range tests {
		s := &Scenario{
			SimDur:      test.Dur,
			BuildPeriod: test.Period,
			BuildOffset: test.Offset,
			MinPower:    test.MinPower,
			MaxPower:    test.MaxPower,
			Facs:        test.Facs,
			Params:      test.Params,
		}

		low, got, up := s.PowerConstr()

		nr, _ := got.Dims()
		if len(test.Want) != nr {
			t.Fatalf("case %v A: want %v rows, got %v", i, len(test.Want), nr)
		}

		for j := range test.Want {
			for k := range test.Want[j] {
				if test.Want[j][k] != got.At(j, k) {
					t.Errorf("case %v A[%v]: want %v, got %v", i, j, test.Want[j], got.Row(nil, j))
					break
				}
			}
		}
		for j := range test.WantLow {
			if test.WantLow[j] != low.At(j, 0) {
				t.Errorf("case %v lower: want %v, got %v", i, test.WantLow, low.Col(nil, 0))
				break
			}
		}
		for j := range test.WantUp {
			if test.WantUp[j] != up.At(j, 0) {
				t.Errorf("case %v upper: want %v, got %v", i, test.WantUp, up.Col(nil, 0))
				break
			}
		}
	}
}

type Mat struct {
	*mat64.Dense
}

func (m Mat) Format(f fmt.State, c rune) {
	mat64.Format(m, 0, 0, f, c)
}
