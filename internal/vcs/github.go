/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vcs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

const (
	// DefaultGitHubAPIURL is the default GitHub API endpoint
	DefaultGitHubAPIURL = "https://api.github.com/"

	// DefaultGitHubScorecardURL is the base URL for GitHub repositories in OpenSSF Scorecard
	DefaultGitHubScorecardURL = "github.com"
)

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	client       *github.Client
	scorecardURL string
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider(config *Config) (Provider, error) {
	var tc *http.Client
	if config.Token != "" {
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: config.Token},
		)
		tc = oauth2.NewClient(ctx, ts)
	}

	client := github.NewClient(tc)

	if config.BaseURL != "" {
		baseURL := config.BaseURL
		// Ensure base URL ends with a slash for go-github
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse base URL: %w", err)
		}
		client.BaseURL = u
	}

	return &GitHubProvider{
		client:       client,
		scorecardURL: DefaultGitHubScorecardURL,
	}, nil
}

// GetRepositories fetches all public repositories for a GitHub organization
func (p *GitHubProvider) GetRepositories(ctx context.Context, organization string) ([]string, error) {
	var allRepos []string
	opts := &github.RepositoryListByOrgOptions{
		Type:        "public",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := p.client.Repositories.ListByOrg(ctx, organization, opts)
		if err != nil {
			return nil, p.handleError(err)
		}

		// Filter and collect repository names
		for _, repo := range repos {
			if p.shouldIncludeRepository(repo) {
				allRepos = append(allRepos, repo.GetName())
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// GetRepositoryDetails fetches detailed information about a specific repository
func (p *GitHubProvider) GetRepositoryDetails(ctx context.Context, organization, repository string) (*Repository, error) {
	repo, _, err := p.client.Repositories.Get(ctx, organization, repository)
	if err != nil {
		return nil, p.handleError(err)
	}

	return p.convertToRepository(repo), nil
}

// GetProviderType returns the provider type
func (p *GitHubProvider) GetProviderType() ProviderType {
	return ProviderTypeGitHub
}

// GetScorecardURL returns the OpenSSF Scorecard URL for a GitHub repository
func (p *GitHubProvider) GetScorecardURL(organization, repository string) string {
	return fmt.Sprintf("%s/%s/%s", p.scorecardURL, organization, repository)
}

// handleError maps GitHub API errors to internal error types
func (p *GitHubProvider) handleError(err error) error {
	if err == nil {
		return nil
	}

	// Handle standard rate limit errors
	if rle, ok := err.(*github.RateLimitError); ok {
		rlErr := NewRateLimitError(ProviderTypeGitHub, err.Error()).
			WithRateLimitInfo(rle.Rate.Limit, rle.Rate.Remaining).
			WithResetTime(rle.Rate.Reset.Time)
		return rlErr
	}

	// Handle secondary rate limit (abuse) errors
	if ale, ok := err.(*github.AbuseRateLimitError); ok {
		rlErr := NewRateLimitError(ProviderTypeGitHub, err.Error())
		if ale.RetryAfter != nil {
			rlErr.WithRetryAfter(*ale.RetryAfter)
		}
		return rlErr
	}

	return err
}

// shouldIncludeRepository determines if a repository should be included in results
func (p *GitHubProvider) shouldIncludeRepository(repo *github.Repository) bool {
	if repo == nil {
		return false
	}
	return !repo.GetPrivate() && !repo.GetArchived() && !repo.GetDisabled() && !repo.GetFork()
}

// convertToRepository converts a GitHub repository to the generic Repository type
func (p *GitHubProvider) convertToRepository(repo *github.Repository) *Repository {
	return &Repository{
		Name:          repo.GetName(),
		FullName:      repo.GetFullName(),
		URL:           repo.GetHTMLURL(),
		DefaultBranch: repo.GetDefaultBranch(),
		IsPrivate:     repo.GetPrivate(),
		IsArchived:    repo.GetArchived(),
		IsFork:        repo.GetFork(),
		IsDisabled:    repo.GetDisabled(),
	}
}
