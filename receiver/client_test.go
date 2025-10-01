package databricksreceiver

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestGetJobsSuccess(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/2.1/jobs/list" {
            t.Errorf("Expected path /api/2.1/jobs/list, got %s", r.URL.Path)
        }
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"jobs": [{"job_id": 123, "settings": {"name": "test"}}]}`))
    }))
    defer server.Close()

    cfg := &Config{Host: server.URL, Token: "test"}
    client := newDatabricksClient(cfg)
    
    jobs, err := client.GetJobs(context.Background())
    if err != nil {
        t.Fatalf("GetJobs failed: %v", err)
    }
    if len(jobs) != 1 {
        t.Errorf("Expected 1 job, got %d", len(jobs))
    }
}

func TestGetJobsUnauthorized(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
    }))
    defer server.Close()

    cfg := &Config{Host: server.URL, Token: "bad"}
    client := newDatabricksClient(cfg)
    
    _, err := client.GetJobs(context.Background())
    if err == nil {
        t.Error("Expected error for unauthorized request")
    }
}
