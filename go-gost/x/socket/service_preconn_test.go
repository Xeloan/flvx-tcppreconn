package socket

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-gost/x/config"
)

func TestCreateServicesPersistsManagedPreconnConfigs(t *testing.T) {
	baseName := "1_1_0"
	restoreConfig := config.Global()
	config.Set(&config.Config{})
	defer config.Set(restoreConfig)

	mgr := GetPreconnManager()
	mgr.StopAll()
	defer mgr.StopAll()

	restorePath := prependStubTCPPoolToPath(t)
	defer restorePath()

	req := createServicesRequest{
		Data: []*config.ServiceConfig{
			newPreconnServiceConfig(baseName+"_tcp", "0.0.0.0:1234", "5.6.7.8:4321", false),
			newPreconnServiceConfig(baseName+"_udp", "0.0.0.0:1234", "5.6.7.8:4321", false),
		},
	}
	if err := createServices(req); err != nil {
		t.Fatalf("createServices: %v", err)
	}

	if !mgr.IsManaged(baseName) {
		t.Fatalf("expected preconn process to be running after create")
	}

	cfg := config.Global()
	want := []string{baseName + "_tcp", baseName + "_udp"}
	got := serviceConfigNames(cfg.Services)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected persisted service configs %v, got %v", want, got)
	}
}

func TestUpdateServicesPersistsManagedPreconnConfigs(t *testing.T) {
	baseName := "1_1_0"
	restoreConfig := config.Global()
	config.Set(&config.Config{
		Services: []*config.ServiceConfig{
			newPreconnServiceConfig(baseName+"_tcp", "0.0.0.0:1234", "5.6.7.8:4321", false),
			newPreconnServiceConfig(baseName+"_udp", "0.0.0.0:1234", "5.6.7.8:4321", false),
		},
	})
	defer config.Set(restoreConfig)

	mgr := GetPreconnManager()
	mgr.StopAll()
	defer mgr.StopAll()

	restorePath := prependStubTCPPoolToPath(t)
	defer restorePath()

	if err := updateServices(updateServicesRequest{
		Data: []*config.ServiceConfig{
			newPreconnServiceConfig(baseName+"_tcp", "0.0.0.0:2234", "9.8.7.6:5432", false),
			newPreconnServiceConfig(baseName+"_udp", "0.0.0.0:2234", "9.8.7.6:5432", false),
		},
	}); err != nil {
		t.Fatalf("updateServices: %v", err)
	}

	if !mgr.IsManaged(baseName) {
		t.Fatalf("expected preconn process to be running after update")
	}

	cfg := config.Global()
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 persisted services, got %d", len(cfg.Services))
	}
	for _, serviceConfig := range cfg.Services {
		if serviceConfig.Addr != "0.0.0.0:2234" {
			t.Fatalf("expected updated addr to persist, got %s", serviceConfig.Addr)
		}
		if target := extractFirstForwarderTarget(serviceConfig); target != "9.8.7.6:5432" {
			t.Fatalf("expected updated target to persist, got %s", target)
		}
	}
}

func TestPauseServicesHandlesManagedPreconn(t *testing.T) {
	baseName := "1_1_0"
	restoreConfig := config.Global()
	config.Set(&config.Config{
		Services: []*config.ServiceConfig{
			newPreconnServiceConfig(baseName+"_tcp", "0.0.0.0:1234", "5.6.7.8:4321", false),
			newPreconnServiceConfig(baseName+"_udp", "0.0.0.0:1234", "5.6.7.8:4321", false),
		},
	})
	defer config.Set(restoreConfig)

	mgr := GetPreconnManager()
	mgr.StopAll()
	defer mgr.StopAll()

	mgr.addMockProcess(baseName)

	if err := pauseServices(pauseServicesRequest{Services: []string{baseName + "_tcp"}}); err != nil {
		t.Fatalf("pauseServices: %v", err)
	}

	if mgr.IsManaged(baseName) {
		t.Fatalf("expected preconn process to be stopped")
	}

	cfg := config.Global()
	for _, serviceConfig := range cfg.Services {
		if !isServicePaused(serviceConfig) {
			t.Fatalf("expected service %s to be marked paused", serviceConfig.Name)
		}
	}
}

func TestResumeServicesHandlesPreconn(t *testing.T) {
	baseName := "1_1_0"
	restoreConfig := config.Global()
	config.Set(&config.Config{
		Services: []*config.ServiceConfig{
			newPreconnServiceConfig(baseName+"_tcp", "0.0.0.0:1234", "5.6.7.8:4321", true),
			newPreconnServiceConfig(baseName+"_udp", "0.0.0.0:1234", "5.6.7.8:4321", true),
		},
	})
	defer config.Set(restoreConfig)

	mgr := GetPreconnManager()
	mgr.StopAll()
	defer mgr.StopAll()

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "tcp_pool")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\ntrap 'exit 0' TERM INT\nwhile :; do sleep 1; done\n"), 0o755); err != nil {
		t.Fatalf("write tcp_pool stub: %v", err)
	}

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	defer os.Setenv("PATH", originalPath)

	if err := resumeServices(resumeServicesRequest{Services: []string{baseName + "_tcp"}}); err != nil {
		t.Fatalf("resumeServices: %v", err)
	}

	if !mgr.IsManaged(baseName) {
		t.Fatalf("expected preconn process to be running")
	}

	cfg := config.Global()
	for _, serviceConfig := range cfg.Services {
		if isServicePaused(serviceConfig) {
			t.Fatalf("expected service %s to be resumed", serviceConfig.Name)
		}
	}

	time.Sleep(50 * time.Millisecond)
}

func newPreconnServiceConfig(name, listenAddr, remoteAddr string, paused bool) *config.ServiceConfig {
	metadata := map[string]any{
		"tcpPreconn": true,
	}
	if paused {
		metadata["paused"] = true
	}
	return &config.ServiceConfig{
		Name: name,
		Addr: listenAddr,
		Forwarder: &config.ForwarderConfig{
			Nodes: []*config.ForwardNodeConfig{
				{Addr: remoteAddr},
			},
		},
		Metadata: metadata,
	}
}

func (m *PreconnManager) addMockProcess(baseName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[baseName] = &preconnProcess{baseName: baseName}
}

func prependStubTCPPoolToPath(t *testing.T) func() {
	t.Helper()

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "tcp_pool")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\ntrap 'exit 0' TERM INT\nwhile :; do sleep 1; done\n"), 0o755); err != nil {
		t.Fatalf("write tcp_pool stub: %v", err)
	}

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}

	return func() {
		_ = os.Setenv("PATH", originalPath)
	}
}

func serviceConfigNames(serviceConfigs []*config.ServiceConfig) []string {
	names := make([]string, 0, len(serviceConfigs))
	for _, serviceConfig := range serviceConfigs {
		if serviceConfig == nil {
			continue
		}
		names = append(names, serviceConfig.Name)
	}
	return names
}
