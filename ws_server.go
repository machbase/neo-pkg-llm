package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"neo-pkg-llm/agent"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/logger"
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
			logger.Infof("[WSServer] Write error: %v", err)
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
		logger.Infof("[WSServer] Upgrade failed: %v", err)
		return
	}

	logger.Infof("[WSServer] Connected")
	s.readLoop(conn)
}

// readLoop reads messages from a connected Chat UI client.
func (s *wsServer) readLoop(conn *websocket.Conn) {
	defer conn.Close()

	for {
		var msg wsInMessage
		if err := conn.ReadJSON(&msg); err != nil {
			logger.Infof("[WSServer] Read error: %v", err)
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
		case "clear":
			s.handleClear(conn, msg.SessionID, userID)
		case "get_models":
			s.handleGetModels(conn, userID)
		default:
			logger.Infof("[WSServer] Unknown type: %s", msg.Type)
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
			logger.Infof("[WSServer] Session %s: owner=%s, caller=%s → rejected", sessionID, sess.userID, userID)
			emitErrorMsg(conn, sessionID, "세션 접근 권한이 없습니다.")
			return
		}
		// Cancel any running query
		sess.cancel()

		// Model changed → reset agent
		if sess.provider != provider || sess.model != model {
			logger.Infof("[WSServer] Model changed: %s/%s → %s/%s, resetting agent", sess.provider, sess.model, provider, model)
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
			logger.Infof("[WSServer] LLM creation failed: %v", err)
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
		logger.Infof("[WSServer] New session: %s (user=%s, %s/%s)", sessionID, userID, provider, model)
	} else {
		sess.cancel = cancel
		sess.lastUsed = time.Now()
		sess.writeMu.Lock()
		sess.conn = conn
		sess.writeMu.Unlock()
		logger.Infof("[WSServer] Continuing session: %s (user=%s, %s/%s)", sessionID, userID, provider, model)
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
		logger.Infof("[WSServer] Stopped session: %s (user=%s)", sessionID, userID)

		// Send stop confirmation to UI
		sess.writeJSON(map[string]any{
			"type":    "stop",
			"session": sessionID,
			"msg":     "답변이 중지되었습니다.",
		})
	}
}

func (s *wsServer) handleClear(conn *websocket.Conn, sessionID, userID string) {
	if val, ok := s.sessions.Load(sessionID); ok {
		sess := val.(*wsSession)
		if sess.userID != userID {
			return
		}
		sess.cancel()
		s.sessions.Delete(sessionID)
		logger.Infof("[WSServer] Session cleared: %s (user=%s)", sessionID, userID)
	}

	writeJSONTo(conn, map[string]any{
		"type":    "clear",
		"session": sessionID,
		"msg":     "세션이 초기화되었습니다.",
	})
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
				logger.Infof("[WSServer] Session expired: %s (user=%s)", key, sess.userID)
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

	if len(providers) == 0 {
		logger.Warnf("[WSServer] No available providers: API key not configured")
		writeJSONTo(conn, map[string]any{
			"type":  "models",
			"msg": "사용 가능한 LLM provider가 없습니다. API key 설정을 확인해주세요.",
		})
		return
	}

	writeJSONTo(conn, map[string]any{
		"type":      "models",
		"providers": providers,
	})
}

func writeJSONTo(conn *websocket.Conn, v any) {
	if err := conn.WriteJSON(v); err != nil {
		logger.Infof("[WSServer] Write error: %v", err)
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
	writeJSONTo(conn, map[string]any{
		"type":    "error",
		"session": sessionID,
		"msg":     errText,
	})
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

// --- Code fence parser for streaming ---

// fenceParser detects markdown code fences in streamed text and emits
// separate contentType values for code blocks (e.g., "sql", "tql").
// Text outside code fences is emitted as contentType "text".
// Code blocks are buffered and emitted as a single chunk.
type fenceParser struct {
	inCode   bool   // inside a code fence
	language string // detected language (sql, tql, etc.)
	textBuf  strings.Builder
	codeBuf  strings.Builder
	lineBuf  strings.Builder // accumulates partial lines
	emit     func(contentType, text string)
}

// Write processes a streaming text chunk. Chunks may split across line boundaries.
func (p *fenceParser) Write(chunk string) {
	for _, ch := range chunk {
		p.lineBuf.WriteRune(ch)
		if ch == '\n' {
			p.processLine(p.lineBuf.String())
			p.lineBuf.Reset()
		}
	}
}

// Flush emits any remaining buffered content.
func (p *fenceParser) Flush() {
	// Process any remaining partial line
	if p.lineBuf.Len() > 0 {
		p.processLine(p.lineBuf.String())
		p.lineBuf.Reset()
	}
	// Emit remaining buffers
	if p.inCode {
		// Unclosed code block — emit as text (include fence marker)
		p.textBuf.WriteString("```" + p.language + "\n")
		p.textBuf.WriteString(p.codeBuf.String())
		p.codeBuf.Reset()
		p.inCode = false
	}
	if p.textBuf.Len() > 0 {
		p.emit("text", p.textBuf.String())
		p.textBuf.Reset()
	}
}

// processLine handles a complete line, detecting code fence boundaries.
func (p *fenceParser) processLine(line string) {
	trimmed := strings.TrimSpace(line)

	if !p.inCode {
		// Check for opening fence: ```lang
		if strings.HasPrefix(trimmed, "```") && trimmed != "```" {
			// Flush pending text before code block
			if p.textBuf.Len() > 0 {
				p.emit("text", p.textBuf.String())
				p.textBuf.Reset()
			}
			p.language = strings.TrimPrefix(trimmed, "```")
			p.language = strings.TrimSpace(p.language)
			p.inCode = true
			return
		}
		// Check for opening fence without language: ```
		if trimmed == "```" {
			if p.textBuf.Len() > 0 {
				p.emit("text", p.textBuf.String())
				p.textBuf.Reset()
			}
			p.language = ""
			p.inCode = true
			return
		}
		p.textBuf.WriteString(line)
	} else {
		// Inside code block — check for closing fence
		if trimmed == "```" {
			// Detect actual language from code content, then emit with markdown fences
			code := p.codeBuf.String()
			lang := detectCodeLanguage(code, p.language)
			fenced := "```" + lang + "\n" + code + "```\n"
			p.emit(lang, fenced)
			p.codeBuf.Reset()
			p.inCode = false
			p.language = ""
			return
		}
		p.codeBuf.WriteString(line)
	}
}

// detectCodeLanguage determines the actual language of a code block by inspecting its content.
// Falls back to the fence-declared language if no pattern matches.
func detectCodeLanguage(code, declared string) string {
	upper := strings.ToUpper(code)

	// TQL patterns — check first (TQL can contain SQL() inside)
	tqlKeywords := []string{"SQL(", "FAKE(", "CHART_", "CHART(", "MAPVALUE(", "POPVALUE(",
		"PUSHVALUE(", "SCRIPT(", "CSV(", "JSON(", "APPEND(", "INSERT(",
		"BRIDGE(", "QUERY(", "SINK_", "DISCARD(", "GROUP("}
	for _, kw := range tqlKeywords {
		if strings.Contains(upper, kw) {
			return "tql"
		}
	}

	// SQL patterns
	sqlKeywords := []string{"SELECT ", "INSERT ", "CREATE ", "DROP ", "ALTER ",
		"DELETE ", "UPDATE ", "ROLLUP(", "GROUP BY", "ORDER BY"}
	for _, kw := range sqlKeywords {
		if strings.Contains(upper, kw) {
			return "sql"
		}
	}

	// Use declared language, or "code" if empty
	if declared == "" {
		return "code"
	}
	return declared
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

	// Fence parser: detects code blocks and emits with proper contentType
	// All content (text + code) streams within one continuous block per response segment.
	parser := &fenceParser{
		emit: func(contentType, text string) {
			if text == "" {
				return
			}
			if !inStreamBlock {
				emit("stream_block_start", nil)
				inStreamBlock = true
			}
			emit("stream_block_delta", &legacyBodyUnion{
				OfStreamBlockDelta: &legacyStreamBlockDelta{
					ContentType: contentType,
					Text:        text,
				},
			})
		},
	}

	for event := range events {
		switch event.Type {
		case "status":
			// status → text block
			emitTextBlock(event.Content)

		case "stream":
			parser.Write(event.Content)

		case "tool_call":
			// flush parser and close any open stream block first
			parser.Flush()
			if inStreamBlock {
				emit("stream_block_stop", nil)
				inStreamBlock = false
			}
			toolMsg := fmt.Sprintf("\n🛠️ Calling tool: **%s**\n", event.Name)
			if len(event.Args) > 0 {
				for k, v := range event.Args {
					vs := fmt.Sprintf("%v", v)
					if len(vs) > 500 {
						vs = vs[:500] + "..."
					}
					toolMsg += fmt.Sprintf("  - `%s`: `%s`\n", k, vs)
				}
			}
			emitTextBlock(toolMsg)

		case "tool_result":
			resultMsg := fmt.Sprintf("```\n%s\n```\n", event.Content)
			emitTextBlock(resultMsg)

		case "final":
			// flush remaining parser buffer
			parser.Flush()
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
			sess.writeJSON(map[string]any{
				"type":    "error",
				"session": sessionID,
				"msg":     event.Content,
			})
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
