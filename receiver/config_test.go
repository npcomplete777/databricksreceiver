package databricksreceiver

import (
    "testing"
)

func TestConfigValidate(t *testing.T) {
    cfg := Config{
        Host: "https://test.databricks.com",
        Token: "test123",
    }
    err := cfg.Validate()
    if err != nil {
        t.Errorf("Valid config failed validation: %v", err)
    }
}

func TestConfigValidateMissingHost(t *testing.T) {
    cfg := Config{
        Token: "test123",
    }
    err := cfg.Validate()
    if err == nil {
        t.Error("Expected error for missing host")
    }
}
