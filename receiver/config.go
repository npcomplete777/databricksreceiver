// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package databricksreceiver // import "github.com/npcomplete777/databricksreceiver"

import (
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
)

// Config defines the configuration for the Databricks receiver.
// It extends confighttp.ClientConfig to leverage standard HTTP client configuration
// and adds Databricks-specific settings for authentication, collection intervals,
// API rate limiting, and cost calculation.
type Config struct {
	confighttp.ClientConfig `mapstructure:",squash"`

	// Host is the Databricks workspace URL (e.g., "https://your-workspace.cloud.databricks.com").
	// Must use HTTPS protocol.
	Host string `mapstructure:"host"`

	// Token is the Databricks Personal Access Token (starting with 'dapi') or
	// Service Principal token (starting with 'dkea'). Uses configopaque.String
	// to prevent token exposure in logs and debug output.
	Token configopaque.String `mapstructure:"token"`

	// CollectionInterval defines how often to scrape metrics from Databricks APIs.
	// Default: "60s". Format: duration string (e.g., "30s", "2m", "1h").
	CollectionInterval string `mapstructure:"collection_interval"`

	// MaxJobRunDetailsPerScrape limits the number of detailed job run API calls per scrape cycle.
	// This prevents API rate limiting on large workspaces. Set to 0 to disable detailed metrics.
	// Default: 20. Maximum: 100.
	MaxJobRunDetailsPerScrape int `mapstructure:"max_job_run_details_per_scrape"`

	// MaxTaskDetailsPerScrape limits the number of task detail API calls per scrape cycle.
	// Set to 0 to disable task-level metrics.
	// Default: 10. Maximum: 50.
	MaxTaskDetailsPerScrape int `mapstructure:"max_task_details_per_scrape"`

	// OnlyRecentRunsHours limits job run details to runs that started within the last N hours.
	// Reduces API load by ignoring old completed runs.
	// Default: 24 hours.
	OnlyRecentRunsHours int `mapstructure:"only_recent_runs_hours"`

	// CloudProvider specifies the cloud platform for accurate DBU rate calculations.
	// Valid values: "azure", "aws", "gcp".
	// Default: "azure".
	CloudProvider string `mapstructure:"cloud_provider"`

	// DBUPricePerUnit is the cost per Databricks Unit (DBU) in USD.
	// This varies by cloud provider and contract terms.
	// Default: 0.15 (Azure standard rate).
	DBUPricePerUnit float64 `mapstructure:"dbu_price_per_unit"`

	// NodeTypeDBURates provides custom DBU consumption rates per node/instance type.
	// Overrides default rates for specific compute types.
	// Format: map[node_type_id]dbu_per_hour
	// Example: {"Standard_DS3_v2": 0.75, "m5.xlarge": 0.69}
	NodeTypeDBURates map[string]float64 `mapstructure:"node_type_dbu_rates"`
}

// Validate checks the receiver configuration for errors and ensures all required
// fields are properly set according to Databricks API and security requirements.
func (cfg *Config) Validate() error {
	if cfg.Host == "" {
		return errors.New("host is required")
	}
	if cfg.Token == "" {
		return errors.New("token is required")
	}

	// Validate HTTPS requirement
	if !strings.HasPrefix(cfg.Host, "https://") {
		return errors.New("host must use HTTPS")
	}

	// Validate token format
	tokenStr := string(cfg.Token)
	if !strings.HasPrefix(tokenStr, "dapi") && !strings.HasPrefix(tokenStr, "dkea") {
		return errors.New("token must start with 'dapi' (personal) or 'dkea' (service principal)")
	}

	// Validate collection interval
	if _, err := time.ParseDuration(cfg.CollectionInterval); err != nil {
		return errors.New("invalid collection_interval format (use format like '60s', '5m')")
	}

	// Validate API limits
	if cfg.MaxJobRunDetailsPerScrape < 0 || cfg.MaxJobRunDetailsPerScrape > 100 {
		return errors.New("max_job_run_details_per_scrape must be between 0 and 100")
	}
	if cfg.MaxTaskDetailsPerScrape < 0 || cfg.MaxTaskDetailsPerScrape > 50 {
		return errors.New("max_task_details_per_scrape must be between 0 and 50")
	}
	if cfg.OnlyRecentRunsHours < 0 {
		return errors.New("only_recent_runs_hours cannot be negative")
	}

	// Validate cloud provider
	if cfg.CloudProvider != "" && cfg.CloudProvider != "azure" && cfg.CloudProvider != "aws" && cfg.CloudProvider != "gcp" {
		return errors.New("cloud_provider must be 'azure', 'aws', or 'gcp'")
	}

	return nil
}

// createDefaultConfig returns a Config with sensible defaults for Databricks monitoring.
// These defaults are optimized for typical enterprise workspaces and can be overridden
// in the receiver configuration.
func createDefaultConfig() component.Config {
	return &Config{
		ClientConfig:              confighttp.NewDefaultClientConfig(),
		CollectionInterval:        "60s",
		MaxJobRunDetailsPerScrape: 20,
		MaxTaskDetailsPerScrape:   10,
		OnlyRecentRunsHours:       24,
		CloudProvider:             "azure",
		DBUPricePerUnit:           0.15,
		NodeTypeDBURates:          getDefaultDBURates("azure"),
	}
}

// getDefaultDBURates returns default DBU consumption rates per node type for each cloud provider.
// These rates are based on Databricks pricing documentation and represent typical consumption patterns.
// Actual rates may vary based on contract terms and should be customized via configuration.
func getDefaultDBURates(provider string) map[string]float64 {
	switch provider {
	case "azure":
		return map[string]float64{
			"Standard_DS3_v2":  0.75,
			"Standard_DS4_v2":  1.50,
			"Standard_DS5_v2":  3.00,
			"Standard_DS12_v2": 1.50,
			"Standard_DS13_v2": 3.00,
			"Standard_DS14_v2": 6.00,
			"Standard_E4s_v3":  1.00,
			"Standard_E8s_v3":  2.00,
			"Standard_E16s_v3": 4.00,
			"Standard_NC6":     2.00,
			"Standard_NC12":    4.00,
			"Standard_NC24":    8.00,
		}
	case "aws":
		return map[string]float64{
			"m5.xlarge":  0.69,
			"m5.2xlarge": 1.38,
			"m5.4xlarge": 2.76,
			"r5.xlarge":  0.87,
			"r5.2xlarge": 1.74,
			"r5.4xlarge": 3.48,
			"c5.2xlarge": 1.38,
			"c5.4xlarge": 2.76,
		}
	case "gcp":
		return map[string]float64{
			"n1-standard-4":  0.75,
			"n1-standard-8":  1.50,
			"n1-standard-16": 3.00,
			"n1-highmem-4":   1.00,
			"n1-highmem-8":   2.00,
		}
	default:
		return map[string]float64{}
	}
}
