package databricksreceiver

import (
    "errors"
    "go.opentelemetry.io/collector/component"
    "go.opentelemetry.io/collector/config/confighttp"
)

type Config struct {
    confighttp.ClientConfig `mapstructure:",squash"`
    
    // Databricks specific config
    Host  string `mapstructure:"host"`
    Token string `mapstructure:"token"`
    
    // Collection interval
    CollectionInterval string `mapstructure:"collection_interval"`
    
    // Cost calculation settings
    CloudProvider string             `mapstructure:"cloud_provider"` // "azure", "aws", "gcp"
    DBUPricePerUnit float64          `mapstructure:"dbu_price_per_unit"` // Price per DBU (e.g., 0.15 for Azure)
    NodeTypeDBURates map[string]float64 `mapstructure:"node_type_dbu_rates"` // Custom DBU rates per node type
}

func (cfg *Config) Validate() error {
    if cfg.Host == "" {
        return errors.New("host is required")
    }
    if cfg.Token == "" {
        return errors.New("token is required")
    }
    if cfg.CloudProvider != "" && cfg.CloudProvider != "azure" && cfg.CloudProvider != "aws" && cfg.CloudProvider != "gcp" {
        return errors.New("cloud_provider must be 'azure', 'aws', or 'gcp'")
    }
    return nil
}

func createDefaultConfig() component.Config {
    return &Config{
        ClientConfig: confighttp.NewDefaultClientConfig(),
        CollectionInterval: "60s",
        CloudProvider: "azure",
        DBUPricePerUnit: 0.15,
        NodeTypeDBURates: getDefaultDBURates("azure"),
    }
}

// Default DBU rates by cloud provider
func getDefaultDBURates(provider string) map[string]float64 {
    switch provider {
    case "azure":
        return map[string]float64{
            "Standard_DS3_v2": 0.75,
            "Standard_DS4_v2": 1.50,
            "Standard_DS5_v2": 3.00,
            "Standard_DS12_v2": 1.50,
            "Standard_DS13_v2": 3.00,
            "Standard_DS14_v2": 6.00,
            "Standard_E4s_v3": 1.00,
            "Standard_E8s_v3": 2.00,
            "Standard_E16s_v3": 4.00,
            "Standard_NC6": 2.00,
            "Standard_NC12": 4.00,
            "Standard_NC24": 8.00,
        }
    case "aws":
        return map[string]float64{
            "m5.xlarge": 0.69,
            "m5.2xlarge": 1.38,
            "m5.4xlarge": 2.76,
            "r5.xlarge": 0.87,
            "r5.2xlarge": 1.74,
            "r5.4xlarge": 3.48,
            "c5.2xlarge": 1.38,
            "c5.4xlarge": 2.76,
        }
    case "gcp":
        return map[string]float64{
            "n1-standard-4": 0.75,
            "n1-standard-8": 1.50,
            "n1-standard-16": 3.00,
            "n1-highmem-4": 1.00,
            "n1-highmem-8": 2.00,
        }
    default:
        return map[string]float64{}
    }
}
