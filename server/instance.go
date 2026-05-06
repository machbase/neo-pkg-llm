package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"neo-pkg-llm/agent"
	"neo-pkg-llm/config"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/logger"
	"neo-pkg-llm/machbase"
	"neo-pkg-llm/tools"
)

// Instance represents a per-config runtime unit.
// Each config file in configs/ becomes one Instance with its own
// machbase client, tool registry, LLM provider, and WebSocket server.
type Instance struct {
	name      string
	cfg       *config.Config
	mc        *machbase.Client
	registry  *tools.Registry
	llmClient llm.LLMProvider
	wsServ    *wsServer
	mux       *http.ServeMux
	mu        sync.RWMutex
}

// NewInstance creates and initializes an Instance from a Config.
func NewInstance(name string, cfg *config.Config) (*Instance, error) {
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.Password)
	registry := tools.NewRegistry(mc)

	llmClient, err := NewLLMSafe(cfg)
	if err != nil {
		logger.Warnf("[Instance:%s] LLM init failed (will report via socket): %v", name, err)
	}

	inst := &Instance{
		name:      name,
		cfg:       cfg,
		mc:        mc,
		registry:  registry,
		llmClient: llmClient, // may be nil
	}

	inst.wsServ = NewWSServer(mc, cfg)
	go inst.wsServ.sessionReaper()

	inst.mux = http.NewServeMux()
	inst.registerHandlers()

	logger.Infof("[Instance:%s] Ready (machbase=%s, provider=%s, model=%s)",
		name, cfg.MachbaseURL(), cfg.ResolveProvider(), cfg.ResolveModelID())

	return inst, nil
}

func (inst *Instance) registerHandlers() {
	inst.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "ok",
			"instance": inst.name,
			"provider": inst.cfg.ResolveProvider(),
			"model":    inst.cfg.ResolveModelID(),
		})
	})

	inst.mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(inst.cfg)
		default:
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
		}
	})

	inst.mux.HandleFunc("/api/restart-llm", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if err := inst.restartLLM(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "restarted",
			"provider": inst.cfg.ResolveProvider(),
			"model":    inst.cfg.ResolveModelID(),
		})
	})

	inst.mux.HandleFunc("/api/chat", inst.handleChat)
	inst.mux.HandleFunc("/api/chat/stream", inst.handleChatStream)
	inst.mux.Handle("/ws", inst.wsServ)

	// Machbase proxy — machbase-neo 로 중계
	inst.mux.HandleFunc("POST /db/tql", inst.proxyMachbase)
	inst.mux.HandleFunc("GET /web/", inst.proxyMachbaseWeb)
	inst.mux.HandleFunc("POST /web/", inst.proxyMachbaseWeb)
}

// ServeHTTP dispatches to the per-instance mux.
func (inst *Instance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	inst.mux.ServeHTTP(w, r)
}

func (inst *Instance) getClients() (llm.LLMProvider, *tools.Registry) {
	inst.mu.RLock()
	defer inst.mu.RUnlock()
	return inst.llmClient, inst.registry
}

func (inst *Instance) restartLLM() error {
	newClient, err := NewLLMSafe(inst.cfg)
	if err != nil {
		return err
	}
	inst.mu.Lock()
	inst.mc = machbase.NewClient(inst.cfg.MachbaseURL(), inst.cfg.Machbase.User, inst.cfg.Machbase.Password)
	inst.registry = tools.NewRegistry(inst.mc)
	inst.llmClient = newClient
	inst.mu.Unlock()
	logger.Infof("[Instance:%s] LLM restarted: %s / %s", inst.name, inst.cfg.ResolveProvider(), inst.cfg.ResolveModelID())
	return nil
}

func (inst *Instance) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	currentLLM, currentRegistry := inst.getClients()
	ag := agent.NewAgent(currentLLM, currentRegistry)
	result, err := ag.Run(r.Context(), req.Query)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func (inst *Instance) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	currentLLM, currentRegistry := inst.getClients()
	ag := agent.NewAgent(currentLLM, currentRegistry)
	events := ag.RunStream(r.Context(), req.Query)

	for event := range events {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}
