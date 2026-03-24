package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// apiResp mirrors the standard configs API response envelope.
type apiResp struct {
	Success bool            `json:"success"`
	Reason  string          `json:"reason"`
	Elapse  string          `json:"elapse"`
	Data    json.RawMessage `json:"data"`
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder) apiResp {
	t.Helper()
	var r apiResp
	if err := json.NewDecoder(w.Body).Decode(&r); err != nil {
		t.Fatalf("failed to decode response: %v\nbody: %s", err, w.Body.String())
	}
	return r
}

// setupConfigsHandler builds the /api/configs handlers backed by a temp directory.
func setupConfigsHandler(t *testing.T) (http.Handler, string) {
	t.Helper()

	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")

	type configsResp struct {
		Success bool   `json:"success"`
		Reason  string `json:"reason"`
		Elapse  string `json:"elapse"`
		Data    any    `json:"data"`
	}
	write := func(w http.ResponseWriter, status int, success bool, reason string, elapsed time.Duration, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(configsResp{
			Success: success,
			Reason:  reason,
			Elapse:  elapsed.String(),
			Data:    data,
		})
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/configs", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		switch r.Method {
		case http.MethodPost:
			var body Config
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				write(w, http.StatusBadRequest, false, "invalid JSON: "+err.Error(), time.Since(start), nil)
				return
			}
			switch {
			case body.Server.Port == "":
				write(w, http.StatusBadRequest, false, "server.port is required", time.Since(start), nil)
				return
			case body.Machbase.Host == "":
				write(w, http.StatusBadRequest, false, "machbase.host is required", time.Since(start), nil)
				return
			case body.Machbase.Port == "":
				write(w, http.StatusBadRequest, false, "machbase.port is required", time.Since(start), nil)
				return
			case body.Machbase.User == "":
				write(w, http.StatusBadRequest, false, "machbase.user is required", time.Since(start), nil)
				return
			case body.Machbase.Password == "":
				write(w, http.StatusBadRequest, false, "machbase.password is required", time.Since(start), nil)
				return
			}
			userName := body.Machbase.User
			if strings.ContainsAny(userName, "/\\..") {
				write(w, http.StatusBadRequest, false, "machbase.user contains invalid characters", time.Since(start), nil)
				return
			}
			if err := os.MkdirAll(configsDir, 0755); err != nil {
				write(w, http.StatusInternalServerError, false, "failed to create configs dir", time.Since(start), nil)
				return
			}
			savePath := filepath.Join(configsDir, userName+".json")
			data, _ := json.MarshalIndent(body, "", "  ")
			if err := os.WriteFile(savePath, data, 0644); err != nil {
				write(w, http.StatusInternalServerError, false, "failed to save config", time.Since(start), nil)
				return
			}
			write(w, http.StatusOK, true, "success", time.Since(start), map[string]string{"name": userName})

		case http.MethodGet:
			names := []string{}
			entries, err := os.ReadDir(configsDir)
			if err == nil {
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
						names = append(names, strings.TrimSuffix(e.Name(), ".json"))
					}
				}
			}
			write(w, http.StatusOK, true, "success", time.Since(start), map[string]any{"configs": names})

		default:
			write(w, http.StatusMethodNotAllowed, false, "GET or POST required", time.Since(start), nil)
		}
	})

	mux.HandleFunc("/api/configs/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		name := strings.TrimPrefix(r.URL.Path, "/api/configs/")
		if name == "" || strings.ContainsAny(name, "/\\.") {
			write(w, http.StatusBadRequest, false, "invalid name", time.Since(start), nil)
			return
		}

		switch r.Method {
		case http.MethodGet:
			raw, err := os.ReadFile(filepath.Join(configsDir, name+".json"))
			if err != nil {
				write(w, http.StatusNotFound, false, "not found", time.Since(start), nil)
				return
			}
			var cfg Config
			json.Unmarshal(raw, &cfg)
			write(w, http.StatusOK, true, "success", time.Since(start), cfg)

		case http.MethodPut:
			savePath := filepath.Join(configsDir, name+".json")
			if _, err := os.Stat(savePath); err != nil {
				write(w, http.StatusNotFound, false, "not found", time.Since(start), nil)
				return
			}
			var body Config
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				write(w, http.StatusBadRequest, false, "invalid JSON: "+err.Error(), time.Since(start), nil)
				return
			}
			switch {
			case body.Server.Port == "":
				write(w, http.StatusBadRequest, false, "server.port is required", time.Since(start), nil)
				return
			case body.Machbase.Host == "":
				write(w, http.StatusBadRequest, false, "machbase.host is required", time.Since(start), nil)
				return
			case body.Machbase.Port == "":
				write(w, http.StatusBadRequest, false, "machbase.port is required", time.Since(start), nil)
				return
			case body.Machbase.User == "":
				write(w, http.StatusBadRequest, false, "machbase.user is required", time.Since(start), nil)
				return
			case body.Machbase.Password == "":
				write(w, http.StatusBadRequest, false, "machbase.password is required", time.Since(start), nil)
				return
			}
			data, _ := json.MarshalIndent(body, "", "  ")
			if err := os.WriteFile(savePath, data, 0644); err != nil {
				write(w, http.StatusInternalServerError, false, "failed to save config", time.Since(start), nil)
				return
			}
			write(w, http.StatusOK, true, "success", time.Since(start), map[string]string{"name": name})

		default:
			write(w, http.StatusMethodNotAllowed, false, "GET or PUT required", time.Since(start), nil)
		}
	})

	return mux, configsDir
}

func sampleConfigBody(user string) string {
	return `{
		"server": { "port": "8884" },
		"machbase": { "host": "192.168.1.238", "port": "5654", "user": "` + user + `", "password": "manager" },
		"claude": { "api_key": "sk-ant-test", "models": [{ "name": "haiku", "model_id": "claude-haiku-4-5-20251001" }] },
		"chatgpt": { "api_key": "sk-proj-test", "models": [{ "name": "gpt-4o-mini" }] },
		"gemini": { "api_key": "AIza-test", "models": [{ "name": "gemini-flash", "model_id": "gemini-flash-lite" }] },
		"ollama": { "base_url": "", "models": [{ "name": "qwen3:8b" }] }
	}`
}

// --- POST /api/configs ---

func TestPostConfigs_Success(t *testing.T) {
	handler, configsDir := setupConfigsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody("alice")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	r := decodeResp(t, w)
	if !r.Success {
		t.Errorf("expected success=true, reason=%q", r.Reason)
	}
	if r.Elapse == "" {
		t.Error("elapse should not be empty")
	}
	if r.Reason != "success" {
		t.Errorf("expected reason='success', got %q", r.Reason)
	}
	var data map[string]string
	json.Unmarshal(r.Data, &data)
	if data["name"] != "alice" {
		t.Errorf("expected data.name=alice, got %q", data["name"])
	}
	// 파일이 machbase.user 이름으로 저장됐는지 확인
	if _, err := os.Stat(filepath.Join(configsDir, "alice.json")); err != nil {
		t.Errorf("configs/alice.json not created: %v", err)
	}
}

func TestPostConfigs_RequiredFields(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	cases := []struct {
		desc string
		body string
	}{
		{
			"missing server.port",
			`{"server":{},"machbase":{"host":"h","port":"5654","user":"u","password":"p"}}`,
		},
		{
			"missing machbase.host",
			`{"server":{"port":"8884"},"machbase":{"host":"","port":"5654","user":"u","password":"p"}}`,
		},
		{
			"missing machbase.port",
			`{"server":{"port":"8884"},"machbase":{"host":"h","port":"","user":"u","password":"p"}}`,
		},
		{
			"missing machbase.user",
			`{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"","password":"p"}}`,
		},
		{
			"missing machbase.password",
			`{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"u","password":""}}`,
		},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("[%s] expected 400, got %d", tc.desc, w.Code)
			continue
		}
		r := decodeResp(t, w)
		if r.Success {
			t.Errorf("[%s] expected success=false", tc.desc)
		}
		if r.Reason == "" {
			t.Errorf("[%s] reason should not be empty", tc.desc)
		}
	}
}

func TestPostConfigs_InvalidUser_PathTraversal(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	for _, user := range []string{"../evil", "foo/bar"} {
		body := `{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"` + user + `","password":"p"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("user=%q: expected 400, got %d", user, w.Code)
		}
	}
}

func TestPostConfigs_InvalidJSON(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	r := decodeResp(t, w)
	if r.Success {
		t.Error("expected success=false")
	}
}

// --- GET /api/configs ---

func TestGetConfigs_Empty(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/configs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	r := decodeResp(t, w)
	if !r.Success {
		t.Error("expected success=true")
	}
	var data map[string][]string
	json.Unmarshal(r.Data, &data)
	if data["configs"] == nil {
		t.Error("data.configs should be an empty slice, not nil")
	}
	if len(data["configs"]) != 0 {
		t.Errorf("expected empty list, got %v", data["configs"])
	}
}

func TestGetConfigs_AfterSave(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	for _, user := range []string{"alice", "bob"} {
		req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody(user)))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/configs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	r := decodeResp(t, w)
	var data map[string][]string
	json.Unmarshal(r.Data, &data)
	if len(data["configs"]) != 2 {
		t.Errorf("expected 2 configs, got %d: %v", len(data["configs"]), data["configs"])
	}
}

// --- GET /api/configs/{name} ---

func TestGetConfigByName_Success(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody("alice")))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	req = httptest.NewRequest(http.MethodGet, "/api/configs/alice", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	r := decodeResp(t, w)
	if !r.Success {
		t.Errorf("expected success=true, reason=%q", r.Reason)
	}
	var cfg Config
	if err := json.Unmarshal(r.Data, &cfg); err != nil {
		t.Fatalf("data is not valid Config JSON: %v", err)
	}
	if cfg.Machbase.User != "alice" {
		t.Errorf("expected machbase.user=alice, got %q", cfg.Machbase.User)
	}
	if cfg.Machbase.Host != "192.168.1.238" {
		t.Errorf("expected host 192.168.1.238, got %q", cfg.Machbase.Host)
	}
}

func TestGetConfigByName_NotFound(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/configs/nobody", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	r := decodeResp(t, w)
	if r.Success {
		t.Error("expected success=false")
	}
	if r.Reason != "not found" {
		t.Errorf("expected reason='not found', got %q", r.Reason)
	}
}

func TestGetConfigByName_OverwriteOnRepost(t *testing.T) {
	handler, configsDir := setupConfigsHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody("alice")))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// 같은 user(alice)로 host만 변경해서 재저장
	updated := `{"server":{"port":"8884"},"machbase":{"host":"10.0.0.1","port":"5654","user":"alice","password":"manager"}}`
	req = httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(updated))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	raw, _ := os.ReadFile(filepath.Join(configsDir, "alice.json"))
	var cfg Config
	json.Unmarshal(raw, &cfg)
	if cfg.Machbase.Host != "10.0.0.1" {
		t.Errorf("expected overwritten host 10.0.0.1, got %q", cfg.Machbase.Host)
	}
}

// --- PUT /api/configs/{name} ---

func TestPutConfigByName_Success(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	// 먼저 생성
	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody("alice")))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// 수정
	updated := `{"server":{"port":"9999"},"machbase":{"host":"10.0.0.1","port":"5654","user":"alice","password":"newpass"}}`
	req = httptest.NewRequest(http.MethodPut, "/api/configs/alice", strings.NewReader(updated))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	r := decodeResp(t, w)
	if !r.Success {
		t.Errorf("expected success=true, reason=%q", r.Reason)
	}
	var data map[string]string
	json.Unmarshal(r.Data, &data)
	if data["name"] != "alice" {
		t.Errorf("expected data.name=alice, got %q", data["name"])
	}

	// 실제 변경됐는지 GET으로 확인
	req = httptest.NewRequest(http.MethodGet, "/api/configs/alice", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	r = decodeResp(t, w)
	var cfg Config
	json.Unmarshal(r.Data, &cfg)
	if cfg.Machbase.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %q", cfg.Machbase.Host)
	}
	if cfg.Server.Port != "9999" {
		t.Errorf("expected server.port 9999, got %q", cfg.Server.Port)
	}
}

func TestPutConfigByName_NotFound(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	body := `{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"nobody","password":"p"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/configs/nobody", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	r := decodeResp(t, w)
	if r.Success {
		t.Error("expected success=false")
	}
}

func TestPutConfigByName_RequiredFields(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	// 먼저 생성
	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody("alice")))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	cases := []struct {
		desc string
		body string
	}{
		{"missing server.port", `{"server":{},"machbase":{"host":"h","port":"5654","user":"alice","password":"p"}}`},
		{"missing machbase.host", `{"server":{"port":"8884"},"machbase":{"host":"","port":"5654","user":"alice","password":"p"}}`},
		{"missing machbase.user", `{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"","password":"p"}}`},
	}

	for _, tc := range cases {
		req = httptest.NewRequest(http.MethodPut, "/api/configs/alice", strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("[%s] expected 400, got %d", tc.desc, w.Code)
		}
	}
}
