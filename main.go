package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"neo-pkg-llm/agent"
	"neo-pkg-llm/config"
	"neo-pkg-llm/logger"
	"neo-pkg-llm/machbase"
	"neo-pkg-llm/mcp"
	"neo-pkg-llm/server"
	"neo-pkg-llm/tools"
)

func main() {
	mode := flag.String("mode", "server", "Run mode: 'server' (HTTP API), 'cli' (interactive), 'mcp' (MCP stdio server), or 'ws' (WebSocket client)")
	port := flag.String("port", "", "HTTP server port (overrides config, server mode)")
	neoWSURL := flag.String("neo-ws-url", "", "Neo WebSocket URL to connect to (ws mode)")
	configPath := flag.String("config", "configs/sys.json", "Path to config file")
	providerFlag := flag.String("provider", "", "Override LLM provider (claude, chatgpt, gemini)")
	modelFlag := flag.String("model", "", "Override model name or model_id")
	flag.Parse()

	// Initialize logger
	if err := logger.Init(&logger.Options{
		Dir:        "logs",
		FilePrefix: "neo-pkg-llm",
		Level:      logger.DEBUG,
		MaxSizeMB:  10,
		MaxFiles:   5,
		ToStdout:   true,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Std().Close()

	cfg := config.LoadConfig(*configPath)

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
			logger.Fatalf("server port is required: set server.port in config or use --port flag")
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

// --- MCP Server Mode (stdio JSON-RPC) ---

func runMCP(cfg *config.Config) {
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.Password)
	registry := tools.NewRegistry(mc)

	srv := mcp.NewServer(registry)
	if err := srv.Run(); err != nil {
		logger.Fatalf("MCP server error: %v", err)
	}
}

// --- WebSocket Client Mode ---

func runWS(cfg *config.Config, neoURL string) {
	if neoURL == "" {
		logger.Fatalf("--neo-ws-url is required for ws mode")
	}
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.Password)
	registry := tools.NewRegistry(mc)

	llmClient, err := server.NewLLMSafe(cfg)
	if err != nil {
		logger.Warnf("LLM init failed (will report on chat): %v", err)
	}

	logger.Infof("WebSocket client mode: connecting to %s", neoURL)
	logger.Infof("Machbase: %s | Provider: %s | Model: %s", cfg.MachbaseURL(), cfg.ResolveProvider(), cfg.ResolveModelID())

	client := server.NewWSClient(neoURL, llmClient, registry)
	client.Run()
}

// --- CLI Mode ---

func runCLI(cfg *config.Config) {
	mc := machbase.NewClient(cfg.MachbaseURL(), cfg.Machbase.User, cfg.Machbase.Password)
	registry := tools.NewRegistry(mc)
	llmClient := server.NewLLM(cfg)

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

func runServer(cfg *config.Config, port string) {
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		logger.Fatalf("failed to create configs directory: %v", err)
	}

	mgr := server.NewManager(configsDir)
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

	logger.Infof("Agentic Loop Go server starting on :%s", port)
	logger.Infof("Configs dir: %s", configsDir)
	logger.Infof("Endpoints (master):")
	logger.Infof("  GET  /health                — Health check")
	logger.Infof("  GET  /settings              — Settings page")
	logger.Infof("  GET  /api/instances         — List running instances")
	logger.Infof("  POST /api/configs           — Save config + start instance")
	logger.Infof("  GET  /api/configs           — List configs")
	logger.Infof("  GET  /api/configs/{name}    — Get config")
	logger.Infof("  PUT  /api/configs/{name}    — Update config + restart instance")
	logger.Infof("  DELETE /api/configs/{name}  — Delete config + stop instance")
	logger.Infof("Endpoints (per-instance: /{name}/...):")
	logger.Infof("  POST /{name}/api/chat         — Non-streaming chat")
	logger.Infof("  POST /{name}/api/chat/stream  — SSE streaming chat")
	logger.Infof("  GET  /{name}/api/settings     — Get instance config")
	logger.Infof("  POST /{name}/api/restart-llm  — Restart instance LLM")
	logger.Infof("  GET  /{name}/ws               — WebSocket (Chat UI)")
	logger.Infof("  GET  /{name}/health           — Instance health")

	if err := http.ListenAndServe(":"+port, corsMiddleware(handler)); err != nil {
		logger.Fatalf("%v", err)
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
