package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"neo-pkg-llm/agent"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/machbase"
	"neo-pkg-llm/tools"
)

// wsSession tracks a persistent agent, user binding, and active connection.
type wsSession struct {
	cancel   context.CancelFunc
	agent    *agent.Agent
	lastUsed time.Time
	userID   string
	provider string // current session's LLM provider
	model    string // current session's model
	conn     *websocket.Conn
	writeMu  sync.Mutex
}

func (s *wsSession) writeJSON(v any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.conn != nil {
		if err := s.conn.WriteJSON(v); err != nil {
			log.Printf("[WSServer] Write error: %v", err)
		}
	}
}

const wsSessionTTL = 30 * time.Minute

// wsServer is a WebSocket server that Chat UI connects to directly.
type wsServer struct {
	upgrader   websocket.Upgrader
	mc         *machbase.Client
	cfg        *Config
	sessions   sync.Map // session_id → *wsSession
	createLLMFn func(provider, model string) (llm.LLMProvider, error) // override for testing
}

func newWSServer(mc *machbase.Client, cfg *Config) *wsServer {
	return &wsServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		mc:  mc,
		cfg: cfg,
	}
}

// CloseAll cancels all running queries and closes all WebSocket connections.
func (s *wsServer) CloseAll() {
	s.sessions.Range(func(key, val any) bool {
		sess := val.(*wsSession)
		sess.cancel()
		sess.writeMu.Lock()
		if sess.conn != nil {
			sess.conn.Close()
			sess.conn = nil
		}
		sess.writeMu.Unlock()
		s.sessions.Delete(key)
		return true
	})
}

// createLLM creates an LLM client using the instance's config.
func (s *wsServer) createLLM(userID, provider, model string) (llm.LLMProvider, error) {
	if s.createLLMFn != nil {
		return s.createLLMFn(provider, model)
	}

	cfgCopy := *s.cfg
	cfgCopy.Provider = provider
	cfgCopy.Model = model
	return newLLMSafe(&cfgCopy)
}

// ServeHTTP handles the WebSocket upgrade.
func (s *wsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WSServer] Upgrade failed: %v", err)
		return
	}

	log.Printf("[WSServer] Connected")
	s.readLoop(conn)
}

// readLoop reads messages from a connected Chat UI client.
func (s *wsServer) readLoop(conn *websocket.Conn) {
	defer conn.Close()

	for {
		var msg wsInMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("[WSServer] Read error: %v", err)
			s.sessions.Range(func(key, val any) bool {
				sess := val.(*wsSession)
				sess.writeMu.Lock()
				if sess.conn == conn {
					sess.conn = nil
				}
				sess.writeMu.Unlock()
				return true
			})
			return
		}

		userID := msg.UserID
		if userID == "" {
			userID = s.mc.User // fallback to config user
		}

		switch msg.Type {
		case "chat":
			go s.handleChat(conn, userID, msg.SessionID, msg.Provider, msg.Model, msg.Query)
		case "stop":
			s.handleStop(msg.SessionID, userID)
		case "get_models":
			s.handleGetModels(conn, userID)
		default:
			log.Printf("[WSServer] Unknown type: %s", msg.Type)
		}
	}
}

func (s *wsServer) handleChat(conn *websocket.Conn, userID, sessionID, provider, model, query string) {
	// --- provider/model required ---
	if provider == "" || model == "" {
		emitErrorMsg(conn, sessionID, "provider와 model은 필수입니다.")
		return
	}

	// --- Session management ---
	var sess *wsSession
	if val, ok := s.sessions.Load(sessionID); ok {
		sess = val.(*wsSession)
		// Verify session ownership
		if sess.userID != userID {
			log.Printf("[WSServer] Session %s: owner=%s, caller=%s → rejected", sessionID, sess.userID, userID)
			emitErrorMsg(conn, sessionID, "세션 접근 권한이 없습니다.")
			return
		}
		// Cancel any running query
		sess.cancel()

		// Model changed → reset agent
		if sess.provider != provider || sess.model != model {
			log.Printf("[WSServer] Model changed: %s/%s → %s/%s, resetting agent", sess.provider, sess.model, provider, model)
			sess = nil // force new session creation below
			s.sessions.Delete(sessionID)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	if sess == nil {
		// Create LLM client for requested provider/model
		llmClient, err := s.createLLM(userID, provider, model)
		if err != nil {
			cancel()
			log.Printf("[WSServer] LLM creation failed: %v", err)
			emitErrorMsg(conn, sessionID, "LLM 생성 실패: "+err.Error())
			return
		}

		registry := tools.NewRegistry(s.mc)
		sess = &wsSession{
			cancel:   cancel,
			agent:    agent.NewAgent(llmClient, registry),
			lastUsed: time.Now(),
			userID:   userID,
			provider: provider,
			model:    model,
			conn:     conn,
		}
		s.sessions.Store(sessionID, sess)
		log.Printf("[WSServer] New session: %s (user=%s, %s/%s)", sessionID, userID, provider, model)
	} else {
		sess.cancel = cancel
		sess.lastUsed = time.Now()
		sess.writeMu.Lock()
		sess.conn = conn
		sess.writeMu.Unlock()
		log.Printf("[WSServer] Continuing session: %s (user=%s, %s/%s)", sessionID, userID, provider, model)
	}

	defer func() {
		cancel()
		sess.cancel = func() {}
	}()

	events := sess.agent.RunStream(ctx, query)
	emitLegacy(sess, sessionID, events)
	sess.lastUsed = time.Now()
}

func (s *wsServer) handleStop(sessionID, userID string) {
	if val, ok := s.sessions.Load(sessionID); ok {
		sess := val.(*wsSession)
		if sess.userID != userID {
			return
		}
		sess.cancel()
		log.Printf("[WSServer] Stopped session: %s (user=%s)", sessionID, userID)
	}
}

func (s *wsServer) sessionReaper() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.sessions.Range(func(key, val any) bool {
			sess := val.(*wsSession)
			if now.Sub(sess.lastUsed) > wsSessionTTL {
				sess.cancel()
				sess.writeMu.Lock()
				if sess.conn != nil {
					sess.conn.Close()
					sess.conn = nil
				}
				sess.writeMu.Unlock()
				s.sessions.Delete(key)
				log.Printf("[WSServer] Session expired: %s (user=%s)", key, sess.userID)
			}
			return true
		})
	}
}

func (s *wsServer) handleGetModels(conn *websocket.Conn, userID string) {
	type modelInfo struct {
		Name    string `json:"name"`
		ModelID string `json:"model_id,omitempty"`
	}
	type providerModels struct {
		Provider string      `json:"provider"`
		Models   []modelInfo `json:"models"`
	}

	cfg := s.cfg

	var providers []providerModels

	if len(cfg.Claude.Models) > 0 && cfg.Claude.APIKey != "" {
		models := make([]modelInfo, len(cfg.Claude.Models))
		for i, m := range cfg.Claude.Models {
			models[i] = modelInfo{Name: m.Name, ModelID: m.ModelID}
		}
		providers = append(providers, providerModels{Provider: "claude", Models: models})
	}
	if len(cfg.ChatGPT.Models) > 0 && cfg.ChatGPT.APIKey != "" {
		models := make([]modelInfo, len(cfg.ChatGPT.Models))
		for i, m := range cfg.ChatGPT.Models {
			models[i] = modelInfo{Name: m.Name, ModelID: m.ModelID}
		}
		providers = append(providers, providerModels{Provider: "chatgpt", Models: models})
	}
	if len(cfg.Gemini.Models) > 0 && cfg.Gemini.APIKey != "" {
		models := make([]modelInfo, len(cfg.Gemini.Models))
		for i, m := range cfg.Gemini.Models {
			models[i] = modelInfo{Name: m.Name, ModelID: m.ModelID}
		}
		providers = append(providers, providerModels{Provider: "gemini", Models: models})
	}
	if len(cfg.Ollama.Models) > 0 {
		models := make([]modelInfo, len(cfg.Ollama.Models))
		for i, m := range cfg.Ollama.Models {
			models[i] = modelInfo{Name: m.Name, ModelID: m.ModelID}
		}
		providers = append(providers, providerModels{Provider: "ollama", Models: models})
	}

	writeJSONTo(conn, map[string]any{
		"type":      "models",
		"providers": providers,
	})
}

func writeJSONTo(conn *websocket.Conn, v any) {
	if err := conn.WriteJSON(v); err != nil {
		log.Printf("[WSServer] Write error: %v", err)
	}
}

// emitErrorMsg sends an error as a legacy msg format (answer_start → error block → answer_stop).
func emitErrorMsg(conn *websocket.Conn, sessionID, errText string) {
	emit := func(typ string, body *legacyBodyUnion) {
		writeJSONTo(conn, legacyMessage{
			Type:    "msg",
			Session: sessionID,
			Message: &legacyMsgBody{
				Ver:  "1.0",
				ID:   0,
				Type: typ,
				Body: body,
			},
		})
	}
	emit("answer_start", nil)
	emit("stream_msg_start", nil)
	emit("stream_block_start", nil)
	emit("stream_block_delta", &legacyBodyUnion{
		OfStreamBlockDelta: &legacyStreamBlockDelta{
			ContentType: "error",
			Text:        errText,
		},
	})
	emit("stream_block_stop", nil)
	emit("stream_msg_stop", nil)
	emit("answer_stop", nil)
}

// --- Legacy format (compatible with Neo Chat UI eventbus protocol) ---

type legacyMessage struct {
	Type    string           `json:"type"`
	Session string           `json:"session,omitempty"`
	Message *legacyMsgBody   `json:"message,omitempty"`
}

type legacyMsgBody struct {
	Ver  string          `json:"ver"`
	ID   int64           `json:"id"`
	Type string          `json:"type"`
	Body *legacyBodyUnion `json:"body,omitempty"`
}

type legacyBodyUnion struct {
	OfStreamBlockDelta *legacyStreamBlockDelta `json:"ofStreamBlockDelta,omitempty"`
}

type legacyStreamBlockDelta struct {
	ContentType string `json:"contentType"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
}

// emitLegacy converts agent events to the legacy Neo Chat UI format.
func emitLegacy(sess *wsSession, sessionID string, events <-chan agent.Event) {
	emit := func(typ string, body *legacyBodyUnion) {
		sess.writeJSON(legacyMessage{
			Type:    "msg",
			Session: sessionID,
			Message: &legacyMsgBody{
				Ver:  "1.0",
				ID:   0,
				Type: typ,
				Body: body,
			},
		})
	}

	emitTextBlock := func(text string) {
		emit("stream_block_start", nil)
		emit("stream_block_delta", &legacyBodyUnion{
			OfStreamBlockDelta: &legacyStreamBlockDelta{
				ContentType: "text",
				Text:        text,
			},
		})
		emit("stream_block_stop", nil)
	}

	// Answer start + message start
	emit("answer_start", nil)
	emit("stream_msg_start", nil)

	inStreamBlock := false

	for event := range events {
		switch event.Type {
		case "status":
			// status → text block
			emitTextBlock(event.Content)

		case "stream":
			// stream → open block once, then deltas
			if !inStreamBlock {
				emit("stream_block_start", nil)
				inStreamBlock = true
			}
			emit("stream_block_delta", &legacyBodyUnion{
				OfStreamBlockDelta: &legacyStreamBlockDelta{
					ContentType: "text",
					Text:        event.Content,
				},
			})

		case "tool_call":
			// close any open stream block first
			if inStreamBlock {
				emit("stream_block_stop", nil)
				inStreamBlock = false
			}
			toolMsg := fmt.Sprintf("\n🛠️ Calling tool: %s\n", event.Name)
			emitTextBlock(toolMsg)

		case "tool_result":
			resultMsg := fmt.Sprintf("```\n%s\n```\n", event.Content)
			emitTextBlock(resultMsg)

		case "final":
			// close any open stream block
			if inStreamBlock {
				emit("stream_block_stop", nil)
				inStreamBlock = false
			}

		case "error":
			if inStreamBlock {
				emit("stream_block_stop", nil)
				inStreamBlock = false
			}
			emit("stream_block_start", nil)
			emit("stream_block_delta", &legacyBodyUnion{
				OfStreamBlockDelta: &legacyStreamBlockDelta{
					ContentType: "error",
					Text:        event.Content,
				},
			})
			emit("stream_block_stop", nil)
		}
	}

	// Close any remaining open block
	if inStreamBlock {
		emit("stream_block_stop", nil)
	}

	// Message stop + answer stop
	emit("stream_msg_stop", nil)
	emit("answer_stop", nil)
}
