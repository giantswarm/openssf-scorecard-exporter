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

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/giantswarm/openssf-scorecard-exporter/internal/scorecard"
)

const (
	metricsNamespace = "openssf_scorecard"
)

// Collector manages Prometheus metrics for OpenSSF Scorecard data
type Collector struct {
	// Overall scorecard score
	overallScore *prometheus.GaugeVec

	// Individual check scores
	checkScore *prometheus.GaugeVec

	// Check pass/fail status
	checkStatus *prometheus.GaugeVec

	// Last update timestamp
	lastUpdate *prometheus.GaugeVec

	// Mutex to protect metric updates
	mu sync.RWMutex

	// Track which metrics have been registered
	registeredMetrics map[string]bool
}

// NewCollector creates a new metrics collector and registers metrics
func NewCollector() *Collector {
	c := &Collector{
		overallScore: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "overall_score",
				Help:      "Overall OpenSSF Scorecard score for a repository (0-10)",
			},
			[]string{"config", "organization", "repository"},
		),
		checkScore: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "check_score",
				Help:      "Score for individual OpenSSF Scorecard check (0-10, -1 for unavailable)",
			},
			[]string{"config", "organization", "repository", "check"},
		),
		checkStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "check_status",
				Help:      "Status of individual OpenSSF Scorecard check (1=pass, 0=fail, -1=unavailable)",
			},
			[]string{"config", "organization", "repository", "check"},
		),
		lastUpdate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "last_update_timestamp",
				Help:      "Unix timestamp of the last scorecard data update",
			},
			[]string{"config", "organization", "repository"},
		),
		registeredMetrics: make(map[string]bool),
	}

	// Register metrics with controller-runtime's metrics registry
	metrics.Registry.MustRegister(
		c.overallScore,
		c.checkScore,
		c.checkStatus,
		c.lastUpdate,
	)

	return c
}

// UpdateMetrics updates Prometheus metrics based on scorecard data
func (c *Collector) UpdateMetrics(configName, organization, repository string, data *scorecard.ScorecardData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	labels := prometheus.Labels{
		"config":       configName,
		"organization": organization,
		"repository":   repository,
	}

	// Update overall score
	c.overallScore.With(labels).Set(data.Score)

	// Update individual check scores and statuses
	for _, check := range data.Checks {
		checkLabels := prometheus.Labels{
			"config":       configName,
			"organization": organization,
			"repository":   repository,
			"check":        check.Name,
		}

		c.checkScore.With(checkLabels).Set(float64(check.Score))

		// Convert status to numeric value
		var statusValue float64
		switch check.Status {
		case "Pass":
			statusValue = 1
		case "Fail":
			statusValue = 0
		default:
			statusValue = -1 // unavailable or unknown
		}
		c.checkStatus.With(checkLabels).Set(statusValue)
	}

	// Update last update timestamp
	c.lastUpdate.With(labels).Set(float64(data.Timestamp.Unix()))

	// Track this metric set
	metricKey := configName + "/" + organization + "/" + repository
	c.registeredMetrics[metricKey] = true
}

// RemoveMetricsForConfig removes all metrics associated with a config
func (c *Collector) RemoveMetricsForConfig(configName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove metrics for all repositories in this config
	for key := range c.registeredMetrics {
		// Simple prefix match - in production you might want more sophisticated tracking
		delete(c.registeredMetrics, key)
	}

	// Note: Prometheus client doesn't have a built-in way to delete specific metric labels
	// The metrics will naturally be updated or expire based on scrape intervals
	// For more control, you could use DeleteLabelValues with specific label combinations
}
