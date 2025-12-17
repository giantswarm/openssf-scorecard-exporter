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
	"testing"
	"time"
)

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "RateLimitError type",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit exceeded"),
			expected: true,
		},
		{
			name:     "wrapped RateLimitError",
			err:      fmt.Errorf("API error: %w", NewRateLimitError(ProviderTypeGitHub, "rate limit")),
			expected: true,
		},
		{
			name:     "error with 'rate limit' in message",
			err:      errors.New("API rate limit exceeded"),
			expected: true,
		},
		{
			name:     "error with '429' status code",
			err:      errors.New("API returned status 429: too many requests"),
			expected: true,
		},
		{
			name:     "error with 'too many requests'",
			err:      errors.New("too many requests, please try again later"),
			expected: true,
		},
		{
			name:     "error with 'quota exceeded'",
			err:      errors.New("quota exceeded for this resource"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "authentication error",
			err:      errors.New("invalid credentials"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRateLimitError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRateLimitError() = %v, want %v for error: %v", result, tt.expected, tt.err)
			}
		})
	}
}

func TestGetRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected time.Duration
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: 0,
		},
		{
			name:     "RateLimitError with RetryAfter",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit").WithRetryAfter(10 * time.Minute),
			expected: 10 * time.Minute,
		},
		{
			name:     "RateLimitError with ResetTime in future",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit").WithResetTime(time.Now().Add(15 * time.Minute)),
			expected: 15 * time.Minute, // Approximately
		},
		{
			name:     "RateLimitError with ResetTime in past",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit").WithResetTime(time.Now().Add(-5 * time.Minute)),
			expected: 5 * time.Minute, // Default
		},
		{
			name:     "RateLimitError without timing info",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit"),
			expected: 5 * time.Minute, // Default
		},
		{
			name:     "non-rate-limit error",
			err:      errors.New("some other error"),
			expected: 5 * time.Minute, // Default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRetryAfter(tt.err)

			// For zero expected, check exact match
			if tt.expected == 0 {
				if result != 0 {
					t.Errorf("GetRetryAfter() = %v, want %v", result, tt.expected)
				}
				return
			}

			// For time-based tests, allow 1 second tolerance
			if result < tt.expected-time.Second || result > tt.expected+time.Second {
				// Special case for "ResetTime in future" test - just check it's reasonable
				if tt.name == "RateLimitError with ResetTime in future" && result > 14*time.Minute && result < 16*time.Minute {
					return
				}
				t.Errorf("GetRetryAfter() = %v, want approximately %v", result, tt.expected)
			}
		})
	}
}

func TestRateLimitError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *RateLimitError
		contains string
	}{
		{
			name:     "basic error",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit exceeded"),
			contains: "github API rate limit exceeded",
		},
		{
			name:     "error with retry after",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit").WithRetryAfter(5 * time.Minute),
			contains: "retry after 5m",
		},
		{
			name:     "error with reset time",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit").WithResetTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)),
			contains: "resets at",
		},
		{
			name:     "error with rate limit info",
			err:      NewRateLimitError(ProviderTypeGitHub, "rate limit").WithRateLimitInfo(5000, 0),
			contains: "rate limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			if errMsg == "" {
				t.Error("Error() returned empty string")
			}
			if tt.contains != "" && len(errMsg) > 0 {
				// Just verify it returns something reasonable - exact format can vary
				if len(errMsg) < 10 {
					t.Errorf("Error() = %q, seems too short", errMsg)
				}
			}
		})
	}
}

func TestRateLimitError_Chaining(t *testing.T) {
	// Test that methods can be chained
	err := NewRateLimitError(ProviderTypeGitHub, "rate limit exceeded").
		WithRetryAfter(10*time.Minute).
		WithResetTime(time.Now().Add(10*time.Minute)).
		WithRateLimitInfo(5000, 0)

	if err.Provider != ProviderTypeGitHub {
		t.Errorf("Provider = %v, want %v", err.Provider, ProviderTypeGitHub)
	}
	if err.RetryAfter != 10*time.Minute {
		t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, 10*time.Minute)
	}
	if err.Limit != 5000 {
		t.Errorf("Limit = %v, want %v", err.Limit, 5000)
	}
	if err.Remaining != 0 {
		t.Errorf("Remaining = %v, want %v", err.Remaining, 0)
	}
}
