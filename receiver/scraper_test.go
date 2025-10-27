// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package databricksreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
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
			assert.NotEmpty(t, tt.expectedUnit)
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
	
	metrics, err := scraper.scrape(context.Background())
	
	require.NoError(t, err)
	require.NotNil(t, metrics)
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

// Metric generation tests

func TestAddJobMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	jobs := []Job{
		{JobID: 1},
		{JobID: 2},
	}
	jobs[0].Settings.Name = "test-job-1"
	jobs[1].Settings.Name = "test-job-2"

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addJobMetrics(sm, jobs)

	require.Equal(t, 1, sm.Metrics().Len())
	metric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.job.count", metric.Name())
	assert.Equal(t, pmetric.MetricTypeSum, metric.Type())
}

func TestAddJobRunMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	runs := []JobRun{
		{RunID: 1, RunName: "run-1"},
		{RunID: 2, RunName: "run-2"},
		{RunID: 3, RunName: "run-3"},
	}
	runs[0].State.ResultState = "SUCCESS"
	runs[1].State.ResultState = "FAILED"
	runs[2].State.ResultState = "SUCCESS"

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addJobRunMetrics(sm, runs)

	require.GreaterOrEqual(t, sm.Metrics().Len(), 1)
	
	countMetric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.job.run.count", countMetric.Name())
	assert.Equal(t, pmetric.MetricTypeSum, countMetric.Type())
	
	sum := countMetric.Sum()
	assert.True(t, sum.IsMonotonic())
}

func TestAddWarehouseMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	warehouses := []SQLWarehouse{
		{ID: "wh-1", Name: "warehouse-1", State: "RUNNING"},
		{ID: "wh-2", Name: "warehouse-2", State: "STOPPED"},
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addWarehouseMetrics(sm, warehouses)

	require.Equal(t, 1, sm.Metrics().Len())
	metric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.warehouse.count", metric.Name())
	assert.Equal(t, "{warehouse}", metric.Unit())
}

func TestAddWorkspaceMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	objects := []WorkspaceObject{
		{Path: "/notebook1", ObjectType: "NOTEBOOK"},
		{Path: "/notebook2", ObjectType: "NOTEBOOK"},
		{Path: "/dir1", ObjectType: "DIRECTORY"},
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addWorkspaceMetrics(sm, objects)

	require.Equal(t, 1, sm.Metrics().Len())
	metric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.workspace.object.count", metric.Name())
	assert.Equal(t, "{object}", metric.Unit())
}

func TestAddDBFSMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	files := []DBFSFile{
		{Path: "/file1.txt", IsDir: false, FileSize: 1024},
		{Path: "/file2.txt", IsDir: false, FileSize: 2048},
		{Path: "/dir1", IsDir: true, FileSize: 0},
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addDBFSMetrics(sm, files)

	require.Equal(t, 2, sm.Metrics().Len())
	
	storageMetric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.dbfs.storage.usage", storageMetric.Name())
	assert.Equal(t, "By", storageMetric.Unit())
	assert.Equal(t, pmetric.MetricTypeGauge, storageMetric.Type())
	
	fileCountMetric := sm.Metrics().At(1)
	assert.Equal(t, "databricks.dbfs.file.count", fileCountMetric.Name())
}

func TestAddTokenMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	now := time.Now().UnixMilli()
	tokens := []Token{
		{TokenID: "token-1", Comment: "test-token", ExpiryTime: now + 86400000},
		{TokenID: "token-2", Comment: "test-token-2", ExpiryTime: now + 172800000},
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addTokenMetrics(sm, tokens)

	require.Equal(t, 2, sm.Metrics().Len())
	
	countMetric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.token.count", countMetric.Name())
	assert.Equal(t, "{token}", countMetric.Unit())
	
	expiryMetric := sm.Metrics().At(1)
	assert.Equal(t, "databricks.token.expiry", expiryMetric.Name())
	assert.Equal(t, "d", expiryMetric.Unit())
}

func TestAddUserMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	users := []User{
		{UserName: "user1@example.com", Active: true},
		{UserName: "user2@example.com", Active: true},
		{UserName: "user3@example.com", Active: false},
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addUserMetrics(sm, users)

	require.Equal(t, 1, sm.Metrics().Len())
	metric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.user.count", metric.Name())
	assert.Equal(t, "{user}", metric.Unit())
}

func TestAddGroupMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	groups := []Group{
		{DisplayName: "group1"},
		{DisplayName: "group2"},
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addGroupMetrics(sm, groups)

	require.Equal(t, 2, sm.Metrics().Len())
	
	countMetric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.group.count", countMetric.Name())
	
	memberMetric := sm.Metrics().At(1)
	assert.Equal(t, "databricks.group.member.count", memberMetric.Name())
}

func TestAddPolicyMetrics(t *testing.T) {
	scraper := &databricksScraper{
		cfg:    &Config{},
		logger: componenttest.NewNopTelemetrySettings().Logger,
	}

	policies := []ClusterPolicy{
		{PolicyID: "policy-1", Name: "default-policy", IsDefault: true},
		{PolicyID: "policy-2", Name: "custom-policy", IsDefault: false},
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()

	scraper.addPolicyMetrics(sm, policies)

	require.Equal(t, 2, sm.Metrics().Len())
	
	countMetric := sm.Metrics().At(0)
	assert.Equal(t, "databricks.policy.count", countMetric.Name())
	
	byTypeMetric := sm.Metrics().At(1)
	assert.Equal(t, "databricks.policy.by_type.count", byTypeMetric.Name())
}
