```markdown
# Databricks OpenTelemetry Receiver

A custom OpenTelemetry receiver that collects workspace-level metrics from Databricks via REST APIs and exports to any OTLP-compatible observability platform.

## What Does This Do?

This tool automatically collects information about your Databricks workspace every 60 seconds:
- How long jobs take to run
- Whether jobs succeed or fail
- How many SQL warehouses are running
- Number of users, groups, and access tokens
- Workspace storage usage

All this data is exported via OTLP to your observability backend (Prometheus, Grafana, New Relic, Datadog, Dynatrace, etc.).

## What You Need Before Starting

### 1. A Linux Server
- Ubuntu 20.04+ or similar
- At least 2GB RAM
- Internet access to Databricks and your observability platform

### 2. Databricks Access
- A Databricks workspace
- Admin access to create a Personal Access Token

### 3. Observability Platform
- Any OTLP-compatible backend (Prometheus, Grafana Cloud, New Relic, Datadog, Splunk, Dynatrace, etc.)
- Ability to create API tokens/credentials

### 4. Basic Linux Knowledge
- How to SSH into a server
- How to copy/paste commands into terminal
- How to edit text files with nano

## Step-by-Step Installation

### Step 1: Install Required Software

SSH into your Linux server and run these commands:

```bash
# Update system packages
sudo apt-get update

# Install Go programming language (version 1.21 or newer)
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz

# Add Go to your PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go is installed
go version
# Should output: go version go1.22.5 linux/amd64
```

```bash
# Install Git
sudo apt-get install -y git
git --version
```

```bash
# Download OpenTelemetry Collector Builder
cd ~
wget https://github.com/open-telemetry/opentelemetry-collector/releases/download/cmd%2Fbuilder%2Fv0.115.0/ocb_0.115.0_linux_amd64
chmod +x ocb_0.115.0_linux_amd64
sudo mv ocb_0.115.0_linux_amd64 /usr/local/bin/ocb

# Verify OCB is installed
ocb version
```

### Step 2: Download This Receiver

```bash
# Clone this repository
cd ~
git clone https://github.com/npcomplete777/databricksreceiver.git
cd databricksreceiver
```

### Step 3: Initialize the Go Module

```bash
cd receiver
go mod init github.com/npcomplete777/databricksreceiver/receiver
go mod tidy
cd ..
```

This downloads all the dependencies needed for the receiver to work.

### Step 4: Get Your Databricks Personal Access Token

1. **Log into Databricks** - Go to your Databricks workspace URL in a web browser

2. **Open User Settings** - Click on your email/username in the top-right corner, select "User Settings"

3. **Create API Token**
   - Click "Developer" tab on the left
   - Click "Manage" next to "Access tokens"
   - Click "Generate new token"
   - Give it a name like "OpenTelemetry Monitoring"
   - Set "Lifetime (days)" to 90 (or leave blank for no expiration)
   - Click "Generate"

4. **Copy the Token** - IMPORTANT: Copy immediately, you won't see it again. It looks like: `dapi1234567890abcdef1234567890ab`

**Required Token Permissions:**
- Jobs: Read
- Clusters: Read
- Workspace: Read
- SQL Analytics: Read
- Users/Groups: Read

### Step 5: Configure Your Observability Backend

You need to configure the exporter for your specific backend. Below are examples for common platforms.

#### Prometheus (with OTLP Receiver)

```yaml
exporters:
  otlphttp:
    endpoint: "http://prometheus-server:9090/api/v1/otlp"
```

#### Grafana Cloud

```yaml
exporters:
  otlphttp:
    endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp"
    headers:
      Authorization: "Basic <base64-encoded-credentials>"
```

#### New Relic

```yaml
exporters:
  otlphttp:
    endpoint: "https://otlp.nr-data.net:4318"
    headers:
      api-key: "YOUR_NEW_RELIC_LICENSE_KEY"
```

#### Datadog

```yaml
exporters:
  otlphttp:
    endpoint: "https://api.datadoghq.com"
    headers:
      DD-API-KEY: "YOUR_DATADOG_API_KEY"
```

#### Dynatrace

```yaml
exporters:
  otlphttp:
    endpoint: "https://YOUR_ENV.live.dynatrace.com/api/v2/otlp"
    headers:
      Authorization: "Api-Token YOUR_DYNATRACE_TOKEN"
```

#### Generic OTLP Endpoint

```yaml
exporters:
  otlphttp:
    endpoint: "https://your-backend.com/v1/metrics"
    headers:
      Authorization: "Bearer YOUR_API_TOKEN"
```

### Step 6: Configure the Receiver

```bash
cd ~/databricksreceiver
cp config-example.yaml config.yaml
nano config.yaml
```

Update these values:
- `host`: Your workspace URL (without https://)
- `token`: Your Databricks token from Step 4
- `exporters` section: Your backend config from Step 5

Example configuration:

```yaml
receivers:
  databricks:
    host: "dbc-abc123-xyz.cloud.databricks.com"
    token: "dapi1234567890abcdef1234567890ab"
    collection_interval: "60s"

processors:
  batch:
    timeout: 10s

exporters:
  otlphttp:
    endpoint: "https://your-backend.com/v1/metrics"
    headers:
      Authorization: "Bearer YOUR_API_TOKEN"

service:
  pipelines:
    metrics:
      receivers: [databricks]
      processors: [batch]
      exporters: [otlphttp]
```

Save with `Ctrl+O`, `Enter`, `Ctrl+X`

### Step 7: Build the Collector

```bash
cd ~/databricksreceiver
ocb --config builder-config-databricks.yaml
```

This takes 2-3 minutes. When done you'll see "Collector built successfully"

### Step 8: Test the Collector

```bash
cd ~/databricksreceiver/otelcol-databricks
./otelcol-databricks --config ../config.yaml
```

Look for: "Everything is ready. Begin running and processing data."

Press `Ctrl+C` to stop once verified.

### Step 9: Run as a Background Service

```bash
sudo nano /etc/systemd/system/otelcol-databricks.service
```

Paste this content (replace `your-username` with your actual Linux username):

```ini
[Unit]
Description=OpenTelemetry Collector - Databricks Monitoring
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/home/your-username/databricksreceiver
ExecStart=/home/your-username/databricksreceiver/otelcol-databricks/otelcol-databricks --config=/home/your-username/databricksreceiver/config.yaml
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=multi-user.target
```

Save and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable otelcol-databricks
sudo systemctl start otelcol-databricks
sudo systemctl status otelcol-databricks
```

You should see: `Active: active (running)`

View live logs:

```bash
sudo journalctl -u otelcol-databricks -f
```

Press `Ctrl+C` to stop viewing logs (service keeps running).

### Step 10: Verify Metrics

Wait 2-3 minutes for first metrics to arrive, then check your observability platform for metrics starting with `databricks.`

## Troubleshooting

### Problem: "401 Unauthorized" Error

**Cause**: Wrong or expired Databricks token

**Solution**:
1. Create a new token (Step 4)
2. Update config: `nano ~/databricksreceiver/config.yaml`
3. Restart service: `sudo systemctl restart otelcol-databricks`

### Problem: No Metrics Appearing

**Cause**: Wrong exporter configuration or firewall blocking

**Solution**:
1. Check logs: `sudo journalctl -u otelcol-databricks -n 50`
2. Verify exporter endpoint and credentials are correct
3. Test connectivity to your backend

### Problem: Service Won't Start

**Solution**:
1. Check status: `sudo systemctl status otelcol-databricks`
2. Check logs: `sudo journalctl -u otelcol-databricks -n 100`
3. Verify binary exists: `ls -l ~/databricksreceiver/otelcol-databricks/otelcol-databricks`

### Problem: "429 Too Many Requests" Error

**Cause**: Hitting Databricks API rate limits

**Solution**:
1. Edit config: `nano ~/databricksreceiver/config.yaml`
2. Change `collection_interval` from "60s" to "120s"
3. Restart: `sudo systemctl restart otelcol-databricks`

## Maintenance

### Viewing Logs

```bash
# See last 50 lines
sudo journalctl -u otelcol-databricks -n 50

# Follow logs in real-time
sudo journalctl -u otelcol-databricks -f

# See logs from last hour
sudo journalctl -u otelcol-databricks --since "1 hour ago"
```

### Stopping/Restarting Service

```bash
sudo systemctl stop otelcol-databricks
sudo systemctl restart otelcol-databricks
```

### Updating Configuration

```bash
nano ~/databricksreceiver/config.yaml
sudo systemctl restart otelcol-databricks
```

### Rotating Databricks Token

When your token expires:
1. Create new token in Databricks
2. Update config: `nano ~/databricksreceiver/config.yaml`
3. Restart: `sudo systemctl restart otelcol-databricks`

## Monitoring Multiple Workspaces

To monitor multiple Databricks workspaces, edit your config:

```yaml
receivers:
  databricks/prod:
    host: "prod-workspace.cloud.databricks.com"
    token: "dapi_prod_token"
    collection_interval: "60s"
  
  databricks/dev:
    host: "dev-workspace.cloud.databricks.com"
    token: "dapi_dev_token"
    collection_interval: "120s"

service:
  pipelines:
    metrics/prod:
      receivers: [databricks/prod]
      processors: [batch]
      exporters: [otlphttp]
    metrics/dev:
      receivers: [databricks/dev]
      processors: [batch]
      exporters: [otlphttp]
```

Each workspace's metrics will be tagged with `databricks.host` attribute for filtering.

## Collected Metrics

### Job Metrics (9)
- `databricks.jobs.total` - Total configured jobs
- `databricks.job_runs.count` - Job runs by state
- `databricks.job.duration.run` - Complete run duration (ms)
- `databricks.job.duration.queue` - Queue time (ms)
- `databricks.job.duration.setup` - Setup time (ms)
- `databricks.job.duration.execution` - Execution time (ms)
- `databricks.job.duration.cleanup` - Cleanup time (ms)
- `databricks.job.success_rate` - Success percentage

### Task Metrics (2)
- `databricks.tasks.duration` - Task execution (ms)
- `databricks.tasks.setup_duration` - Task setup (ms)

### Infrastructure Metrics (6)
- `databricks.warehouses.count` - SQL warehouses by state
- `databricks.workspace.objects` - Objects by type
- `databricks.dbfs.storage_bytes` - DBFS usage
- `databricks.dbfs.file_count` - File count
- `databricks.users.count` - Users by status
- `databricks.groups.count` - Total groups

### Management Metrics (3)
- `databricks.tokens.count` - Active tokens
- `databricks.tokens.days_until_expiry` - Token expiration tracking
- `databricks.policies.count` - Cluster policies

### Health Metric (1)
- `databricks.scraper.errors` - Failed API calls (should be 0)

All metrics include attributes like `job_name`, `result_state`, `task_key`, `databricks.host` for filtering.

## Security Notes

- **Keep config.yaml secret** - It contains your API tokens
- Rotate tokens every 90 days
- Use read-only Databricks tokens (no write permissions needed)
- Don't commit config.yaml to Git

## Limitations

This receiver collects **workspace-level metrics only** via REST APIs. It cannot collect:
- Spark executor metrics (memory, CPU per executor)
- Hardware metrics (requires agent on cluster nodes)
- Real-time streaming metrics
- Detailed RDD/stage metrics

For cluster-internal metrics, you need to deploy an agent inside Databricks clusters.

## Performance

Per workspace (60s collection interval):
- Memory: ~50MB
- CPU: <5%
- Network: ~500KB/minute
- API calls: 8-11 base endpoints + up to 30 cached detail calls



