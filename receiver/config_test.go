// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package databricksreceiver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/configopaque"
)

func TestConfigValidate(t *testing.T) {
	cfg := &Config{
		Host:               "https://test.databricks.com",
		Token:              configopaque.String("dapi_valid_test_token"), // Fixed: proper prefix
		CollectionInterval: "60s",
	}

	err := cfg.Validate()
	assert.NoError(t, err, "Valid config failed validation")
}

func TestConfigValidateMissingHost(t *testing.T) {
	cfg := &Config{
		Token:              configopaque.String("dapi_test_token"),
		CollectionInterval: "60s",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

func TestConfigValidateMissingToken(t *testing.T) {
	cfg := &Config{
		Host:               "https://test.databricks.com",
		CollectionInterval: "60s",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token is required")
}

func TestConfigValidateHTTPSRequired(t *testing.T) {
	cfg := &Config{
		Host:               "http://test.databricks.com",
		Token:              configopaque.String("dapi_test_token"),
		CollectionInterval: "60s",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must use HTTPS")
}

func TestConfigValidateTokenFormat(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectError bool
	}{
		{
			name:        "valid_personal_token",
			token:       "dapi_valid_token",
			expectError: false,
		},
		{
			name:        "valid_service_principal_token",
			token:       "dkea_valid_token",
			expectError: false,
		},
		{
			name:        "invalid_token_prefix",
			token:       "invalid_token",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:               "https://test.databricks.com",
				Token:              configopaque.String(tt.token),
				CollectionInterval: "60s",
			}

			err := cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "token must start")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigValidateCollectionInterval(t *testing.T) {
	tests := []struct {
		name        string
		interval    string
		expectError bool
	}{
		{
			name:        "valid_seconds",
			interval:    "60s",
			expectError: false,
		},
		{
			name:        "valid_minutes",
			interval:    "5m",
			expectError: false,
		},
		{
			name:        "invalid_format",
			interval:    "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:               "https://test.databricks.com",
				Token:              configopaque.String("dapi_test_token"),
				CollectionInterval: tt.interval,
			}

			err := cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigValidateAPILimits(t *testing.T) {
	tests := []struct {
		name        string
		jobLimit    int
		taskLimit   int
		expectError bool
	}{
		{
			name:        "valid_limits",
			jobLimit:    20,
			taskLimit:   10,
			expectError: false,
		},
		{
			name:        "job_limit_too_high",
			jobLimit:    150,
			taskLimit:   10,
			expectError: true,
		},
		{
			name:        "task_limit_too_high",
			jobLimit:    20,
			taskLimit:   100,
			expectError: true,
		},
		{
			name:        "negative_job_limit",
			jobLimit:    -1,
			taskLimit:   10,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:                      "https://test.databricks.com",
				Token:                     configopaque.String("dapi_test_token"),
				CollectionInterval:        "60s",
				MaxJobRunDetailsPerScrape: tt.jobLimit,
				MaxTaskDetailsPerScrape:   tt.taskLimit,
			}

			err := cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigValidateCloudProvider(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		expectError bool
	}{
		{
			name:        "azure",
			provider:    "azure",
			expectError: false,
		},
		{
			name:        "aws",
			provider:    "aws",
			expectError: false,
		},
		{
			name:        "gcp",
			provider:    "gcp",
			expectError: false,
		},
		{
			name:        "invalid",
			provider:    "invalid",
			expectError: true,
		},
		{
			name:        "empty_is_valid",
			provider:    "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:               "https://test.databricks.com",
				Token:              configopaque.String("dapi_test_token"),
				CollectionInterval: "60s",
				CloudProvider:      tt.provider,
			}

			err := cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)

	assert.NotNil(t, cfg)
	assert.Equal(t, "60s", cfg.CollectionInterval)
	assert.Equal(t, 20, cfg.MaxJobRunDetailsPerScrape)
	assert.Equal(t, 10, cfg.MaxTaskDetailsPerScrape)
	assert.Equal(t, 24, cfg.OnlyRecentRunsHours)
	assert.Equal(t, "azure", cfg.CloudProvider)
	assert.Equal(t, 0.15, cfg.DBUPricePerUnit)
	assert.NotEmpty(t, cfg.NodeTypeDBURates)
}

func TestGetDefaultDBURates(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected int
	}{
		{
			name:     "azure",
			provider: "azure",
			expected: 12,
		},
		{
			name:     "aws",
			provider: "aws",
			expected: 8,
		},
		{
			name:     "gcp",
			provider: "gcp",
			expected: 5,
		},
		{
			name:     "unknown",
			provider: "unknown",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rates := getDefaultDBURates(tt.provider)
			assert.Len(t, rates, tt.expected)
		})
	}
}

func TestConfigValidateRecentRunsHours(t *testing.T) {
	tests := []struct {
		name        string
		hours       int
		expectError bool
	}{
		{
			name:        "valid_24_hours",
			hours:       24,
			expectError: false,
		},
		{
			name:        "valid_48_hours",
			hours:       48,
			expectError: false,
		},
		{
			name:        "negative_hours",
			hours:       -1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:                "https://test.databricks.com",
				Token:               configopaque.String("dapi_test_token"),
				CollectionInterval:  "60s",
				OnlyRecentRunsHours: tt.hours,
			}

			err := cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
