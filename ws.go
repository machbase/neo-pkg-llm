package main

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"neo-pkg-llm/agent"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/logger"
	"neo-pkg-llm/tools"
)

// WebSocket message types (Neo ↔ neo-pkg-llm protocol)

type wsInMessage struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Query     string `json:"query,omitempty"`
}

type wsOutMessage struct {
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	Step      int            `json:"step,omitempty"`
	Name      string         `json:"name,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
	Status    string         `json:"status,omitempty"`
	Content   string         `json:"content,omitempty"`
}

// session tracks a persistent agent and its current cancel function.
type session struct {
	cancel   context.CancelFunc
	agent    *agent.Agent // persistent agent for conversation continuity
	lastUsed time.Time
}

const sessionTTL = 30 * time.Minute // sessions expire after 30 min of inactivity

// wsClient manages the WebSocket connection to Neo.
type wsClient struct {
	url      string
	llm      llm.LLMProvider
	registry *tools.Registry
	conn     *websocket.Conn
	writeMu  sync.Mutex
	sessions sync.Map // session_id → *session
}

func newWSClient(url string, llmClient llm.LLMProvider, registry *tools.Registry) *wsClient {
	return &wsClient{
		url:      url,
		llm:      llmClient,
		registry: registry,
	}
}

// Run connects to Neo and processes messages. Reconnects on failure.
func (w *wsClient) Run() {
	go w.sessionReaper()
	for {
		err := w.connect()
		if err != nil {
			logger.Infof("[WS] Connection failed: %v", err)
		}
		logger.Infof("[WS] Reconnecting in 3 seconds...")
		time.Sleep(3 * time.Second)
	}
}

func (w *wsClient) connect() error {
	logger.Infof("[WS] Connecting to %s", w.url)
	conn, _, err := websocket.DefaultDialer.Dial(w.url, nil)
	if err != nil {
		return err
	}
	w.conn = conn
	defer conn.Close()
	logger.Infof("[WS] Connected to Neo")

	for {
		var msg wsInMessage
		if err := conn.ReadJSON(&msg); err != nil {
			// Cancel all running queries on disconnect
			w.sessions.Range(func(key, val any) bool {
				if s, ok := val.(*session); ok {
					s.cancel()
				}
				w.sessions.Delete(key)
				return true
			})
			return err
		}

		switch msg.Type {
		case "chat":
			go w.handleChat(msg.SessionID, msg.Query)
		case "stop":
			w.handleStop(msg.SessionID)
		default:
			logger.Infof("[WS] Unknown message type: %s", msg.Type)
		}
	}
}

func (w *wsClient) handleChat(sessionID, query string) {
	// --- Session management ---
	var sess *session
	if val, ok := w.sessions.Load(sessionID); ok {
		sess = val.(*session)
		// Cancel any previously running query in this session
		sess.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())

	if sess == nil {
		// New session: create agent with fresh conversation
		sess = &session{
			cancel:   cancel,
			agent:    agent.NewAgent(w.llm, w.registry),
			lastUsed: time.Now(),
		}
		w.sessions.Store(sessionID, sess)
		logger.Infof("[WS] New session: %s", sessionID)
	} else {
		// Existing session: update cancel func and timestamp, reuse agent
		sess.cancel = cancel
		sess.lastUsed = time.Now()
		logger.Infof("[WS] Continuing session: %s", sessionID)
	}

	defer func() {
		cancel()
		// Keep session alive for conversation continuity.
		// Set a no-op cancel so the reaper doesn't panic.
		sess.cancel = func() {}
	}()

	events := sess.agent.RunStream(ctx, query)

	for event := range events {
		out := wsOutMessage{
			Type:      event.Type,
			SessionID: sessionID,
			Step:      event.Step,
			Name:      event.Name,
			Args:      event.Args,
			Status:    event.Status,
			Content:   event.Content,
		}
		w.writeJSON(out)
	}

	// Update lastUsed after query completes
	sess.lastUsed = time.Now()
}

func (w *wsClient) handleStop(sessionID string) {
	if val, ok := w.sessions.Load(sessionID); ok {
		if s, ok := val.(*session); ok {
			s.cancel()
			logger.Infof("[WS] Stopped session: %s", sessionID)
		}
	}
}

// sessionReaper periodically removes expired sessions.
func (w *wsClient) sessionReaper() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		w.sessions.Range(func(key, val any) bool {
			s := val.(*session)
			if now.Sub(s.lastUsed) > sessionTTL {
				s.cancel()
				w.sessions.Delete(key)
				logger.Infof("[WS] Session expired: %s", key)
			}
			return true
		})
	}
}

func (w *wsClient) writeJSON(v any) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if w.conn != nil {
		if err := w.conn.WriteJSON(v); err != nil {
			logger.Infof("[WS] Write error: %v", err)
		}
	}
}
