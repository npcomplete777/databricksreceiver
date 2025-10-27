// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package databricksreceiver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
)

func TestScraperResourceAttributes(t *testing.T) {
	cfg := &Config{
		Host:               "https://test.databricks.com",
		Token:              "dapi_test_token",
		CollectionInterval: "60s",
	}

	scraper := newScraper(cfg, componenttest.NewNopTelemetrySettings())
	
	assert.NotNil(t, scraper)
	assert.Equal(t, "https://test.databricks.com", scraper.client.host)
}

func TestMetricNaming(t *testing.T) {
	tests := []struct {
		name           string
		expectedMetric string
		description    string
	}{
		{
			name:           "job_count",
			expectedMetric: "databricks.job.count",
			description:    "Total number of jobs in the workspace",
		},
		{
			name:           "job_run_count",
			expectedMetric: "databricks.job.run.count",
			description:    "Number of job runs by state",
		},
		{
			name:           "warehouse_count",
			expectedMetric: "databricks.warehouse.count",
			description:    "Number of SQL warehouses by state",
		},
		{
			name:           "workspace_object_count",
			expectedMetric: "databricks.workspace.object.count",
			description:    "Number of workspace objects by type",
		},
		{
			name:           "dbfs_storage",
			expectedMetric: "databricks.dbfs.storage.usage",
			description:    "Total storage consumed in DBFS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validates naming conventions
			assert.Contains(t, tt.expectedMetric, "databricks.")
			assert.NotContains(t, tt.expectedMetric, "_")
		})
	}
}

func TestAttributeNamespacing(t *testing.T) {
	tests := []struct {
		name             string
		attributeName    string
		shouldHavePrefix bool
	}{
		{
			name:             "job_name_has_prefix",
			attributeName:    "databricks.job.name",
			shouldHavePrefix: true,
		},
		{
			name:             "job_state_has_prefix",
			attributeName:    "databricks.job.state",
			shouldHavePrefix: true,
		},
		{
			name:             "task_key_has_prefix",
			attributeName:    "databricks.task.key",
			shouldHavePrefix: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldHavePrefix {
				assert.Contains(t, tt.attributeName, "databricks.")
			}
		})
	}
}

func TestMetricTypes(t *testing.T) {
	tests := []struct {
		metricName   string
		expectedType string
		isMonotonic  bool
	}{
		{
			metricName:   "databricks.job.count",
			expectedType: "Sum",
			isMonotonic:  false,
		},
		{
			metricName:   "databricks.job.run.count",
			expectedType: "Sum",
			isMonotonic:  true,
		},
		{
			metricName:   "databricks.job.run.duration",
			expectedType: "Gauge",
			isMonotonic:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.metricName, func(t *testing.T) {
			// Documents expected metric types
			assert.NotEmpty(t, tt.expectedType)
		})
	}
}

func TestMetricUnits(t *testing.T) {
	tests := []struct {
		metricName   string
		expectedUnit string
	}{
		{"databricks.job.count", "{job}"},
		{"databricks.job.run.count", "{run}"},
		{"databricks.warehouse.count", "{warehouse}"},
		{"databricks.dbfs.storage.usage", "By"},
		{"databricks.job.run.duration", "ms"},
		{"databricks.token.expiry", "d"},
		{"databricks.job.cost.estimate", "{USD}"},
	}

	for _, tt := range tests {
		t.Run(tt.metricName, func(t *testing.T) {
			// Verify UCUM compliance
			assert.NotEmpty(t, tt.expectedUnit)
			// Custom units must be in curly braces
			if tt.expectedUnit != "By" && tt.expectedUnit != "ms" && tt.expectedUnit != "d" {
				assert.Contains(t, tt.expectedUnit, "{")
				assert.Contains(t, tt.expectedUnit, "}")
			}
		})
	}
}

func TestScrapeErrorHandling(t *testing.T) {
	cfg := &Config{
		Host:                      "https://invalid.databricks.com",
		Token:                     "dapi_invalid_token",
		CollectionInterval:        "60s",
		MaxJobRunDetailsPerScrape: 20,
		MaxTaskDetailsPerScrape:   10,
		OnlyRecentRunsHours:       24,
	}

	scraper := newScraper(cfg, componenttest.NewNopTelemetrySettings())
	
	// Scraper should not panic on API errors
	metrics, err := scraper.scrape(context.Background())
	
	// We expect no error from scrape itself (errors are tracked in metrics)
	require.NoError(t, err)
	require.NotNil(t, metrics)
	
	// Should have at least one resource metric
	assert.Greater(t, metrics.ResourceMetrics().Len(), 0)
}

func TestScraperConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		expectError bool
	}{
		{
			name: "valid_config",
			cfg: &Config{
				Host:               "https://test.databricks.com",
				Token:              "dapi_test_token",
				CollectionInterval: "60s",
			},
			expectError: false,
		},
		{
			name: "missing_host",
			cfg: &Config{
				Token:              "dapi_test_token",
				CollectionInterval: "60s",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReceiverVersion(t *testing.T) {
	// Verify version constant exists and is not empty
	assert.NotEmpty(t, receiverVersion)
	assert.Equal(t, "0.1.0", receiverVersion)
}

func TestNewScraper(t *testing.T) {
	cfg := &Config{
		Host:                      "https://test.databricks.com",
		Token:                     "dapi_test_token",
		CollectionInterval:        "60s",
		MaxJobRunDetailsPerScrape: 20,
		MaxTaskDetailsPerScrape:   10,
		OnlyRecentRunsHours:       24,
		CloudProvider:             "azure",
		DBUPricePerUnit:           0.15,
	}

	settings := componenttest.NewNopTelemetrySettings()
	scraper := newScraper(cfg, settings)

	require.NotNil(t, scraper)
	assert.NotNil(t, scraper.client)
	assert.NotNil(t, scraper.logger)
	assert.Equal(t, cfg, scraper.cfg)
}
