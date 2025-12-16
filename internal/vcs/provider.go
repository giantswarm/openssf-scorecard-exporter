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
)

// ProviderType represents the type of version control system
type ProviderType string

const (
	// ProviderTypeGitHub represents GitHub as the VCS provider
	ProviderTypeGitHub ProviderType = "github"

	// ProviderTypeGitLab represents GitLab as the VCS provider (future)
	ProviderTypeGitLab ProviderType = "gitlab"

	// ProviderTypeBitbucket represents Bitbucket as the VCS provider (future)
	ProviderTypeBitbucket ProviderType = "bitbucket"
)

// Repository represents a version control repository
type Repository struct {
	// Name is the repository name
	Name string

	// FullName is the full name (e.g., "org/repo")
	FullName string

	// URL is the repository URL
	URL string

	// DefaultBranch is the default branch name
	DefaultBranch string

	// IsPrivate indicates if the repository is private
	IsPrivate bool

	// IsArchived indicates if the repository is archived
	IsArchived bool

	// IsFork indicates if the repository is a fork
	IsFork bool

	// IsDisabled indicates if the repository is disabled
	IsDisabled bool
}

// Provider defines the interface for version control system providers
type Provider interface {
	// GetRepositories fetches all repositories for an organization
	// Returns a list of repository names and any error encountered
	GetRepositories(ctx context.Context, organization string) ([]string, error)

	// GetRepositoryDetails fetches detailed information about a repository
	// Returns repository metadata and any error encountered
	GetRepositoryDetails(ctx context.Context, organization, repository string) (*Repository, error)

	// GetProviderType returns the type of this provider
	GetProviderType() ProviderType

	// GetScorecardURL returns the OpenSSF Scorecard URL format for this provider
	// Used to construct the URL for fetching scorecard data
	GetScorecardURL(organization, repository string) string
}

// Config represents configuration for a VCS provider
type Config struct {
	// Type is the provider type (github, gitlab, etc.)
	Type ProviderType

	// Token is the authentication token (optional)
	Token string

	// BaseURL is the base URL for the VCS API (for self-hosted instances)
	BaseURL string

	// Organization is the organization/group to monitor
	Organization string
}

// ProviderFactory creates VCS providers based on configuration
type ProviderFactory struct {
	providers map[ProviderType]func(*Config) (Provider, error)
}

// NewProviderFactory creates a new provider factory with registered providers
func NewProviderFactory() *ProviderFactory {
	factory := &ProviderFactory{
		providers: make(map[ProviderType]func(*Config) (Provider, error)),
	}

	// Register built-in providers
	factory.Register(ProviderTypeGitHub, NewGitHubProvider)

	return factory
}

// Register registers a provider constructor with the factory
func (f *ProviderFactory) Register(providerType ProviderType, constructor func(*Config) (Provider, error)) {
	f.providers[providerType] = constructor
}

// CreateProvider creates a new provider instance based on the configuration
func (f *ProviderFactory) CreateProvider(config *Config) (Provider, error) {
	constructor, exists := f.providers[config.Type]
	if !exists {
		return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
	}

	return constructor(config)
}

// GetSupportedProviders returns a list of supported provider types
func (f *ProviderFactory) GetSupportedProviders() []ProviderType {
	types := make([]ProviderType, 0, len(f.providers))
	for t := range f.providers {
		types = append(types, t)
	}
	return types
}
