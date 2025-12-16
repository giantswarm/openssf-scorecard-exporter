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

package scorecard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultAPIEndpoint is the default OpenSSF Scorecard API endpoint
	DefaultAPIEndpoint = "https://api.securityscorecards.dev"
)

// Client is a client for interacting with OpenSSF Scorecard API
type Client struct {
	httpClient  *http.Client
	apiEndpoint string
}

// ScorecardData represents the scorecard data for a repository
type ScorecardData struct {
	// Overall score (0-10)
	Score float64

	// Individual check results
	Checks []Check

	// Timestamp of the scorecard data
	Timestamp time.Time

	// Repository metadata
	Repository string
	Commit     string
}

// Check represents an individual scorecard check result
type Check struct {
	Name   string
	Score  int
	Status string
	Reason string
}

// NewClient creates a new OpenSSF Scorecard API client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiEndpoint: DefaultAPIEndpoint,
	}
}

// GetScorecardData fetches scorecard data for a specific repository
// The vcsPath should be in the format expected by the scorecard API (e.g., "github.com/org/repo")
func (c *Client) GetScorecardData(ctx context.Context, vcsPath, token string) (*ScorecardData, error) {
	// OpenSSF Scorecard API endpoint format
	url := fmt.Sprintf("%s/projects/%s", c.apiEndpoint, vcsPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if token provided
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scorecard data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("scorecard data not found for %s", vcsPath)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var apiResponse struct {
		Score float64 `json:"score"`
		Date  string  `json:"date"`
		Repo  struct {
			Name   string `json:"name"`
			Commit string `json:"commit"`
		} `json:"repo"`
		Scorecard struct {
			Version string `json:"version"`
		} `json:"scorecard"`
		Checks []struct {
			Name          string `json:"name"`
			Score         int    `json:"score"`
			Reason        string `json:"reason"`
			Documentation struct {
				Short string `json:"short"`
				URL   string `json:"url"`
			} `json:"documentation"`
		} `json:"checks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse timestamp
	timestamp, err := time.Parse(time.RFC3339, apiResponse.Date)
	if err != nil {
		timestamp = time.Now() // fallback to current time
	}

	// Convert to our internal format
	data := &ScorecardData{
		Score:      apiResponse.Score,
		Repository: apiResponse.Repo.Name,
		Commit:     apiResponse.Repo.Commit,
		Timestamp:  timestamp,
		Checks:     make([]Check, 0, len(apiResponse.Checks)),
	}

	for _, check := range apiResponse.Checks {
		status := "Unknown"
		if check.Score >= 0 && check.Score < 5 {
			status = "Fail"
		} else if check.Score >= 5 {
			status = "Pass"
		}

		data.Checks = append(data.Checks, Check{
			Name:   check.Name,
			Score:  check.Score,
			Status: status,
			Reason: check.Reason,
		})
	}

	return data, nil
}
