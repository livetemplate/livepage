// Package tinkerdown provides the core library for building interactive documentation
// with markdown files and embedded executable code blocks.
package tinkerdown

import "github.com/livetemplate/tinkerdown/internal/schedule"

// Page represents a parsed tinkerdown tutorial/guide/playground.
type Page struct {
	ID                string
	Title             string
	Description       string // From frontmatter; used for <meta description> + og:description
	Image             string // From frontmatter; used for og:image
	Type              string // tutorial, guide, reference, playground
	SourceFile        string // Absolute path to source .md file (for error messages)
	SourceRepo        string // Origin GitHub repo URL from frontmatter (for edit link)
	SourcePath        string // Path within origin repo from frontmatter (for edit link)
	Sidebar           *bool  // nil = use default, true/false = explicit override
	Config            PageConfig
	StaticHTML        string
	ServerBlocks      map[string]*ServerBlock
	WasmBlocks        map[string]*WasmBlock
	InteractiveBlocks map[string]*InteractiveBlock
	// Expressions maps expression IDs to expression strings (e.g., "expr-0" -> "count(tasks where done)")
	// These are computed expressions found in inline code spans like `=count(tasks where done)`
	Expressions map[string]string

	// Schedules contains parsed schedule tokens found in the markdown content
	Schedules []*schedule.Token

	// Imperatives contains parsed imperative commands (Notify, Run action) from the markdown
	Imperatives []*schedule.Imperative

	// ScheduleWarnings contains any warnings generated during schedule parsing
	ScheduleWarnings []schedule.ParseWarning

	// HasCharts indicates the page has chart annotations requiring Chart.js
	HasCharts bool

	// HasMermaid indicates the page contains ```mermaid fenced blocks
	// requiring the Mermaid runtime (~3.3MB). Used by the server to
	// conditionally inject the script and avoid penalising pages without
	// any diagrams.
	HasMermaid bool
}

// PageConfig contains configuration for a page.
type PageConfig struct {
	// Page behavior
	Persist   PersistMode
	MultiStep bool
	StepCount int

	// Effective config (merged from frontmatter + site config)
	Sources  map[string]SourceConfig
	Actions  map[string]Action
	Styling  StylingConfig
	Blocks   BlocksConfig
	Features FeaturesConfig
}

// PersistMode determines how tutorial state is persisted.
type PersistMode string

const (
	PersistNone         PersistMode = "none"
	PersistLocalStorage PersistMode = "localstorage"
	PersistServer       PersistMode = "server"
)

// New creates a new Page with the given ID.
func New(id string) *Page {
	return &Page{
		ID:                id,
		ServerBlocks:      make(map[string]*ServerBlock),
		WasmBlocks:        make(map[string]*WasmBlock),
		InteractiveBlocks: make(map[string]*InteractiveBlock),
		Config: PageConfig{
			Persist: PersistLocalStorage,
		},
	}
}
