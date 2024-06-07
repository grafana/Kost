package main

import (
	"testing"

	"github.com/grafana/kost/pkg/git"
)

func TestFindCluster(t *testing.T) {
	tests := map[string]string{
		"flux/ops-us-east-0/exporters/Deployment-gcp-compute-exporter-grafanalabs-dev.yaml":          "ops-us-east-0",
		"flux/dev-us-central-0/default/StatefulSet-prometheus.yaml":                                  "dev-us-central-0",
		"flux/prod-us-central-0/default/StatefulSet-prometheus.yaml":                                 "prod-us-central-0",
		"flux-disabled/ops-us-east-0/ctank-migrations/StatefulSet-cassandra-chunk-extractor-us.yaml": "ops-us-east-0",
	}

	for f, exp := range tests {
		if got := findCluster(f); exp != got {
			t.Errorf("expecting cluster %s for file %s, got %s", exp, f, got)
		}
	}
}

func TestFindClusters(t *testing.T) {
	cf := git.ChangedFiles{
		Added: []string{
			"flux/ops-us-east-0/exporters/Deployment-gcp-compute-exporter-grafanalabs-dev.yaml",
			"flux/ops-us-east-0/exporters/Deployment-gcp-compute-exporter-grafanalabs-global.yaml",
		},
		Modified: []string{
			"flux/dev-us-central-0/default/StatefulSet-prometheus.yaml",
			"flux/prod-us-central-0/default/StatefulSet-prometheus.yaml",
		},
		Deleted: []string{
			"flux/prod-us-central-0/default/StatefulSet-prometheus.yaml",
		},
		Renamed: map[string]string{
			"flux/prod-eu-west-2/default/StatefulSet-prometheus.yaml": "flux/prod-eu-west-2/default/Deployment-prometheus.yaml",
		},
	}

	exp := []string{"dev-us-central-0", "ops-us-east-0", "prod-eu-west-2", "prod-us-central-0"}

	got := findClusters(cf)

	if e, g := len(exp), len(got); e != g {
		t.Fatalf("expecting %d clusters, got %d", e, g)
	}

	for i, e := range exp {
		if g := got[i]; e != g {
			t.Errorf("expecting cluster %s at index %d, got %s", e, i, g)
		}
	}
}
