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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	// DefaultGitHubAPIURL is the default GitHub API endpoint
	DefaultGitHubAPIURL = "https://api.github.com"

	// DefaultGitHubScorecardURL is the base URL for GitHub repositories in OpenSSF Scorecard
	DefaultGitHubScorecardURL = "github.com"
)

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	httpClient   *http.Client
	baseURL      string
	scorecardURL string
	token        string
}

// gitHubRepository represents a GitHub repository from the API
type gitHubRepository struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
	Archived      bool   `json:"archived"`
	Disabled      bool   `json:"disabled"`
	DefaultBranch string `json:"default_branch"`
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider(config *Config) (Provider, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = DefaultGitHubAPIURL
	}

	return &GitHubProvider{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:      baseURL,
		scorecardURL: DefaultGitHubScorecardURL,
		token:        config.Token,
	}, nil
}

// GetRepositories fetches all public repositories for a GitHub organization
func (p *GitHubProvider) GetRepositories(ctx context.Context, organization string) ([]string, error) {
	var allRepos []string
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("%s/orgs/%s/repos?type=public&per_page=%d&page=%d",
			p.baseURL, organization, perPage, page)

		repos, err := p.fetchRepositoryPage(ctx, url)
		if err != nil {
			return nil, err
		}

		// No more repositories
		if len(repos) == 0 {
			break
		}

		// Filter and collect repository names
		for _, repo := range repos {
			if p.shouldIncludeRepository(repo) {
				allRepos = append(allRepos, repo.Name)
			}
		}

		// If we got fewer repos than requested, we've reached the end
		if len(repos) < perPage {
			break
		}

		page++
	}

	return allRepos, nil
}

// GetRepositoryDetails fetches detailed information about a specific repository
func (p *GitHubProvider) GetRepositoryDetails(ctx context.Context, organization, repository string) (*Repository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", p.baseURL, organization, repository)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	p.addAuthHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repository details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var ghRepo gitHubRepository
	if err := json.NewDecoder(resp.Body).Decode(&ghRepo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return p.convertToRepository(ghRepo), nil
}

// GetProviderType returns the provider type
func (p *GitHubProvider) GetProviderType() ProviderType {
	return ProviderTypeGitHub
}

// GetScorecardURL returns the OpenSSF Scorecard URL for a GitHub repository
func (p *GitHubProvider) GetScorecardURL(organization, repository string) string {
	return fmt.Sprintf("%s/%s/%s", p.scorecardURL, organization, repository)
}

// fetchRepositoryPage fetches a single page of repositories from the GitHub API
func (p *GitHubProvider) fetchRepositoryPage(ctx context.Context, url string) ([]gitHubRepository, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	p.addAuthHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repositories: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limiting (HTTP 403 or 429)
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)

		// Extract rate limit information from headers
		rateLimitErr := NewRateLimitError(ProviderTypeGitHub, string(body))

		// X-RateLimit-Limit: maximum number of requests per hour
		if limit := resp.Header.Get("X-RateLimit-Limit"); limit != "" {
			if limitInt, err := strconv.Atoi(limit); err == nil {
				// X-RateLimit-Remaining: remaining requests
				remaining, _ := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
				rateLimitErr.WithRateLimitInfo(limitInt, remaining)
			}
		}

		// X-RateLimit-Reset: timestamp when rate limit resets
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if resetInt, err := strconv.ParseInt(reset, 10, 64); err == nil {
				resetTime := time.Unix(resetInt, 0)
				rateLimitErr.WithResetTime(resetTime)
			}
		}

		// Retry-After header (in seconds)
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				rateLimitErr.WithRetryAfter(time.Duration(seconds) * time.Second)
			}
		}

		return nil, rateLimitErr
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var repos []gitHubRepository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return repos, nil
}

// addAuthHeaders adds authentication headers to the request
func (p *GitHubProvider) addAuthHeaders(req *http.Request) {
	if p.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.token))
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}

// shouldIncludeRepository determines if a repository should be included in results
func (p *GitHubProvider) shouldIncludeRepository(repo gitHubRepository) bool {
	return !repo.Private && !repo.Archived && !repo.Disabled && !repo.Fork
}

// convertToRepository converts a GitHub repository to the generic Repository type
func (p *GitHubProvider) convertToRepository(ghRepo gitHubRepository) *Repository {
	return &Repository{
		Name:          ghRepo.Name,
		FullName:      ghRepo.FullName,
		URL:           ghRepo.HTMLURL,
		DefaultBranch: ghRepo.DefaultBranch,
		IsPrivate:     ghRepo.Private,
		IsArchived:    ghRepo.Archived,
		IsFork:        ghRepo.Fork,
		IsDisabled:    ghRepo.Disabled,
	}
}
