# Databricks Receiver

| Status                   |           |
| ------------------------ |-----------|
| Stability                | [alpha]   |
| Supported pipeline types | metrics   |
| Distributions            | []        |

The Databricks receiver collects workspace-level metrics from Databricks via REST APIs and exports them via OTLP. It provides comprehensive observability into job performance, cluster utilization, costs, and workspace resources.

## Configuration
```yaml
receivers:
  databricks:
    # Required: Databricks workspace URL
    host: "https://your-workspace.cloud.databricks.com"
    
    # Required: Personal Access Token or Service Principal token
    token: "dapi..."
    
    # Optional: How often to collect metrics (default: 60s)
    collection_interval: "60s"
    
    # Optional: Maximum job run details to fetch per scrape (default: 20, max: 100)
    max_job_run_details_per_scrape: 20
    
    # Optional: Maximum task details to fetch per scrape (default: 10, max: 50)
    max_task_details_per_scrape: 10
    
    # Optional: Only fetch details for runs from last N hours (default: 24)
    only_recent_runs_hours: 24
    
    # Optional: Cloud provider for DBU pricing (default: azure)
    cloud_provider: "azure"  # Options: azure, aws, gcp
    
    # Optional: Price per DBU in USD (default: 0.15)
    dbu_price_per_unit: 0.15
    
    # Optional: Override default DBU rates per node type
    node_type_dbu_rates:
      Standard_DS3_v2: 0.75
      Standard_DS4_v2: 1.50
```

## Metrics

The receiver collects the following metrics:

### Job Metrics
- `databricks.jobs.total` - Total configured jobs
- `databricks.job_runs.count` - Job runs by state
- `databricks.job.duration.run` - Complete run duration (ms)
- `databricks.job.duration.queue` - Queue time (ms)
- `databricks.job.duration.setup` - Setup time (ms)
- `databricks.job.duration.execution` - Execution time (ms)
- `databricks.job.duration.cleanup` - Cleanup time (ms)
- `databricks.job.success_rate` - Success percentage (%)
- `databricks.job.cost` - Estimated job cost (USD)

### Task Metrics
- `databricks.tasks.duration` - Task execution duration (ms)
- `databricks.tasks.setup_duration` - Task setup duration (ms)

### Infrastructure Metrics
- `databricks.warehouses.count` - SQL warehouses by state
- `databricks.workspace.objects` - Workspace objects by type
- `databricks.dbfs.storage_bytes` - DBFS storage usage (By)
- `databricks.dbfs.file_count` - DBFS file count

### Management Metrics
- `databricks.tokens.count` - Active tokens
- `databricks.tokens.days_until_expiry` - Token expiration (d)
- `databricks.users.count` - Users by status
- `databricks.groups.count` - Total groups
- `databricks.groups.members` - Members per group
- `databricks.policies.count` - Cluster policies
- `databricks.policies.by_type` - Policies by type

### Health Metric
- `databricks.scraper.errors` - Failed API calls

## Resource Attributes

All metrics include:
- `databricks.host` - Workspace URL

Metric-specific attributes:
- `job_name` - Job name
- `result_state` - Job/task result state
- `task_key` - Task identifier
- `state` - Warehouse state
- `type` - Object/policy type
- `status` - User status
- `group_name` - Group name

## Example
```yaml
receivers:
  databricks:
    host: "https://example.cloud.databricks.com"
    token: "dapi..."
    collection_interval: "60s"
    max_job_run_details_per_scrape: 20
    cloud_provider: "azure"

processors:
  batch:

exporters:
  otlphttp:
    endpoint: "https://your-backend.com/v1/metrics"

service:
  pipelines:
    metrics:
      receivers: [databricks]
      processors: [batch]
      exporters: [otlphttp]
```

## Prerequisites

- Databricks workspace with admin access
- Personal Access Token or Service Principal with permissions:
  - Jobs: Read
  - Clusters: Read
  - Workspace: Read
  - SQL Analytics: Read
  - Users/Groups: Read

## Limitations

- Collects workspace-level metrics only (no cluster-internal metrics)
- API rate limits apply (configure collection_interval accordingly)
- Cost estimates are approximations based on configured DBU rates

[alpha]: https://github.com/open-telemetry/opentelemetry-collector#alpha
