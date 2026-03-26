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
	configPath := flag.String("config", "configs/config.json", "Path to config file")
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

	mgr := NewManager(configsDir)
	mgr.LoadAll()

	// Master-level routes
	masterMux := http.NewServeMux()

	masterMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	masterMux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/settings.html")
	})

	// Manager APIs: /api/instances, /api/configs
	mgr.RegisterMasterHandlers(masterMux)

	// Main handler: master routes vs per-instance routing
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Master-level routes (no instance prefix)
		switch {
		case path == "/health" || path == "/settings":
			masterMux.ServeHTTP(w, r)
			return
		case strings.HasPrefix(path, "/api/"):
			masterMux.ServeHTTP(w, r)
			return
		}

		// Per-instance routing: /{name}/ws, /{name}/api/chat, ...
		mgr.RouteInstance(w, r)
	})

	log.Printf("Agentic Loop Go server starting on :%s", port)
	log.Printf("Configs dir: %s", configsDir)
	log.Printf("Endpoints (master):")
	log.Printf("  GET  /health                — Health check")
	log.Printf("  GET  /settings              — Settings page")
	log.Printf("  GET  /api/instances         — List running instances")
	log.Printf("  POST /api/configs           — Save config + start instance")
	log.Printf("  GET  /api/configs           — List configs")
	log.Printf("  GET  /api/configs/{name}    — Get config")
	log.Printf("  PUT  /api/configs/{name}    — Update config + restart instance")
	log.Printf("  DELETE /api/configs/{name}  — Delete config + stop instance")
	log.Printf("Endpoints (per-instance: /{name}/...):")
	log.Printf("  POST /{name}/api/chat         — Non-streaming chat")
	log.Printf("  POST /{name}/api/chat/stream  — SSE streaming chat")
	log.Printf("  GET  /{name}/api/settings     — Get instance config")
	log.Printf("  POST /{name}/api/restart-llm  — Restart instance LLM")
	log.Printf("  GET  /{name}/ws               — WebSocket (Chat UI)")
	log.Printf("  GET  /{name}/health           — Instance health")

	if err := http.ListenAndServe(":"+port, corsMiddleware(handler)); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
