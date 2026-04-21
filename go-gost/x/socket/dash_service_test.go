package socket

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-gost/x/config"
)

func TestPauseServicesDashModeUsesConfigAndDashAPI(t *testing.T) {
	origDashMode := IsDashMode
	origBaseURL := dashAPIBaseURL
	origClient := dashHTTPClient
	origConfig := config.Global()
	defer func() {
		IsDashMode = origDashMode
		dashAPIBaseURL = origBaseURL
		dashHTTPClient = origClient
		config.Set(origConfig)
	}()

	IsDashMode = true
	config.Set(&config.Config{
		Services: []*config.ServiceConfig{
			{Name: "service1_1_0"},
		},
	})

	var deletePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deletePath = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dashAPIBaseURL = srv.URL
	dashHTTPClient = &http.Client{Timeout: time.Second}

	if err := pauseServices(pauseServicesRequest{Services: []string{"service1_1_0"}}); err != nil {
		t.Fatalf("pauseServices: %v", err)
	}

	if deletePath != "DELETE /config/services/service1_1_0" {
		t.Fatalf("unexpected dash request: %s", deletePath)
	}

	cfg := config.Global()
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service config, got %d", len(cfg.Services))
	}
	if paused := cfg.Services[0].Metadata["paused"]; paused != true {
		t.Fatalf("expected paused metadata to be true, got %#v", paused)
	}
}

func TestResumeServicesDashModeUsesConfigAndDashAPI(t *testing.T) {
	origDashMode := IsDashMode
	origBaseURL := dashAPIBaseURL
	origClient := dashHTTPClient
	origConfig := config.Global()
	defer func() {
		IsDashMode = origDashMode
		dashAPIBaseURL = origBaseURL
		dashHTTPClient = origClient
		config.Set(origConfig)
	}()

	IsDashMode = true
	config.Set(&config.Config{
		Services: []*config.ServiceConfig{
			{
				Name:     "service1_1_0",
				Metadata: map[string]any{"paused": true},
			},
		},
	})

	var postPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postPath = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dashAPIBaseURL = srv.URL
	dashHTTPClient = &http.Client{Timeout: time.Second}

	if err := resumeServices(resumeServicesRequest{Services: []string{"service1_1_0"}}); err != nil {
		t.Fatalf("resumeServices: %v", err)
	}

	if postPath != "POST /config/services" {
		t.Fatalf("unexpected dash request: %s", postPath)
	}

	cfg := config.Global()
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service config, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Metadata != nil {
		if _, ok := cfg.Services[0].Metadata["paused"]; ok {
			t.Fatalf("expected paused metadata to be removed, got %#v", cfg.Services[0].Metadata)
		}
	}
}
