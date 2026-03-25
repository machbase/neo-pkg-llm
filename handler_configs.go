package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type configsResp struct {
	Success bool   `json:"success"`
	Reason  string `json:"reason"`
	Elapse  string `json:"elapse"`
	Data    any    `json:"data"`
}

func writeConfigsResp(w http.ResponseWriter, status int, success bool, reason string, elapsed time.Duration, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(configsResp{
		Success: success,
		Reason:  reason,
		Elapse:  elapsed.String(),
		Data:    data,
	})
}

func registerConfigsHandlers(mux *http.ServeMux, dir string) {
	// POST /api/configs — save user config to configs/{machbase.user}.json
	// GET  /api/configs — list saved configs
	mux.HandleFunc("/api/configs", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		switch r.Method {
		case http.MethodPost:
			var body Config
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeConfigsResp(w, http.StatusBadRequest, false, "invalid JSON: "+err.Error(), time.Since(start), nil)
				return
			}
			switch {
			case body.Server.Port == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "server.port is required", time.Since(start), nil)
				return
			case body.Machbase.Host == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "machbase.host is required", time.Since(start), nil)
				return
			case body.Machbase.Port == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "machbase.port is required", time.Since(start), nil)
				return
			case body.Machbase.User == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "machbase.user is required", time.Since(start), nil)
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
			log.Printf("Config saved: %s", savePath)
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]string{"name": userName})

		case http.MethodGet:
			names := []string{}
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
						names = append(names, strings.TrimSuffix(e.Name(), ".json"))
					}
				}
			}
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]any{"configs": names})

		default:
			writeConfigsResp(w, http.StatusMethodNotAllowed, false, "GET or POST required", time.Since(start), nil)
		}
	})

	// GET    /api/configs/{name} — retrieve a specific user config
	// PUT    /api/configs/{name} — update an existing user config
	// DELETE /api/configs/{name} — delete a specific user config
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
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), cfg)

		case http.MethodPut:
			savePath := filepath.Join(dir, name+".json")
			if _, err := os.Stat(savePath); err != nil {
				writeConfigsResp(w, http.StatusNotFound, false, "not found", time.Since(start), nil)
				return
			}
			var body Config
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeConfigsResp(w, http.StatusBadRequest, false, "invalid JSON: "+err.Error(), time.Since(start), nil)
				return
			}
			switch {
			case body.Server.Port == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "server.port is required", time.Since(start), nil)
				return
			case body.Machbase.Host == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "machbase.host is required", time.Since(start), nil)
				return
			case body.Machbase.Port == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "machbase.port is required", time.Since(start), nil)
				return
			case body.Machbase.User == "":
				writeConfigsResp(w, http.StatusBadRequest, false, "machbase.user is required", time.Since(start), nil)
				return
			}
			newName := body.Machbase.User
			newPath := filepath.Join(dir, newName+".json")
			data, _ := json.MarshalIndent(body, "", "  ")
			if err := os.WriteFile(newPath, data, 0644); err != nil {
				writeConfigsResp(w, http.StatusInternalServerError, false, "failed to save config", time.Since(start), nil)
				return
			}
			if newName != name {
				os.Remove(savePath)
				log.Printf("Config renamed: %s → %s", savePath, newPath)
			} else {
				log.Printf("Config updated: %s", newPath)
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
			log.Printf("Config deleted: %s", savePath)
			writeConfigsResp(w, http.StatusOK, true, "success", time.Since(start), map[string]string{"name": name})

		default:
			writeConfigsResp(w, http.StatusMethodNotAllowed, false, "GET, PUT or DELETE required", time.Since(start), nil)
		}
	})
}
