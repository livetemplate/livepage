package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/livetemplate/lvt/components/base"
	"github.com/livetemplate/lvt/components/datatable"
	"github.com/livetemplate/livetemplate"
	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/runtime"
	"github.com/livetemplate/tinkerdown/internal/source"
)

// newUpgrader creates a WebSocket upgrader with origin validation.
// If allowedOrigins is empty, it defaults to same-origin policy.
func newUpgrader(allowedOrigins []string) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return checkWebSocketOrigin(r, allowedOrigins)
		},
	}
}

// checkWebSocketOrigin validates the Origin header of a WebSocket upgrade request.
func checkWebSocketOrigin(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser clients (CLI tools, etc.) may not send Origin
		return true
	}

	// Check explicit allow list
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return true
		}
		if allowed == origin {
			return true
		}
	}

	// Default: same-origin policy — origin host must match request host,
	// and scheme must match (http→ws, https→wss)
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if originURL.Host != r.Host {
		return false
	}
	// Validate scheme consistency: reject http origins on TLS connections and vice versa
	if r.TLS != nil && originURL.Scheme != "https" {
		return false
	}
	return true
}

// ExpressionsBlockID is the special block ID used for routing expression update messages.
// This must match the value expected by the client-side MessageRouter.
const ExpressionsBlockID = "__expressions__"

// MessageEnvelope represents a multiplexed WebSocket message.
type MessageEnvelope struct {
	BlockID   string          `json:"blockID"`
	Action    string          `json:"action"`
	Data      json.RawMessage `json:"data"`
	ExecMeta  *ExecMeta       `json:"execMeta,omitempty"`  // Optional exec source metadata
	CacheMeta *CacheMeta      `json:"cacheMeta,omitempty"` // Optional cache metadata
}

// ExecMeta contains execution state for exec source blocks
type ExecMeta struct {
	Status   string `json:"status"`
	Duration int64  `json:"duration,omitempty"`
	Output   string `json:"output,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Command  string `json:"command,omitempty"`
}

// CacheMeta contains cache state for cached source blocks
type CacheMeta struct {
	Cached     bool   `json:"cached"`
	Age        string `json:"age,omitempty"`
	ExpiresIn  string `json:"expires_in,omitempty"`
	Stale      bool   `json:"stale"`
	Refreshing bool   `json:"refreshing"`
}

// WebSocketHandler handles WebSocket connections for interactive blocks.
type WebSocketHandler struct {
	page           *tinkerdown.Page
	mu             sync.RWMutex
	writeMu        sync.Mutex                      // Serializes all writes to the websocket connection
	instances      map[string]*BlockInstance      // blockID -> instance
	sourceFiles    map[string][]string            // blockID -> source file paths (for file watching)
	debug          bool
	server         *Server                        // Reference to server for connection tracking
	stateFactories map[string]func() runtime.Store // State factories for lvt-source blocks
	rootDir        string                          // Site root directory for database path
	config         *config.Config                  // Site configuration with sources
	conn           *websocket.Conn                 // Current connection for this handler
	actionSources  map[string]source.Source       // Cached sources for custom actions
}

// BlockInstance represents a running LiveTemplate instance for an interactive block.
type BlockInstance struct {
	blockID  string
	state    runtime.Store
	template *livetemplate.Template
	conn     *websocket.Conn
	mu       sync.Mutex
}

// NewWebSocketHandler creates a new WebSocket handler for a page.
func NewWebSocketHandler(page *tinkerdown.Page, server *Server, debug bool, rootDir string, cfg *config.Config) *WebSocketHandler {
	if debug {
		log.Printf("[WS] Creating WebSocket handler for page: %s", page.ID)
		log.Printf("[WS] Page has %d server blocks", len(page.ServerBlocks))
		log.Printf("[WS] Page has %d interactive blocks", len(page.InteractiveBlocks))
	}

	h := &WebSocketHandler{
		page:           page,
		instances:      make(map[string]*BlockInstance),
		sourceFiles:    make(map[string][]string),
		debug:          debug,
		server:         server,
		stateFactories: make(map[string]func() runtime.Store),
		rootDir:        rootDir,
		config:         cfg,
		actionSources:  make(map[string]source.Source),
	}

	// Initialize lvt-source blocks (no compilation needed)
	h.initializeSourceBlocks()

	return h
}

// Close cleans up all resources
func (h *WebSocketHandler) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.debug {
		log.Printf("[WS] Closing WebSocket handler, cleaning up %d instances", len(h.instances))
	}

	// Close all state instances
	for blockID, instance := range h.instances {
		if instance.state != nil {
			if err := instance.state.Close(); err != nil && h.debug {
				log.Printf("[WS] Error closing state for block %s: %v", blockID, err)
			}
		}
	}

	// Close cached action sources
	for name, src := range h.actionSources {
		if err := src.Close(); err != nil && h.debug {
			log.Printf("[WS] Error closing action source %s: %v", name, err)
		}
	}

	// Clear maps
	h.instances = make(map[string]*BlockInstance)
	h.actionSources = make(map[string]source.Source)
}

// initializeSourceBlocks initializes all lvt-source blocks with runtime state.
// Regular server blocks (Go code) are no longer supported.
func (h *WebSocketHandler) initializeSourceBlocks() {
	// Debug: list all server block IDs first
	if h.debug {
		var blockIDs []string
		for id := range h.page.ServerBlocks {
			blockIDs = append(blockIDs, id)
		}
		log.Printf("[WS] All server block IDs: %v", blockIDs)
	}

	for blockID, block := range h.page.ServerBlocks {
		if h.debug {
			log.Printf("[WS] Processing server block: %s (metadata: %v)", blockID, block.Metadata)
		}

		// Only lvt-source blocks are supported
		sourceName := block.Metadata["lvt-source"]
		if sourceName == "" {
			// Regular server blocks (Go code) are no longer supported
			log.Printf("[WS] ERROR: Server block %s is not an lvt-source block. Go code blocks are no longer supported.", blockID)
			log.Printf("[WS] Please migrate to lvt-source by defining a source in frontmatter or tinkerdown.yaml")
			continue
		}

		// lvt-source block - use runtime.GenericState (no compilation needed!)
		// Check page-level sources first (from frontmatter), then site-level (from tinkerdown.yaml)
		sourceCfg, found := h.getEffectiveSource(sourceName)
		if !found {
			log.Printf("[WS] Source %q not found (checked frontmatter and tinkerdown.yaml) for block %s", sourceName, blockID)
			continue
		}
		if h.debug {
			log.Printf("[WS] Creating runtime state for lvt-source block: %s (source: %s, type: %s)", blockID, sourceName, sourceCfg.Type)
		}
		// Pass the current markdown file path for same-file markdown sources
		currentFile := ""
		if h.page != nil {
			currentFile = h.page.SourceFile
		}

		// Create runtime state factory
		// Capture variables for closure
		srcName, srcCfg, rootDir, curFile := sourceName, sourceCfg, h.rootDir, currentFile
		// Copy metadata for closure
		blockMeta := make(map[string]string)
		for k, v := range block.Metadata {
			blockMeta[k] = v
		}

		// Build page-level actions map (convert from parser types to config types)
		pageActions := h.getPageActions()

		factory := func() runtime.Store {
			var state *runtime.GenericState
			var err error

			if srcCfg.Type == "computed" {
				// Computed sources need a source lookup to find their parent
				state, err = runtime.NewGenericStateForComputed(srcName, srcCfg, rootDir, curFile, blockMeta, h.lookupSource)
			} else {
				state, err = runtime.NewGenericStateWithMetadata(srcName, srcCfg, rootDir, curFile, blockMeta)
			}
			if err != nil {
				log.Printf("[WS] Failed to create runtime state for %s: %v", srcName, err)
				return nil
			}

			// Configure page-level settings for custom actions
			if len(pageActions) > 0 {
				state.SetPageConfig(pageActions, h.lookupSource)
			}

			return state
		}

		h.stateFactories[blockID] = factory

		// Track source files for markdown sources (for live refresh)
		if sourceCfg.Type == "markdown" {
			var sourceFilePath string
			if sourceCfg.File != "" {
				// External file - resolve relative to root or current file
				if filepath.IsAbs(sourceCfg.File) {
					sourceFilePath = sourceCfg.File
				} else {
					// Try relative to current file first, then root
					if currentFile != "" {
						sourceFilePath = filepath.Join(filepath.Dir(currentFile), sourceCfg.File)
					} else {
						sourceFilePath = filepath.Join(h.rootDir, sourceCfg.File)
					}
				}
			} else {
				// Same-file source
				sourceFilePath = currentFile
			}
			if sourceFilePath != "" {
				// Make path relative to rootDir for consistent matching with watcher events
				if relPath, err := filepath.Rel(h.rootDir, sourceFilePath); err == nil {
					sourceFilePath = relPath
				}
				h.sourceFiles[blockID] = append(h.sourceFiles[blockID], sourceFilePath)
				if h.debug {
					log.Printf("[WS] Block %s tracks source file: %s", blockID, sourceFilePath)
				}
			}
		}

		if h.debug {
			log.Printf("[WS] Successfully initialized block: %s", blockID)
		}
	}

	// Debug: list all state factories
	if h.debug {
		var factoryIDs []string
		for id := range h.stateFactories {
			factoryIDs = append(factoryIDs, id)
		}
		log.Printf("[WS] State factories: %v", factoryIDs)
	}
}

// getEffectiveSource looks up a source by name.
//
// Precedence, highest first:
//
//  1. A site-level source named in the project's `generation.sources` approved set.
//     Approved definitions are *pinned*: a page cannot redefine them.
//  2. Page-level sources (from frontmatter).
//  3. Site-level sources (from tinkerdown.yaml).
//
// Tiers 2 and 3 are the long-standing rule and are unchanged. Tier 1 exists only when
// the project declares a `generation:` block, and it closes a hole that block would
// otherwise have: because frontmatter can declare its own sources, a generated page
// could reference an approved name while defining that name as something the operator
// never approved — passing any check that reasons about names alone. Pinning makes an
// approved name mean one thing regardless of what the page says.
func (h *WebSocketHandler) getEffectiveSource(name string) (config.SourceConfig, bool) {
	// Tier 1: an approved source is pinned to its manifest definition.
	if h.config.IsManifest() && h.config.ApprovedSource(name) {
		if src, ok := h.config.Sources[name]; ok {
			if h.page != nil && h.page.Config.Sources != nil {
				if _, shadowed := h.page.Config.Sources[name]; shadowed {
					// Surfaced rather than silently dropped: a generating agent that
					// tries this needs to see why its definition had no effect.
					log.Printf("[WS] source %q is approved in tinkerdown.yaml and cannot be "+
						"redefined by page frontmatter; using the approved definition", name)
				}
			}
			return src, true
		}
	}

	// Check page-level sources first (from frontmatter)
	if h.page != nil && h.page.Config.Sources != nil {
		if src, ok := h.page.Config.Sources[name]; ok {
			// Convert tinkerdown.SourceConfig to config.SourceConfig
			return config.SourceConfig{
				Type:        src.Type,
				Cmd:         src.Cmd,
				Query:       src.Query,
				From:        src.From,
				File:        src.File,
				Anchor:      src.Anchor,
				DB:          src.DB,
				Table:       src.Table,
				Path:        src.Path,
				QueryFile:   src.QueryFile,
				Variables:   src.Variables,
				Headers:     src.Headers,
				QueryParams: src.QueryParams,
				ResultPath:  src.ResultPath,
				Readonly:    src.Readonly,
				Options:     src.Options,
				Manual:      src.Manual,
				Format:      src.Format,
				Delimiter:   src.Delimiter,
				Env:         src.Env,
				Timeout:     src.Timeout,
				GroupBy:     src.GroupBy,
				Aggregate:   src.Aggregate,
				Filter:      src.Filter,
			}, true
		}
	}

	// Fall back to site-level sources (from tinkerdown.yaml)
	if h.config != nil && h.config.Sources != nil {
		if src, ok := h.config.Sources[name]; ok {
			return src, true
		}
	}

	return config.SourceConfig{}, false
}

// ServeHTTP handles WebSocket upgrade and message routing.
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create upgrader with origin validation from config
	var allowedOrigins []string
	if h.config != nil && h.config.API != nil {
		allowedOrigins = h.config.API.GetCORSOrigins()
	}
	wsUpgrader := newUpgrader(allowedOrigins)

	// Upgrade connection
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Failed to upgrade connection: %v", err)
		return
	}

	// Store connection in handler for source refresh
	h.conn = conn

	defer func() {
		// Unregister connection
		if h.server != nil {
			h.server.UnregisterConnection(conn)
		}
		conn.Close()
	}()

	// Register connection for reload broadcasts (with handler for source refresh)
	if h.server != nil {
		h.server.RegisterConnection(conn, h)
	}

	if h.debug {
		log.Printf("[WS] Client connected: %s", conn.RemoteAddr())
	}

	// Initialize instances for all interactive blocks
	h.initializeInstances(conn)

	// Handle messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] Unexpected close: %v", err)
			}
			break
		}

		if h.debug {
			log.Printf("[WS] Received: %s", message)
		}

		h.handleMessage(conn, message)
	}

	if h.debug {
		log.Printf("[WS] Client disconnected: %s", conn.RemoteAddr())
	}
}

// initializeInstances creates LiveTemplate instances for each interactive block.
func (h *WebSocketHandler) initializeInstances(conn *websocket.Conn) {
	// Collect block info under lock, then create instances outside lock.
	// Factory calls (especially for computed sources) may call lookupSource
	// which also acquires h.mu — so we must not hold the lock during factory calls.
	type blockInfo struct {
		blockID    string
		block      *tinkerdown.InteractiveBlock
		stateBlock *tinkerdown.ServerBlock
		factory    func() runtime.Store
	}
	var toInit []blockInfo

	h.mu.Lock()
	for blockID, block := range h.page.InteractiveBlocks {
		stateBlock, ok := h.page.ServerBlocks[block.StateRef]
		if !ok {
			log.Printf("[WS] Interactive block %s references unknown state %s", blockID, block.StateRef)
			continue
		}
		factory, ok := h.stateFactories[block.StateRef]
		if !ok {
			log.Printf("[WS] No compiled factory for state %s", block.StateRef)
			continue
		}
		toInit = append(toInit, blockInfo{blockID, block, stateBlock, factory})
	}
	h.mu.Unlock()

	// Create instances under lock, temporarily releasing it during each factory call
	// (computed source factories call lookupSource which also acquires h.mu).
	var instances []*BlockInstance
	func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		for _, bi := range toInit {
			blockID := bi.blockID
			block := bi.block
			stateBlock := bi.stateBlock

			// Create state instance outside the lock — factory calls may acquire h.mu
			// (e.g., computed sources call lookupSource which locks h.mu).
			// Use a helper to ensure the lock is always re-acquired, even on panic.
			state := func() runtime.Store {
				h.mu.Unlock()
				defer h.mu.Lock()
				return bi.factory()
			}()

			// Create template from inline content
			// Since livetemplate.New() requires template files, we use a workaround:
			// Write content to a temp file, parse it, then delete
			tmpFile := fmt.Sprintf("/tmp/lvt-%s.tmpl", blockID)
			if h.debug {
				log.Printf("[WS] Block %s template content:\n%s", blockID, block.Content)
			}
			if err := os.WriteFile(tmpFile, []byte(block.Content), 0644); err != nil {
				log.Printf("[WS] Failed to write temp template for block %s: %v", blockID, err)
				continue
			}
			defer os.Remove(tmpFile)

			tmpl, err := livetemplate.New(blockID,
				livetemplate.WithComponentTemplates(getComponentTemplates()...),
				livetemplate.WithParseFiles(tmpFile))
			if err != nil {
				log.Printf("[WS] Failed to create template for block %s: %v", blockID, err)
				continue
			}

			// Register component-specific template functions for tree generation
			// These are needed because WithComponentTemplates adds funcs to t.tmpl but not t.funcs
			tmpl.Funcs(getComponentFuncs())

			instance := &BlockInstance{
				blockID:  blockID,
				state:    state,
				template: tmpl,
				conn:     conn,
			}

			h.instances[blockID] = instance
			instances = append(instances, instance)

			if h.debug {
				log.Printf("[WS] Initialized block: %s (state ref: %s)", blockID, block.StateRef)
			}

			_ = stateBlock // Mark as used
		}
	}()

	// Send initial states (outside the h.mu lock)
	for _, instance := range instances {
		h.sendInitialState(instance)
	}

	// Evaluate and send expression updates once after all blocks are initialized
	h.evaluateAndSendExpressions(conn)
}

// sendInitialState sends the initial tree update to the client.
func (h *WebSocketHandler) sendInitialState(instance *BlockInstance) {
	// Get state data and render under instance lock
	var response MessageEnvelope
	func() {
		instance.mu.Lock()
		defer instance.mu.Unlock()

		// Get state data for rendering
		var stateData interface{}

		// Check if this is an RPC adapter with GetStateAsInterface method
		type StateGetter interface {
			GetStateAsInterface() (interface{}, error)
		}

		if getter, ok := instance.state.(StateGetter); ok {
			// RPC plugin - fetch state via RPC
			var err error
			stateData, err = getter.GetStateAsInterface()
			if err != nil {
				log.Printf("[WS] Failed to get state for %s: %v", instance.blockID, err)
				return
			}
			// Hydrate datatable structs so template methods work
			stateData = hydrateDataTableState(stateData)
			if h.debug {
				log.Printf("[WS] RPC state for %s: %+v (type: %T)", instance.blockID, stateData, stateData)
			}
		} else {
			// Regular in-process state
			stateData = instance.state
			if h.debug {
				log.Printf("[WS] Direct state for %s: %+v (type: %T)", instance.blockID, stateData, stateData)
			}
		}

		// Render tree update using ExecuteUpdates (follows LiveTemplate tree-update specification)
		var buf bytes.Buffer
		if err := instance.template.ExecuteUpdates(&buf, stateData); err != nil {
			log.Printf("[WS] Failed to render initial state for %s: %v", instance.blockID, err)
			return
		}

		// The buffer contains the tree JSON directly
		response = MessageEnvelope{
			BlockID:   instance.blockID,
			Action:    "tree",
			Data:      json.RawMessage(buf.Bytes()),
			ExecMeta:  extractExecMeta(stateData),
			CacheMeta: extractCacheMeta(stateData),
		}
	}()

	// Send tree update (outside the instance lock)
	if response.BlockID != "" {
		h.sendMessage(instance.conn, response)
	}
	// Note: Expression updates are sent by initializeInstances after all blocks are initialized
}

// handleMessage routes incoming messages to the appropriate block instance.
func (h *WebSocketHandler) handleMessage(conn *websocket.Conn, message []byte) {
	var envelope MessageEnvelope
	if err := json.Unmarshal(message, &envelope); err != nil {
		log.Printf("[WS] Failed to parse message: %v", err)
		return
	}

	h.mu.RLock()
	instance, ok := h.instances[envelope.BlockID]
	h.mu.RUnlock()

	if !ok {
		log.Printf("[WS] Unknown block ID: %s", envelope.BlockID)
		return
	}

	// Handle action
	if err := h.handleAction(instance, envelope.Action, envelope.Data); err != nil {
		log.Printf("[WS] Error handling action: %v", err)
		return
	}

	// Mark source files as recently written by this action, so the file
	// watcher knows to skip a full page reload for this change.
	if h.server != nil {
		h.mu.RLock()
		for _, sb := range h.page.ServerBlocks {
			if files, ok := h.sourceFiles[sb.ID]; ok {
				for _, f := range files {
					h.server.MarkSourceWrite(f)
				}
			}
		}
		h.mu.RUnlock()
	}

	// Re-render and send update
	h.sendUpdate(instance)

	// Refresh computed sources that depend on the modified source,
	// but only for write-mutating actions (add, delete, update, toggle).
	// Read-only actions (refresh, filter, edit, canceledit, sort, page) don't change data.
	actionLower := strings.ToLower(envelope.Action)
	if actionLower == "add" || actionLower == "delete" || actionLower == "update" || actionLower == "toggle" {
		h.refreshDependentComputedSources(instance, conn)
	}
}

// refreshDependentComputedSources finds computed source blocks whose parent
// matches the modified block's source, refreshes their data, and sends updates.
func (h *WebSocketHandler) refreshDependentComputedSources(modified *BlockInstance, conn *websocket.Conn) {
	// Get the source name of the modified block
	modifiedSource := ""
	h.mu.RLock()
	for _, sb := range h.page.ServerBlocks {
		if sb.Metadata["lvt-source"] != "" {
			// Find the server block that matches the modified instance
			for _, ib := range h.page.InteractiveBlocks {
				if ib.StateRef == sb.ID {
					if inst, ok := h.instances[ib.ID]; ok && inst == modified {
						modifiedSource = sb.Metadata["lvt-source"]
					}
				}
			}
		}
	}
	h.mu.RUnlock()

	if modifiedSource == "" {
		return
	}

	// Find and refresh computed sources that depend on this source
	h.mu.RLock()
	var toRefresh []*BlockInstance
	for _, sb := range h.page.ServerBlocks {
		srcName := sb.Metadata["lvt-source"]
		if srcName == "" {
			continue
		}
		// Check if this is a computed source with From == modifiedSource
		if src, ok := h.page.Config.Sources[srcName]; ok {
			if src.Type == "computed" && src.From == modifiedSource {
				// Find the instance for this block
				for _, ib := range h.page.InteractiveBlocks {
					if ib.StateRef == sb.ID {
						if inst, ok := h.instances[ib.ID]; ok {
							toRefresh = append(toRefresh, inst)
						}
					}
				}
			}
		}
	}
	h.mu.RUnlock()

	// Refresh each dependent computed source
	for _, inst := range toRefresh {
		// Trigger a Refresh action to re-fetch from parent
		func() {
			inst.mu.Lock()
			defer inst.mu.Unlock()
			if store, ok := inst.state.(*runtime.GenericState); ok {
				store.HandleAction("Refresh", nil)
			}
		}()
		h.sendUpdate(inst)
	}
}

// handleAction executes an action on the state.
func (h *WebSocketHandler) handleAction(instance *BlockInstance, action string, data json.RawMessage) error {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	// Parse data into map
	var dataMap map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &dataMap); err != nil {
			return fmt.Errorf("failed to parse action data: %w", err)
		}
	} else {
		dataMap = make(map[string]interface{})
	}

	// Execute action via HandleAction method
	if err := instance.state.HandleAction(action, dataMap); err != nil {
		return fmt.Errorf("action failed: %w", err)
	}

	if h.debug {
		log.Printf("[WS] Executed action %s on block %s", action, instance.blockID)
	}

	return nil
}

// sendUpdate re-renders the state and sends a tree update to the client.
func (h *WebSocketHandler) sendUpdate(instance *BlockInstance) {
	// Get state data and render under instance lock
	var response MessageEnvelope
	func() {
		instance.mu.Lock()
		defer instance.mu.Unlock()

		// Get state data for rendering
		var stateData interface{}

		// Check if this is an RPC adapter with GetStateAsInterface method
		type StateGetter interface {
			GetStateAsInterface() (interface{}, error)
		}

		if getter, ok := instance.state.(StateGetter); ok {
			// RPC plugin - fetch state via RPC
			var err error
			stateData, err = getter.GetStateAsInterface()
			if err != nil {
				log.Printf("[WS] Failed to get state for %s: %v", instance.blockID, err)
				return
			}
			// Hydrate datatable structs so template methods work
			stateData = hydrateDataTableState(stateData)
		} else {
			// Regular in-process state
			stateData = instance.state
		}

		// Render tree update using ExecuteUpdates (follows LiveTemplate tree-update specification)
		// ExecuteUpdates returns only changed dynamics after the first render
		var buf bytes.Buffer
		if err := instance.template.ExecuteUpdates(&buf, stateData); err != nil {
			log.Printf("[WS] Failed to render update for %s: %v", instance.blockID, err)
			return
		}

		// The buffer contains the tree JSON directly
		response = MessageEnvelope{
			BlockID:   instance.blockID,
			Action:    "tree",
			Data:      json.RawMessage(buf.Bytes()),
			ExecMeta:  extractExecMeta(stateData),
			CacheMeta: extractCacheMeta(stateData),
		}
	}()

	// Send tree update (outside the instance lock)
	if response.BlockID != "" {
		h.sendMessage(instance.conn, response)
	}

	// Evaluate and send expression updates (outside the instance lock)
	h.evaluateAndSendExpressions(instance.conn)
}

// sendMessage sends a message envelope over WebSocket.
// Uses writeMu to serialize concurrent writes (e.g., watcher refresh + action response).
func (h *WebSocketHandler) sendMessage(conn *websocket.Conn, envelope MessageEnvelope) {
	data, err := json.Marshal(envelope)
	if err != nil {
		log.Printf("[WS] Failed to marshal response: %v", err)
		return
	}

	h.writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	h.writeMu.Unlock()

	if err != nil {
		log.Printf("[WS] Failed to send message: %v", err)
		return
	}

	if h.debug {
		log.Printf("[WS] Sent: %s", data)
	}
}

// evaluateAndSendExpressions evaluates all page expressions and sends updates to the client.
// This should be called after any block state update.
func (h *WebSocketHandler) evaluateAndSendExpressions(conn *websocket.Conn) {
	if h.page.Expressions == nil || len(h.page.Expressions) == 0 {
		if h.debug {
			log.Printf("[WS] No expressions to evaluate (expressions=%v)", h.page.Expressions)
		}
		return
	}

	if h.debug {
		log.Printf("[WS] Evaluating %d expressions: %v", len(h.page.Expressions), h.page.Expressions)
	}

	// Build evaluation context from all block instances
	ctx := h.buildEvalContext()

	if h.debug {
		log.Printf("[WS] Eval context sources: %v", getSourceNames(ctx))
	}

	// Evaluate all expressions
	results := runtime.EvaluateExpressions(h.page.Expressions, ctx)

	if h.debug {
		log.Printf("[WS] Expression evaluation returned %d results", len(results))
		for id, result := range results {
			if result.Error != "" {
				log.Printf("[WS] Expression %s error: %s", id, result.Error)
			} else {
				log.Printf("[WS] Expression %s = %v", id, result.Value)
			}
		}
	}

	// Build the expression values map
	exprValues := make(map[string]interface{})
	for id, result := range results {
		if result.Error != "" {
			exprValues[id] = map[string]interface{}{
				"error": result.Error,
			}
		} else {
			exprValues[id] = result.Value
		}
	}

	// Send expression update message
	exprData, err := json.Marshal(exprValues)
	if err != nil {
		log.Printf("[WS] Failed to marshal expression results: %v", err)
		return
	}

	if h.debug {
		log.Printf("[WS] Sending expression update: %s", string(exprData))
	}

	response := MessageEnvelope{
		BlockID: ExpressionsBlockID, // Special block ID for expressions
		Action:  "expr-update",
		Data:    json.RawMessage(exprData),
	}

	h.sendMessage(conn, response)

	if h.debug {
		log.Printf("[WS] Expression update sent successfully")
	}
}

// buildEvalContext builds an evaluation context from all block instances.
// It collects Data from each block's state and maps it by source name.
func (h *WebSocketHandler) buildEvalContext() *runtime.EvalContext {
	ctx := runtime.NewEvalContext()

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Iterate through interactive blocks (which have instances) and get source names
	// from the server blocks they reference
	for blockID, interactiveBlock := range h.page.InteractiveBlocks {
		// Get the instance for this interactive block
		instance, ok := h.instances[blockID]
		if !ok {
			continue
		}

		// Get the server block this interactive block references
		serverBlock, ok := h.page.ServerBlocks[interactiveBlock.StateRef]
		if !ok {
			continue
		}

		// Get state data
		var stateData interface{}
		type StateGetter interface {
			GetStateAsInterface() (interface{}, error)
		}

		instance.mu.Lock()
		if getter, ok := instance.state.(StateGetter); ok {
			var err error
			stateData, err = getter.GetStateAsInterface()
			if err != nil {
				instance.mu.Unlock()
				continue
			}
		} else {
			stateData = instance.state
		}
		instance.mu.Unlock()

		// Extract source name from server block metadata
		sourceName := ""
		if serverBlock.Metadata != nil {
			if sn, ok := serverBlock.Metadata["lvt-source"]; ok {
				sourceName = sn
			}
		}

		// If no source name in metadata, use the StateRef as fallback
		if sourceName == "" {
			sourceName = interactiveBlock.StateRef
		}

		// Extract Data array from state
		if stateMap, ok := stateData.(map[string]interface{}); ok {
			// Try lowercase "data" first, then titlecase "Data"
			// Handle both []interface{} (from JSON unmarshal) and []map[string]interface{} (from GetFilteredData)
			if data, ok := stateMap["data"].([]interface{}); ok {
				ctx.Sources[sourceName] = convertToMapSlice(data)
			} else if data, ok := stateMap["Data"].([]interface{}); ok {
				ctx.Sources[sourceName] = convertToMapSlice(data)
			} else if data, ok := stateMap["data"].([]map[string]interface{}); ok {
				// Direct slice of maps (e.g., from GetFilteredData after Filter action)
				ctx.Sources[sourceName] = data
			} else if data, ok := stateMap["Data"].([]map[string]interface{}); ok {
				ctx.Sources[sourceName] = data
			}
		}
	}

	return ctx
}

// convertToMapSlice converts a []interface{} to []map[string]interface{}
func convertToMapSlice(data []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

// getSourceNames returns the names of sources in an eval context (for debugging)
func getSourceNames(ctx *runtime.EvalContext) []string {
	names := make([]string, 0, len(ctx.Sources))
	for name := range ctx.Sources {
		names = append(names, name)
	}
	return names
}

// extractExecMeta extracts exec state metadata from a state object.
// Returns nil if the state doesn't contain exec metadata fields (Status).
func extractExecMeta(stateData interface{}) *ExecMeta {
	stateMap, ok := stateData.(map[string]interface{})
	if !ok {
		return nil
	}

	// Check if this state has exec metadata by looking for Status field
	status, hasStatus := stateMap["Status"].(string)
	if !hasStatus {
		return nil
	}

	meta := &ExecMeta{
		Status: status,
	}

	// Extract other optional fields
	if duration, ok := stateMap["Duration"].(int); ok {
		meta.Duration = int64(duration)
	} else if duration, ok := stateMap["Duration"].(int64); ok {
		meta.Duration = duration
	} else if duration, ok := stateMap["Duration"].(float64); ok {
		meta.Duration = int64(duration)
	}

	if output, ok := stateMap["Output"].(string); ok {
		meta.Output = output
	}

	if stderr, ok := stateMap["Stderr"].(string); ok {
		meta.Stderr = stderr
	}

	if command, ok := stateMap["Command"].(string); ok {
		meta.Command = command
	}

	return meta
}

// extractCacheMeta extracts cache state metadata from a state object.
// Returns nil if the state doesn't contain cache metadata (CacheInfo field).
func extractCacheMeta(stateData interface{}) *CacheMeta {
	stateMap, ok := stateData.(map[string]interface{})
	if !ok {
		return nil
	}

	// Check if this state has cache metadata by looking for CacheInfo or cache_info field
	var cacheInfo map[string]interface{}
	if ci, ok := stateMap["CacheInfo"].(map[string]interface{}); ok {
		cacheInfo = ci
	} else if ci, ok := stateMap["cache_info"].(map[string]interface{}); ok {
		cacheInfo = ci
	}

	if cacheInfo == nil {
		return nil
	}

	meta := &CacheMeta{}

	if cached, ok := cacheInfo["cached"].(bool); ok {
		meta.Cached = cached
	}

	if age, ok := cacheInfo["age"].(string); ok {
		meta.Age = age
	}

	if expiresIn, ok := cacheInfo["expires_in"].(string); ok {
		meta.ExpiresIn = expiresIn
	}

	if stale, ok := cacheInfo["stale"].(bool); ok {
		meta.Stale = stale
	}

	if refreshing, ok := cacheInfo["refreshing"].(bool); ok {
		meta.Refreshing = refreshing
	}

	return meta
}

// Note: State types are now dynamically compiled from server blocks
// No need for hardcoded placeholder states

// hydrateDataTableState converts JSON map data back to typed datatable structs.
// When state is transmitted via RPC as JSON, structs become maps and lose their methods.
// This function reconstructs datatable.DataTable so template methods work correctly.
func hydrateDataTableState(stateData interface{}) interface{} {
	stateMap, ok := stateData.(map[string]interface{})
	if !ok {
		return stateData
	}

	// Check for Table field with datatable data (used by lvt-source datatables)
	// Try lowercase first (json:"table"), then uppercase (json:"Table")
	for _, key := range []string{"table", "Table"} {
		if tableData, ok := stateMap[key].(map[string]interface{}); ok {
			if hydratedTable := hydrateDataTable(tableData); hydratedTable != nil {
				stateMap[key] = hydratedTable
			}
			break
		}
	}

	return stateData
}

// hydrateDataTable converts a map[string]interface{} to a *datatable.DataTable.
func hydrateDataTable(tableData map[string]interface{}) *datatable.DataTable {
	// Re-serialize and deserialize to get proper types
	tableJSON, err := json.Marshal(tableData)
	if err != nil {
		log.Printf("[WS] hydrateDataTable: marshal error: %v", err)
		return nil
	}

	var dt datatable.DataTable
	if err := json.Unmarshal(tableJSON, &dt); err != nil {
		log.Printf("[WS] hydrateDataTable: unmarshal error: %v", err)
		return nil
	}

	log.Printf("[WS] hydrateDataTable: success, ID=%s, Columns=%d, Rows=%d",
		dt.ID(), len(dt.Columns), len(dt.Rows))

	return &dt
}

// convertTemplateSet converts a base.TemplateSet to a livetemplate.TemplateSet.
// This is needed because the components library uses base.TemplateSet while
// livetemplate.WithComponentTemplates expects livetemplate.TemplateSet.
// The types are structurally identical so we just copy the fields.
func convertTemplateSet(bs *base.TemplateSet) *livetemplate.TemplateSet {
	return &livetemplate.TemplateSet{
		FS:        bs.FS,
		Pattern:   bs.Pattern,
		Namespace: bs.Namespace,
		Funcs:     bs.Funcs,
	}
}

// getComponentTemplates returns livetemplate.TemplateSet versions of all component templates
func getComponentTemplates() []*livetemplate.TemplateSet {
	return []*livetemplate.TemplateSet{
		convertTemplateSet(datatable.Templates()),
	}
}

// getComponentFuncs returns all component-specific template functions
// These need to be registered with tmpl.Funcs() for tree generation to work
func getComponentFuncs() template.FuncMap {
	funcs := template.FuncMap{
		// Standard math functions used by components
		"mod": func(a, b int) int {
			return a % b
		},
	}
	// Merge all component funcs
	for _, set := range getComponentTemplates() {
		for name, fn := range set.Funcs {
			funcs[name] = fn
		}
	}
	return funcs
}

// RefreshSourcesForFile refreshes all sources that use the given file.
// This is called by the server when a markdown source file changes externally.
func (h *WebSocketHandler) RefreshSourcesForFile(filePath string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.debug {
		log.Printf("[WS] RefreshSourcesForFile called for: %s", filePath)
	}

	// Find all server blocks that use this file
	// Note: sourceFiles uses server block IDs (e.g., auto-persist-lvt-0)
	// but instances uses interactive block IDs (e.g., lvt-0)
	// We need to find the instance by matching StateRef
	for serverBlockID, files := range h.sourceFiles {
		for _, sourceFile := range files {
			if sourceFile == filePath {
				if h.debug {
					log.Printf("[WS] Server block %s uses file %s, looking for matching instance", serverBlockID, filePath)
				}

				// Find the instance whose StateRef matches this server block ID
				var instance *BlockInstance
				for _, block := range h.page.InteractiveBlocks {
					if block.StateRef == serverBlockID {
						instance = h.instances[block.ID]
						if h.debug && instance != nil {
							log.Printf("[WS] Found instance %s for server block %s", block.ID, serverBlockID)
						}
						break
					}
				}

				if instance == nil {
					log.Printf("[WS] No instance found for server block %s", serverBlockID)
					continue
				}

				// Trigger a Refresh action on the source
				// This re-fetches data from the markdown file
				if err := h.handleAction(instance, "Refresh", nil); err != nil {
					log.Printf("[WS] Failed to refresh block %s: %v", instance.blockID, err)
					continue
				}

				// Send the updated state to the client
				h.sendUpdate(instance)

				if h.debug {
					log.Printf("[WS] Successfully refreshed block %s", instance.blockID)
				}
				break // File matched, no need to check other files for this block
			}
		}
	}
}

// TracksSourceFile returns true if any source in this handler's sourceFiles map
// matches the given file path. Used to detect same-file sources (auto-tasks).
func (h *WebSocketHandler) TracksSourceFile(filePath string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, files := range h.sourceFiles {
		for _, f := range files {
			if f == filePath {
				return true
			}
		}
	}
	return false
}

// getPageActions builds the action set a page may invoke, converting page-level
// actions from parser types to config types. Returns nil if the page can invoke none.
//
// Precedence mirrors getEffectiveSource:
//
//  1. Site-level actions named in `generation.actions` — pinned, so a page cannot
//     redefine an approved action to do something else.
//  2. Page-level actions (from frontmatter).
//
// Tier 1 is also what makes approved actions *reachable*. Actions have never had the
// site-config fallback sources have: a page could only invoke actions it declared in
// its own frontmatter. That left `generation.actions` inert — it named a surface a
// generated page had no way to use, forcing it to declare those actions itself, which
// is precisely what approval exists to prevent.
//
// The fallback is deliberately limited to *approved* actions rather than every action
// in tinkerdown.yaml. Exposing all of them to every page would make actions written for
// schedules or webhooks callable from any page — a broad change nobody asked for.
// Projects with no generation block are unaffected in either respect.
func (h *WebSocketHandler) getPageActions() map[string]*config.Action {
	approved := h.approvedSiteActions()
	if (h.page == nil || h.page.Config.Actions == nil) && len(approved) == 0 {
		return nil
	}
	if h.page == nil || h.page.Config.Actions == nil {
		return approved
	}

	result := make(map[string]*config.Action)
	for name, action := range h.page.Config.Actions {
		// Convert parser ParamDef to config ParamDef
		params := make(map[string]config.ParamDef)
		for pname, pdef := range action.Params {
			params[pname] = config.ParamDef{
				Type:     pdef.Type,
				Required: pdef.Required,
				Default:  pdef.Default,
			}
		}

		result[name] = &config.Action{
			Kind:      action.Kind,
			Source:    action.Source,
			Statement: action.Statement,
			URL:       action.URL,
			Method:    action.Method,
			Body:      action.Body,
			Cmd:       action.Cmd,
			Params:    params,
			Confirm:   action.Confirm,
		}
	}

	// Tier 1 last so it wins: an approved action is pinned to its manifest
	// definition, overwriting any same-named page action. Logged rather than
	// silently dropped so a generating agent sees why its definition had no effect.
	for name, action := range approved {
		if _, shadowed := result[name]; shadowed {
			log.Printf("[WS] action %q is approved in tinkerdown.yaml and cannot be "+
				"redefined by page frontmatter; using the approved definition", name)
		}
		result[name] = action
	}
	return result
}

// approvedSiteActions returns the site-level actions this project has approved for
// generated apps, or nil when the project declares no generation block.
//
// An approved name that the project never defined is skipped rather than synthesised —
// the same rule getEffectiveSource follows, so approving a typo yields "unknown action"
// rather than a confusing partial object.
func (h *WebSocketHandler) approvedSiteActions() map[string]*config.Action {
	if !h.config.IsManifest() || h.config.Actions == nil {
		return nil
	}
	var result map[string]*config.Action
	for _, name := range h.config.Generation.Actions {
		action, ok := h.config.Actions[name]
		if !ok || action == nil {
			continue
		}
		if result == nil {
			result = make(map[string]*config.Action)
		}
		result[name] = action
	}
	return result
}

// lookupSource looks up a source by name for custom SQL actions.
// It checks page-level sources first, then site-level sources.
// Sources are cached for reuse and closed when the handler is closed.
func (h *WebSocketHandler) lookupSource(name string) (source.Source, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check cache first
	if src, exists := h.actionSources[name]; exists {
		return src, true
	}

	// Get source config (checks page then site level)
	srcCfg, found := h.getEffectiveSource(name)
	if !found {
		return nil, false
	}

	// Create and cache the source
	currentFile := ""
	if h.page != nil {
		currentFile = h.page.SourceFile
	}

	src, err := createSourceForAction(name, srcCfg, h.rootDir, currentFile)
	if err != nil {
		log.Printf("[WS] Failed to create source %s for action: %v", name, err)
		return nil, false
	}

	h.actionSources[name] = src
	return src, true
}

// createSourceForAction creates a source instance for use in custom actions.
// This is similar to runtime.createSource but imported here for action execution.
func createSourceForAction(name string, cfg config.SourceConfig, siteDir, currentFile string) (source.Source, error) {
	switch cfg.Type {
	case "sqlite":
		return source.NewSQLiteSource(name, cfg.DB, cfg.Table, siteDir, cfg.IsReadonly())
	case "pg":
		return source.NewPostgresSource(name, cfg.Query, cfg.Options)
	case "json":
		return source.NewJSONFileSource(name, cfg.File, siteDir)
	case "csv":
		return source.NewCSVFileSource(name, cfg.File, siteDir, cfg.Options)
	case "markdown":
		return source.NewMarkdownSource(name, cfg.File, cfg.Anchor, siteDir, currentFile, cfg.IsReadonly())
	case "rest":
		return source.NewRestSourceWithConfig(name, cfg)
	case "exec":
		if !config.IsExecAllowed() {
			return nil, fmt.Errorf("exec sources disabled (use --allow-exec)")
		}
		return source.NewExecSourceWithConfig(name, cfg, siteDir)
	default:
		return nil, fmt.Errorf("unsupported source type %q", cfg.Type)
	}
}
