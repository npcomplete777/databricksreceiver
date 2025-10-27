# Metric Name Migration Guide

## Overview
All metrics have been updated to follow OpenTelemetry Semantic Conventions v1.27+.

## Breaking Changes - Metric Names

| Old Name | New Name | Notes |
|----------|----------|-------|
| `databricks.jobs.total` | `databricks.job.count` | Changed to follow entity.metric pattern |
| `databricks.job_runs.count` | `databricks.job.run.count` | Removed underscore, added proper hierarchy |
| `databricks.job.duration.run` | `databricks.job.run.duration` | Reordered for consistency |
| `databricks.warehouses.count` | `databricks.warehouse.count` | Singular entity name |
| `databricks.workspace.objects` | `databricks.workspace.object.count` | Added .count suffix |
| `databricks.dbfs.storage_bytes` | `databricks.dbfs.storage.usage` | More semantic naming |
| `databricks.dbfs.file_count` | `databricks.dbfs.file.count` | Consistent .count pattern |
| `databricks.tokens.count` | `databricks.token.count` | Singular entity name |
| `databricks.tokens.days_until_expiry` | `databricks.token.expiry` | Simplified name |
| `databricks.users.count` | `databricks.user.count` | Singular entity name |
| `databricks.groups.count` | `databricks.group.count` | Singular entity name |
| `databricks.groups.members` | `databricks.group.member.count` | Added .count suffix |
| `databricks.policies.count` | `databricks.policy.count` | Singular entity name |
| `databricks.policies.by_type` | `databricks.policy.by_type.count` | Added .count suffix |
| `databricks.tasks.duration` | `databricks.task.duration` | Singular entity name |
| `databricks.tasks.setup_duration` | `databricks.task.setup_duration` | Singular entity name |
| `databricks.job.cost` | `databricks.job.cost.estimate` | Clarified as estimate |
| `databricks.scraper.errors` | `databricks.receiver.scrape.errors` | Better namespacing |

## Breaking Changes - Attribute Names

| Old Attribute | New Attribute | Metrics Affected |
|---------------|---------------|------------------|
| `job_name` | `databricks.job.name` | All job metrics |
| `result_state` | `databricks.job.state` | Job and task metrics |
| `task_key` | `databricks.task.key` | Task metrics |
| `state` | `databricks.warehouse.state` | Warehouse metrics |
| `type` | `databricks.workspace.object.type` | Workspace metrics |
| `type` | `databricks.policy.type` | Policy metrics |
| `status` | `databricks.user.status` | User metrics |
| `group_name` | `databricks.group.name` | Group metrics |
| `comment` | `databricks.token.comment` | Token metrics |
| `cluster_id` | `databricks.cluster.id` | Cost metrics |

## Breaking Changes - Metric Types

Several count metrics changed from **Gauge** to **Sum**:
- `databricks.job.count`
- `databricks.job.run.count`
- `databricks.warehouse.count`
- `databricks.workspace.object.count`
- `databricks.dbfs.file.count`
- `databricks.token.count`
- `databricks.user.count`
- `databricks.group.count`
- `databricks.group.member.count`
- `databricks.policy.count`
- `databricks.policy.by_type.count`
- `databricks.receiver.scrape.errors`

## Breaking Changes - Units

| Old Unit | New Unit | Metrics Affected |
|----------|----------|------------------|
| `"1"` | `"{job}"` | databricks.job.count |
| `"1"` | `"{run}"` | databricks.job.run.count |
| `"1"` | `"{warehouse}"` | databricks.warehouse.count |
| `"1"` | `"{object}"` | databricks.workspace.object.count |
| `"By"` | `"By"` | No change (UCUM compliant) |
| `"1"` | `"{file}"` | databricks.dbfs.file.count |
| `"1"` | `"{token}"` | databricks.token.count |
| `"1"` | `"{user}"` | databricks.user.count |
| `"1"` | `"{group}"` | databricks.group.count |
| `"1"` | `"{member}"` | databricks.group.member.count |
| `"1"` | `"{policy}"` | databricks.policy.count |
| `"USD"` | `"{USD}"` | databricks.job.cost.estimate |
| `"1"` | `"{error}"` | databricks.receiver.scrape.errors |

## New Resource Attributes

All metrics now include these resource attributes:
- `databricks.workspace.host` - Your workspace URL
- `service.name` - Always "databricksreceiver"
- `service.version` - Receiver version (e.g., "0.1.0")

## Migration Steps

### 1. Update Dashboards
Search and replace metric names in your dashboards:
```
databricks.jobs.total → databricks.job.count
databricks.job_runs.count → databricks.job.run.count
(etc.)
```

### 2. Update Alerts
Update alert rules to use new metric names and attributes:
```yaml
# OLD
- alert: HighJobFailures
  expr: databricks.job_runs.count{result_state="FAILED"} > 10
  
# NEW
- alert: HighJobFailures
  expr: databricks.job.run.count{databricks.job.state="FAILED"} > 10
```

### 3. Update Queries
Update any saved queries or reports:
```promql
# OLD
rate(databricks.job_runs.count{result_state="SUCCESS"}[5m])

# NEW
rate(databricks.job.run.count{databricks.job.state="SUCCESS"}[5m])
```

### 4. Historical Data
Note that historical data will still use old metric names. Consider:
- Keeping both old and new dashboards during transition
- Using PromQL queries that combine old and new metrics
- Planning a cutover date after sufficient new data is collected

## Example Queries

### Job Success Rate
```promql
# OLD
databricks.job.success_rate

# NEW (unchanged)
databricks.job.success_rate
```

### Failed Jobs Count
```promql
# OLD
databricks.job_runs.count{result_state="FAILED"}

# NEW
databricks.job.run.count{databricks.job.state="FAILED"}
```

### Warehouse by State
```promql
# OLD
databricks.warehouses.count{state="RUNNING"}

# NEW
databricks.warehouse.count{databricks.warehouse.state="RUNNING"}
```

### Total Storage Usage
```promql
# OLD
databricks.dbfs.storage_bytes

# NEW
databricks.dbfs.storage.usage
```

## Support

For issues or questions about the migration, please open an issue at:
https://github.com/npcomplete777/databricksreceiver/issues
