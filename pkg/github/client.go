package github

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v50/github"
	"github.com/gregjones/httpcache"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type Client struct {
	// GitHub REST API.
	c *github.Client

	// GitHub GraphQL API client.
	// Used to hide old comments.
	//
	// TODO I've been looking at the GraphQL API and it seems it could
	// be possible to use that to retrieve the comments and changed
	// files, however, due to the time-limitation of the hackathon we
	// won't explore that option.
	g *githubv4.Client
}

func NewClient(ctx context.Context, cfg Config) (Client, error) {
	var token = cfg.Token

	if cfg.AppID > 0 {
		slog.Info("Using GitHub app authentication")
		t, err := ghinstallation.NewAppsTransport(
			httpcache.NewMemoryCacheTransport(),
			cfg.AppID,
			[]byte(cfg.AppPrivateKey),
		)
		if err != nil {
			return Client{}, fmt.Errorf("creating GitHub application installation transport: %w", err)
		}

		c := github.NewClient(&http.Client{Transport: t})

		tok, res, err := c.Apps.CreateInstallationToken(ctx, cfg.AppInstallationID, nil)
		if err != nil {
			return Client{}, fmt.Errorf("getting GitHub application installation token: %w", err)
		}
		if res.StatusCode >= 400 {
			return Client{}, fmt.Errorf("unexpected status code from GitHub: %s", res.Status)
		}

		token = tok.GetToken()
	} else {
		slog.Info("Using GitHub token authentication")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	c := oauth2.NewClient(ctx, ts)

	return Client{
		c: github.NewClient(c),
		g: githubv4.NewClient(c),
	}, nil
}

func (c Client) Comment(ctx context.Context, org, repo string, nr int, comment string) error {
	_, _, err := c.c.Issues.CreateComment(ctx, org, repo, nr, &github.IssueComment{
		Body: github.String(comment),
	})
	return err
}

func (c Client) HideCommentsWithPrefix(ctx context.Context, org, repo string, nr int, prefix string) error {
	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	for {
		cs, res, err := c.c.Issues.ListComments(ctx, org, repo, nr, opts)
		if err != nil {
			return fmt.Errorf("retrieving PR comments: %w", err)
		}

		for _, cm := range cs {
			if !strings.HasPrefix(cm.GetBody(), prefix) {
				continue
			}

			// hide comment
			var m struct {
				MinimizeComment struct {
					MinimizedComment struct {
						IsMinimized githubv4.Boolean
					}
				} `graphql:"minimizeComment(input: $input)"`
			}

			i := githubv4.MinimizeCommentInput{
				SubjectID:  cm.GetNodeID(),
				Classifier: githubv4.ReportedContentClassifiersOutdated,
			}

			if err := c.g.Mutate(ctx, &m, i, nil); err != nil {
				// TODO don't fail here, just handle it better
				log.Printf("hiding comment %v: %v", cm.GetHTMLURL(), err)
			}
		}

		if res.NextPage == 0 {
			break
		}
		opts.Page = res.NextPage
	}

	return nil
}
