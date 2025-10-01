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

var typeStr = component.MustNewType("databricks")

func NewFactory() receiver.Factory {
    return receiver.NewFactory(
        typeStr,
        createDefaultConfig,
        receiver.WithMetrics(createMetricsReceiver, component.StabilityLevelAlpha),
    )
}

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
        scraperhelper.AddScraper(typeStr, sc),
    )
}
