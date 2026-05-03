package server

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/livetemplate/tinkerdown"
)

//go:embed playground.html
var playgroundHTML embed.FS

// PlaygroundSession represents an active playground session.
type PlaygroundSession struct {
	ID        string
	Page      *tinkerdown.Page
	Markdown  string
	CreatedAt time.Time
}

// PlaygroundHandler handles playground-related requests.
type PlaygroundHandler struct {
	server   *Server
	sessions map[string]*PlaygroundSession
	mu       sync.RWMutex
}

// NewPlaygroundHandler creates a new playground handler.
func NewPlaygroundHandler(s *Server) *PlaygroundHandler {
	h := &PlaygroundHandler{
		server:   s,
		sessions: make(map[string]*PlaygroundSession),
	}

	// Start cleanup goroutine for expired sessions
	go h.cleanupLoop()

	return h
}

// cleanupLoop removes expired sessions every 5 minutes.
func (h *PlaygroundHandler) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		for id, session := range h.sessions {
			// Remove sessions older than 1 hour
			if now.Sub(session.CreatedAt) > time.Hour {
				delete(h.sessions, id)
			}
		}
		h.mu.Unlock()
	}
}

// generateSessionID creates a random session ID.
func generateSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ServePlaygroundPage serves the playground HTML page.
func (h *PlaygroundHandler) ServePlaygroundPage(w http.ResponseWriter, r *http.Request) {
	content, err := playgroundHTML.ReadFile("playground.html")
	if err != nil {
		http.Error(w, "Playground not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

// RenderRequest is the JSON request body for /playground/render.
type RenderRequest struct {
	Markdown string `json:"markdown"`
}

// RenderResponse is the JSON response for /playground/render.
type RenderResponse struct {
	SessionID string `json:"sessionId"`
	Error     string `json:"error,omitempty"`
}

// HandleRender handles POST /playground/render.
func (h *PlaygroundHandler) HandleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size to prevent DoS (1 MB)
	const maxPlaygroundSize = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxPlaygroundSize)

	var req RenderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Distinguish between body too large (413) and malformed JSON (400)
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.jsonError(w, "Request too large (max 1 MB)", http.StatusRequestEntityTooLarge)
		} else {
			h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
		}
		return
	}

	if strings.TrimSpace(req.Markdown) == "" {
		h.jsonError(w, "Markdown content is required", http.StatusBadRequest)
		return
	}

	// Parse the markdown
	page, err := tinkerdown.ParseString(req.Markdown)
	if err != nil {
		h.jsonError(w, fmt.Sprintf("Failed to parse markdown: %v", err), http.StatusBadRequest)
		return
	}

	// Create session
	sessionID := generateSessionID()
	session := &PlaygroundSession{
		ID:        sessionID,
		Page:      page,
		Markdown:  req.Markdown,
		CreatedAt: time.Now(),
	}

	h.mu.Lock()
	h.sessions[sessionID] = session
	h.mu.Unlock()

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RenderResponse{
		SessionID: sessionID,
	})
}

// HandlePreview handles GET /playground/preview/{sessionId}.
func (h *PlaygroundHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path
	path := strings.TrimPrefix(r.URL.Path, "/playground/preview/")
	sessionID := strings.Split(path, "/")[0]

	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	session, exists := h.sessions[sessionID]
	h.mu.RUnlock()

	if !exists {
		http.Error(w, "Session not found or expired", http.StatusNotFound)
		return
	}

	// Render the page
	html := h.renderPage(session.Page, r.Host, detectWSScheme(r))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// HandlePreviewWS handles WebSocket connections for playground previews.
func (h *PlaygroundHandler) HandlePreviewWS(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from query parameter
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	session, exists := h.sessions[sessionID]
	h.mu.RUnlock()

	if !exists {
		http.Error(w, "Session not found or expired", http.StatusNotFound)
		return
	}

	// Create WebSocket handler for this session's page
	wsHandler := NewWebSocketHandler(session.Page, h.server, true, "", h.server.config)
	wsHandler.ServeHTTP(w, r)
}

// renderPage renders a page to HTML using the server's rendering logic.
func (h *PlaygroundHandler) renderPage(page *tinkerdown.Page, host string, wsScheme string) string {
	// Use the server's renderPage method for consistent output
	return h.server.renderPage(page, "/playground/preview", host, wsScheme)
}

// jsonError sends a JSON error response.
func (h *PlaygroundHandler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(RenderResponse{Error: message})
}
