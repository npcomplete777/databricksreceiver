// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package databricksreceiver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	
	require.NotNil(t, factory)
	assert.Equal(t, component.MustNewType("databricks"), factory.Type())
}

func TestFactoryCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	
	require.NotNil(t, cfg)
	assert.NoError(t, componenttest.CheckConfigStruct(cfg))
	
	databricksCfg, ok := cfg.(*Config)
	require.True(t, ok)
	assert.Equal(t, "60s", databricksCfg.CollectionInterval)
}

func TestCreateMetricsReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := &Config{
		Host:               "https://test.databricks.com",
		Token:              "dapi_test_token",
		CollectionInterval: "60s",
	}

	receiver, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(component.MustNewType("databricks")),
		cfg,
		consumertest.NewNop(),
	)

	require.NoError(t, err)
	require.NotNil(t, receiver)
}

func TestCreateMetricsReceiverInvalidConfig(t *testing.T) {
	factory := NewFactory()
	cfg := &Config{
		// Missing required fields
		CollectionInterval: "60s",
	}

	receiver, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(component.MustNewType("databricks")),
		cfg,
		consumertest.NewNop(),
	)

	// Should still create receiver (validation happens elsewhere)
	require.NotNil(t, receiver)
	require.NoError(t, err)
}

func TestCreateMetricsReceiverInvalidInterval(t *testing.T) {
	factory := NewFactory()
	cfg := &Config{
		Host:               "https://test.databricks.com",
		Token:              "dapi_test_token",
		CollectionInterval: "invalid",
	}

	receiver, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(component.MustNewType("databricks")),
		cfg,
		consumertest.NewNop(),
	)

	require.Error(t, err)
	require.Nil(t, receiver)
}

func TestFactoryType(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, "databricks", factory.Type().String())
}
