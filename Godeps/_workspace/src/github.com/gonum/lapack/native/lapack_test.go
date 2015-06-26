package native

import (
	"testing"

	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/lapack/testlapack"
)

var impl = Implementation{}

func TestDpotf2(t *testing.T) {
	testlapack.Dpotf2Test(t, impl)
}

func TestDpotrf(t *testing.T) {
	testlapack.DpotrfTest(t, impl)
}
