// Package assets embeds the client JavaScript, CSS, and vendor libraries
package assets

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed client/*
var clientFS embed.FS

//go:embed vendor/prism/*
var prismFS embed.FS

//go:embed vendor/mermaid/*
var mermaidFS embed.FS

//go:embed vendor/chartjs/*
var chartjsFS embed.FS

//go:embed vendor/pico/*
var picoFS embed.FS

// ClientFS returns the embedded client files
func ClientFS() fs.FS {
	sub, err := fs.Sub(clientFS, "client")
	if err != nil {
		panic(err)
	}
	return sub
}

// GetClientJS returns the browser JavaScript bundle
func GetClientJS() ([]byte, error) {
	return clientFS.ReadFile("client/tinkerdown-client.browser.js")
}

// GetClientCSS returns the browser CSS bundle
func GetClientCSS() ([]byte, error) {
	return clientFS.ReadFile("client/tinkerdown-client.browser.css")
}

// GetPrismJS returns the Prism.js core library
func GetPrismJS() ([]byte, error) {
	return prismFS.ReadFile("vendor/prism/prism.min.js")
}

// GetPrismCSS returns the vendored light Prism.js theme CSS.
func GetPrismCSS() ([]byte, error) {
	return prismFS.ReadFile("vendor/prism/prism-light.css")
}

// GetPrismLineHighlightJS returns the Prism Line Highlight plugin JS,
// vendored from prismjs@1.29.0. Used by `LANG include="..." highlight="3-5"`
// fences to draw an emphasis overlay on selected lines.
func GetPrismLineHighlightJS() ([]byte, error) {
	return prismFS.ReadFile("vendor/prism/prism-line-highlight.min.js")
}

// GetPrismLineHighlightCSS returns the Prism Line Highlight plugin CSS.
func GetPrismLineHighlightCSS() ([]byte, error) {
	return prismFS.ReadFile("vendor/prism/prism-line-highlight.min.css")
}

// GetPrismLineNumbersJS returns the Prism Line Numbers plugin JS, which
// numbers lines in the gutter — used by every `LANG include="..."`
// block so the snippet has stable coordinates the footer label
// (`counter.go:13-35`) and the optional `highlight=` overlay can refer
// to.
func GetPrismLineNumbersJS() ([]byte, error) {
	return prismFS.ReadFile("vendor/prism/prism-line-numbers.min.js")
}

// GetPrismLineNumbersCSS returns the Prism Line Numbers plugin CSS.
func GetPrismLineNumbersCSS() ([]byte, error) {
	return prismFS.ReadFile("vendor/prism/prism-line-numbers.min.css")
}

// SupportedPrismLanguages is the whitelist of available Prism language components
var SupportedPrismLanguages = map[string]bool{
	"go":         true,
	"javascript": true,
	"jsx":        true,
	"markup":     true,
	"css":        true,
	"yaml":       true,
	"json":       true,
	"bash":       true,
	"markdown":   true,
}

// GetPrismLanguage returns a Prism language component.
// The language must be in the SupportedPrismLanguages whitelist.
func GetPrismLanguage(lang string) ([]byte, error) {
	if !SupportedPrismLanguages[lang] {
		return nil, fmt.Errorf("unsupported prism language: %q", lang)
	}
	return prismFS.ReadFile("vendor/prism/prism-" + lang + ".min.js")
}

// GetMermaidJS returns the Mermaid.js library
func GetMermaidJS() ([]byte, error) {
	return mermaidFS.ReadFile("vendor/mermaid/mermaid.min.js")
}

// GetChartJS returns the Chart.js library
func GetChartJS() ([]byte, error) {
	return chartjsFS.ReadFile("vendor/chartjs/chart.umd.min.js")
}

// GetPicoCSS returns the Pico CSS framework
func GetPicoCSS() ([]byte, error) {
	return picoFS.ReadFile("vendor/pico/pico.min.css")
}
