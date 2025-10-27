// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package databricksreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type databricksClient struct {
	client *http.Client
	host   string
	token  string
	cfg    *Config

	// Cache for job run details to avoid excessive API calls
	runDetailsCache map[int64]*cachedRunDetails
	cacheMu         sync.RWMutex
	cacheExpiry     time.Duration
}

type cachedRunDetails struct {
	details   *JobRunDetailed
	timestamp time.Time
}

func newDatabricksClient(cfg *Config) *databricksClient {
	return &databricksClient{
		client:          &http.Client{Timeout: 30 * time.Second},
		host:            cfg.Host,
		token:           string(cfg.Token),
		cfg:             cfg,
		runDetailsCache: make(map[int64]*cachedRunDetails),
		cacheExpiry:     5 * time.Minute,
	}
}

// Job Runs structures
type JobRun struct {
	JobID         int64  `json:"job_id"`
	RunID         int64  `json:"run_id"`
	RunName       string `json:"run_name"`
	State         struct {
		LifeCycleState string `json:"life_cycle_state"`
		ResultState    string `json:"result_state"`
	} `json:"state"`
	StartTime         int64 `json:"start_time"`
	EndTime           int64 `json:"end_time"`
	ExecutionDuration int64 `json:"execution_duration"`
	RunDuration       int64 `json:"run_duration"`
}

type JobRunsResponse struct {
	Runs    []JobRun `json:"runs"`
	HasMore bool     `json:"has_more"`
}

// Jobs structures
type Job struct {
	JobID       int64  `json:"job_id"`
	CreatorUser string `json:"creator_user_name"`
	Settings    struct {
		Name              string `json:"name"`
		MaxConcurrentRuns int    `json:"max_concurrent_runs"`
		TimeoutSeconds    int    `json:"timeout_seconds"`
	} `json:"settings"`
	CreatedTime int64 `json:"created_time"`
}

type JobsResponse struct {
	Jobs []Job `json:"jobs"`
}

// SQL Warehouses structures
type SQLWarehouse struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	Size                    string `json:"size"`
	State                   string `json:"state"`
	NumClusters             int    `json:"num_clusters"`
	AutoStopMins            int    `json:"auto_stop_mins"`
	EnableServerlessCompute bool   `json:"enable_serverless_compute"`
}

type WarehousesResponse struct {
	Warehouses []SQLWarehouse `json:"warehouses"`
}

// Workspace structures
type WorkspaceObject struct {
	Path       string `json:"path"`
	ObjectType string `json:"object_type"`
	ObjectID   int64  `json:"object_id"`
}

type WorkspaceResponse struct {
	Objects []WorkspaceObject `json:"objects"`
}

// DBFS structures
type DBFSFile struct {
	Path             string `json:"path"`
	IsDir            bool   `json:"is_dir"`
	FileSize         int64  `json:"file_size"`
	ModificationTime int64  `json:"modification_time"`
}

type DBFSResponse struct {
	Files []DBFSFile `json:"files"`
}

// Token structures
type Token struct {
	TokenID      string `json:"token_id"`
	CreationTime int64  `json:"creation_time"`
	ExpiryTime   int64  `json:"expiry_time"`
	Comment      string `json:"comment"`
	OwnerID      int64  `json:"owner_id"`
	LastUsedDay  int64  `json:"last_used_day"`
}

type TokensResponse struct {
	TokenInfos []Token `json:"token_infos"`
}

// Users structures
type User struct {
	ID          string `json:"id"`
	UserName    string `json:"userName"`
	DisplayName string `json:"displayName"`
	Active      bool   `json:"active"`
}

type UsersResponse struct {
	Resources    []User `json:"Resources"`
	TotalResults int    `json:"totalResults"`
}

// Groups structures
type Group struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Members     []struct {
		Display string `json:"display"`
		Value   string `json:"value"`
	} `json:"members"`
}

type GroupsResponse struct {
	Resources    []Group `json:"Resources"`
	TotalResults int     `json:"totalResults"`
}

// Cluster Policy structures
type ClusterPolicy struct {
	PolicyID    string `json:"policy_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	CreatedAt   int64  `json:"created_at_timestamp"`
}

type PoliciesResponse struct {
	Policies   []ClusterPolicy `json:"policies"`
	TotalCount int             `json:"total_count"`
}

// Extended Job Run structures (with tasks)
type TaskRun struct {
	TaskKey string `json:"task_key"`
	State   struct {
		LifeCycleState string `json:"life_cycle_state"`
		ResultState    string `json:"result_state"`
	} `json:"state"`
	StartTime         int64 `json:"start_time"`
	EndTime           int64 `json:"end_time"`
	ExecutionDuration int64 `json:"execution_duration"`
	SetupDuration     int64 `json:"setup_duration"`
}

type JobRunDetailed struct {
	JobID             int64     `json:"job_id"`
	RunID             int64     `json:"run_id"`
	State             struct {
		LifeCycleState string `json:"life_cycle_state"`
		ResultState    string `json:"result_state"`
	} `json:"state"`
	Tasks             []TaskRun `json:"tasks"`
	StartTime         int64     `json:"start_time"`
	EndTime           int64     `json:"end_time"`
	ExecutionDuration int64     `json:"execution_duration"`
	SetupDuration     int64     `json:"setup_duration"`
	CleanupDuration   int64     `json:"cleanup_duration"`
	ClusterInstance   struct {
		ClusterID string `json:"cluster_id"`
	} `json:"cluster_instance"`
}

// Extended structures for cost calculation
type ClusterInfo struct {
	ClusterID      string `json:"cluster_id"`
	NodeTypeID     string `json:"node_type_id"`
	DriverNodeType string `json:"driver_node_type_id"`
	NumWorkers     int    `json:"num_workers"`
	AutoScale      struct {
		MinWorkers int `json:"min_workers"`
		MaxWorkers int `json:"max_workers"`
	} `json:"autoscale"`
	ClusterSource string `json:"cluster_source"`
}

// API Methods
func (c *databricksClient) GetJobRuns(ctx context.Context) ([]JobRun, error) {
	url := fmt.Sprintf("%s/api/2.1/jobs/runs/list", c.host)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response JobRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Runs, nil
}

func (c *databricksClient) GetJobs(ctx context.Context) ([]Job, error) {
	url := fmt.Sprintf("%s/api/2.1/jobs/list", c.host)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response JobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Jobs, nil
}

func (c *databricksClient) GetSQLWarehouses(ctx context.Context) ([]SQLWarehouse, error) {
	url := fmt.Sprintf("%s/api/2.0/sql/warehouses", c.host)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response WarehousesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Warehouses, nil
}

// FIXED: Use query parameters instead of GET with body
func (c *databricksClient) GetWorkspace(ctx context.Context, path string) ([]WorkspaceObject, error) {
	baseURL := fmt.Sprintf("%s/api/2.0/workspace/list", c.host)

	// Add path as query parameter
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("path", path)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response WorkspaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Objects, nil
}

// FIXED: Use query parameters instead of GET with body
func (c *databricksClient) GetDBFS(ctx context.Context) ([]DBFSFile, error) {
	baseURL := fmt.Sprintf("%s/api/2.0/dbfs/list", c.host)

	// Add path as query parameter
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("path", "/")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response DBFSResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Files, nil
}

func (c *databricksClient) GetTokens(ctx context.Context) ([]Token, error) {
	url := fmt.Sprintf("%s/api/2.0/token-management/tokens", c.host)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response TokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.TokenInfos, nil
}

func (c *databricksClient) GetUsers(ctx context.Context) ([]User, error) {
	url := fmt.Sprintf("%s/api/2.0/preview/scim/v2/Users", c.host)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response UsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Resources, nil
}

func (c *databricksClient) GetGroups(ctx context.Context) ([]Group, error) {
	url := fmt.Sprintf("%s/api/2.0/preview/scim/v2/Groups", c.host)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response GroupsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Resources, nil
}

func (c *databricksClient) GetClusterPolicies(ctx context.Context) ([]ClusterPolicy, error) {
	url := fmt.Sprintf("%s/api/2.0/policies/clusters/list", c.host)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response PoliciesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Policies, nil
}

// IMPROVED: Added caching to reduce API calls
func (c *databricksClient) GetJobRunDetails(ctx context.Context, runID int64) (*JobRunDetailed, error) {
	// Check cache first
	c.cacheMu.RLock()
	if cached, ok := c.runDetailsCache[runID]; ok {
		if time.Since(cached.timestamp) < c.cacheExpiry {
			c.cacheMu.RUnlock()
			return cached.details, nil
		}
	}
	c.cacheMu.RUnlock()

	// Cache miss or expired, fetch from API
	url := fmt.Sprintf("%s/api/2.1/jobs/runs/get?run_id=%d", c.host, runID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var jobRun JobRunDetailed
	if err := json.NewDecoder(resp.Body).Decode(&jobRun); err != nil {
		return nil, err
	}

	// Store in cache
	c.cacheMu.Lock()
	c.runDetailsCache[runID] = &cachedRunDetails{
		details:   &jobRun,
		timestamp: time.Now(),
	}
	c.cacheMu.Unlock()

	return &jobRun, nil
}

func (c *databricksClient) GetClusterInfo(ctx context.Context, clusterID string) (*ClusterInfo, error) {
	url := fmt.Sprintf("%s/api/2.0/clusters/get?cluster_id=%s", c.host, clusterID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cluster not found or API error")
	}

	var clusterInfo ClusterInfo
	if err := json.NewDecoder(resp.Body).Decode(&clusterInfo); err != nil {
		return nil, err
	}

	return &clusterInfo, nil
}

// IMPROVED: Use configurable rates
func (c *databricksClient) calculateJobCost(clusterInfo *ClusterInfo, durationMs int64) float64 {
	// Convert ms to hours
	durationHours := float64(durationMs) / (1000 * 60 * 60)

	// Get DBU rate from config (with fallback to defaults)
	dbuRate := 1.0 // default
	if c.cfg.NodeTypeDBURates != nil {
		if rate, ok := c.cfg.NodeTypeDBURates[clusterInfo.NodeTypeID]; ok {
			dbuRate = rate
		}
	}

	// Calculate worker count
	workerCount := clusterInfo.NumWorkers
	if workerCount == 0 && clusterInfo.AutoScale.MinWorkers > 0 {
		// Use average of autoscale range
		workerCount = (clusterInfo.AutoScale.MinWorkers + clusterInfo.AutoScale.MaxWorkers) / 2
	}
	if workerCount == 0 {
		workerCount = 1
	}

	// Total DBUs = (1 driver + N workers) * DBU rate * hours
	totalDBUs := float64(1+workerCount) * dbuRate * durationHours

	// Use configured price per DBU
	costPerDBU := c.cfg.DBUPricePerUnit
	if costPerDBU == 0 {
		costPerDBU = 0.15 // fallback
	}

	return totalDBUs * costPerDBU
}

// Clean up old cache entries periodically
func (c *databricksClient) cleanCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	now := time.Now()
	for runID, cached := range c.runDetailsCache {
		if now.Sub(cached.timestamp) > c.cacheExpiry {
			delete(c.runDetailsCache, runID)
		}
	}
}
