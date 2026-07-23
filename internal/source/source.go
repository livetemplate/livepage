// Package source provides data source implementations for lvt-source blocks.
// Sources fetch data from external systems (exec, databases, APIs) and make
// it available to templates.
package source

import (
	"context"
	"fmt"

	"github.com/livetemplate/tinkerdown/internal/cache"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/wasm"
)

// Source is the interface for data providers.
// Implementations fetch data from various backends (exec, pg, rest, etc.)
type Source interface {
	// Name returns the source identifier
	Name() string

	// Fetch retrieves data from the source.
	// Returns a slice of maps suitable for template iteration.
	Fetch(ctx context.Context) ([]map[string]interface{}, error)

	// Close releases any resources held by the source
	Close() error
}

// WritableSource extends Source with write capability.
// Sources like markdown and sqlite implement this to support Add, Update, Delete actions.
type WritableSource interface {
	Source

	// WriteItem performs a write operation on the source.
	// action is one of: "add", "toggle", "delete", "update"
	// data contains the item data (e.g., form fields, id for delete)
	WriteItem(ctx context.Context, action string, data map[string]interface{}) error

	// IsReadonly returns whether the source is in read-only mode
	IsReadonly() bool
}

// ColumnInfo describes a column in a data source's schema.
type ColumnInfo struct {
	Name     string // Column name (e.g., "amount")
	Type     string // Normalized type: "text", "integer", "real", "boolean", "date", "datetime"
	Required bool   // Whether the column is NOT NULL
}

// SchemaProvider is an optional interface for sources that can expose their schema.
// This is used by auto-tables to generate form inputs with appropriate types
// (e.g., <input type="number"> for integer columns).
type SchemaProvider interface {
	// Schema returns column info for the source's data.
	// Columns like "id" and "created_at" (internal) should be excluded.
	Schema(ctx context.Context) ([]ColumnInfo, error)
}

// SQLStatement is one parameterized statement in a transactional batch.
// Query uses positional placeholders (?), already substituted from :name form.
type SQLStatement struct {
	Query string
	Args  []interface{}
}

// SQLExecutor extends Source with ability to execute arbitrary SQL statements.
// This is used by custom actions defined in frontmatter (action kind: "sql").
type SQLExecutor interface {
	Source

	// Exec executes a SQL statement with the given arguments.
	// Returns the number of rows affected.
	Exec(ctx context.Context, query string, args ...interface{}) (int64, error)

	// ExecTx runs stmts in a single transaction: either every statement
	// commits, or the first error rolls all of them back. It exists so one
	// action can change state and append its audit record atomically — a
	// partial success (state changed, audit missing) would defeat the durable
	// trail that makes an approval workflow auditable.
	ExecTx(ctx context.Context, stmts []SQLStatement) error
}

// Registry holds configured sources for a site
type Registry struct {
	sources map[string]Source
	cache   cache.Cache
	cfg     *config.Config
}

// NewRegistry creates a source registry from config
func NewRegistry(cfg *config.Config, siteDir string) (*Registry, error) {
	return NewRegistryWithFile(cfg, siteDir, "")
}

// NewRegistryWithFile creates a source registry with knowledge of the current markdown file
// This is needed for markdown sources that reference anchors in the current file
func NewRegistryWithFile(cfg *config.Config, siteDir, currentFile string) (*Registry, error) {
	memCache := cache.NewMemoryCache()
	r := &Registry{
		sources: make(map[string]Source),
		cache:   memCache,
		cfg:     cfg,
	}

	if cfg.Sources == nil {
		return r, nil
	}

	// Two-pass initialization: regular sources first, computed sources second.
	// Computed sources reference other sources by name and need them to exist.
	var computedSources []struct {
		name string
		cfg  config.SourceConfig
	}

	for name, srcCfg := range cfg.Sources {
		if srcCfg.Type == "computed" {
			computedSources = append(computedSources, struct {
				name string
				cfg  config.SourceConfig
			}{name, srcCfg})
			continue
		}

		src, err := createSource(name, srcCfg, siteDir, currentFile)
		if err != nil {
			memCache.Stop()
			return nil, err
		}

		// Wrap with caching if enabled
		if srcCfg.IsCacheEnabled() {
			if ws, ok := src.(WritableSource); ok {
				src = NewCachedWritableSource(ws, r.cache, srcCfg)
			} else {
				src = NewCachedSource(src, r.cache, srcCfg)
			}
		}

		r.sources[name] = src
	}

	// Second pass: computed sources (can reference sources from first pass)
	// Note: chained computed sources (computed → computed) are not supported.
	for _, cs := range computedSources {
		// Validate that the parent is not itself a computed source (chaining not supported)
		if parentCfg, exists := cfg.Sources[cs.cfg.From]; exists && parentCfg.Type == "computed" {
			memCache.Stop()
			return nil, fmt.Errorf("computed source %q: chaining computed sources is not supported (parent %q is also computed)", cs.name, cs.cfg.From)
		}

		computedSrc, err := NewComputedSource(cs.name, cs.cfg, r)
		if err != nil {
			memCache.Stop()
			return nil, err
		}

		// Wrap with caching if enabled
		if cs.cfg.IsCacheEnabled() {
			var wrapped Source = NewCachedSource(computedSrc, r.cache, cs.cfg)
			r.sources[cs.name] = wrapped
		} else {
			r.sources[cs.name] = computedSrc
		}
	}

	return r, nil
}

// NewEmptyRegistry creates an empty registry (for testing or temporary use).
func NewEmptyRegistry() *Registry {
	return &Registry{
		sources: make(map[string]Source),
	}
}

// Get returns a source by name
func (r *Registry) Get(name string) (Source, bool) {
	src, ok := r.sources[name]
	return src, ok
}

// Set registers a source by name. Used for building registries incrementally.
func (r *Registry) Set(name string, src Source) {
	r.sources[name] = src
}

// Close releases all sources and stops the cache
func (r *Registry) Close() error {
	// Stop the cache cleanup goroutine
	if mc, ok := r.cache.(*cache.MemoryCache); ok {
		mc.Stop()
	}

	for _, src := range r.sources {
		if err := src.Close(); err != nil {
			return err
		}
	}
	return nil
}

// InvalidateCache invalidates the cache for a specific source
func (r *Registry) InvalidateCache(name string) {
	src, ok := r.sources[name]
	if !ok {
		// Source not found; nothing to invalidate
		return
	}

	if cs, ok := src.(*CachedSource); ok {
		cs.Invalidate()
	} else if cws, ok := src.(*CachedWritableSource); ok {
		cws.Invalidate()
	}
}

// InvalidateAllCaches invalidates all cached data
func (r *Registry) InvalidateAllCaches() {
	r.cache.InvalidateAll()
}

// createSource instantiates a source based on config type
func createSource(name string, cfg config.SourceConfig, siteDir, currentFile string) (Source, error) {
	switch cfg.Type {
	case "exec":
		if !config.IsExecAllowed() {
			return nil, &ValidationError{
				Source: name,
				Field:  "type",
				Reason: "exec sources are disabled by default for security. Use --allow-exec flag to enable.",
			}
		}
		return NewExecSourceWithConfig(name, cfg, siteDir)
	case "pg":
		return NewPostgresSourceWithConfig(name, cfg.Query, cfg.Options, cfg)
	case "rest":
		return NewRestSourceWithConfig(name, cfg)
	case "json":
		return NewJSONFileSource(name, cfg.File, siteDir)
	case "csv":
		return NewCSVFileSource(name, cfg.File, siteDir, cfg.Options)
	case "markdown":
		// Use IsReadonly() which defaults to true if not specified
		return NewMarkdownSource(name, cfg.File, cfg.Anchor, siteDir, currentFile, cfg.IsReadonly())
	case "sqlite":
		return NewSQLiteSource(name, cfg.DB, cfg.Table, siteDir, cfg.IsReadonly())
	case "wasm":
		return wasm.NewWasmSource(name, cfg.Path, siteDir, cfg.Options)
	case "graphql":
		return NewGraphQLSource(name, cfg, siteDir)
	default:
		return nil, &UnsupportedSourceError{Type: cfg.Type}
	}
}

// UnsupportedSourceError is returned for unknown source types
type UnsupportedSourceError struct {
	Type string
}

func (e *UnsupportedSourceError) Error() string {
	return "unsupported source type: " + e.Type
}
