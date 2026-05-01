package costmodel

import (
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseManifest(t *testing.T) {
	pq := func(s string) *resource.Quantity {
		t.Helper()
		q, err := resource.ParseQuantity(s)
		if err != nil {
			t.Fatalf("parsing CPU %q: %v", s, err)
		}
		return &q
	}

	cpu := func(s string) int64 {
		t.Helper()
		return pq(s).MilliValue()
	}

	mem := func(s string) int64 {
		t.Helper()
		return pq(s).Value()
	}

	pv := func(s string) int64 {
		t.Helper()
		return pq(s).Value()
	}

	tests := map[string]Requirements{
		"Deployment": {
			CPUPerPod:    cpu("500m"),
			MemoryPerPod: mem("1000Mi"),
			Replicas:     1,
			Kind:         "Deployment",
			Namespace:    "opencost",
			Name:         "prom-label-proxy",
		},
		"Job": {
			CPUPerPod:    cpu("50m"),
			MemoryPerPod: mem("200Mi"),
			Replicas:     1,
			Kind:         "Job",
			Namespace:    "hosted-grafana",
			Name:         "hosted-grafana-source-ips-update-27973440",
		},

		"StatefulSet": {
			CPUPerPod:              cpu("1"),
			MemoryPerPod:           mem("4Gi"),
			PersistentVolumePerPod: pv("32Gi"),
			Replicas:               1,
			Kind:                   "StatefulSet",
			Namespace:              "opencost",
			Name:                   "opencost",
		},

		"DaemonSet": {
			CPUPerPod:    cpu("50m"),
			MemoryPerPod: mem("50Mi"),
			Replicas:     1,
			Kind:         "DaemonSet",
			Namespace:    "conntrack-exporter",
			Name:         "conntrack-exporter",
		},

		"Pod": {
			CPUPerPod:    cpu("45"),
			MemoryPerPod: mem("320Gi"),
			Replicas:     1,
			Kind:         "Pod",
			Namespace:    "default",
			Name:         "prometheus-0",
		},

		// Multi-container manifests
		"StatefulSet-with-2-containers": {
			CPUPerPod:              cpu("1") + cpu("10m"),
			MemoryPerPod:           mem("4Gi") + mem("55M"),
			PersistentVolumePerPod: pv("32Gi"),
			Replicas:               1,
			Kind:                   "StatefulSet",
			Namespace:              "opencost",
			Name:                   "opencost",
		},

		// With replicas
		"StatefulSet-with-replicas": {
			CPUPerPod:              cpu("45"),
			MemoryPerPod:           mem("320Gi"),
			PersistentVolumePerPod: pv("7500Gi"),
			Replicas:               2,
			Kind:                   "StatefulSet",
			Namespace:              "default",
			Name:                   "prometheus",
		},
	}

	for kind, exp := range tests {
		t.Run(kind, func(t *testing.T) {
			src, err := os.ReadFile("testdata/resource/" + kind + ".json")
			if err != nil {
				t.Fatalf("unexpected error reading manifest file: %v", err)
			}

			got, err := ParseManifest(src, &CostModel{})
			if err != nil {
				t.Fatalf("unexpected error parsing manifest: %v", err)
			}

			if exp != got {
				t.Fatalf("wrong parsed values:\nexp: %#v\ngot: %#v", exp, got)
			}
		})
	}

	t.Run("panics on StatefulSet without replicas", func(t *testing.T) {
		// This was first reported on Slack and it was failing all
		// Alertmanager PRs, blocking auto-rollouts.
		// https://raintank-corp.slack.com/archives/C051ALUR9LG/p1681286565973609
		src, err := os.ReadFile("testdata/resource/StatefulSet-without-replicas.yaml")
		if err != nil {
			t.Fatalf("unexpected error reading manifest file: %v", err)
		}

		got, err := ParseManifest(src, &CostModel{})
		if err != nil {
			t.Fatalf("unexpected error parsing manifest: %v", err)
		}

		exp := Requirements{
			CPUPerPod:              cpu("200m"),
			MemoryPerPod:           mem("1Gi"),
			PersistentVolumePerPod: pv("100Gi"),
			Replicas:               1,
			Kind:                   "StatefulSet",
			Namespace:              "alertmanager",
			Name:                   "alertmanager",
		}

		if exp != got {
			t.Fatalf("wrong parsed values:\nexp: %#v\ngot: %#v", exp, got)
		}
	})

	t.Run("panics on Daemonset if costmodel is nil", func(t *testing.T) {
		src, err := os.ReadFile("testdata/resource/DaemonSet.json")
		if err != nil {
			t.Fatalf("unexpected error reading manifest file: %v", err)
		}

		_, err = ParseManifest(src, nil)
		if err == nil {
			t.Fatalf("expected error parsing manifest")
		}
	})
}

func TestDelta(t *testing.T) {
	tests := map[string]struct {
		from Requirements
		to   Requirements
		want Requirements
	}{
		"Two equal resources should result in all values being zero": {
			from: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			to: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			want: Requirements{
				CPUPerPod:    0,
				MemoryPerPod: 0,
			},
		},
		"Two resources with more resources should result in the correct positive delta": {
			from: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			to: Requirements{
				CPUPerPod:    2,
				MemoryPerPod: 4,
			},
			want: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
		},
		"To resources with less resources should result in the correct negative delta": {
			from: Requirements{
				CPUPerPod:    2,
				MemoryPerPod: 4,
			},
			to: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			want: Requirements{
				CPUPerPod:    -1,
				MemoryPerPod: -2,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Delta(test.from, test.to); got != test.want {
				t.Errorf("Delta() = %v, want %v", got, test.want)
			}
		})
	}
}
