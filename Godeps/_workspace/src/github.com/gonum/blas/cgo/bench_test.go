package cgo

import (
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/blas"
	"github.com/rwcarlsen/cloudlus/Godeps/_workspace/src/github.com/gonum/blas/testblas"
)

const (
	Sm  = testblas.SmallMat
	Med = testblas.MediumMat
	Lg  = testblas.LargeMat
	Hg  = testblas.HugeMat
)

const (
	T  = blas.Trans
	NT = blas.NoTrans
)
