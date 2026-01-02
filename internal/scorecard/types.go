package scorecard

import "time"

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

// APIResponse represents the raw response from the OpenSSF Scorecard API
type APIResponse struct {
	Score     float64    `json:"score"`
	Date      string     `json:"date"`
	Repo      APIRepo    `json:"repo"`
	Scorecard APIMeta    `json:"scorecard"`
	Checks    []APICheck `json:"checks"`
}

// APIRepo represents repository metadata in the API response
type APIRepo struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// APIMeta represents scorecard metadata in the API response
type APIMeta struct {
	Version string `json:"version"`
}

// APICheck represents an individual check in the API response
type APICheck struct {
	Name          string           `json:"name"`
	Score         int              `json:"score"`
	Reason        string           `json:"reason"`
	Documentation APIDocumentation `json:"documentation"`
}

// APIDocumentation represents check documentation in the API response
type APIDocumentation struct {
	Short string `json:"short"`
	URL   string `json:"url"`
}
