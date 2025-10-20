package databricksreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/scraper"
	"go.opentelemetry.io/collector/scraper/scraperhelper"
)

const (
	// typeStr is the value of "type" key in configuration.
	typeStr = "databricks"
)

var (
	// Compile-time assertion that the receiver type is valid
	typeVal = component.MustNewType(typeStr)
)

// NewFactory creates a factory for Databricks receiver.
// The receiver collects workspace-level metrics from Databricks REST APIs
// including jobs, clusters, warehouses, and cost estimates.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		typeVal,
		createDefaultConfig,
		receiver.WithMetrics(createMetricsReceiver, component.StabilityLevelAlpha),
	)
}

// createMetricsReceiver creates a metrics receiver based on the provided config.
func createMetricsReceiver(
	ctx context.Context,
	settings receiver.Settings,
	cfg component.Config,
	consumer consumer.Metrics,
) (receiver.Metrics, error) {
	databricksCfg := cfg.(*Config)

	interval, err := time.ParseDuration(databricksCfg.CollectionInterval)
	if err != nil {
		return nil, err
	}

	s := newScraper(databricksCfg, settings.TelemetrySettings)

	scraperCfg := &scraperhelper.ControllerConfig{
		CollectionInterval: interval,
		InitialDelay:       time.Second,
	}

	sc, err := scraper.NewMetrics(s.scrape)
	if err != nil {
		return nil, err
	}

	return scraperhelper.NewMetricsController(
		scraperCfg,
		settings,
		consumer,
		scraperhelper.AddScraper(typeVal, sc),
	)
}
