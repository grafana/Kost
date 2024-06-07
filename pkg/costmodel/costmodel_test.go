package costmodel

import (
	"math"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

// feq returns true if two floats are equal within certain tolerance
func feq(a, b float64) bool {
	const tolerance = 0.001
	return math.Abs(a-b) < tolerance
}

func TestCostModelForPeriod(t *testing.T) {
	t.Run("Spot", func(t *testing.T) {
		tests := []struct {
			req    string
			hourly float64
			period Period
			exp    float64
		}{
			// Properly progress the values
			{"1", 0.0069, Hourly, 0.0069},
			{"1", 0.0069, Daily, 0.0069 * 24},
			{"1", 0.0069, Weekly, 0.0069 * 24 * 7},
			{"1", 0.0069, Monthly, 0.0069 * 24 * 30},
			{"1", 0.0069, Yearly, 0.0069 * 24 * 365},

			// General tests
			{"10", 0.0125, Daily, 3},
			{"500m", 2.7108, Weekly, 227.707},
		}

		for _, tt := range tests {
			c := Cost{Spot: tt.hourly}
			q := resource.MustParse(tt.req)
			r := q.MilliValue()
			g := c.SpotCPUForPeriod(tt.period, r)
			if !feq(tt.exp, g) {
				t.Errorf("%v CPU expecting %v cost %f, got %f", tt.req, tt.period, tt.exp, g)
			}
		}
	})

	t.Run("Non-Spot", func(t *testing.T) {
		tests := []struct {
			req    string
			hourly float64
			period Period
			exp    float64
		}{
			// Properly progress the values
			{"1", 0.0832, Hourly, 0.0832},
			{"1", 0.0832, Daily, 0.0832 * 24},
			{"1", 0.0832, Weekly, 0.0832 * 24 * 7},
			{"1", 0.0832, Monthly, 0.0832 * 24 * 30},
			{"1", 0.0832, Yearly, 0.0832 * 24 * 365},

			// General tests
			{"10", 0.3328, Daily, 79.8723},
			{"500m", 0.408, Hourly, 0.204},
		}

		for _, tt := range tests {
			c := Cost{NonSpot: tt.hourly}
			q := resource.MustParse(tt.req)
			r := q.MilliValue()
			g := c.NonSpotCPUForPeriod(tt.period, r)
			if !feq(tt.exp, g) {
				t.Errorf("%v CPU expecting %v cost %f, got %f", tt.req, tt.period, tt.exp, g)
			}
		}
	})

	t.Run("Dollars", func(t *testing.T) {
		tests := []struct {
			req    string
			hourly float64
			period Period
			exp    float64
		}{
			// Properly progress the values
			{"1Gi", 1.336, Hourly, 1.336},
			{"1Gi", 1.336, Daily, 1.336 * 24},
			{"1Gi", 1.336, Weekly, 1.336 * 24 * 7},
			{"1Gi", 1.336, Monthly, 1.336 * 24 * 30},
			{"1Gi", 1.336, Yearly, 1.336 * 24 * 365},

			// General tests
			{"10Gi", 0.0835, Daily, 20.04},
			{"0.5Gi", 0.167, Weekly, 14.028},
		}

		for _, tt := range tests {
			c := Cost{Dollars: tt.hourly}
			q := resource.MustParse(tt.req)
			r := q.Value()
			g := c.DollarsForPeriod(tt.period, r)
			if !feq(tt.exp, g) {
				t.Errorf("%v RAM expecting %v cost %f, got %f", tt.req, tt.period, tt.exp, g)
			}
		}
	})
}

func TestCostModelYearly(t *testing.T) {
	c := Cost{
		Dollars: 22,
		Spot:    121,
		NonSpot: 4,
	}

	const hoursInYear = 24 * 365

	t.Run("CPU", func(t *testing.T) {
		tests := []struct {
			in      string
			spot    float64
			nonSpot float64
		}{
			{"1", 121 * hoursInYear, 4 * hoursInYear},
			{"10", 1210 * hoursInYear, 40 * hoursInYear},
			{"5", 605 * hoursInYear, 20 * hoursInYear},
			{"500m", 60.5 * hoursInYear, 2 * hoursInYear},
		}

		for _, tt := range tests {
			q, err := resource.ParseQuantity(tt.in)
			if err != nil {
				t.Fatalf("cannot parse quantity %q: %v", tt.in, err)
			}
			r := q.MilliValue()
			if s := c.SpotYearly(r); s != tt.spot {
				t.Errorf("%v CPU expecting yearly spot %f, got %f", tt.in, tt.spot, s)
			}
			if ns := c.NonSpotYearly(r); ns != tt.nonSpot {
				t.Errorf("%v CPU expecting yearly non-spot %f, got %f", tt.in, tt.nonSpot, ns)
			}
		}
	})

	t.Run("Memory", func(t *testing.T) {
		tests := []struct {
			in      string
			spot    float64
			nonSpot float64
		}{
			{"1Gi", c.Spot * Yearly, c.NonSpot * Yearly},
			{"2Gi", c.Spot * 2 * Yearly, c.NonSpot * 2 * Yearly},
			{"10Gi", c.Spot * 10 * Yearly, c.NonSpot * 10 * Yearly},
			{"0.5Gi", c.Spot * 0.5 * Yearly, c.NonSpot * 0.5 * Yearly},
			{"0.25Gi", c.Spot * 0.25 * Yearly, c.NonSpot * 0.25 * Yearly},
		}

		for _, tt := range tests {
			q, err := resource.ParseQuantity(tt.in)
			if err != nil {
				t.Fatalf("cannot parse quantity %q: %v", tt.in, err)
			}
			r := q.Value()

			if s := c.SpotMemoryForPeriod(Yearly, r); s != tt.spot {
				t.Errorf("%v memory expecting yearly dollars %f, got %f", tt.in, tt.spot, s)
			}

			if g := c.NonSpotMemoryForPeriod(Yearly, r); tt.nonSpot != g {
				t.Errorf("%v memory expecting yearly dollars %f, got %f", tt.in, tt.nonSpot, g)
			}
		}
	})
}
