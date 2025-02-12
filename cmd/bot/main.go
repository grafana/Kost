package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/grafana/kost/pkg/costmodel"

	"github.com/grafana/kost/pkg/git"
	"github.com/grafana/kost/pkg/github"
)

//go:embed comment.md
//nolint:unused
var commentTemplate string

var (
	// ErrNoClustersFound is returned when no clusters are found in the changed files
	ErrNoClustersFound = errors.New("no clusters found for changed file")
)

func main() {
	ctx := context.TODO()
	start := time.Now()
	defer func() {
		slog.Info("finished", "method", "main", "duration", time.Since(start))
	}()

	if err := realMain(ctx); err != nil {
		slog.Error("failed to run", "method", "main", "error", err)
		// TODO: Once we have a better handle on the app, let's exit 1
		os.Exit(0)
	}
}

func realMain(ctx context.Context) error {
	start := time.Now()
	cfg, err := parseConfig()
	if err != nil {
		return fmt.Errorf("parsing configuration: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return fmt.Errorf("validating configuration: %w", err)
	}

	prometheusClients, err := costmodel.NewClients(
		&costmodel.ClientConfig{
			Address:                     cfg.Prometheus.Prod.Address,
			HTTPConfigFile:              cfg.Prometheus.Prod.HTTPConfigFile,
			Username:                    cfg.Prometheus.Prod.Username,
			Password:                    cfg.Prometheus.Prod.Password,
			UseCloudCostExporterMetrics: cfg.UseCloudCostExporterMetrics,
		},
		&costmodel.ClientConfig{
			Address:                     cfg.Prometheus.Dev.Address,
			HTTPConfigFile:              cfg.Prometheus.Dev.HTTPConfigFile,
			Username:                    cfg.Prometheus.Dev.Username,
			Password:                    cfg.Prometheus.Dev.Password,
			UseCloudCostExporterMetrics: cfg.UseCloudCostExporterMetrics,
		})
	if err != nil {
		return fmt.Errorf("creating cost model client: %w", err)
	}

	repo := git.NewRepository(cfg.Manifests.RepoPath)

	oldCommit, err := repo.GetCommit(ctx, "HEAD^")
	if err != nil {
		return fmt.Errorf("getting commit: %w", err)
	}

	newCommit, err := repo.GetCommit(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("getting commit: %w", err)
	}

	cf, err := repo.ChangedFiles(ctx, oldCommit, newCommit)
	if err != nil {
		return err
	}

	var (
		comment  strings.Builder
		reporter = costmodel.New(&comment, "markdown")
	)

	costPerCluster := make(map[string]*costmodel.CostModel)
	var mu sync.RWMutex
	var warnings []error

	clusters := findClusters(cf)
	g := &errgroup.Group{}
	g.SetLimit(cfg.Prometheus.Prod.MaxConcurrentQueries)
	for _, cluster := range clusters {
		mu.RLock()
		_, ok := costPerCluster[cluster]
		mu.RUnlock()
		if !ok {
			cluster := cluster //  https://golang.org/doc/faq#closures_and_goroutines
			g.Go(func() error {
				slog.Info("fetching cost model for cluster", "cluster", cluster)
				cost, err := prometheusClients.GetClusterCosts(ctx, cluster)
				mu.Lock()
				if err != nil {
					// TODO here we should probably return an error like below
					warnings = append(warnings, fmt.Errorf("fetching cost model for cluster %s: %w", cluster, err))
				}
				costPerCluster[cluster] = cost
				mu.Unlock()
				slog.Info("finished fetching cost model for cluster", "cluster", cluster, "duration", time.Since(start))
				return nil
			})
		}
	}

	// We currently don't return an error if one of the goroutines fails
	_ = g.Wait()

	parseManifest := func(commit, path string) (*costmodel.CostModel, costmodel.Requirements, error) {
		slog.Info("parseManifest", "commit", commit, "path", path)
		var req costmodel.Requirements

		src, err := repo.Contents(ctx, commit, path)
		if err != nil {
			return nil, req, fmt.Errorf("checking %s:%s contents: %w", commit, path, err)
		}

		cm := costPerCluster[findCluster(path)]
		if cm == nil {
			slog.Error("no cost model found for path", "path", path)
			return nil, req, ErrNoClustersFound
		}
		req, err = costmodel.ParseManifest(src, cm)
		if err != nil {
			return nil, req, fmt.Errorf("parsing manifest %s:%s: %w", commit, path, err)
		}

		return cm, req, nil
	}

	start = time.Now()
	// Added files only increase
	for _, f := range cf.Added {
		cost, req, err := parseManifest(newCommit, f)
		if errors.Is(err, costmodel.ErrUnknownKind) || errors.Is(err, ErrNoClustersFound) {
			slog.Error("parsing manifest", "path", f, "error", err)
			continue
		} else if err != nil {
			return fmt.Errorf("added manifests: %w", err)
		}

		reporter.AddReport(cost, costmodel.Requirements{}, req)
	}
	slog.Info("Finished processing added files", "count", len(cf.Added), "duration", time.Since(start))

	start = time.Now()
	// Deleted files only decrease
	for _, f := range cf.Deleted {
		cost, req, err := parseManifest(oldCommit, f) // get contents at previous commit
		if errors.Is(err, costmodel.ErrUnknownKind) || errors.Is(err, ErrNoClustersFound) {
			slog.Error("parsing manifest", "path", f, "error", err)
			continue
		} else if err != nil {
			return fmt.Errorf("deleted manifest: %w", err)
		}

		reporter.AddReport(cost, req, costmodel.Requirements{})
	}
	slog.Info("Finished processing deleted files", "count", len(cf.Deleted), "duration", time.Since(start))

	// Modified files
	for _, f := range cf.Modified {
		cost, from, err := parseManifest(oldCommit, f) // get contents at previous commit
		if errors.Is(err, costmodel.ErrUnknownKind) || errors.Is(err, ErrNoClustersFound) {
			slog.Error("parsing manifest", "path", f, "error", err)
			continue
		} else if err != nil {
			return fmt.Errorf("previous manifest: %w", err)
		}

		_, to, err := parseManifest(newCommit, f)
		if errors.Is(err, costmodel.ErrUnknownKind) || errors.Is(err, ErrNoClustersFound) {
			slog.Error("parsing manifest", "path", f, "error", err)
			continue
		} else if err != nil {
			return fmt.Errorf("new manifest: %w", err)
		}

		reporter.AddReport(cost, from, to)
	}
	slog.Info("Finished processing modified files", "count", len(cf.Modified), "duration", time.Since(start))

	for old, f := range cf.Renamed {
		cost, from, err := parseManifest(oldCommit, old) // get contents at previous commit
		if errors.Is(err, costmodel.ErrUnknownKind) || errors.Is(err, ErrNoClustersFound) {
			slog.Error("parsing manifest", "path", old, "error", err)
			continue
		} else if err != nil {
			return fmt.Errorf("manifest before renaming: %w", err)
		}

		// TODO here we assume the cluster of the renamed file is the
		// same, but it could be a new one.
		_, to, err := parseManifest(newCommit, f)
		if errors.Is(err, costmodel.ErrUnknownKind) || errors.Is(err, ErrNoClustersFound) {
			slog.Error("parsing manifest", "path", f, "error", err)
			continue
		} else if err != nil {
			return fmt.Errorf("manifest after renaming: %w", err)
		}

		reporter.AddReport(cost, from, to)
	}
	slog.Info("Finished processing renamed files", "count", len(cf.Renamed), "duration", time.Since(start))

	if err := reporter.Write(); errors.Is(err, costmodel.ErrNoReports) {
		return nil
	} else if err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	slog.Info("Finished", "method", "cost-model:write-report", "duration", time.Since(start))

	gh, err := github.NewClient(ctx, cfg.GitHub)
	if err != nil {
		return fmt.Errorf("creating GitHub client: %w", err)
	}

	start = time.Now()
	if err := gh.HideCommentsWithPrefix(ctx, cfg.GitHub.Owner, cfg.GitHub.Repo, cfg.PR, costmodel.CommentPrefix); err != nil {
		// Here we log this because there's no point in stopping the
		// program if it can't hide old comments.
		warnings = append(warnings, fmt.Errorf("hiding previous GitHub comments: %w", err))
	}
	slog.Info("Finished", "method", "GitHub:hide-previous-comments", "duration", time.Since(start))

	start = time.Now()
	if err := gh.Comment(ctx, cfg.GitHub.Owner, cfg.GitHub.Repo, cfg.PR, comment.String()); err != nil {
		return fmt.Errorf("commenting on GitHub: %w", err)
	}
	slog.Info("Finished", "method", "GitHub:comment", "duration", time.Since(start))

	if len(warnings) > 0 {
		fmt.Fprintln(os.Stderr, "WARNINGS:")
		for _, w := range warnings {
			slog.Error("warning", "message", w)
		}
	}

	return nil
}

func findCluster(path string) string {
	ps := strings.SplitN(path, "/", 3)
	// Prevent panic if the path is not in the expected format
	if len(ps) <= 1 {
		return ""
	}
	return ps[1]
}

func findClusters(cf git.ChangedFiles) []string {
	cs := make(map[string]struct{})

	var fs []string

	fs = append(fs, cf.Added...)
	fs = append(fs, cf.Modified...)
	fs = append(fs, cf.Deleted...)
	for o, n := range cf.Renamed {
		fs = append(fs, o, n)
	}

	for _, f := range fs {
		// TODO find a better way to find the clusters
		if strings.HasPrefix(f, "flux/") || strings.HasPrefix(f, "flux-disabled/") {
			cs[findCluster(f)] = struct{}{}
		}
	}

	keys := make([]string, 0, len(cs))
	for c := range cs {
		keys = append(keys, c)
	}

	sort.Strings(keys)

	return keys
}
