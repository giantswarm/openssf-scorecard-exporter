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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/giantswarm/openssf-scorecard-exporter/internal/metrics"
	"github.com/giantswarm/openssf-scorecard-exporter/internal/scorecard"
	"github.com/giantswarm/openssf-scorecard-exporter/internal/utils"
	"github.com/giantswarm/openssf-scorecard-exporter/internal/vcs"
)

const (
	// ScorecardLabelKey is the label key that identifies ConfigMaps to reconcile
	ScorecardLabelKey = "openssf-scorecard.giantswarm.io/enabled"

	// OrganizationKey is the ConfigMap data key for the organization/group
	OrganizationKey = "organization"

	// ProviderTypeKey is the ConfigMap data key for the VCS provider type
	ProviderTypeKey = "providerType"

	// TokenSecretKey is the ConfigMap data key for the VCS token secret reference
	TokenSecretKey = "tokenSecret"

	// TokenSecretKeyName is the ConfigMap data key for the token secret key name
	TokenSecretKeyName = "tokenSecretKey"

	// BaseURLKey is the ConfigMap data key for custom VCS API base URL
	BaseURLKey = "baseURL"
)

// ConfigMapReconciler reconciles ConfigMap objects for OpenSSF Scorecard
type ConfigMapReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	ScorecardClient  *scorecard.Client
	MetricsCollector *metrics.Collector
	ProviderFactory  *vcs.ProviderFactory
	MaxJitterPercent int
	RequeueInterval  time.Duration
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch

// Reconcile is the main reconciliation loop for ConfigMaps
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ConfigMap
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &configMap); err != nil {
		// ConfigMap not found, likely deleted. Remove metrics for this config.
		r.MetricsCollector.RemoveMetricsForConfig(req.NamespacedName.String())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling ConfigMap for OpenSSF Scorecard",
		"namespace", configMap.Namespace,
		"name", configMap.Name)

	// Extract organization from ConfigMap
	organization, ok := configMap.Data[OrganizationKey]
	if !ok || organization == "" {
		logger.Error(fmt.Errorf("missing required field"), "ConfigMap must have 'organization' key in data")
		return ctrl.Result{}, nil
	}

	// Extract provider type (defaults to GitHub)
	providerType := vcs.ProviderType(configMap.Data[ProviderTypeKey])
	if providerType == "" {
		providerType = vcs.ProviderTypeGitHub
	}

	// Extract optional base URL for custom VCS instances
	baseURL := configMap.Data[BaseURLKey]

	// Extract optional VCS token from referenced secret
	var vcsToken string
	if tokenSecretName, hasToken := configMap.Data[TokenSecretKey]; hasToken && tokenSecretName != "" {
		tokenKeyName := configMap.Data[TokenSecretKeyName]
		if tokenKeyName == "" {
			tokenKeyName = "token" // default key name
		}

		var secret corev1.Secret
		secretKey := client.ObjectKey{
			Namespace: configMap.Namespace,
			Name:      tokenSecretName,
		}

		if err := r.Get(ctx, secretKey, &secret); err != nil {
			logger.Error(err, "Failed to fetch VCS token secret", "secret", tokenSecretName)
			return ctrl.Result{}, err
		}

		tokenBytes, ok := secret.Data[tokenKeyName]
		if !ok {
			logger.Error(fmt.Errorf("token key not found in secret"),
				"Failed to find token key",
				"secret", tokenSecretName,
				"key", tokenKeyName)
			return ctrl.Result{}, nil
		}
		vcsToken = string(tokenBytes)
	}

	// Create VCS provider
	provider, err := r.ProviderFactory.CreateProvider(&vcs.Config{
		Type:         providerType,
		Token:        vcsToken,
		BaseURL:      baseURL,
		Organization: organization,
	})
	if err != nil {
		logger.Error(err, "Failed to create VCS provider", "providerType", providerType)
		return ctrl.Result{}, err
	}

	logger.Info("Using VCS provider",
		"provider", provider.GetProviderType(),
		"organization", organization)

	// Fetch repositories using the VCS provider
	logger.Info("Fetching repositories", "organization", organization)
	repos, err := provider.GetRepositories(ctx, organization)
	if err != nil {
		// Check if this is a rate limit error
		if vcs.IsRateLimitError(err) {
			retryAfter := vcs.GetRetryAfter(err)
			logger.Info("VCS API rate limit encountered, will retry later",
				"organization", organization,
				"provider", provider.GetProviderType(),
				"retryAfter", retryAfter,
				"error", err.Error())

			// Return with requeue after the rate limit period
			// This prevents immediate retry and respects the rate limit
			return ctrl.Result{RequeueAfter: retryAfter}, nil
		}

		// For other errors, log and return error to trigger standard retry
		logger.Error(err, "Failed to fetch repositories", "organization", organization)
		return ctrl.Result{}, err
	}

	logger.Info("Found repositories", "organization", organization, "count", len(repos))

	// Fetch scorecard data for each repository
	for _, repo := range repos {
		logger.Info("Fetching scorecard data", "repository", repo)

		// Construct the VCS path for the scorecard API
		vcsPath := provider.GetScorecardURL(organization, repo)

		scorecardData, err := r.ScorecardClient.GetScorecardData(ctx, vcsPath, vcsToken)
		if err != nil {
			// Check if this is a "not found" error (scorecard data not available yet)
			if isNotFoundError(err) {
				logger.Info("Scorecard data not yet available for repository",
					"organization", organization,
					"repository", repo,
					"vcsPath", vcsPath)

				// Create scorecard data with -1 score to indicate unavailable data
				scorecardData = &scorecard.ScorecardData{
					Score:      -1,
					Repository: repo,
					Timestamp:  time.Now(),
					Checks:     []scorecard.Check{},
				}

				// Update metrics with -1 score
				r.MetricsCollector.UpdateMetrics(
					req.NamespacedName.String(),
					organization,
					repo,
					scorecardData,
				)

				// Continue to next repository
				continue
			}

			// For other errors, log as error and return to retry
			logger.Error(err, "Failed to fetch scorecard data",
				"organization", organization,
				"repository", repo,
				"vcsPath", vcsPath)
			return ctrl.Result{}, err
		}

		// Update metrics
		r.MetricsCollector.UpdateMetrics(
			req.NamespacedName.String(),
			organization,
			repo,
			scorecardData,
		)
	}

	logger.Info("Successfully reconciled ConfigMap",
		"namespace", configMap.Namespace,
		"name", configMap.Name,
		"provider", provider.GetProviderType(),
		"repositories", len(repos))

	return utils.JitterRequeue(r.RequeueInterval, r.MaxJitterPercent, logger), nil
}

// isNotFoundError checks if an error indicates that scorecard data was not found
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "scorecard data not found for")
}

// SetupWithManager sets up the controller with the Manager
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Only watch ConfigMaps with the specific label
	labelPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		labels := object.GetLabels()
		if labels == nil {
			return false
		}
		_, hasLabel := labels[ScorecardLabelKey]
		return hasLabel
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(labelPredicate).
		Complete(r)
}
