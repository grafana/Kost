package costmodel

import (
	"context"
	"errors"
	"testing"
)

type fakeResolver struct {
	hpaName     string
	hpaErr      error
	observed    float64
	observedErr error

	hpaCalls      int
	observedCalls int
}

func (f *fakeResolver) HPATargeting(_ context.Context, _, _, _, _ string) (string, error) {
	f.hpaCalls++
	return f.hpaName, f.hpaErr
}

func (f *fakeResolver) GetObservedReplicas(_ context.Context, _, _, _, _ string) (float64, error) {
	f.observedCalls++
	return f.observed, f.observedErr
}

func TestResolveReplicas(t *testing.T) {
	ctx := context.Background()

	t.Run("no HPA returns manifest replicas", func(t *testing.T) {
		fr := &fakeResolver{hpaName: ""}
		got, src, err := ResolveReplicas(ctx, fr, "c", "ns", "Deployment", "foo", 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 3 {
			t.Errorf("got replicas %d, want 3", got)
		}
		if src != SourceManifest {
			t.Errorf("got source %v, want SourceManifest", src)
		}
		if fr.observedCalls != 0 {
			t.Errorf("observed should not be queried when no HPA, got %d calls", fr.observedCalls)
		}
	})

	t.Run("HPA managed substitutes observed and rounds", func(t *testing.T) {
		fr := &fakeResolver{hpaName: "keda-hpa-foo", observed: 878.6}
		got, src, err := ResolveReplicas(ctx, fr, "c", "ns", "Deployment", "foo", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 879 {
			t.Errorf("got replicas %d, want 879 (rounded from 878.6)", got)
		}
		if src != SourceObservedHPA {
			t.Errorf("got source %v, want SourceObservedHPA", src)
		}
	})

	t.Run("HPA detection error propagates without querying observed", func(t *testing.T) {
		sentinel := errors.New("boom")
		fr := &fakeResolver{hpaErr: sentinel}
		got, _, err := ResolveReplicas(ctx, fr, "c", "ns", "Deployment", "foo", 5)
		if !errors.Is(err, sentinel) {
			t.Errorf("expected error to wrap sentinel, got %v", err)
		}
		if got != 0 {
			t.Errorf("got replicas %d, want 0 on error", got)
		}
		if fr.observedCalls != 0 {
			t.Errorf("observed should not be queried on detection error, got %d calls", fr.observedCalls)
		}
	})

	t.Run("HPA detected but observed empty returns ErrNoResults", func(t *testing.T) {
		fr := &fakeResolver{hpaName: "keda-hpa-foo", observedErr: ErrNoResults}
		got, _, err := ResolveReplicas(ctx, fr, "c", "ns", "Deployment", "foo", 5)
		if !errors.Is(err, ErrNoResults) {
			t.Errorf("expected ErrNoResults, got %v", err)
		}
		if got != 0 {
			t.Errorf("got replicas %d, want 0 on error", got)
		}
	})

	t.Run("HPA detected and observed transport error propagates", func(t *testing.T) {
		fr := &fakeResolver{hpaName: "keda-hpa-foo", observedErr: ErrBadQuery}
		_, _, err := ResolveReplicas(ctx, fr, "c", "ns", "Deployment", "foo", 5)
		if !errors.Is(err, ErrBadQuery) {
			t.Errorf("expected ErrBadQuery, got %v", err)
		}
	})

	t.Run("rounds 0.5 up", func(t *testing.T) {
		fr := &fakeResolver{hpaName: "h", observed: 10.5}
		got, _, err := ResolveReplicas(ctx, fr, "c", "ns", "Deployment", "foo", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 11 {
			t.Errorf("got %d, want 11", got)
		}
	})
}
