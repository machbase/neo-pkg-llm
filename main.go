package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"neo-pkg-llm/agent"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/machbase"
	"neo-pkg-llm/mcp"
	"neo-pkg-llm/tools"
)

func main() {
	mode := flag.String("mode", "server", "Run mode: 'server' (HTTP API), 'cli' (interactive), 'mcp' (MCP stdio server), or 'ws' (WebSocket client)")
	port := flag.String("port", "", "HTTP server port (overrides config, server mode)")
	neoWSURL := flag.String("neo-ws-url", "", "Neo WebSocket URL to connect to (ws mode)")
	configPath := flag.String("config", "config.json", "Path to config file")
	providerFlag := flag.String("provider", "", "Override LLM provider (claude, chatgpt, gemini)")
	modelFlag := flag.String("model", "", "Override model name or model_id")
	flag.Parse()

	cfg := LoadConfig(*configPath)

	// CLI flags override config.json
	if *providerFlag != "" {
		cfg.Provider = *providerFlag
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	switch *mode {
	case "cli":
		runCLI(cfg)
	case "server":
		serverPort := cfg.Server.Port
		if *port != "" {
			serverPort = *port
		}
		if serverPort == "" {
			log.Fatal("server port is required: set server.port in config or use --port flag")
		}
		runServer(cfg, serverPort)
	case "mcp":
		runMCP(cfg)
	case "ws":
		runWS(cfg, *neoWSURL)
	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s (use 'server', 'cli', 'mcp', or 'ws')\n", *mode)
		os.Exit(1)
	}
}

// newLLMSafe creates the appropriate LLM client based on config, returning an error instead of fatal.
func newLLMSafe(cfg *Config) (llm.LLMProvider, error) {
	modelID := cfg.ResolveModelID()

	switch strings.ToLower(cfg.ResolveProvider()) {
	case "claude":
		if cfg.Claude.APIKey == "" {
			return nil, fmt.Errorf("Claude API key is required")
		}
		log.Printf("LLM: Claude (%s)", modelID)
		return llm.NewClaudeClient(cfg.Claude.APIKey, modelID), nil

	case "chatgpt":
		if cfg.ChatGPT.APIKey == "" {
			return nil, fmt.Errorf("ChatGPT API key is required")
		}
		log.Printf("LLM: ChatGPT (%s)", modelID)
		return llm.NewChatGPTClient(cfg.ChatGPT.APIKey, modelID), nil

	case "gemini":
		if cfg.Gemini.APIKey == "" {
			return nil, fmt.Errorf("Gemini API key is required")
		}
		log.Printf("LLM: Gemini (%s)", modelID)
		return llm.NewGeminiClient(cfg.Gemini.APIKey, modelID), nil

	case "ollama":
		ollamaURL := cfg.OllamaURL()
		log.Printf("LLM: Ollama (%s) at %s", modelID, ollamaURL)
		return llm.NewOllamaClient(ollamaURL, modelID), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s (use 'claude', 'chatgpt', 'gemini', or 'ollama')", cfg.ResolveProvider())
	}
}

// newLLM creates the LLM client, fataling on error (used at startup).
func newLLM(cfg *Config) llm.LLMProvider {
	client, err := newLLMSafe(cfg)
	if err != nil {
		log.Fatal(err)
	}
	return client
}

// --- MCP Server Mode (stdio JSON-RPC) ---

func runMCP(cfg *Config) {
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.WorkDir)
	registry := tools.NewRegistry(mc)

	server := mcp.NewServer(registry)
	if err := server.Run(); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}

// --- WebSocket Client Mode ---

func runWS(cfg *Config, neoURL string) {
	if neoURL == "" {
		log.Fatal("--neo-ws-url is required for ws mode")
	}
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.WorkDir)
	registry := tools.NewRegistry(mc)
	llmClient := newLLM(cfg)

	log.Printf("WebSocket client mode: connecting to %s", neoURL)
	log.Printf("Machbase: %s | Provider: %s | Model: %s", cfg.MachbaseURL(), cfg.ResolveProvider(), cfg.ResolveModelID())

	client := newWSClient(neoURL, llmClient, registry)
	client.Run()
}

// --- CLI Mode ---

func runCLI(cfg *Config) {
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.WorkDir)
	registry := tools.NewRegistry(mc)
	llmClient := newLLM(cfg)

	fmt.Println("=== Agentic Loop Go (CLI) ===")
	fmt.Printf("Machbase: %s | Provider: %s | Model: %s\n", cfg.MachbaseURL(), cfg.ResolveProvider(), cfg.ResolveModelID())
	fmt.Printf("Tools: %d loaded\n", len(registry.ToolNames()))
	fmt.Println("Type your query (empty line to quit):")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			break
		}

		ag := agent.NewAgent(llmClient, registry)
		result, err := ag.Run(context.Background(), query)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println(result)
		fmt.Println(strings.Repeat("=", 60))
	}
}

// --- HTTP Server Mode ---

const configsDir = "configs"

func runServer(cfg *Config, port string) {
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		log.Fatalf("failed to create configs directory: %v", err)
	}

	var mu sync.RWMutex
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.WorkDir)
	registry := tools.NewRegistry(mc)
	llmClient := newLLM(cfg)

	// getClients returns current LLM client and registry with read lock.
	getClients := func() (llm.LLMProvider, *tools.Registry) {
		mu.RLock()
		defer mu.RUnlock()
		return llmClient, registry
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// --- Settings API ---
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfg)
		case http.MethodPost:
			var newCfg Config
			if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
				http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
			// Update in-memory config
			cfg.Machbase = newCfg.Machbase
			cfg.Claude = newCfg.Claude
			cfg.ChatGPT = newCfg.ChatGPT
			cfg.Gemini = newCfg.Gemini
			cfg.Ollama = newCfg.Ollama
			// Save to file
			if err := cfg.Save(); err != nil {
				http.Error(w, "Save failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
		default:
			http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
		}
	})

	// --- Restart LLM API ---
	mux.HandleFunc("/api/restart-llm", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		newClient, err := newLLMSafe(cfg)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		mu.Lock()
		mc = machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.WorkDir)
		registry = tools.NewRegistry(mc)
		llmClient = newClient
		mu.Unlock()

		log.Printf("LLM restarted: %s / %s", cfg.ResolveProvider(), cfg.ResolveModelID())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "restarted",
			"provider": cfg.ResolveProvider(),
			"model":    cfg.ResolveModelID(),
		})
	})

	// --- Settings page ---
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/settings.html")
	})

	// --- Chat API ---
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
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

		currentLLM, currentRegistry := getClients()
		ag := agent.NewAgent(currentLLM, currentRegistry)
		result, err := ag.Run(r.Context(), req.Query)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
	})

	mux.HandleFunc("/api/chat/stream", func(w http.ResponseWriter, r *http.Request) {
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

		currentLLM, currentRegistry := getClients()
		ag := agent.NewAgent(currentLLM, currentRegistry)
		events := ag.RunStream(r.Context(), req.Query)

		for event := range events {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	})

	// --- Configs API ---
	registerConfigsHandlers(mux, configsDir)

	// --- WebSocket Server (Chat UI direct connection) ---
	wsServ := newWSServer(mc, cfg)
	go wsServ.sessionReaper()
	mux.Handle("/ws", wsServ)

	handler := corsMiddleware(mux)

	log.Printf("Agentic Loop Go server starting on :%s", port)
	log.Printf("Machbase: %s | Provider: %s | Model: %s", cfg.MachbaseURL(), cfg.ResolveProvider(), cfg.ResolveModelID())
	log.Printf("Tools: %d loaded", len(registry.ToolNames()))
	log.Printf("Endpoints:")
	log.Printf("  GET  /settings            — Settings page")
	log.Printf("  GET  /api/settings        — Get config")
	log.Printf("  POST /api/settings        — Save config")
	log.Printf("  POST /api/restart-llm     — Restart LLM client")
	log.Printf("  POST /api/chat            — Non-streaming")
	log.Printf("  POST /api/chat/stream     — SSE streaming")
	log.Printf("  GET  /ws                  — WebSocket (Chat UI)")
	log.Printf("  GET  /health              — Health check")
	log.Printf("  POST /api/configs         — Save user config (configs/{name}.json)")
	log.Printf("  GET  /api/configs         — List saved configs")
	log.Printf("  GET    /api/configs/{name}  — Get specific user config")
	log.Printf("  PUT    /api/configs/{name}  — Update specific user config")
	log.Printf("  DELETE /api/configs/{name}  — Delete specific user config")

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
