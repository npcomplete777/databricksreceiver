// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package databricksreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

const receiverVersion = "0.1.0"

type databricksScraper struct {
	client   *databricksClient
	logger   *zap.Logger
	settings component.TelemetrySettings
	cfg      *Config
}

func newScraper(cfg *Config, settings component.TelemetrySettings) *databricksScraper {
	return &databricksScraper{
		client:   newDatabricksClient(cfg),
		logger:   settings.Logger,
		settings: settings,
		cfg:      cfg,
	}
}

func (s *databricksScraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
	metrics := pmetric.NewMetrics()
	resourceMetrics := metrics.ResourceMetrics().AppendEmpty()

	// Add OTEL-compliant resource attributes
	resource := resourceMetrics.Resource()
	resource.Attributes().PutStr("databricks.workspace.host", s.client.host)
	resource.Attributes().PutStr("service.name", "databricksreceiver")
	resource.Attributes().PutStr("service.version", receiverVersion)

	scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
	scopeMetrics.Scope().SetName("databricksreceiver")
	scopeMetrics.Scope().SetVersion(receiverVersion)

	// Track scrape success/failure for observability
	var scraperErrors []string

	// Get jobs for additional data
	jobs, err := s.client.GetJobs(ctx)
	if err != nil {
		s.logger.Error("Failed to get jobs", zap.Error(err))
		scraperErrors = append(scraperErrors, "jobs")
	} else {
		s.addJobMetrics(scopeMetrics, jobs)
	}

	// Get job runs with enhanced metrics
	runs, err := s.client.GetJobRuns(ctx)
	if err != nil {
		s.logger.Error("Failed to get job runs", zap.Error(err))
		scraperErrors = append(scraperErrors, "job_runs")
	} else {
		s.addJobRunMetrics(scopeMetrics, runs)
		s.addEnhancedJobMetrics(ctx, scopeMetrics, runs, jobs)
		s.addTaskMetrics(ctx, scopeMetrics, runs)
	}

	// Existing endpoints with error tracking
	if warehouses, err := s.client.GetSQLWarehouses(ctx); err == nil {
		s.addWarehouseMetrics(scopeMetrics, warehouses)
	} else {
		s.logger.Error("Failed to get SQL warehouses", zap.Error(err))
		scraperErrors = append(scraperErrors, "warehouses")
	}

	if workspace, err := s.client.GetWorkspace(ctx, "/"); err == nil {
		s.addWorkspaceMetrics(scopeMetrics, workspace)
	} else {
		s.logger.Error("Failed to get workspace", zap.Error(err))
		scraperErrors = append(scraperErrors, "workspace")
	}

	if dbfs, err := s.client.GetDBFS(ctx); err == nil {
		s.addDBFSMetrics(scopeMetrics, dbfs)
	} else {
		s.logger.Error("Failed to get DBFS", zap.Error(err))
		scraperErrors = append(scraperErrors, "dbfs")
	}

	if tokens, err := s.client.GetTokens(ctx); err == nil {
		s.addTokenMetrics(scopeMetrics, tokens)
	} else {
		s.logger.Error("Failed to get tokens", zap.Error(err))
		scraperErrors = append(scraperErrors, "tokens")
	}

	if users, err := s.client.GetUsers(ctx); err == nil {
		s.addUserMetrics(scopeMetrics, users)
	} else {
		s.logger.Error("Failed to get users", zap.Error(err))
		scraperErrors = append(scraperErrors, "users")
	}

	if groups, err := s.client.GetGroups(ctx); err == nil {
		s.addGroupMetrics(scopeMetrics, groups)
	} else {
		s.logger.Error("Failed to get groups", zap.Error(err))
		scraperErrors = append(scraperErrors, "groups")
	}

	if policies, err := s.client.GetClusterPolicies(ctx); err == nil {
		s.addPolicyMetrics(scopeMetrics, policies)
	} else {
		s.logger.Error("Failed to get cluster policies", zap.Error(err))
		scraperErrors = append(scraperErrors, "policies")
	}

	// Add scrape health metric
	s.addScrapeHealthMetric(scopeMetrics, len(scraperErrors))

	// Clean up old cache entries
	s.client.cleanCache()

	return metrics, nil
}

// addScrapeHealthMetric reports the number of failed API calls during scraping
func (s *databricksScraper) addScrapeHealthMetric(scopeMetrics pmetric.ScopeMetrics, errorCount int) {
	healthMetric := scopeMetrics.Metrics().AppendEmpty()
	healthMetric.SetName("databricks.receiver.scrape.errors")
	healthMetric.SetDescription("Number of API endpoints that failed during the scrape operation")
	healthMetric.SetUnit("{error}")

	sum := healthMetric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	dp := sum.DataPoints().AppendEmpty()
	dp.SetIntValue(int64(errorCount))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
}

func (s *databricksScraper) addEnhancedJobMetrics(ctx context.Context, scopeMetrics pmetric.ScopeMetrics, runs []JobRun, jobs []Job) {
	now := pcommon.NewTimestampFromTime(time.Now())

	// Success rate calculation
	if len(runs) > 0 {
		successCount := 0
		failureCount := 0
		for _, run := range runs {
			if run.State.ResultState == "SUCCESS" {
				successCount++
			} else if run.State.ResultState == "FAILED" {
				failureCount++
			}
		}

		totalCompleted := successCount + failureCount
		if totalCompleted > 0 {
			successRate := float64(successCount) / float64(totalCompleted)

			successRateMetric := scopeMetrics.Metrics().AppendEmpty()
			successRateMetric.SetName("databricks.job.success_rate")
			successRateMetric.SetDescription("Percentage of successful job completions")
			successRateMetric.SetUnit("1")

			gauge := successRateMetric.SetEmptyGauge()
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetDoubleValue(successRate)
			dp.SetTimestamp(now)
		}
	}

	// Use configurable limits from config
	maxDetailsToFetch := s.cfg.MaxJobRunDetailsPerScrape
	detailsFetched := 0

	for _, run := range runs {
		// Skip if we've fetched enough details
		if detailsFetched >= maxDetailsToFetch {
			s.logger.Debug("Reached max job run details limit", zap.Int("limit", maxDetailsToFetch))
			break
		}

		// Only fetch details for recent runs or failed runs
		recentCutoff := time.Now().Add(-time.Duration(s.cfg.OnlyRecentRunsHours) * time.Hour).UnixMilli()
		isRecent := run.StartTime > recentCutoff
		isFailed := run.State.ResultState == "FAILED"

		if !isRecent && !isFailed {
			continue
		}

		details, err := s.client.GetJobRunDetails(ctx, run.RunID)
		if err != nil {
			s.logger.Debug("Failed to get job run details", zap.Int64("run_id", run.RunID), zap.Error(err))
			continue
		}
		detailsFetched++

		// Queue duration
		var queueDuration int64
		for _, job := range jobs {
			if job.JobID == run.JobID {
				queueDuration = run.StartTime - job.CreatedTime
				break
			}
		}

		if queueDuration > 0 {
			queueMetric := scopeMetrics.Metrics().AppendEmpty()
			queueMetric.SetName("databricks.job.duration.queue")
			queueMetric.SetDescription("Time spent in queue before job execution starts")
			queueMetric.SetUnit("ms")

			gauge := queueMetric.SetEmptyGauge()
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(queueDuration)
			dp.SetTimestamp(now)
			dp.Attributes().PutStr("databricks.job.name", run.RunName)
		}

		// Cleanup duration
		if details.CleanupDuration > 0 {
			cleanupMetric := scopeMetrics.Metrics().AppendEmpty()
			cleanupMetric.SetName("databricks.job.duration.cleanup")
			cleanupMetric.SetDescription("Time spent cleaning up after job completion")
			cleanupMetric.SetUnit("ms")

			gauge := cleanupMetric.SetEmptyGauge()
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(details.CleanupDuration)
			dp.SetTimestamp(now)
			dp.Attributes().PutStr("databricks.job.name", run.RunName)
		}

		// Setup duration
		if details.SetupDuration > 0 {
			setupMetric := scopeMetrics.Metrics().AppendEmpty()
			setupMetric.SetName("databricks.job.duration.setup")
			setupMetric.SetDescription("Time spent setting up the job execution environment")
			setupMetric.SetUnit("ms")

			gauge := setupMetric.SetEmptyGauge()
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(details.SetupDuration)
			dp.SetTimestamp(now)
			dp.Attributes().PutStr("databricks.job.name", run.RunName)
		}

		// Execution duration
		if details.ExecutionDuration > 0 {
			execMetric := scopeMetrics.Metrics().AppendEmpty()
			execMetric.SetName("databricks.job.duration.execution")
			execMetric.SetDescription("Time spent executing the job workload")
			execMetric.SetUnit("ms")

			gauge := execMetric.SetEmptyGauge()
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetIntValue(details.ExecutionDuration)
			dp.SetTimestamp(now)
			dp.Attributes().PutStr("databricks.job.name", run.RunName)
		}

		// Cost calculation using cluster info
		if run.RunDuration > 0 && details.ClusterInstance.ClusterID != "" {
			clusterInfo, err := s.client.GetClusterInfo(ctx, details.ClusterInstance.ClusterID)
			if err == nil {
				estimatedCost := s.client.calculateJobCost(clusterInfo, run.RunDuration)

				costMetric := scopeMetrics.Metrics().AppendEmpty()
				costMetric.SetName("databricks.job.cost.estimate")
				costMetric.SetDescription("Estimated cost of job execution in USD (custom unit, not UCUM)")
				costMetric.SetUnit("{USD}")

				gauge := costMetric.SetEmptyGauge()
				dp := gauge.DataPoints().AppendEmpty()
				dp.SetDoubleValue(estimatedCost)
				dp.SetTimestamp(now)
				dp.Attributes().PutStr("databricks.job.name", run.RunName)
				dp.Attributes().PutStr("databricks.cluster.id", details.ClusterInstance.ClusterID)
			} else {
				// Fallback to simple estimation
				estimatedCost := float64(run.RunDuration) / (1000 * 60 * 60) * 0.50

				costMetric := scopeMetrics.Metrics().AppendEmpty()
				costMetric.SetName("databricks.job.cost.estimate")
				costMetric.SetDescription("Estimated cost of job execution in USD (simplified)")
				costMetric.SetUnit("{USD}")

				gauge := costMetric.SetEmptyGauge()
				dp := gauge.DataPoints().AppendEmpty()
				dp.SetDoubleValue(estimatedCost)
				dp.SetTimestamp(now)
				dp.Attributes().PutStr("databricks.job.name", run.RunName)
			}
		}
	}
}

func (s *databricksScraper) addJobRunMetrics(scopeMetrics pmetric.ScopeMetrics, runs []JobRun) {
	jobRunCountMetric := scopeMetrics.Metrics().AppendEmpty()
	jobRunCountMetric.SetName("databricks.job.run.count")
	jobRunCountMetric.SetDescription("Number of job runs by state")
	jobRunCountMetric.SetUnit("{run}")

	sum := jobRunCountMetric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	stateCount := make(map[string]int)
	for _, run := range runs {
		key := run.State.ResultState
		if key == "" {
			key = "RUNNING"
		}
		stateCount[key]++
	}

	now := pcommon.NewTimestampFromTime(time.Now())
	for state, count := range stateCount {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(count))
		dp.SetTimestamp(now)
		dp.Attributes().PutStr("databricks.job.state", state)
	}

	if len(runs) > 0 {
		runDurationMetric := scopeMetrics.Metrics().AppendEmpty()
		runDurationMetric.SetName("databricks.job.run.duration")
		runDurationMetric.SetDescription("Duration of job run from start to completion")
		runDurationMetric.SetUnit("ms")

		runGauge := runDurationMetric.SetEmptyGauge()
		for _, run := range runs {
			if run.RunDuration > 0 {
				dp := runGauge.DataPoints().AppendEmpty()
				dp.SetIntValue(run.RunDuration)
				dp.SetTimestamp(now)
				dp.Attributes().PutStr("databricks.job.name", run.RunName)
				dp.Attributes().PutStr("databricks.job.state", run.State.ResultState)
			}
		}
	}
}

func (s *databricksScraper) addJobMetrics(scopeMetrics pmetric.ScopeMetrics, jobs []Job) {
	jobCountMetric := scopeMetrics.Metrics().AppendEmpty()
	jobCountMetric.SetName("databricks.job.count")
	jobCountMetric.SetDescription("Total number of jobs in the workspace")
	jobCountMetric.SetUnit("{job}")

	sum := jobCountMetric.SetEmptySum()
	sum.SetIsMonotonic(false)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	dp := sum.DataPoints().AppendEmpty()
	dp.SetIntValue(int64(len(jobs)))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
}

func (s *databricksScraper) addWarehouseMetrics(scopeMetrics pmetric.ScopeMetrics, warehouses []SQLWarehouse) {
	whCountMetric := scopeMetrics.Metrics().AppendEmpty()
	whCountMetric.SetName("databricks.warehouse.count")
	whCountMetric.SetDescription("Number of SQL warehouses by state")
	whCountMetric.SetUnit("{warehouse}")

	sum := whCountMetric.SetEmptySum()
	sum.SetIsMonotonic(false)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	stateCount := make(map[string]int)
	for _, wh := range warehouses {
		stateCount[wh.State]++
	}

	now := pcommon.NewTimestampFromTime(time.Now())
	for state, count := range stateCount {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(count))
		dp.SetTimestamp(now)
		dp.Attributes().PutStr("databricks.warehouse.state", state)
	}
}

func (s *databricksScraper) addWorkspaceMetrics(scopeMetrics pmetric.ScopeMetrics, objects []WorkspaceObject) {
	objCountMetric := scopeMetrics.Metrics().AppendEmpty()
	objCountMetric.SetName("databricks.workspace.object.count")
	objCountMetric.SetDescription("Number of workspace objects by type")
	objCountMetric.SetUnit("{object}")

	sum := objCountMetric.SetEmptySum()
	sum.SetIsMonotonic(false)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	typeCount := make(map[string]int)
	for _, obj := range objects {
		typeCount[obj.ObjectType]++
	}

	now := pcommon.NewTimestampFromTime(time.Now())
	for objType, count := range typeCount {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(count))
		dp.SetTimestamp(now)
		dp.Attributes().PutStr("databricks.workspace.object.type", objType)
	}
}

func (s *databricksScraper) addDBFSMetrics(scopeMetrics pmetric.ScopeMetrics, files []DBFSFile) {
	storageMetric := scopeMetrics.Metrics().AppendEmpty()
	storageMetric.SetName("databricks.dbfs.storage.usage")
	storageMetric.SetDescription("Total storage consumed in DBFS")
	storageMetric.SetUnit("By")

	gauge := storageMetric.SetEmptyGauge()
	var totalSize int64
	for _, file := range files {
		if !file.IsDir {
			totalSize += file.FileSize
		}
	}

	dp := gauge.DataPoints().AppendEmpty()
	dp.SetIntValue(totalSize)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	fileCountMetric := scopeMetrics.Metrics().AppendEmpty()
	fileCountMetric.SetName("databricks.dbfs.file.count")
	fileCountMetric.SetDescription("Number of files and directories in DBFS")
	fileCountMetric.SetUnit("{file}")

	fileSum := fileCountMetric.SetEmptySum()
	fileSum.SetIsMonotonic(false)
	fileSum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	dp2 := fileSum.DataPoints().AppendEmpty()
	dp2.SetIntValue(int64(len(files)))
	dp2.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
}

func (s *databricksScraper) addTokenMetrics(scopeMetrics pmetric.ScopeMetrics, tokens []Token) {
	tokenCountMetric := scopeMetrics.Metrics().AppendEmpty()
	tokenCountMetric.SetName("databricks.token.count")
	tokenCountMetric.SetDescription("Number of active access tokens")
	tokenCountMetric.SetUnit("{token}")

	sum := tokenCountMetric.SetEmptySum()
	sum.SetIsMonotonic(false)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	dp := sum.DataPoints().AppendEmpty()
	dp.SetIntValue(int64(len(tokens)))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	expiryMetric := scopeMetrics.Metrics().AppendEmpty()
	expiryMetric.SetName("databricks.token.expiry")
	expiryMetric.SetDescription("Days remaining until token expiration")
	expiryMetric.SetUnit("d")

	expiryGauge := expiryMetric.SetEmptyGauge()
	now := time.Now().UnixMilli()

	for _, token := range tokens {
		if token.ExpiryTime > 0 {
			daysUntilExpiry := (token.ExpiryTime - now) / (1000 * 60 * 60 * 24)
			dp := expiryGauge.DataPoints().AppendEmpty()
			dp.SetIntValue(daysUntilExpiry)
			dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
			dp.Attributes().PutStr("databricks.token.comment", token.Comment)
		}
	}
}

func (s *databricksScraper) addUserMetrics(scopeMetrics pmetric.ScopeMetrics, users []User) {
	userCountMetric := scopeMetrics.Metrics().AppendEmpty()
	userCountMetric.SetName("databricks.user.count")
	userCountMetric.SetDescription("Number of users by status")
	userCountMetric.SetUnit("{user}")

	sum := userCountMetric.SetEmptySum()
	sum.SetIsMonotonic(false)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	activeCount := 0
	inactiveCount := 0
	for _, user := range users {
		if user.Active {
			activeCount++
		} else {
			inactiveCount++
		}
	}

	now := pcommon.NewTimestampFromTime(time.Now())
	dpActive := sum.DataPoints().AppendEmpty()
	dpActive.SetIntValue(int64(activeCount))
	dpActive.SetTimestamp(now)
	dpActive.Attributes().PutStr("databricks.user.status", "active")

	dpInactive := sum.DataPoints().AppendEmpty()
	dpInactive.SetIntValue(int64(inactiveCount))
	dpInactive.SetTimestamp(now)
	dpInactive.Attributes().PutStr("databricks.user.status", "inactive")
}

func (s *databricksScraper) addGroupMetrics(scopeMetrics pmetric.ScopeMetrics, groups []Group) {
	groupCountMetric := scopeMetrics.Metrics().AppendEmpty()
	groupCountMetric.SetName("databricks.group.count")
	groupCountMetric.SetDescription("Total number of groups in the workspace")
	groupCountMetric.SetUnit("{group}")

	sum := groupCountMetric.SetEmptySum()
	sum.SetIsMonotonic(false)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	dp := sum.DataPoints().AppendEmpty()
	dp.SetIntValue(int64(len(groups)))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	memberCountMetric := scopeMetrics.Metrics().AppendEmpty()
	memberCountMetric.SetName("databricks.group.member.count")
	memberCountMetric.SetDescription("Number of members in each group")
	memberCountMetric.SetUnit("{member}")

	memberSum := memberCountMetric.SetEmptySum()
	memberSum.SetIsMonotonic(false)
	memberSum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	for _, group := range groups {
		dp := memberSum.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(len(group.Members)))
		dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		dp.Attributes().PutStr("databricks.group.name", group.DisplayName)
	}
}

func (s *databricksScraper) addPolicyMetrics(scopeMetrics pmetric.ScopeMetrics, policies []ClusterPolicy) {
	policyCountMetric := scopeMetrics.Metrics().AppendEmpty()
	policyCountMetric.SetName("databricks.policy.count")
	policyCountMetric.SetDescription("Total number of cluster policies")
	policyCountMetric.SetUnit("{policy}")

	sum := policyCountMetric.SetEmptySum()
	sum.SetIsMonotonic(false)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	dp := sum.DataPoints().AppendEmpty()
	dp.SetIntValue(int64(len(policies)))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	defaultCount := 0
	customCount := 0
	for _, policy := range policies {
		if policy.IsDefault {
			defaultCount++
		} else {
			customCount++
		}
	}

	policyTypeMetric := scopeMetrics.Metrics().AppendEmpty()
	policyTypeMetric.SetName("databricks.policy.by_type.count")
	policyTypeMetric.SetDescription("Number of cluster policies by type")
	policyTypeMetric.SetUnit("{policy}")

	typeSum := policyTypeMetric.SetEmptySum()
	typeSum.SetIsMonotonic(false)
	typeSum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	
	now := pcommon.NewTimestampFromTime(time.Now())

	dpDefault := typeSum.DataPoints().AppendEmpty()
	dpDefault.SetIntValue(int64(defaultCount))
	dpDefault.SetTimestamp(now)
	dpDefault.Attributes().PutStr("databricks.policy.type", "default")

	dpCustom := typeSum.DataPoints().AppendEmpty()
	dpCustom.SetIntValue(int64(customCount))
	dpCustom.SetTimestamp(now)
	dpCustom.Attributes().PutStr("databricks.policy.type", "custom")
}

func (s *databricksScraper) addTaskMetrics(ctx context.Context, scopeMetrics pmetric.ScopeMetrics, runs []JobRun) {
	taskDurationMetric := scopeMetrics.Metrics().AppendEmpty()
	taskDurationMetric.SetName("databricks.task.duration")
	taskDurationMetric.SetDescription("Duration of task execution")
	taskDurationMetric.SetUnit("ms")

	taskGauge := taskDurationMetric.SetEmptyGauge()

	setupDurationMetric := scopeMetrics.Metrics().AppendEmpty()
	setupDurationMetric.SetName("databricks.task.setup_duration")
	setupDurationMetric.SetDescription("Duration of task setup phase")
	setupDurationMetric.SetUnit("ms")

	setupGauge := setupDurationMetric.SetEmptyGauge()

	// Use configurable limit from config
	maxTaskDetailsToFetch := s.cfg.MaxTaskDetailsPerScrape
	taskDetailsFetched := 0

	for _, run := range runs {
		if taskDetailsFetched >= maxTaskDetailsToFetch {
			break
		}

		// Only fetch task details for recent or failed runs
		recentCutoff := time.Now().Add(-time.Duration(s.cfg.OnlyRecentRunsHours) * time.Hour).UnixMilli()
		if run.StartTime < recentCutoff && run.State.ResultState != "FAILED" {
			continue
		}

		details, err := s.client.GetJobRunDetails(ctx, run.RunID)
		if err != nil {
			continue
		}
		taskDetailsFetched++

		for _, task := range details.Tasks {
			dp := taskGauge.DataPoints().AppendEmpty()
			dp.SetIntValue(task.ExecutionDuration)
			dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
			dp.Attributes().PutStr("databricks.task.key", task.TaskKey)
			dp.Attributes().PutStr("databricks.job.state", task.State.ResultState)

			if task.SetupDuration > 0 {
				dp2 := setupGauge.DataPoints().AppendEmpty()
				dp2.SetIntValue(task.SetupDuration)
				dp2.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
				dp2.Attributes().PutStr("databricks.task.key", task.TaskKey)
			}
		}
	}
}
