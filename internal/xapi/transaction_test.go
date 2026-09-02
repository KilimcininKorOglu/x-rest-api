package xapi

import (
	"math"
	"testing"
)

// Golden values are captured from a known-good run so the implementation stays
// byte-compatible on the tricky transforms.

func TestFloatToHex(t *testing.T) {
	cases := map[float64]string{
		0.5:  ".8",
		1.0:  "1",
		0.0:  "",
		0.87: ".DEB851EB851EB8",
		0.71: ".B5C28F5C28F5C",
	}
	for in, want := range cases {
		if got := floatToHex(in); got != want {
			t.Errorf("floatToHex(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestCubicValue(t *testing.T) {
	cases := []struct {
		curves []float64
		t      float64
		want   float64
	}{
		{[]float64{0.1, 0.2, 0.3, 0.4}, 0.5, 0.5624697925},
		{[]float64{0.0, 0.5, 1.0, 0.5}, 0.25, 0.3645303878},
	}
	for _, c := range cases {
		got := newCubic(c.curves).value(c.t)
		if math.Abs(got-c.want) > 1e-4 {
			t.Errorf("cubic(%v).value(%v) = %v, want %v", c.curves, c.t, got, c.want)
		}
	}
}

func TestSolve(t *testing.T) {
	if got := solve(200, 60.0, 360.0, true); got != 295 {
		t.Errorf("solve rounding = %v, want 295", got)
	}
	if got := solve(128, 0.0, 1.0, false); got != 0.5 {
		t.Errorf("solve = %v, want 0.5", got)
	}
	if got := solve(10, -1.0, 1.0, false); got != -0.92 {
		t.Errorf("solve neg = %v, want -0.92", got)
	}
}

func TestRotationMatrix(t *testing.T) {
	m := rotationMatrix(0)
	if len(m) != 4 || m[0] != 1 || m[3] != 1 {
		t.Errorf("rotationMatrix(0) = %v, want [1,-0,0,1]", m)
	}
}
