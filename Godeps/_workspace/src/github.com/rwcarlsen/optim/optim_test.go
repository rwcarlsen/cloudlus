package optim

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
)

func testpoints() []*Point {
	return []*Point{
		&Point{[]float64{1, 2, 3}, 0},
		&Point{[]float64{1, 2, 3}, 0}, // duplicate point on purpose
		&Point{[]float64{1, 2, 4}, 0},
		&Point{[]float64{1, 2, 5}, 0},
		&Point{[]float64{1, 2, 6}, 0},
		&Point{[]float64{1, 2, 7}, 0},
	}
}

type ObjTest struct {
	count int
	max   int
	sync.Mutex
}

func (o *ObjTest) Objective(x []float64) (float64, error) {
	o.Lock()
	defer o.Unlock()

	o.count++
	if o.count >= o.max {
		return math.Inf(1), errors.New("fake error")
	}
	tot := 0.0
	for _, v := range x {
		tot += v
	}
	return tot, nil
}

func TestUniqOfPoints(t *testing.T) {
	points := testpoints()
	want := map[*Point]bool{}
	want[points[0]] = true
	want[points[2]] = true
	want[points[3]] = true
	want[points[4]] = true
	want[points[5]] = true

	uniq := uniqof(points)
	uniqmap := map[*Point]bool{}
	for _, p := range uniq {
		uniqmap[p] = true
		if !want[p] {
			t.Errorf("uniqof returned duplicate point")
		}
	}
	for p, want := range want {
		if want != uniqmap[p] {
			t.Errorf("uniqof excluded non-duplicate point")
		}
	}
}

func TestSerialEvaler_DupPoints(t *testing.T) {
	obj := &ObjTest{max: 10000}
	ev := SerialEvaler{}

	tpoints := testpoints()
	r, _, _ := ev.Eval(obj, tpoints...)

	dups := testpoints()
	for i, p := range r {
		orig := dups[i]
		for k := 0; k < p.Len(); k++ {
			if p.Pos[k] != p.Pos[k] {
				t.Errorf("result[%v] wrong point: want %v, got %v", orig, p)
				break
			}
		}
	}
}

func TestSerialEvalerErr(t *testing.T) {
	errcount := 3
	exprlen := errcount
	expn := exprlen
	obj := &ObjTest{max: errcount}
	ev := SerialEvaler{}

	tpoints := testpoints()
	r, n, err := ev.Eval(obj, tpoints...)

	if len(r) != exprlen {
		// if this fires, duplicate point avoidance may be broken
		t.Errorf("returned wrong number of results: expected %v, got %v", exprlen, len(r))
	}
	if n != expn {
		t.Errorf("returned wrong evaluation count: expected %v, got %v", expn, n)
	}
	if err == nil {
		t.Errorf("did not propagate error through return")
	}

	// exclude last entry in r because it was the error'd obj evaluation

	want := map[*Point]bool{
		tpoints[0]: true,
		tpoints[2]: true,
		tpoints[3]: true,
	}
	for i, p := range r {
		if !want[p] {
			t.Errorf("result point %v (%v) was not expected", i, p)
		}
	}
}

func TestParallelEvalerErr(t *testing.T) {
	tpoints := testpoints()
	errcount := 4
	exprlen := len(tpoints) - 1 // for duplicate
	expn := exprlen
	obj := &ObjTest{max: errcount}
	ev := ParallelEvaler{}

	r, n, err := ev.Eval(obj, tpoints...)

	// parallel always evaluates all points
	if len(r) != exprlen {
		t.Errorf("returned wrong number of results: expected %v, got %v", exprlen, len(r))
	}
	if n == len(tpoints) {
		t.Errorf("failed to avoid evaluation of duplicate points", errcount, n)
	}
	if n != expn {
		t.Errorf("returned wrong evaluation count: expected %v, got %v", errcount, n)
	}
	if err == nil {
		t.Errorf("did not propagate error through return")
	}

	for i, p := range r {
		expobj := 0.0
		for _, v := range p.Pos {
			expobj += v
		}
		if p.Val != expobj && p.Val != math.Inf(1) {
			t.Errorf("point %v (%v) objective value: expected %v, got %v", i, p.Pos, expobj, p.Val)
		}
	}
}

func TestCacheEvalerErr(t *testing.T) {
	tpoints := testpoints()
	errcount := 3
	exprlen := errcount
	expn := exprlen
	obj := &ObjTest{max: errcount}
	ev := NewCacheEvaler(SerialEvaler{})

	r, n, err := ev.Eval(obj, tpoints...)

	if len(r) != exprlen {
		t.Errorf("returned wrong number of r: expected %v, got %v", exprlen, len(r))
	}
	if n != expn {
		t.Errorf("returned wrong evaluation count: expected %v, got %v", expn, n)
	}
	if err == nil {
		t.Errorf("did not propogate error through return")
	}
}

func TestCacheEvaler(t *testing.T) {
	tpoints := testpoints()
	obj := &ObjTest{max: 100000}
	ev := NewCacheEvaler(SerialEvaler{})
	expn := len(tpoints) - 1
	exprlen := 2 * (len(tpoints) - 1) // for duplicate

	r1, n1, err1 := ev.Eval(obj, tpoints...)
	r2, n2, err2 := ev.Eval(obj, tpoints...)
	fmt.Println(n1)
	fmt.Println(n2)

	if v := len(r1) + len(r2); v != exprlen {
		t.Errorf("returned wrong number of results: expected %v, got %v", exprlen, v)
	}
	if n1+n2 != expn {
		t.Errorf("returned wrong evaluation count: expected %v, got %v", expn, n1+n2)
	}
	if err1 != nil || err2 != nil {
		t.Errorf("got unexpected err (err1 and err2): %v and %v", err1, err2)
	}

	tpoints = testpoints()
	tpoints = append(tpoints[:1], tpoints[2:]...) // results should exclude the single duplicate point
	for i := range r1 {
		for j := range tpoints[i].Pos {
			if exp, got := tpoints[i].Pos[j], r1[i].Pos[j]; exp != got {
				t.Errorf("bad pos: expected %+v, got %+v", tpoints[i].Pos, r1[i].Pos)
			}
			if exp, got := tpoints[i].Pos[j], r2[i].Pos[j]; exp != got {
				t.Errorf("bad cached pos: expected %+v, got %+v", tpoints[i].Pos, r2[i].Pos)
			}
		}
	}
}
