package socket

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-gost/core/service"
	"github.com/go-gost/x/config"
	parser "github.com/go-gost/x/config/parsing/service"
	kill "github.com/go-gost/x/internal/util/port"
	"github.com/go-gost/x/registry"
)

// isPreconnService checks if a service config has tcpPreconn metadata.
func isPreconnService(cfg *config.ServiceConfig) bool {
	if cfg == nil || cfg.Metadata == nil {
		return false
	}
	v, ok := cfg.Metadata["tcpPreconn"]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val == "true" || val == "1"
	}
	return false
}

// extractFirstForwarderTarget extracts the first target address from a ServiceConfig's forwarder.
func extractFirstForwarderTarget(cfg *config.ServiceConfig) string {
	if cfg == nil || cfg.Forwarder == nil {
		return ""
	}
	for _, n := range cfg.Forwarder.Nodes {
		if n != nil && strings.TrimSpace(n.Addr) != "" {
			return strings.TrimSpace(n.Addr)
		}
	}
	return ""
}

// collectPreconnConfigs returns the service configs from allConfigs that are NOT
// in remaining, i.e. those that were delegated to the PreconnManager.
func collectPreconnConfigs(allConfigs, remaining []*config.ServiceConfig) []*config.ServiceConfig {
	remainingSet := make(map[string]bool, len(remaining))
	for _, c := range remaining {
		remainingSet[c.Name] = true
	}
	var out []*config.ServiceConfig
	for _, c := range allConfigs {
		if !remainingSet[c.Name] {
			out = append(out, c)
		}
	}
	return out
}

// upsertServiceConfigs updates existing entries in c.Services or appends new ones.
func upsertServiceConfigs(c *config.Config, cfgs []*config.ServiceConfig) {
	for _, cfg := range cfgs {
		found := false
		for j := range c.Services {
			if c.Services[j].Name == cfg.Name {
				c.Services[j] = cfg
				found = true
				break
			}
		}
		if !found {
			c.Services = append(c.Services, cfg)
		}
	}
}

// isServicePaused reports whether the service config carries a "paused" metadata flag.
func isServicePaused(cfg *config.ServiceConfig) bool {
	if cfg == nil || cfg.Metadata == nil {
		return false
	}
	v, ok := cfg.Metadata["paused"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func createServices(req createServicesRequest) error {

	if len(req.Data) == 0 {
		return errors.New("services list cannot be empty")
	}

	// Check for preconn services — delegate to PreconnManager instead of gost
	preconnHandled, remaining, err := handlePreconnServices(req.Data, true)
	if err != nil {
		return err
	}
	// Collect preconn service configs so they can be persisted in global config.
	var preconnConfigs []*config.ServiceConfig
	if preconnHandled {
		preconnConfigs = collectPreconnConfigs(req.Data, remaining)
	}
	if preconnHandled && len(remaining) == 0 {
		// Persist preconn service configs so pause/resume can find them later.
		if len(preconnConfigs) > 0 {
			config.OnUpdate(func(c *config.Config) error {
				c.Services = append(c.Services, preconnConfigs...)
				return nil
			})
		}
		return nil
	}
	req.Data = remaining

	// 第一阶段：验证所有服务配置
	var parsedServices []struct {
		config  *config.ServiceConfig
		service service.Service
	}

	for _, serviceConfig := range req.Data {
		name := strings.TrimSpace(serviceConfig.Name)
		if name == "" {
			return errors.New("service name is required")
		}
		serviceConfig.Name = name

		if registry.ServiceRegistry().IsRegistered(name) {
			return errors.New("service " + name + " already exists")
		}

		svc, err := parser.ParseService(serviceConfig)
		if err != nil {
			return errors.New("create service " + name + " failed: " + err.Error())
		}

		parsedServices = append(parsedServices, struct {
			config  *config.ServiceConfig
			service service.Service
		}{serviceConfig, svc})
	}

	// 第二阶段：注册所有服务
	var registeredServices []string
	for _, ps := range parsedServices {
		if err := registry.ServiceRegistry().Register(ps.config.Name, ps.service); err != nil {
			// 如果注册失败，回滚已注册的服务
			for _, regName := range registeredServices {
				if svc := registry.ServiceRegistry().Get(regName); svc != nil {
					registry.ServiceRegistry().Unregister(regName)
					svc.Close()
				}
			}
			return errors.New("service " + ps.config.Name + " already exists")
		}
		registeredServices = append(registeredServices, ps.config.Name)
	}

	// 第三阶段：启动所有服务
	if !IsDashMode {
		for _, ps := range parsedServices {
			if svc := registry.ServiceRegistry().Get(ps.config.Name); svc != nil {
				go svc.Serve()
			}
		}
	}

	// 第四阶段：更新配置
	config.OnUpdate(func(c *config.Config) error {
		for _, ps := range parsedServices {
			c.Services = append(c.Services, ps.config)
		}
		// Also persist any preconn service configs that were handled separately.
		c.Services = append(c.Services, preconnConfigs...)
		return nil
	})

	return nil
}

func updateServices(req updateServicesRequest) error {

	if len(req.Data) == 0 {
		return errors.New("services list cannot be empty")
	}

	// 第一阶段：验证所有服务名称有效性
	for i := range req.Data {
		name := strings.TrimSpace(req.Data[i].Name)
		if name == "" {
			return errors.New("service name is required")
		}
		req.Data[i].Name = name
	}

	// Check for preconn services — delegate to PreconnManager
	// On update, first stop any existing preconn process and old gost services for these forwards
	preconnHandled, remaining, err := handlePreconnServices(req.Data, false)
	if err != nil {
		return err
	}
	// Collect preconn service configs so they can be persisted in global config.
	var preconnConfigs []*config.ServiceConfig
	if preconnHandled {
		preconnConfigs = collectPreconnConfigs(req.Data, remaining)
	}
	if preconnHandled && len(remaining) == 0 {
		// Persist preconn service configs (upsert) so pause/resume can find them later.
		if len(preconnConfigs) > 0 {
			config.OnUpdate(func(c *config.Config) error {
				upsertServiceConfigs(c, preconnConfigs)
				return nil
			})
		}
		return nil
	}
	req.Data = remaining

	// For remaining non-preconn services, also stop any preconn process that
	// may have been running for the same forward (user toggled preconn off)
	for _, cfg := range req.Data {
		baseName := ExtractPreconnBaseName(cfg.Name)
		mgr := GetPreconnManager()
		if mgr.IsManaged(baseName) {
			mgr.StopPreconn(baseName)
			fmt.Printf("[preconn] stopped preconn for %s (switched to gost mode)\n", baseName)
		}
	}

	// 第二阶段：逐个更新服务（Upsert模式：存在则更新，不存在则创建）
	for i := range req.Data {
		serviceConfig := req.Data[i]
		name := serviceConfig.Name

		// 1. 获取旧服务
		old := registry.ServiceRegistry().Get(name)

		// 2. 关闭旧服务 (如果存在)
		if old != nil {
			if !IsDashMode {
				old.Close()
			}
			// 3. 从注册表移除旧服务
			registry.ServiceRegistry().Unregister(name)
		}

		// 4. 解析新服务配置
		svc, err := parser.ParseService(serviceConfig)
		if err != nil {
			return errors.New("create service " + name + " failed: " + err.Error())
		}

		// 5. 注册新服务
		if err := registry.ServiceRegistry().Register(name, svc); err != nil {
			svc.Close()
			return errors.New("service " + name + " already exists")
		}

		// 6. 启动新服务
		if !IsDashMode {
			go svc.Serve()
		}
	}

	// 第三阶段：更新配置
	config.OnUpdate(func(c *config.Config) error {
		// Upsert gost service configs
		upsertServiceConfigs(c, req.Data)
		// Also upsert any preconn service configs that were handled separately.
		upsertServiceConfigs(c, preconnConfigs)
		return nil
	})

	return nil
}

func deleteServices(req deleteServicesRequest) error {

	if len(req.Services) == 0 {
		return errors.New("services list cannot be empty")
	}

	// Stop any preconn processes for these services
	mgr := GetPreconnManager()
	for _, serviceName := range req.Services {
		baseName := ExtractPreconnBaseName(strings.TrimSpace(serviceName))
		if mgr.IsManaged(baseName) {
			mgr.StopPreconn(baseName)
		}
	}

	// 第一阶段：验证所有服务是否存在
	var servicesToDelete []struct {
		name    string
		service service.Service
	}
	var namesToRemove []string

	for _, serviceName := range req.Services {
		name := strings.TrimSpace(serviceName)
		if name == "" {
			return errors.New("service name is required")
		}
		namesToRemove = append(namesToRemove, name)

		svc := registry.ServiceRegistry().Get(name)
		if svc != nil {
			servicesToDelete = append(servicesToDelete, struct {
				name    string
				service service.Service
			}{name, svc})
		}
	}

	// 第二阶段：删除所有服务
	for _, std := range servicesToDelete {
		registry.ServiceRegistry().Unregister(std.name)
		if !IsDashMode {
			std.service.Close()
		}
	}
	// 确保所有请求删除的服务都从注册表中移除（即使之前未找到实例）
	for _, name := range namesToRemove {
		if registry.ServiceRegistry().IsRegistered(name) {
			registry.ServiceRegistry().Unregister(name)
		}
	}

	// 第三阶段：更新配置
	config.OnUpdate(func(c *config.Config) error {
		services := c.Services
		c.Services = nil
		for _, s := range services {
			shouldDelete := false
			for _, name := range namesToRemove {
				if s.Name == name {
					shouldDelete = true
					break
				}
			}
			if !shouldDelete {
				c.Services = append(c.Services, s)
			}
		}
		return nil
	})

	return nil
}

func pauseServices(req pauseServicesRequest) error {

	if len(req.Services) == 0 {
		return errors.New("services list cannot be empty")
	}

	// 获取服务配置（提前获取，以便在验证阶段检测预连接服务）
	cfg := config.Global()
	serviceConfigs := make(map[string]*config.ServiceConfig)
	for _, s := range cfg.Services {
		serviceConfigs[s.Name] = s
	}

	// Stop any preconn processes for paused services
	mgr := GetPreconnManager()
	for _, serviceName := range req.Services {
		baseName := ExtractPreconnBaseName(strings.TrimSpace(serviceName))
		if mgr.IsManaged(baseName) {
			mgr.StopPreconn(baseName)
		}
	}

	// 第一阶段：验证所有服务是否存在，并筛选需要暂停的服务
	var servicesToPause []struct {
		name    string
		service service.Service
	}

	for _, serviceName := range req.Services {
		name := strings.TrimSpace(serviceName)
		if name == "" {
			return errors.New("service name is required")
		}

		svc := registry.ServiceRegistry().Get(name)
		if svc == nil {
			// Check if this is a preconn-managed service (already stopped by the leading loop).
			// If so, treat it as paused without requiring a gost registry entry.
			cfgSvc := serviceConfigs[name]
			if cfgSvc != nil && isPreconnService(cfgSvc) {
				servicesToPause = append(servicesToPause, struct {
					name    string
					service service.Service
				}{name, nil})
				continue
			}
			return errors.New(fmt.Sprintf("service %s not found", name))
		}

		servicesToPause = append(servicesToPause, struct {
			name    string
			service service.Service
		}{name, svc})
	}

	// 第二阶段：事务性暂停所有服务
	var pausedServices []struct {
		name          string
		service       service.Service
		serviceConfig *config.ServiceConfig
	}

	// 逐个暂停服务，如果失败则回滚
	for _, stp := range servicesToPause {
		serviceConfig := serviceConfigs[stp.name]
		if serviceConfig == nil {
			// 找不到配置，回滚已暂停的服务
			rollbackPausedServices(pausedServices)
			return errors.New(fmt.Sprintf("service %s configuration not found", stp.name))
		}

		// 暂停服务（预连接服务已由前面的 mgr.StopPreconn 停止，跳过 gost close 步骤）
		if !IsDashMode && stp.service != nil {
			stp.service.Close()

			// 强制断开端口的所有连接
			if serviceConfig.Addr != "" {
				_ = kill.ForceClosePortConnections(serviceConfig.Addr)
			}
		}

		// 记录已暂停的服务
		pausedServices = append(pausedServices, struct {
			name          string
			service       service.Service
			serviceConfig *config.ServiceConfig
		}{stp.name, stp.service, serviceConfig})
	}

	// 第三阶段：更新配置，标记暂停状态
	err := config.OnUpdate(func(c *config.Config) error {
		for _, stp := range servicesToPause {
			for i := range c.Services {
				if c.Services[i].Name == stp.name {
					if c.Services[i].Metadata == nil {
						c.Services[i].Metadata = make(map[string]any)
					}
					c.Services[i].Metadata["paused"] = true
					break
				}
			}
		}
		return nil
	})

	if err != nil {
		// 配置更新失败，需要回滚所有暂停的服务
		rollbackPausedServices(pausedServices)
		return errors.New(fmt.Sprintf("Failed to update config, rolling back paused services: %v", err))
	}

	return nil
}

func resumeServices(req resumeServicesRequest) error {
	if len(req.Services) == 0 {
		return errors.New("services list cannot be empty")
	}

	// 第一阶段：验证所有服务是否存在，并筛选需要恢复的服务
	var servicesToResume []struct {
		name          string
		service       service.Service
		serviceConfig *config.ServiceConfig
	}
	var skippedServices []string

	cfg := config.Global()
	for _, serviceName := range req.Services {
		name := strings.TrimSpace(serviceName)
		if name == "" {
			return errors.New("service name is required")
		}

		// 检查服务是否存在
		svc := registry.ServiceRegistry().Get(name)
		if svc == nil {
			// Check if this is a preconn service whose config is stored in global config.
			var preconnCfg *config.ServiceConfig
			for _, s := range cfg.Services {
				if s.Name == name {
					if isPreconnService(s) {
						preconnCfg = s
					}
					break
				}
			}
			if preconnCfg != nil {
				// Preconn service — check if it's marked as paused before resuming.
				if !isServicePaused(preconnCfg) {
					skippedServices = append(skippedServices, name)
					continue
				}
				servicesToResume = append(servicesToResume, struct {
					name          string
					service       service.Service
					serviceConfig *config.ServiceConfig
				}{name, nil, preconnCfg})
				continue
			}
			return errors.New(fmt.Sprintf("service %s not found", name))
		}

		// 查找配置中的服务
		var serviceConfig *config.ServiceConfig
		for _, s := range cfg.Services {
			if s.Name == name {
				serviceConfig = s
				break
			}
		}

		if serviceConfig == nil {
			return errors.New(fmt.Sprintf("service %s configuration not found", name))
		}

		// 如果服务没有暂停(即正在运行)，跳过
		if !isServicePaused(serviceConfig) {
			skippedServices = append(skippedServices, name)
			continue
		}

		servicesToResume = append(servicesToResume, struct {
			name          string
			service       service.Service
			serviceConfig *config.ServiceConfig
		}{name, svc, serviceConfig})
	}

	// 第二阶段：事务性恢复所有服务
	var resumedServices []struct {
		name          string
		service       service.Service
		serviceConfig *config.ServiceConfig
	}

	mgr := GetPreconnManager()

	// 逐个恢复服务，如果失败则回滚
	for _, str := range servicesToResume {
		// For preconn services (nil svc), restart the tcp_pool process instead of gost.
		if str.service == nil && isPreconnService(str.serviceConfig) {
			baseName := ExtractPreconnBaseName(str.name)
			target := extractFirstForwarderTarget(str.serviceConfig)
			if target == "" {
				rollbackResumedServices(resumedServices)
				return fmt.Errorf("preconn: no forwarder target for service %s", str.name)
			}
			if err := mgr.StartPreconn(baseName, str.serviceConfig.Addr, target); err != nil {
				rollbackResumedServices(resumedServices)
				return err
			}
			resumedServices = append(resumedServices, str)
			continue
		}

		// 先关闭现有服务
		if !IsDashMode {
			str.service.Close()
		}
		registry.ServiceRegistry().Unregister(str.name)

		// 强制断开端口的所有连接
		if !IsDashMode {
			if str.serviceConfig.Addr != "" {
				_ = kill.ForceClosePortConnections(str.serviceConfig.Addr)
			}

			// 等待端口释放
			time.Sleep(500 * time.Millisecond)
		}

		// 重新解析并启动服务
		svc, err := parser.ParseService(str.serviceConfig)
		if err != nil {
			// 恢复失败，回滚已恢复的服务
			rollbackResumedServices(resumedServices)
			return errors.New(fmt.Sprintf("resume service %s failed: %s", str.name, err.Error()))
		}

		if err := registry.ServiceRegistry().Register(str.name, svc); err != nil {
			svc.Close()
			// 恢复失败，回滚已恢复的服务
			rollbackResumedServices(resumedServices)
			return errors.New(fmt.Sprintf("service %s already exists", str.name))
		}

		if !IsDashMode {
			go svc.Serve()
		}

		// 记录已成功恢复的服务
		resumedServices = append(resumedServices, str)
	}

	// 第三阶段：更新配置，移除暂停状态
	err := config.OnUpdate(func(c *config.Config) error {
		for _, str := range servicesToResume {
			for i := range c.Services {
				if c.Services[i].Name == str.name {
					if c.Services[i].Metadata != nil {
						delete(c.Services[i].Metadata, "paused")
						// 如果 metadata 为空，设置为 nil
						if len(c.Services[i].Metadata) == 0 {
							c.Services[i].Metadata = nil
						}
					}
					break
				}
			}
		}
		return nil
	})

	if err != nil {
		// 配置更新失败，回滚所有已恢复的服务
		rollbackResumedServices(resumedServices)
		return errors.New(fmt.Sprintf("Failed to update config, rolling back resumed services: %v", err))
	}

	return nil
}

func rollbackPausedServices(pausedServices []struct {
	name          string
	service       service.Service
	serviceConfig *config.ServiceConfig
}) {
	for _, pss := range pausedServices {
		// For preconn services (nil service), restart the tcp_pool process.
		if pss.service == nil && isPreconnService(pss.serviceConfig) {
			mgr := GetPreconnManager()
			target := extractFirstForwarderTarget(pss.serviceConfig)
			if target != "" {
				_ = mgr.StartPreconn(ExtractPreconnBaseName(pss.name), pss.serviceConfig.Addr, target)
			}
			continue
		}

		// 重新解析并启动服务
		svc, err := parser.ParseService(pss.serviceConfig)
		if err != nil {
			continue // 回滚失败，记录日志但继续处理其他服务
		}

		if err := registry.ServiceRegistry().Register(pss.name, svc); err != nil {
			svc.Close()
			continue // 回滚失败，记录日志但继续处理其他服务
		}

		go svc.Serve()

		// 移除暂停状态标记
		config.OnUpdate(func(c *config.Config) error {
			for i := range c.Services {
				if c.Services[i].Name == pss.name {
					if c.Services[i].Metadata != nil {
						delete(c.Services[i].Metadata, "paused")
						if len(c.Services[i].Metadata) == 0 {
							c.Services[i].Metadata = nil
						}
					}
					break
				}
			}
			return nil
		})
	}
}

func rollbackResumedServices(resumedServices []struct {
	name          string
	service       service.Service
	serviceConfig *config.ServiceConfig
}) {
	for _, rss := range resumedServices {
		// For preconn services (nil service), stop the tcp_pool process and re-mark as paused.
		if rss.service == nil && isPreconnService(rss.serviceConfig) {
			mgr := GetPreconnManager()
			mgr.StopPreconn(ExtractPreconnBaseName(rss.name))
			config.OnUpdate(func(c *config.Config) error {
				for i := range c.Services {
					if c.Services[i].Name == rss.name {
						if c.Services[i].Metadata == nil {
							c.Services[i].Metadata = make(map[string]any)
						}
						c.Services[i].Metadata["paused"] = true
						break
					}
				}
				return nil
			})
			continue
		}

		// 关闭已恢复的服务
		if svc := registry.ServiceRegistry().Get(rss.name); svc != nil {
			svc.Close()
		}

		// 重新标记为暂停状态
		config.OnUpdate(func(c *config.Config) error {
			for i := range c.Services {
				if c.Services[i].Name == rss.name {
					if c.Services[i].Metadata == nil {
						c.Services[i].Metadata = make(map[string]any)
					}
					c.Services[i].Metadata["paused"] = true
					break
				}
			}
			return nil
		})
	}
}

type resumeServicesRequest struct {
	Services []string `json:"services"`
}

type pauseServicesRequest struct {
	Services []string `json:"services"`
}

type deleteServicesRequest struct {
	Services []string `json:"services"`
}

type updateServicesRequest struct {
	Data []*config.ServiceConfig `json:"data"`
}

type createServicesRequest struct {
	Data []*config.ServiceConfig `json:"data"`
}

// handlePreconnServices processes service configs that have tcpPreconn enabled.
// It launches tcp_pool processes for preconn services and returns the remaining
// non-preconn services for normal gost processing.
// For each forward, there are typically 2 services (_tcp and _udp).
// tcp_pool handles both protocols in a single process, so we only launch
// one process per forward (on the _tcp service) and skip the _udp service.
// If isCreate is true, old gost services for the same name are not cleaned up.
func handlePreconnServices(services []*config.ServiceConfig, isCreate bool) (bool, []*config.ServiceConfig, error) {
	mgr := GetPreconnManager()
	var remaining []*config.ServiceConfig
	handledBases := make(map[string]bool)
	anyHandled := false

	for _, cfg := range services {
		if !isPreconnService(cfg) {
			remaining = append(remaining, cfg)
			continue
		}

		baseName := ExtractPreconnBaseName(cfg.Name)

		// Only start one tcp_pool per forward (on the _tcp service)
		if handledBases[baseName] {
			anyHandled = true
			// Remove old gost service if updating
			if !isCreate {
				cleanupOldGostService(cfg.Name)
			}
			continue
		}

		// Remove old gost services for both _tcp and _udp before launching preconn
		if !isCreate {
			cleanupOldGostService(baseName + "_tcp")
			cleanupOldGostService(baseName + "_udp")
		}

		target := extractFirstForwarderTarget(cfg)
		if target == "" {
			return false, nil, fmt.Errorf("preconn: no forwarder target for service %s", cfg.Name)
		}

		if err := mgr.StartPreconn(baseName, cfg.Addr, target); err != nil {
			return false, nil, err
		}

		handledBases[baseName] = true
		anyHandled = true
	}

	return anyHandled, remaining, nil
}

// cleanupOldGostService stops and unregisters an old gost service.
func cleanupOldGostService(name string) {
	old := registry.ServiceRegistry().Get(name)
	if old != nil {
		if !IsDashMode {
			old.Close()
		}
		registry.ServiceRegistry().Unregister(name)
	}
}
