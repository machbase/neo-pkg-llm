package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"neo-pkg-llm/logger"
)

// Manager manages all Instances and routes HTTP requests by URL prefix.
// URL pattern: /{instance_name}/... → dispatched to the matching Instance.
type Manager struct {
	instances  map[string]*Instance
	configsDir string
	mu         sync.RWMutex
}

func NewManager(configsDir string) *Manager {
	return &Manager{
		instances:  make(map[string]*Instance),
		configsDir: configsDir,
	}
}

// LoadAll loads all config files from configsDir and starts an Instance for each.
func (m *Manager) LoadAll() {
	entries, err := os.ReadDir(m.configsDir)
	if err != nil {
		logger.Infof("[Manager] No configs found in %s: %v", m.configsDir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		cfg, err := m.loadConfigFile(name)
		if err != nil {
			logger.Infof("[Manager] Failed to load config %s: %v", name, err)
			continue
		}
		if err := m.startInstance(name, cfg); err != nil {
			logger.Infof("[Manager] Failed to start instance %s: %v", name, err)
		}
	}
}

func (m *Manager) loadConfigFile(name string) (*Config, error) {
	path := filepath.Join(m.configsDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.configPath = path
	cfg.applyEnvOverrides()
	return &cfg, nil
}

func (m *Manager) startInstance(name string, cfg *Config) error {
	inst, err := NewInstance(name, cfg)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.instances[name] = inst
	m.mu.Unlock()
	logger.Infof("[Manager] Instance started: %s", name)
	return nil
}

func (m *Manager) stopInstance(name string) {
	m.mu.Lock()
	inst, ok := m.instances[name]
	if ok {
		delete(m.instances, name)
	}
	m.mu.Unlock()

	if ok {
		inst.wsServ.CloseAll()
		logger.Infof("[Manager] Instance stopped: %s", name)
	}
}

func (m *Manager) getInstance(name string) *Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instances[name]
}

// RouteInstance extracts the instance name from the first URL path segment
// and forwards the request to the matching Instance with the prefix stripped.
//
//   /{name}/ws         → Instance.ServeHTTP with path=/ws
//   /{name}/api/chat   → Instance.ServeHTTP with path=/api/chat
func (m *Manager) RouteInstance(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	name := parts[0]

	if name == "" {
		http.Error(w, "instance name required in URL path", http.StatusBadRequest)
		return
	}

	inst := m.getInstance(name)
	if inst == nil {
		http.Error(w, "instance not found: "+name, http.StatusNotFound)
		return
	}

	// Strip /{name} prefix before forwarding
	remaining := "/"
	if len(parts) > 1 {
		remaining = "/" + parts[1]
	}
	r.URL.Path = remaining
	inst.ServeHTTP(w, r)
}

// RegisterMasterHandlers registers manager-level API endpoints on the mux.
func (m *Manager) RegisterMasterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/instances", m.handleInstances)
	m.registerConfigsHandlers(mux)
}

// --- /api/instances ---

func (m *Manager) handleInstances(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.Method != http.MethodGet {
		writeConfigsResp(w, http.StatusMethodNotAllowed, false, "GET required", time.Since(start), nil)
		return
	}

	type instanceInfo struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Machbase string `json:"machbase_url"`
	}

	m.mu.RLock()
	infos := make([]instanceInfo, 0, len(m.instances))
	for name, inst := range m.instances {
		infos = append(infos, instanceInfo{
			Name:     name,
			Provider: inst.cfg.ResolveProvider(),
			Model:    inst.cfg.ResolveModelID(),
			Machbase: inst.cfg.MachbaseURL(),
		})
	}
	m.mu.RUnlock()

	writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]any{"instances": infos})
}

// --- /api/configs (CRUD + instance lifecycle) ---

func (m *Manager) registerConfigsHandlers(mux *http.ServeMux) {
	dir := m.configsDir

	// POST /api/configs — save config + start instance
	// GET  /api/configs — list configs (with running status)
	mux.HandleFunc("/api/configs", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		switch r.Method {
		case http.MethodPost:
			var body Config
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeConfigsResp(w, http.StatusBadRequest, false, "invalid JSON: "+err.Error(), time.Since(start), nil)
				return
			}
			if reason := validateConfig(&body); reason != "" {
				writeConfigsResp(w, http.StatusBadRequest, false, reason, time.Since(start), nil)
				return
			}
			userName := body.Machbase.User
			if strings.ContainsAny(userName, "/\\..") {
				writeConfigsResp(w, http.StatusBadRequest, false, "machbase.user contains invalid characters", time.Since(start), nil)
				return
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				writeConfigsResp(w, http.StatusInternalServerError, false, "failed to create configs dir", time.Since(start), nil)
				return
			}
			savePath := filepath.Join(dir, userName+".json")
			data, _ := json.MarshalIndent(body, "", "  ")
			if err := os.WriteFile(savePath, data, 0644); err != nil {
				writeConfigsResp(w, http.StatusInternalServerError, false, "failed to save config", time.Since(start), nil)
				return
			}

			// Stop existing instance, then start new one
			m.stopInstance(userName)
			body.configPath = savePath
			body.applyEnvOverrides()
			if err := m.startInstance(userName, &body); err != nil {
				logger.Infof("[Manager] Instance start failed for %s: %v", userName, err)
				writeConfigsResp(w, http.StatusOK, true, "config saved but instance failed: "+err.Error(), time.Since(start), map[string]string{"name": userName})
				return
			}
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]string{"name": userName})

		case http.MethodGet:
			type configEntry struct {
				Name    string `json:"name"`
				Running bool   `json:"running"`
			}
			entries := make([]configEntry, 0)
			dirEntries, err := os.ReadDir(dir)
			if err == nil {
				for _, e := range dirEntries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
						name := strings.TrimSuffix(e.Name(), ".json")
						entries = append(entries, configEntry{
							Name:    name,
							Running: m.getInstance(name) != nil,
						})
					}
				}
			}
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]any{"configs": entries})

		default:
			writeConfigsResp(w, http.StatusMethodNotAllowed, false, "GET or POST required", time.Since(start), nil)
		}
	})

	// GET/PUT/DELETE /api/configs/{name}
	mux.HandleFunc("/api/configs/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		name := strings.TrimPrefix(r.URL.Path, "/api/configs/")
		if name == "" || strings.ContainsAny(name, "/\\.") {
			writeConfigsResp(w, http.StatusBadRequest, false, "invalid name", time.Since(start), nil)
			return
		}

		switch r.Method {
		case http.MethodGet:
			raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
			if err != nil {
				writeConfigsResp(w, http.StatusNotFound, false, "not found", time.Since(start), nil)
				return
			}
			var cfg Config
			json.Unmarshal(raw, &cfg)
			masked := cfg.MaskedCopy()
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]any{
				"config":  masked,
				"running": m.getInstance(name) != nil,
			})

		case http.MethodPut:
			savePath := filepath.Join(dir, name+".json")
			existingRaw, err := os.ReadFile(savePath)
			if err != nil {
				writeConfigsResp(w, http.StatusNotFound, false, "not found", time.Since(start), nil)
				return
			}
			var existing Config
			json.Unmarshal(existingRaw, &existing)

			var body Config
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeConfigsResp(w, http.StatusBadRequest, false, "invalid JSON: "+err.Error(), time.Since(start), nil)
				return
			}
			// Restore masked secrets from existing config
			body.RestoreSecrets(&existing)

			if reason := validateConfig(&body); reason != "" {
				writeConfigsResp(w, http.StatusBadRequest, false, reason, time.Since(start), nil)
				return
			}
			newName := body.Machbase.User
			newPath := filepath.Join(dir, newName+".json")
			data, _ := json.MarshalIndent(body, "", "  ")
			if err := os.WriteFile(newPath, data, 0644); err != nil {
				writeConfigsResp(w, http.StatusInternalServerError, false, "failed to save config", time.Since(start), nil)
				return
			}

			// Stop old instance
			m.stopInstance(name)
			if newName != name {
				os.Remove(savePath)
			}

			// Start with updated config
			body.configPath = newPath
			body.applyEnvOverrides()
			if err := m.startInstance(newName, &body); err != nil {
				logger.Infof("[Manager] Instance restart failed for %s: %v", newName, err)
				writeConfigsResp(w, http.StatusOK, true, "config saved but instance failed: "+err.Error(), time.Since(start), map[string]string{"name": newName})
				return
			}
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]string{"name": newName})

		case http.MethodDelete:
			savePath := filepath.Join(dir, name+".json")
			if _, err := os.Stat(savePath); err != nil {
				writeConfigsResp(w, http.StatusBadRequest, false, "config '"+name+"' does not exist", time.Since(start), nil)
				return
			}
			if err := os.Remove(savePath); err != nil {
				writeConfigsResp(w, http.StatusInternalServerError, false, "failed to delete config", time.Since(start), nil)
				return
			}
			m.stopInstance(name)
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]string{"name": name})

		default:
			writeConfigsResp(w, http.StatusMethodNotAllowed, false, "GET, PUT or DELETE required", time.Since(start), nil)
		}
	})
}

// validateConfig checks required fields.
func validateConfig(c *Config) string {
	switch {
	case c.Machbase.Host == "":
		return "machbase.host is required"
	case c.Machbase.Port == "":
		return "machbase.port is required"
	case c.Machbase.User == "":
		return "machbase.user is required"
	case c.Machbase.Password == "":
		return "machbase.password is required"
	}
	return ""
}
