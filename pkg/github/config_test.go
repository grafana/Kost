package github_test

import (
	"errors"
	"testing"

	"github.com/kelseyhightower/envconfig"

	"github.com/grafana/deployment_tools/docker/k8s-cost-estimator/pkg/github"
)

func TestConfigValidate(t *testing.T) {
	// clear GitHub environment variables
	// keys := []string{"GITHUB_TOKEN", "GITHUB_APP_PRIVATE_KEY", "GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_REPOSITORY_OWNER", "GITHUB_REPOSITORY_NAME"}

	tests := map[string]struct {
		env map[string]string
		err error
	}{
		"missing authentication": {
			map[string]string{}, github.ErrMissingAuth,
		},

		"mixed authentications": {
			map[string]string{
				"GITHUB_TOKEN":  "abc123",
				"GITHUB_APP_ID": "1024",
			},
			github.ErrInvalidAuth,
		},

		"missing application private key": {
			map[string]string{
				"GITHUB_APP_ID":              "1024",
				"GITHUB_APP_INSTALLATION_ID": "2048",
			},
			github.ErrInvalidAuth,
		},

		"missing application installation ID": {
			map[string]string{
				"GITHUB_APP_ID":          "1024",
				"GITHUB_APP_PRIVATE_KEY": "def456",
			},
			github.ErrInvalidAuth,
		},

		"valid PAT": {
			map[string]string{
				"GITHUB_TOKEN": "abc123",
			},
			nil,
		},

		"valid application configuration": {
			map[string]string{
				"GITHUB_APP_ID":              "1024",
				"GITHUB_APP_PRIVATE_KEY":     "def456",
				"GITHUB_APP_INSTALLATION_ID": "2048",
			},
			nil,
		},
	}

	for n, tt := range tests {
		t.Run(n, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg := github.Config{}

			if err := envconfig.Process("", &cfg); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			err := cfg.Validate()

			if !errors.Is(err, tt.err) {
				t.Fatalf("expecting validation to fail with %v, got %v", tt.err, err)
			}

			t.Log("validation error message:", err)
		})
	}
}
