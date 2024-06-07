package costmodel

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/grafana/kost/pkg/costmodel/utils"
)

// This is for making it easier to assign resources for testing.
type requirementsHelperFuncs struct {
	cpu, mem, pv func(string) int64

	cpuCost, memCost, pvCost func(string, float64) float64
}

func requirementsHelpers(t *testing.T) requirementsHelperFuncs {
	pq := func(s string) *resource.Quantity {
		t.Helper()
		q, err := resource.ParseQuantity(s)
		if err != nil {
			t.Fatalf("parsing CPU %q: %v", s, err)
		}
		return &q
	}

	h := requirementsHelperFuncs{
		cpu: func(s string) int64 {
			t.Helper()
			return pq(s).MilliValue()
		},

		mem: func(s string) int64 {
			t.Helper()
			return pq(s).Value()
		},

		pv: func(s string) int64 {
			t.Helper()
			return pq(s).Value()
		},
	}

	const period float64 = 24 * 30

	h.cpuCost = func(s string, c float64) float64 {
		return float64(h.cpu(s)) / 1000 * c * period
	}
	h.memCost = func(s string, c float64) float64 {
		return utils.BytesToGiB(h.mem(s)) * c * period
	}
	h.pvCost = h.cpuCost

	return h
}
