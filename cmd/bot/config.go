package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"

	"github.com/grafana/kost/pkg/github"
)

type promConfig struct {
	Address              string `envconfig:"PROMETHEUS_ADDRESS" default:"http://localhost:9090/" required:"true"`
	HTTPConfigFile       string `envconfig:"HTTP_CONFIG_FILE"`
	Username             string `envconfig:"MIMIR_USER_ID"`
	Password             string `envconfig:"MIMIR_USER_PASSWORD"`
	MaxConcurrentQueries int    `envconfig:"MAX_CONCURRENT_QUERIES" default:"-1"` // -1 means unlimited
}

type config struct {
	Manifests struct {
		RepoPath string `envconfig:"KUBE_MANIFESTS_PATH" required:"true"`
		Head     string `envconfig:"KUBE_MANIFESTS_SHA1"`
	}

	Prometheus struct {
		Prod, Dev promConfig
	}

	GitHub github.Config

	IsCI     bool   `envconfig:"CI"`
	PR       int    `envconfig:"DRONE_PULL_REQUEST" required:"true"`
	Event    string `envconfig:"DRONE_BUILD_EVENT"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
}

const pullRequestEvent = "pull_request"

func parseConfig() (config, error) {
	var c config
	if err := envconfig.Process("", &c); err != nil {
		return c, err
	}
	if err := envconfig.Process("DEV", &c.Prometheus.Dev); err != nil {
		return c, fmt.Errorf("parsing envconfig for Prometheus dev: %w", err)
	}
	return c, nil
}

func (c config) validate() error {
	if !c.IsCI {
		return errors.New("this can only be run in CI")
	}

	if c.PR == 0 || c.Event != pullRequestEvent {
		return errors.New("expecting DRONE_PULL_REQUEST and DRONE_BUILD_EVENT to be set")
	}

	if err := c.GitHub.Validate(); err != nil {
		return fmt.Errorf("github configuration: %w", err)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(c.LogLevel)); err != nil {
		return fmt.Errorf("parsing log level: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	// TODO check repo exists

	return nil
}
