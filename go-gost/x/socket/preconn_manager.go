package socket

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// PreconnManager manages tcp_pool child processes for forwards with TCP pre-connection enabled.
// Each managed forward has one tcp_pool process that handles both TCP and UDP forwarding.
type PreconnManager struct {
	mu        sync.Mutex
	processes map[string]*preconnProcess // keyed by forward service base name (e.g. "forward_1")
}

type preconnProcess struct {
	cmd      *exec.Cmd
	baseName string // e.g. "forward_1"
}

var preconnMgr = &PreconnManager{
	processes: make(map[string]*preconnProcess),
}

// GetPreconnManager returns the singleton preconn manager.
func GetPreconnManager() *PreconnManager {
	return preconnMgr
}

// tcpPoolBinaryPath returns the path to the tcp_pool binary.
// It looks in the same directory as the running agent binary first,
// then falls back to well-known install paths.
func tcpPoolBinaryPath() string {
	// Try same directory as the agent binary
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "tcp_pool")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback paths
	for _, p := range []string{"/etc/flux_agent/tcp_pool", "/usr/local/bin/tcp_pool"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "tcp_pool" // rely on PATH
}

// StartPreconn launches a tcp_pool process for the given forward.
// serviceName is the base service name (e.g. "forward_1") without the _tcp/_udp suffix.
// listenAddr is the bind address (e.g. "[::]:10000").
// remoteAddr is the first target address (e.g. "1.2.3.4:8080").
func (m *PreconnManager) StartPreconn(serviceName, listenAddr, remoteAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop existing process if any
	m.stopLocked(serviceName)

	localIP, localPort, err := parseListenAddr(listenAddr)
	if err != nil {
		return fmt.Errorf("preconn: parse listen addr %q: %w", listenAddr, err)
	}

	remoteIP, remotePort, err := parseRemoteAddr(remoteAddr)
	if err != nil {
		return fmt.Errorf("preconn: parse remote addr %q: %w", remoteAddr, err)
	}

	binPath := tcpPoolBinaryPath()
	cmd := exec.Command(binPath)
	cmd.Env = append(os.Environ(),
		"LOCAL_IP="+localIP,
		"LOCAL_PORT="+localPort,
		"REMOTE_IP="+remoteIP,
		"REMOTE_TCP_PORT="+remotePort,
		"REMOTE_UDP_PORT="+remotePort,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Create new process group so we can kill the entire group on stop
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("preconn: start tcp_pool for %s: %w", serviceName, err)
	}

	fmt.Printf("[preconn] started tcp_pool pid=%d for %s (%s -> %s)\n",
		cmd.Process.Pid, serviceName, listenAddr, remoteAddr)

	p := &preconnProcess{
		cmd:      cmd,
		baseName: serviceName,
	}
	m.processes[serviceName] = p

	// Reap the child process in background
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if cur, ok := m.processes[serviceName]; ok && cur == p {
			delete(m.processes, serviceName)
		}
		m.mu.Unlock()
		fmt.Printf("[preconn] tcp_pool for %s exited\n", serviceName)
	}()

	return nil
}

// StopPreconn stops the tcp_pool process for the given forward.
func (m *PreconnManager) StopPreconn(serviceName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(serviceName)
}

func (m *PreconnManager) stopLocked(serviceName string) {
	p, ok := m.processes[serviceName]
	if !ok {
		return
	}
	if p.cmd != nil && p.cmd.Process != nil {
		// Kill the entire process group
		pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = p.cmd.Process.Kill()
		}
		fmt.Printf("[preconn] stopped tcp_pool pid=%d for %s\n", p.cmd.Process.Pid, serviceName)
	}
	delete(m.processes, serviceName)
}

// IsManaged returns true if a tcp_pool process is running for this forward.
func (m *PreconnManager) IsManaged(serviceName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.processes[serviceName]
	return ok
}

// StopAll stops all tcp_pool processes.
func (m *PreconnManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name := range m.processes {
		m.stopLocked(name)
	}
}

// parseListenAddr parses "[::]:10000" or "0.0.0.0:10000" into IP and port.
func parseListenAddr(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", err
	}
	if host == "" || host == "0.0.0.0" {
		// Keep the legacy empty-host behavior mapped to IPv4 wildcard, but
		// preserve an explicit "::" so dual-stack listeners stay IPv6-capable.
		host = "0.0.0.0"
	}
	return host, port, nil
}

// parseRemoteAddr parses "1.2.3.4:8080" or "[2001:db8::1]:8080" or "domain.com:8080" into host and port.
func parseRemoteAddr(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("invalid remote address %q: %w", addr, err)
	}
	return host, port, nil
}

// ExtractPreconnBaseName extracts the base forward name from a service name.
// e.g. "forward_1_tcp" -> "forward_1", "forward_1_udp" -> "forward_1"
func ExtractPreconnBaseName(serviceName string) string {
	if strings.HasSuffix(serviceName, "_tcp") {
		return strings.TrimSuffix(serviceName, "_tcp")
	}
	if strings.HasSuffix(serviceName, "_udp") {
		return strings.TrimSuffix(serviceName, "_udp")
	}
	return serviceName
}
