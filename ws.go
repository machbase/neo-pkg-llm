package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"neo-pkg-llm/agent"
	"neo-pkg-llm/llm"
	"neo-pkg-llm/tools"
)

// WebSocket message types (Neo ↔ neo-pkg-llm protocol)

type wsInMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
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

// session tracks a running agent and its cancel function.
type session struct {
	cancel context.CancelFunc
}

// wsClient manages the WebSocket connection to Neo.
type wsClient struct {
	url       string
	llm       llm.LLMProvider
	registry  *tools.Registry
	conn      *websocket.Conn
	writeMu   sync.Mutex
	sessions  sync.Map // session_id → *session
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
	for {
		err := w.connect()
		if err != nil {
			log.Printf("[WS] Connection failed: %v", err)
		}
		log.Println("[WS] Reconnecting in 3 seconds...")
		time.Sleep(3 * time.Second)
	}
}

func (w *wsClient) connect() error {
	log.Printf("[WS] Connecting to %s", w.url)
	conn, _, err := websocket.DefaultDialer.Dial(w.url, nil)
	if err != nil {
		return err
	}
	w.conn = conn
	defer conn.Close()
	log.Println("[WS] Connected to Neo")

	for {
		var msg wsInMessage
		if err := conn.ReadJSON(&msg); err != nil {
			// Cancel all running sessions on disconnect
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
			log.Printf("[WS] Unknown message type: %s", msg.Type)
		}
	}
}

func (w *wsClient) handleChat(sessionID, query string) {
	ctx, cancel := context.WithCancel(context.Background())
	w.sessions.Store(sessionID, &session{cancel: cancel})
	defer func() {
		cancel()
		w.sessions.Delete(sessionID)
	}()

	ag := agent.NewAgent(w.llm, w.registry)
	events := ag.RunStream(ctx, query)

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
}

func (w *wsClient) handleStop(sessionID string) {
	if val, ok := w.sessions.Load(sessionID); ok {
		if s, ok := val.(*session); ok {
			s.cancel()
			log.Printf("[WS] Stopped session: %s", sessionID)
		}
	}
}

func (w *wsClient) writeJSON(v any) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if w.conn != nil {
		if err := w.conn.WriteJSON(v); err != nil {
			log.Printf("[WS] Write error: %v", err)
		}
	}
}
