package costmodel

import (
	"context"
	"math"
)

// ReplicaSource describes which signal ResolveReplicas used.
type ReplicaSource int

const (
	// SourceManifest indicates the manifest's spec.replicas was authoritative
	// (no HPA targets the workload).
	SourceManifest ReplicaSource = iota
	// SourceObservedHPA indicates an HPA targets the workload and the manifest
	// replicas were replaced with the observed average from kube-state-metrics.
	SourceObservedHPA
)

// HPAResolver is the subset of *Client behavior ResolveReplicas needs.
// Lets policy be tested without httptest.
type HPAResolver interface {
	HPATargeting(ctx context.Context, cluster, namespace, kind, name string) (string, error)
	GetObservedReplicas(ctx context.Context, cluster, namespace, kind, name string) (float64, error)
}

var _ HPAResolver = (*Client)(nil)

// ResolveReplicas decides the right replica count for a workload at PR-cost-estimation time.
// If no HPA targets the workload, the manifest count is authoritative.
// If an HPA is detected, the observed 7d-average from kube-state-metrics is substituted
// (rounded to nearest int) — manifest replicas are a lie under HPA control.
// Errors from either query are returned so the caller can surface them rather than
// silently fall back to a wrong number.
func ResolveReplicas(ctx context.Context, r HPAResolver, cluster, namespace, kind, name string, manifestReplicas int) (int, ReplicaSource, error) {
	hpa, err := r.HPATargeting(ctx, cluster, namespace, kind, name)
	if err != nil {
		return 0, SourceManifest, err
	}
	if hpa == "" {
		return manifestReplicas, SourceManifest, nil
	}
	observed, err := r.GetObservedReplicas(ctx, cluster, namespace, kind, name)
	if err != nil {
		return 0, SourceObservedHPA, err
	}
	return int(math.Round(observed)), SourceObservedHPA, nil
}
