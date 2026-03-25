package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	mux := http.NewServeMux()
	registerConfigsHandlers(mux, configsDir)
	return mux, configsDir
}

func sampleConfigBody(user string) string {
	return `{
		"server": { "port": "8884" },
		"machbase": { "host": "192.168.1.238", "port": "5654", "user": "` + user + `", "work_dir": "/tmp/neo" },
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
			`{"server":{},"machbase":{"host":"h","port":"5654","user":"u","work_dir":"/tmp/neo"}}`,
		},
		{
			"missing machbase.host",
			`{"server":{"port":"8884"},"machbase":{"host":"","port":"5654","user":"u","work_dir":"/tmp/neo"}}`,
		},
		{
			"missing machbase.port",
			`{"server":{"port":"8884"},"machbase":{"host":"h","port":"","user":"u","work_dir":"/tmp/neo"}}`,
		},
		{
			"missing machbase.user",
			`{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"","work_dir":"/tmp/neo"}}`,
		},
		{
			"missing machbase.work_dir",
			`{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"u","work_dir":""}}`,
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
		body := `{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"` + user + `","work_dir":"/tmp/neo"}}`
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
	updated := `{"server":{"port":"8884"},"machbase":{"host":"10.0.0.1","port":"5654","user":"alice","work_dir":"/tmp/neo"}}`
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
	updated := `{"server":{"port":"9999"},"machbase":{"host":"10.0.0.1","port":"5654","user":"alice","work_dir":"/tmp/neo"}}`
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

	body := `{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"nobody","work_dir":"/tmp/neo"}}`
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
		{"missing server.port", `{"server":{},"machbase":{"host":"h","port":"5654","user":"alice","work_dir":"/tmp/neo"}}`},
		{"missing machbase.host", `{"server":{"port":"8884"},"machbase":{"host":"","port":"5654","user":"alice","work_dir":"/tmp/neo"}}`},
		{"missing machbase.user", `{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"","work_dir":"/tmp/neo"}}`},
		{"missing machbase.work_dir", `{"server":{"port":"8884"},"machbase":{"host":"h","port":"5654","user":"alice","work_dir":""}}`},
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

func TestPutConfigByName_RenameOnUserChange(t *testing.T) {
	handler, configsDir := setupConfigsHandler(t)

	// alice로 생성
	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody("alice")))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// machbase.user를 bob으로 변경
	updated := `{"server":{"port":"8884"},"machbase":{"host":"192.168.1.238","port":"5654","user":"bob","work_dir":"/tmp/neo"}}`
	req = httptest.NewRequest(http.MethodPut, "/api/configs/alice", strings.NewReader(updated))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	r := decodeResp(t, w)
	var data map[string]string
	json.Unmarshal(r.Data, &data)
	if data["name"] != "bob" {
		t.Errorf("expected data.name=bob, got %q", data["name"])
	}

	// bob.json 생성됐는지 확인
	if _, err := os.Stat(filepath.Join(configsDir, "bob.json")); err != nil {
		t.Error("bob.json should exist after rename")
	}
	// alice.json 삭제됐는지 확인
	if _, err := os.Stat(filepath.Join(configsDir, "alice.json")); err == nil {
		t.Error("alice.json should be deleted after rename")
	}
}

// --- DELETE /api/configs/{name} ---

func TestDeleteConfigByName_Success(t *testing.T) {
	handler, configsDir := setupConfigsHandler(t)

	// 먼저 생성
	req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody("alice")))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// 삭제
	req = httptest.NewRequest(http.MethodDelete, "/api/configs/alice", nil)
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

	// 파일이 실제로 삭제됐는지 확인
	if _, err := os.Stat(filepath.Join(configsDir, "alice.json")); err == nil {
		t.Error("alice.json should be deleted")
	}
}

func TestDeleteConfigByName_NotExist(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/configs/nobody", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	r := decodeResp(t, w)
	if r.Success {
		t.Error("expected success=false")
	}
	if r.Reason != "config 'nobody' does not exist" {
		t.Errorf("unexpected reason: %q", r.Reason)
	}
}

func TestDeleteConfigByName_NotInListAfterDelete(t *testing.T) {
	handler, _ := setupConfigsHandler(t)

	// alice, bob 생성
	for _, user := range []string{"alice", "bob"} {
		req := httptest.NewRequest(http.MethodPost, "/api/configs", strings.NewReader(sampleConfigBody(user)))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	// alice 삭제
	req := httptest.NewRequest(http.MethodDelete, "/api/configs/alice", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// 목록 확인
	req = httptest.NewRequest(http.MethodGet, "/api/configs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	r := decodeResp(t, w)
	var data map[string][]string
	json.Unmarshal(r.Data, &data)
	if len(data["configs"]) != 1 {
		t.Errorf("expected 1 config after delete, got %d: %v", len(data["configs"]), data["configs"])
	}
	if data["configs"][0] != "bob" {
		t.Errorf("expected bob to remain, got %q", data["configs"][0])
	}
}
