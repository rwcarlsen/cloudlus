package optim

import (
	"fmt"
	"math"

	"github.com/gonum/matrix/mat64"
)

// Mesh is an interface for projecting arbitrary dimensional points onto some
// kind of (potentially discrete) mesh.
type Mesh interface {
	Step() float64
	// Nearest returns the nearest location to x on the mesh.  x must have the
	// same dimensionality as the mesh.  Nearest never modifies x.
	Nearest(x []float64) []float64
	SetStep(step float64)
	SetOrigin(origin []float64)
	Origin() []float64
}

type MaxStepMesh struct {
	Mesh
	MaxStep float64
}

func (m *MaxStepMesh) SetStep(step float64) {
	if step <= m.MaxStep {
		m.Mesh.SetStep(step)
	}
}

type IntMesh struct {
	Mesh
}

func (m *IntMesh) Nearest(p []float64) []float64 {
	gridp := m.Mesh.Nearest(p)
	for i := range gridp {
		gridp[i] = math.Floor(gridp[i] + .5) // round to nearest int
	}
	return gridp
}

func (m *IntMesh) SetStep(step float64) {
	m.Mesh.SetStep(math.Max(step, 1))
}

func (m *IntMesh) SetOrigin(origin []float64) {
	m.Mesh.SetOrigin(m.Nearest(origin))
}

// Inifinite is a grid-based, linear-axis mesh that extends in all dimensions
// without bounds.  The length of Origin defines the dimensionality of the
// mesh. If Origin == nil, the dimensionality is set by the first call to
// Nearest.  If Basis == nil, a unit basis (the identify matrix) is used.  If
// Step == 0, then the mesh represents continuous space and the Nearest method
// just returns the point passed to it.
type InfMesh struct {
	Center []float64
	// Basis contains a set of row vectors defining the directions of each
	// mesh axis for the car.
	Basis *mat64.Dense
	// Step represents the discretization or grid size of the mesh.
	StepSize float64
	inverter *mat64.Dense
}

func (m *InfMesh) Step() float64              { return m.StepSize }
func (m *InfMesh) SetStep(step float64)       { m.StepSize = step }
func (m *InfMesh) Origin() []float64          { return m.Center }
func (m *InfMesh) SetOrigin(origin []float64) { m.Center = origin }

// Nearest returns the nearest grid point to p by rounding each dimensional
// position to the nearest grid point.  If the mesh basis is not the identity
// matrix, then p is transformed to the mesh basis before rounding and then
// retransformed back.
func (m *InfMesh) Nearest(p []float64) []float64 {
	if m.StepSize == 0 {
		return append([]float64{}, p...)
	} else if l := len(m.Center); l != 0 && l != len(p) {
		panic(fmt.Sprintf("origin len %v incompatible with point len %v", l, len(p)))
	}

	// set up origin and inverter matrix if necessary
	if len(m.Center) == 0 {
		m.Center = make([]float64, len(p))
	}
	if m.Basis != nil && m.inverter == nil {
		var err error
		m.inverter, err = mat64.Inverse(m.Basis)
		if err != nil {
			panic("basis inversion failed: " + err.Error())
		}
	}

	// translate p based on origin and transform to new vector space
	newp := make([]float64, len(p))
	for i := range newp {
		newp[i] = p[i] - m.Center[i]
	}
	v := mat64.NewDense(len(m.Center), 1, newp)
	rotv := v
	if m.inverter != nil {
		rotv.Mul(m.inverter, v)
	}

	// calculate nearest point
	nearest := mat64.NewDense(len(p), 1, nil)
	for i := range m.Center {
		n, rem := math.Modf(rotv.At(i, 0) / m.StepSize)
		if rem/m.StepSize > 0.5 {
			n++
		}
		nearest.Set(i, 0, float64(n)*m.StepSize)
	}

	// transform back to standard space
	if m.Basis != nil {
		nearest.Mul(m.Basis, nearest)
	}
	nv := nearest.Col(nil, 0)
	for i := range nv {
		nv[i] += m.Center[i]
	}
	return nv
}

type BoxMesh struct {
	Mesh
	Lower []float64
	Upper []float64
}

// Nearest returns the nearest bounded grid point to p by sliding each
// dimensional position to the nearest value inside bounds and then rounding
// to the nearest grid point.  If the mesh basis is not the identity matrix,
// then p is transformed to the mesh basis before rounding and then
// retransformed back.
func (m *BoxMesh) Nearest(p []float64) []float64 {
	pdup := make([]float64, len(p))
	copy(pdup, p)
	for i := range pdup {
		pdup[i] = math.Max(m.Lower[i], pdup[i])
		pdup[i] = math.Min(m.Upper[i], pdup[i])
	}
	return m.Mesh.Nearest(pdup)
}
