package socket

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func (w *WebSocketReporter) handleSetEngine(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var req struct {
		Engine string `json:"engine"`
	}
	if err := json.Unmarshal(jsonData, &req); err != nil {
		return err
	}

	engine := req.Engine
	if engine == "" {
		engine = "gost"
	}

	// Read existing config.json
	path := "config.json"
	var cfg map[string]interface{}
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, &cfg)
	} else {
		cfg = make(map[string]interface{})
	}

	currentEngine, _ := cfg["engine"].(string)
	if currentEngine == "" {
		currentEngine = "gost"
	}
	if currentEngine == engine {
		return nil
	}

	cfg["engine"] = engine

	newData, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, newData, 0644)

	// Restart the agent to apply the new engine immediately.
	// Use exit code 1 so systemd Restart=on-failure will restart us.
	go func() {
		fmt.Println("🔄 Engine selection updated to", engine, "- Restarting agent...")
		time.Sleep(1 * time.Second)
		os.Exit(1)
	}()
	return nil
}
