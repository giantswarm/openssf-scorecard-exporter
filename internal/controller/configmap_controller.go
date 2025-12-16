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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/giantswarm/openssf-scorecard-exporter/internal/metrics"
	"github.com/giantswarm/openssf-scorecard-exporter/internal/scorecard"
)

const (
	// ScorecardLabelKey is the label key that identifies ConfigMaps to reconcile
	ScorecardLabelKey = "openssf-scorecard.giantswarm.io/enabled"
	
	// OrganizationKey is the ConfigMap data key for the GitHub organization
	OrganizationKey = "organization"
	
	// TokenSecretKey is the ConfigMap data key for the GitHub token secret reference
	TokenSecretKey = "tokenSecret"
	
	// TokenSecretKeyName is the ConfigMap data key for the GitHub token secret key name
	TokenSecretKeyName = "tokenSecretKey"
)

// ConfigMapReconciler reconciles ConfigMap objects for OpenSSF Scorecard
type ConfigMapReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ScorecardClient   *scorecard.Client
	MetricsCollector  *metrics.Collector
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

	// Extract optional GitHub token from referenced secret
	var githubToken string
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
			logger.Error(err, "Failed to fetch GitHub token secret", "secret", tokenSecretName)
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
		githubToken = string(tokenBytes)
	}

	// Fetch repositories for the organization
	logger.Info("Fetching repositories for organization", "organization", organization)
	repos, err := r.ScorecardClient.GetRepositories(ctx, organization, githubToken)
	if err != nil {
		logger.Error(err, "Failed to fetch repositories", "organization", organization)
		return ctrl.Result{}, err
	}

	logger.Info("Found repositories", "organization", organization, "count", len(repos))

	// Fetch scorecard data for each repository
	for _, repo := range repos {
		logger.Info("Fetching scorecard data", "repository", repo)
		
		scorecardData, err := r.ScorecardClient.GetScorecardData(ctx, organization, repo, githubToken)
		if err != nil {
			logger.Error(err, "Failed to fetch scorecard data", 
				"organization", organization, 
				"repository", repo)
			continue
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
		"repositories", len(repos))

	return ctrl.Result{}, nil
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

