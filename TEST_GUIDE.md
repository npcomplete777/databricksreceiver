# Testing Guide

## Running Tests

Change to receiver directory and run:
go test -v

## Running with Coverage

go test -v -cover

## Current Tests

- config_test.go - Configuration validation tests
- client_test.go - API client tests with mocked HTTP responses

## Test Coverage

Current coverage: 3% (basic smoke tests)

To improve coverage, add tests for scraper metric generation and additional API endpoints.

## Integration Tests

For integration tests against a real Databricks workspace set these environment variables first:

export DATABRICKS_HOST="https://your-workspace.cloud.databricks.com"
export DATABRICKS_TOKEN="your-token"

Then run: go test -v
