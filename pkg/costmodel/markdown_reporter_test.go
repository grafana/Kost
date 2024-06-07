package costmodel

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/grafana/deployment_tools/docker/k8s-cost-estimator/pkg/costmodel/utils"
)

func TestResourcesCost(t *testing.T) {
	rc := resourcesCost{
		CPU:     1024,
		Memory:  512,
		Storage: 256,
	}

	var exp float64 = 1024 + 512 + 256

	if got := rc.Total(); exp != got {
		t.Fatalf("expecting %v total %v, got %v", rc, exp, got)
	}
}

func TestResourcesCosts(t *testing.T) {
	h := requirementsHelpers(t)

	cm := &CostModel{
		CPU: Cost{NonSpot: 1},
		RAM: Cost{NonSpot: 2},

		PersistentVolume: Cost{Dollars: 3},
	}

	req := Requirements{
		CPU:              h.cpu("100m"),
		Memory:           h.mem("2Gi"),
		PersistentVolume: h.pv("4Gi"),
	}

	got := resourcesCosts(cm, req)

	exp := resourcesCost{
		CPU:     0.1 * 1 * 24 * 30,
		Memory:  utils.BytesToGiB(2147483648) * 2 * 24 * 30,
		Storage: utils.BytesToGiB(1024*1024*1024*4) * 3 * 24 * 30,
	}

	if e, g := exp.CPU, got.CPU; !eq(e, g) {
		t.Errorf("expecting CPU %.05f, got %.05f", e, g)
	}
	if e, g := exp.Memory, got.Memory; !eq(e, g) {
		t.Errorf("expecting Memory %.05f, got %.05f", e, g)
	}
	if e, g := exp.Storage, got.Storage; !eq(e, g) {
		t.Errorf("expecting Storage %.05f, got %.05f", e, g)
	}
}

func eq(a, b float64) bool {
	const d = 0.001
	return math.Abs(a-b) < d
}

func TestCostReport(t *testing.T) {
	tests := []struct {
		cr  costReport
		exp float64
	}{
		{
			costReport{
				Old: resourcesCost{
					CPU:     1,
					Memory:  2,
					Storage: 3,
				},
				New: resourcesCost{
					CPU:     1,
					Memory:  2,
					Storage: 3,
				},
			},
			0,
		},
		{
			costReport{
				Old: resourcesCost{
					CPU:     1,
					Memory:  2,
					Storage: 3,
				},
				New: resourcesCost{
					CPU:     10,
					Memory:  20,
					Storage: 30,
				},
			},
			54,
		},
		{
			costReport{
				Old: resourcesCost{
					CPU:     2,
					Memory:  4,
					Storage: 6,
				},
				New: resourcesCost{
					CPU:     1,
					Memory:  2,
					Storage: 3,
				},
			},
			-6,
		},
	}

	for _, tt := range tests {
		if g := tt.cr.Delta(); !eq(tt.exp, g) {
			t.Errorf("expecting cost report %v delta %.05f, got %0.5f", tt.cr, tt.exp, g)
		}
	}
}

func TestCostReports(t *testing.T) {
	crs := costReports{
		{
			New: resourcesCost{CPU: 1, Memory: 2, Storage: 3},
			Old: resourcesCost{CPU: 2, Memory: 4, Storage: 6},
		},
		{
			New: resourcesCost{CPU: 1, Memory: 2, Storage: 3},
			Old: resourcesCost{CPU: 2, Memory: 4, Storage: 6},
		},
	}

	var en, eo float64 = 12, 24

	n, o := crs.Totals()

	if !eq(n, en) || !eq(o, eo) {
		t.Errorf("expecting cost reports new & old totals %.05f, %.05f; got %.05f, %.05f", en, eo, n, o)
	}
}

type templateTestInput struct {
	cm     *CostModel
	oldReq Requirements
	newReq Requirements
}

func TestTemplate(t *testing.T) {
	// TODO this isn't really testing anything, just printing the
	// messages when verbose testing is enabled.

	h := requirementsHelpers(t)

	cm1 := &CostModel{
		Cluster: &Cluster{
			Name: "ops-us-east-0",
		},

		CPU: Cost{NonSpot: 1},
		RAM: Cost{NonSpot: 2},

		PersistentVolume: Cost{NonSpot: 0.0003},
	}
	cm2 := &CostModel{
		Cluster: &Cluster{
			Name: "prod-us-central-4",
		},

		CPU: Cost{NonSpot: 1},
		RAM: Cost{NonSpot: 2},

		PersistentVolume: Cost{NonSpot: 0.003},
	}

	req1 := Requirements{
		CPU:              h.cpu("500m"),
		Memory:           h.mem("2Gi"),
		PersistentVolume: h.pv("4Gi"),
	}
	req2 := Requirements{
		CPU:              h.cpu("1"),
		Memory:           h.mem("4Gi"),
		PersistentVolume: h.pv("8Gi"),
	}

	tests := map[string]struct {
		reports []templateTestInput
	}{
		"no changes": {
			[]templateTestInput{
				{cm: cm1, oldReq: req1, newReq: req1},
				{cm: cm2, oldReq: req2, newReq: req2},
			},
		},
		"one cluster unchanged": {
			[]templateTestInput{
				{cm: cm1, oldReq: req1, newReq: req1},
			},
		},
		"increase": {
			[]templateTestInput{
				{cm: cm1, oldReq: req1, newReq: req2},
			},
		},
		"decrease": {
			[]templateTestInput{
				{cm: cm1, oldReq: req2, newReq: req1},
			},
		},
		"mixed": {
			[]templateTestInput{
				{cm: cm1, oldReq: req2, newReq: req1},
				{cm: cm2, oldReq: req1, newReq: req2},
			},
		},
	}

	for n, tt := range tests {
		t.Run(n, func(t *testing.T) {
			var s strings.Builder
			r := New(&s, "markdown")

			for _, rep := range tt.reports {
				r.AddReport(rep.cm, rep.oldReq, rep.newReq)
			}

			if err := r.Write(); err != nil {
				t.Fatalf("unexpected: %v", err)
			}

			t.Log(s.String())
		})
	}
}

func TestCostRerpots_Sort(t *testing.T) {
	crs := costReports{
		costReport{
			Cluster: "foo",
			New:     resourcesCost{CPU: 1, Memory: 1, Storage: 1},
		},
		costReport{
			Cluster: "bar",
			New:     resourcesCost{CPU: 1, Memory: 1, Storage: 1},
			Old:     resourcesCost{CPU: 1, Memory: 1, Storage: 1},
		},
		costReport{
			Cluster: "quux",
			Old:     resourcesCost{CPU: 1, Memory: 1, Storage: 1},
		},
	}

	rand.Shuffle(len(crs), func(i, j int) {
		crs[i], crs[j] = crs[j], crs[i]
	})

	crs.Sort()

	for i, exp := range []string{"foo", "bar", "quux"} {
		if g := crs[i]; g.Cluster != exp {
			t.Errorf("expecting %s at index %d, got %s", exp, i, g.Cluster)
		}
	}
}
