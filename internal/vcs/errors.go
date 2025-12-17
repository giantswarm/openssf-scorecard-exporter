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
	"errors"
	"fmt"
	"strings"
	"time"
)

// RateLimitError represents an error due to VCS API rate limiting
type RateLimitError struct {
	// Provider is the VCS provider that returned the rate limit
	Provider ProviderType

	// Message is the error message from the API
	Message string

	// RetryAfter is the duration to wait before retrying (if known)
	RetryAfter time.Duration

	// Limit is the rate limit (requests per period)
	Limit int

	// Remaining is the number of requests remaining
	Remaining int

	// ResetTime is when the rate limit resets
	ResetTime time.Time
}

// Error implements the error interface
func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s API rate limit exceeded: %s (retry after %v)",
			e.Provider, e.Message, e.RetryAfter)
	}
	if !e.ResetTime.IsZero() {
		return fmt.Sprintf("%s API rate limit exceeded: %s (resets at %v)",
			e.Provider, e.Message, e.ResetTime.Format(time.RFC3339))
	}
	return fmt.Sprintf("%s API rate limit exceeded: %s", e.Provider, e.Message)
}

// IsRateLimitError checks if an error is a rate limit error
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's our custom RateLimitError type
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		return true
	}

	// Check for common rate limit indicators in error message
	errMsg := strings.ToLower(err.Error())
	rateLimitIndicators := []string{
		"rate limit",
		"rate-limit",
		"ratelimit",
		"too many requests",
		"429",
		"quota exceeded",
		"api rate limit exceeded",
	}

	for _, indicator := range rateLimitIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}

	return false
}

// GetRetryAfter extracts the retry duration from a rate limit error
// Returns a default duration if none is specified
func GetRetryAfter(err error) time.Duration {
	if err == nil {
		return 0
	}

	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		if rateLimitErr.RetryAfter > 0 {
			return rateLimitErr.RetryAfter
		}
		if !rateLimitErr.ResetTime.IsZero() {
			duration := time.Until(rateLimitErr.ResetTime)
			if duration > 0 {
				return duration
			}
		}
	}

	// Default retry after 5 minutes if no specific duration is known
	return 5 * time.Minute
}

// NewRateLimitError creates a new rate limit error
func NewRateLimitError(provider ProviderType, message string) *RateLimitError {
	return &RateLimitError{
		Provider: provider,
		Message:  message,
	}
}

// WithRetryAfter sets the retry after duration
func (e *RateLimitError) WithRetryAfter(duration time.Duration) *RateLimitError {
	e.RetryAfter = duration
	return e
}

// WithResetTime sets the rate limit reset time
func (e *RateLimitError) WithResetTime(resetTime time.Time) *RateLimitError {
	e.ResetTime = resetTime
	return e
}

// WithRateLimitInfo sets the rate limit information
func (e *RateLimitError) WithRateLimitInfo(limit, remaining int) *RateLimitError {
	e.Limit = limit
	e.Remaining = remaining
	return e
}
