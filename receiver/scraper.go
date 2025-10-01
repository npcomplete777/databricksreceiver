package databricksreceiver

import (
    "context"
    "time"
    
    "go.opentelemetry.io/collector/component"
    "go.opentelemetry.io/collector/pdata/pcommon"
    "go.opentelemetry.io/collector/pdata/pmetric"
    "go.uber.org/zap"
)

type databricksScraper struct {
    client   *databricksClient
    logger   *zap.Logger
    settings component.TelemetrySettings
}

func newScraper(cfg *Config, settings component.TelemetrySettings) *databricksScraper {
    return &databricksScraper{
        client:   newDatabricksClient(cfg),
        logger:   settings.Logger,
        settings: settings,
    }
}

func (s *databricksScraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
    metrics := pmetric.NewMetrics()
    resourceMetrics := metrics.ResourceMetrics().AppendEmpty()
    
    resource := resourceMetrics.Resource()
    resource.Attributes().PutStr("databricks.host", s.client.host)
    
    scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
    scopeMetrics.Scope().SetName("databricksreceiver")
    
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

// NEW: Scrape health metric for monitoring
func (s *databricksScraper) addScrapeHealthMetric(scopeMetrics pmetric.ScopeMetrics, errorCount int) {
    healthMetric := scopeMetrics.Metrics().AppendEmpty()
    healthMetric.SetName("databricks.scraper.errors")
    healthMetric.SetDescription("Number of API endpoints that failed during scraping")
    healthMetric.SetUnit("1")
    
    gauge := healthMetric.SetEmptyGauge()
    dp := gauge.DataPoints().AppendEmpty()
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
            successRate := float64(successCount) / float64(totalCompleted) * 100
            
            successRateMetric := scopeMetrics.Metrics().AppendEmpty()
            successRateMetric.SetName("databricks.job.success_rate")
            successRateMetric.SetDescription("Job success rate percentage")
            successRateMetric.SetUnit("%")
            
            gauge := successRateMetric.SetEmptyGauge()
            dp := gauge.DataPoints().AppendEmpty()
            dp.SetDoubleValue(successRate)
            dp.SetTimestamp(now)
        }
    }
    
    // IMPROVED: Limit detailed API calls to recent/important runs
    // Only get details for runs from last 24 hours or failed runs
    maxDetailsToFetch := 20
    detailsFetched := 0
    
    for _, run := range runs {
        // Skip if we've fetched enough details
        if detailsFetched >= maxDetailsToFetch {
            s.logger.Debug("Reached max job run details limit", zap.Int("limit", maxDetailsToFetch))
            break
        }
        
        // Only fetch details for recent runs (last 24h) or failed runs
        oneDayAgo := time.Now().Add(-24 * time.Hour).UnixMilli()
        isRecent := run.StartTime > oneDayAgo
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
            queueMetric.SetDescription("Job queue duration")
            queueMetric.SetUnit("ms")
            
            gauge := queueMetric.SetEmptyGauge()
            dp := gauge.DataPoints().AppendEmpty()
            dp.SetIntValue(queueDuration)
            dp.SetTimestamp(now)
            dp.Attributes().PutStr("job_name", run.RunName)
        }
        
        // Cleanup duration
        if details.CleanupDuration > 0 {
            cleanupMetric := scopeMetrics.Metrics().AppendEmpty()
            cleanupMetric.SetName("databricks.job.duration.cleanup")
            cleanupMetric.SetDescription("Job cleanup duration")
            cleanupMetric.SetUnit("ms")
            
            gauge := cleanupMetric.SetEmptyGauge()
            dp := gauge.DataPoints().AppendEmpty()
            dp.SetIntValue(details.CleanupDuration)
            dp.SetTimestamp(now)
            dp.Attributes().PutStr("job_name", run.RunName)
        }
        
        // Setup duration
        if details.SetupDuration > 0 {
            setupMetric := scopeMetrics.Metrics().AppendEmpty()
            setupMetric.SetName("databricks.job.duration.setup")
            setupMetric.SetDescription("Job setup duration")
            setupMetric.SetUnit("ms")
            
            gauge := setupMetric.SetEmptyGauge()
            dp := gauge.DataPoints().AppendEmpty()
            dp.SetIntValue(details.SetupDuration)
            dp.SetTimestamp(now)
            dp.Attributes().PutStr("job_name", run.RunName)
        }
        
        // Execution duration
        if details.ExecutionDuration > 0 {
            execMetric := scopeMetrics.Metrics().AppendEmpty()
            execMetric.SetName("databricks.job.duration.execution")
            execMetric.SetDescription("Job execution duration")
            execMetric.SetUnit("ms")
            
            gauge := execMetric.SetEmptyGauge()
            dp := gauge.DataPoints().AppendEmpty()
            dp.SetIntValue(details.ExecutionDuration)
            dp.SetTimestamp(now)
            dp.Attributes().PutStr("job_name", run.RunName)
        }
        
        // IMPROVED: Better cost calculation using cluster info
        if run.RunDuration > 0 && details.ClusterInstance.ClusterID != "" {
            clusterInfo, err := s.client.GetClusterInfo(ctx, details.ClusterInstance.ClusterID)
            if err == nil {
                estimatedCost := s.client.calculateJobCost(clusterInfo, run.RunDuration)
                
                costMetric := scopeMetrics.Metrics().AppendEmpty()
                costMetric.SetName("databricks.job.cost")
                costMetric.SetDescription("Estimated job cost in dollars")
                costMetric.SetUnit("USD")
                
                gauge := costMetric.SetEmptyGauge()
                dp := gauge.DataPoints().AppendEmpty()
                dp.SetDoubleValue(estimatedCost)
                dp.SetTimestamp(now)
                dp.Attributes().PutStr("job_name", run.RunName)
                dp.Attributes().PutStr("cluster_id", details.ClusterInstance.ClusterID)
            } else {
                // Fallback to simple estimation
                estimatedCost := float64(run.RunDuration) / (1000 * 60 * 60) * 0.50
                
                costMetric := scopeMetrics.Metrics().AppendEmpty()
                costMetric.SetName("databricks.job.cost")
                costMetric.SetDescription("Estimated job cost in dollars (simplified)")
                costMetric.SetUnit("USD")
                
                gauge := costMetric.SetEmptyGauge()
                dp := gauge.DataPoints().AppendEmpty()
                dp.SetDoubleValue(estimatedCost)
                dp.SetTimestamp(now)
                dp.Attributes().PutStr("job_name", run.RunName)
            }
        }
    }
}

func (s *databricksScraper) addJobRunMetrics(scopeMetrics pmetric.ScopeMetrics, runs []JobRun) {
    jobRunCountMetric := scopeMetrics.Metrics().AppendEmpty()
    jobRunCountMetric.SetName("databricks.job_runs.count")
    jobRunCountMetric.SetDescription("Number of job runs by state")
    jobRunCountMetric.SetUnit("1")
    
    gauge := jobRunCountMetric.SetEmptyGauge()
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
        dp := gauge.DataPoints().AppendEmpty()
        dp.SetIntValue(int64(count))
        dp.SetTimestamp(now)
        dp.Attributes().PutStr("result_state", state)
    }
    
    if len(runs) > 0 {
        runDurationMetric := scopeMetrics.Metrics().AppendEmpty()
        runDurationMetric.SetName("databricks.job.duration.run")
        runDurationMetric.SetDescription("Complete job run duration")
        runDurationMetric.SetUnit("ms")
        
        runGauge := runDurationMetric.SetEmptyGauge()
        for _, run := range runs {
            if run.RunDuration > 0 {
                dp := runGauge.DataPoints().AppendEmpty()
                dp.SetIntValue(run.RunDuration)
                dp.SetTimestamp(now)
                dp.Attributes().PutStr("job_name", run.RunName)
                dp.Attributes().PutStr("result_state", run.State.ResultState)
            }
        }
    }
}

func (s *databricksScraper) addJobMetrics(scopeMetrics pmetric.ScopeMetrics, jobs []Job) {
    jobCountMetric := scopeMetrics.Metrics().AppendEmpty()
    jobCountMetric.SetName("databricks.jobs.total")
    jobCountMetric.SetDescription("Total number of configured jobs")
    jobCountMetric.SetUnit("1")
    
    gauge := jobCountMetric.SetEmptyGauge()
    dp := gauge.DataPoints().AppendEmpty()
    dp.SetIntValue(int64(len(jobs)))
    dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
}

func (s *databricksScraper) addWarehouseMetrics(scopeMetrics pmetric.ScopeMetrics, warehouses []SQLWarehouse) {
    whCountMetric := scopeMetrics.Metrics().AppendEmpty()
    whCountMetric.SetName("databricks.warehouses.count")
    whCountMetric.SetDescription("Number of SQL warehouses by state")
    whCountMetric.SetUnit("1")
    
    gauge := whCountMetric.SetEmptyGauge()
    stateCount := make(map[string]int)
    for _, wh := range warehouses {
        stateCount[wh.State]++
    }
    
    now := pcommon.NewTimestampFromTime(time.Now())
    for state, count := range stateCount {
        dp := gauge.DataPoints().AppendEmpty()
        dp.SetIntValue(int64(count))
        dp.SetTimestamp(now)
        dp.Attributes().PutStr("state", state)
    }
}

func (s *databricksScraper) addWorkspaceMetrics(scopeMetrics pmetric.ScopeMetrics, objects []WorkspaceObject) {
    objCountMetric := scopeMetrics.Metrics().AppendEmpty()
    objCountMetric.SetName("databricks.workspace.objects")
    objCountMetric.SetDescription("Number of workspace objects by type")
    objCountMetric.SetUnit("1")
    
    gauge := objCountMetric.SetEmptyGauge()
    typeCount := make(map[string]int)
    for _, obj := range objects {
        typeCount[obj.ObjectType]++
    }
    
    now := pcommon.NewTimestampFromTime(time.Now())
    for objType, count := range typeCount {
        dp := gauge.DataPoints().AppendEmpty()
        dp.SetIntValue(int64(count))
        dp.SetTimestamp(now)
        dp.Attributes().PutStr("type", objType)
    }
}

func (s *databricksScraper) addDBFSMetrics(scopeMetrics pmetric.ScopeMetrics, files []DBFSFile) {
    storageMetric := scopeMetrics.Metrics().AppendEmpty()
    storageMetric.SetName("databricks.dbfs.storage_bytes")
    storageMetric.SetDescription("Total DBFS storage in bytes")
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
    fileCountMetric.SetName("databricks.dbfs.file_count")
    fileCountMetric.SetDescription("Number of files and directories in DBFS root")
    fileCountMetric.SetUnit("1")
    
    fileGauge := fileCountMetric.SetEmptyGauge()
    dp2 := fileGauge.DataPoints().AppendEmpty()
    dp2.SetIntValue(int64(len(files)))
    dp2.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
}

func (s *databricksScraper) addTokenMetrics(scopeMetrics pmetric.ScopeMetrics, tokens []Token) {
    tokenCountMetric := scopeMetrics.Metrics().AppendEmpty()
    tokenCountMetric.SetName("databricks.tokens.count")
    tokenCountMetric.SetDescription("Number of active tokens")
    tokenCountMetric.SetUnit("1")
    
    gauge := tokenCountMetric.SetEmptyGauge()
    dp := gauge.DataPoints().AppendEmpty()
    dp.SetIntValue(int64(len(tokens)))
    dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
    
    expiryMetric := scopeMetrics.Metrics().AppendEmpty()
    expiryMetric.SetName("databricks.tokens.days_until_expiry")
    expiryMetric.SetDescription("Days until token expiry")
    expiryMetric.SetUnit("d")
    
    expiryGauge := expiryMetric.SetEmptyGauge()
    now := time.Now().UnixMilli()
    
    for _, token := range tokens {
        daysUntilExpiry := (token.ExpiryTime - now) / (1000 * 60 * 60 * 24)
        dp := expiryGauge.DataPoints().AppendEmpty()
        dp.SetIntValue(daysUntilExpiry)
        dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
        dp.Attributes().PutStr("comment", token.Comment)
    }
}

func (s *databricksScraper) addUserMetrics(scopeMetrics pmetric.ScopeMetrics, users []User) {
    userCountMetric := scopeMetrics.Metrics().AppendEmpty()
    userCountMetric.SetName("databricks.users.count")
    userCountMetric.SetDescription("Number of users by status")
    userCountMetric.SetUnit("1")
    
    gauge := userCountMetric.SetEmptyGauge()
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
    dpActive := gauge.DataPoints().AppendEmpty()
    dpActive.SetIntValue(int64(activeCount))
    dpActive.SetTimestamp(now)
    dpActive.Attributes().PutStr("status", "active")
    
    dpInactive := gauge.DataPoints().AppendEmpty()
    dpInactive.SetIntValue(int64(inactiveCount))
    dpInactive.SetTimestamp(now)
    dpInactive.Attributes().PutStr("status", "inactive")
}

func (s *databricksScraper) addGroupMetrics(scopeMetrics pmetric.ScopeMetrics, groups []Group) {
    groupCountMetric := scopeMetrics.Metrics().AppendEmpty()
    groupCountMetric.SetName("databricks.groups.count")
    groupCountMetric.SetDescription("Number of groups")
    groupCountMetric.SetUnit("1")
    
    gauge := groupCountMetric.SetEmptyGauge()
    dp := gauge.DataPoints().AppendEmpty()
    dp.SetIntValue(int64(len(groups)))
    dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
    
    memberCountMetric := scopeMetrics.Metrics().AppendEmpty()
    memberCountMetric.SetName("databricks.groups.members")
    memberCountMetric.SetDescription("Number of members per group")
    memberCountMetric.SetUnit("1")
    
    memberGauge := memberCountMetric.SetEmptyGauge()
    for _, group := range groups {
        dp := memberGauge.DataPoints().AppendEmpty()
        dp.SetIntValue(int64(len(group.Members)))
        dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
        dp.Attributes().PutStr("group_name", group.DisplayName)
    }
}

func (s *databricksScraper) addPolicyMetrics(scopeMetrics pmetric.ScopeMetrics, policies []ClusterPolicy) {
    policyCountMetric := scopeMetrics.Metrics().AppendEmpty()
    policyCountMetric.SetName("databricks.policies.count")
    policyCountMetric.SetDescription("Number of cluster policies")
    policyCountMetric.SetUnit("1")
    
    gauge := policyCountMetric.SetEmptyGauge()
    dp := gauge.DataPoints().AppendEmpty()
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
    policyTypeMetric.SetName("databricks.policies.by_type")
    policyTypeMetric.SetDescription("Cluster policies by type")
    policyTypeMetric.SetUnit("1")
    
    typeGauge := policyTypeMetric.SetEmptyGauge()
    now := pcommon.NewTimestampFromTime(time.Now())
    
    dpDefault := typeGauge.DataPoints().AppendEmpty()
    dpDefault.SetIntValue(int64(defaultCount))
    dpDefault.SetTimestamp(now)
    dpDefault.Attributes().PutStr("type", "default")
    
    dpCustom := typeGauge.DataPoints().AppendEmpty()
    dpCustom.SetIntValue(int64(customCount))
    dpCustom.SetTimestamp(now)
    dpCustom.Attributes().PutStr("type", "custom")
}

func (s *databricksScraper) addTaskMetrics(ctx context.Context, scopeMetrics pmetric.ScopeMetrics, runs []JobRun) {
    taskDurationMetric := scopeMetrics.Metrics().AppendEmpty()
    taskDurationMetric.SetName("databricks.tasks.duration")
    taskDurationMetric.SetDescription("Task execution duration")
    taskDurationMetric.SetUnit("ms")
    
    taskGauge := taskDurationMetric.SetEmptyGauge()
    
    setupDurationMetric := scopeMetrics.Metrics().AppendEmpty()
    setupDurationMetric.SetName("databricks.tasks.setup_duration")
    setupDurationMetric.SetDescription("Task setup duration")
    setupDurationMetric.SetUnit("ms")
    
    setupGauge := setupDurationMetric.SetEmptyGauge()
    
    // IMPROVED: Limit task detail fetching to avoid excessive API calls
    maxTaskDetailsToFetch := 10
    taskDetailsFetched := 0
    
    for _, run := range runs {
        if taskDetailsFetched >= maxTaskDetailsToFetch {
            break
        }
        
        // Only fetch task details for recent or failed runs
        oneDayAgo := time.Now().Add(-24 * time.Hour).UnixMilli()
        if run.StartTime < oneDayAgo && run.State.ResultState != "FAILED" {
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
            dp.Attributes().PutStr("task_key", task.TaskKey)
            dp.Attributes().PutStr("result_state", task.State.ResultState)
            
            if task.SetupDuration > 0 {
                dp2 := setupGauge.DataPoints().AppendEmpty()
                dp2.SetIntValue(task.SetupDuration)
                dp2.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
                dp2.Attributes().PutStr("task_key", task.TaskKey)
            }
        }
    }
}
