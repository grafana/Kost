package github

import (
	"errors"
	"fmt"
)

var (
	ErrMissingAuth = errors.New("missing GitHub authentication environment variables")

	ErrInvalidAuth = errors.New("invalid GitHub authentication")
)

type Config struct {
	Token string `envconfig:"GITHUB_TOKEN"`

	AppPrivateKey     string `envconfig:"GITHUB_APP_PRIVATE_KEY"`
	AppID             int64  `envconfig:"GITHUB_APP_ID"`
	AppInstallationID int64  `envconfig:"GITHUB_APP_INSTALLATION_ID"`

	Owner string `envconfig:"GITHUB_REPOSITORY_OWNER" default:"grafana"`
	Repo  string `envconfig:"GITHUB_REPOSITORY_NAME" default:"deployment_tools"`
}

func (c Config) Validate() error {
	if c.Token != "" && c.AppID != 0 {
		return fmt.Errorf("%w: only one of token or application configuration are valid", ErrInvalidAuth)
	}
	if c.Token == "" && c.AppID == 0 {
		return fmt.Errorf("%w: missing token or application configuration", ErrMissingAuth)
	}

	if c.AppID > 0 && (c.AppPrivateKey == "" || c.AppInstallationID == 0) {
		return fmt.Errorf("%w: incomplete application configuration", ErrInvalidAuth)
	}

	return nil
}
