package pattern

import (
	"database/sql"
	"log"
	"math"
	"testing"

	_ "github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/go-sqlite/sqlite3"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/rwcarlsen/optim/bench"
)

func TestDb(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	fn := bench.Basic[0]
	optimum := fn.Optima()[0].Val
	it, m := patternsolver(fn, db)

	solv := &optim.Solver{
		Method:  it,
		Obj:     optim.Func(fn.Eval),
		Mesh:    m,
		MaxIter: 100,
		MinStep: -1,
	}
	err = solv.Run()
	if err != nil {
		log.Fatal(err)
	}

	t.Logf("[INFO] %v evals: want %v, got %v", solv.Neval(), optimum, solv.Best().Val)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM " + TblPolls).Scan(&count)
	if err != nil {
		t.Errorf("[ERROR] polls table query failed: %v", err)
	} else if count == 0 {
		t.Errorf("[ERROR] polls table has no rows")
	}

	count = 0
	err = db.QueryRow("SELECT COUNT(*) FROM " + TblInfo).Scan(&count)
	if err != nil {
		t.Errorf("[ERROR] info table query failed: %v", err)
	} else if count == 0 {
		t.Errorf("[ERROR] info table has no rows")
	}
}

func TestRandomN(t *testing.T) {
	r := &RandomN{
		N:    10,
		Mask: []bool{true, true, true, false, true, true, false},
	}

	type update struct {
		Step        float64
		PrevSuccess bool
	}
	upds := []update{
		{1, false},
		{.5, true},
		{.5, true},
		{.5, true},
		{.5, true},
		{1, false},
		{0.5, false},
	}

	for _, upd := range upds {
		r.Update(upd.Step, upd.PrevSuccess)
	}

	dirs := r.Span(len(r.Mask))

	for i, dir := range dirs {
		t.Logf("dir %v: %v", i, dir)
	}
}

func patternsolver(fn bench.Func, db *sql.DB) (optim.Method, optim.Mesh) {
	low, up := fn.Bounds()
	max, min := up[0], low[0]
	pos := make([]float64, len(low))
	for i := range pos {
		pos[i] = low[i] + (up[i]-low[i])/3
	}
	m := &optim.BoxMesh{&optim.InfMesh{StepSize: (max - min) / 10}, low, up}
	m.SetOrigin(pos)
	p := &optim.Point{pos, math.Inf(1)}
	return New(p, DB(db)), m
}
