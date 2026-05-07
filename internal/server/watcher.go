package server

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches for file changes and triggers reload.
type Watcher struct {
	watcher  *fsnotify.Watcher
	rootDir  string
	onReload func(filePath string) error
	done     chan bool
	debug    bool

	// extra is the set of absolute paths to non-.md files that should
	// also trigger a reload when changed — typically files referenced
	// by ` ```LANG include="..." ` fences. Mutable across re-discovery
	// so the watcher can pick up new includes without restarting.
	extra   map[string]struct{}
	extraMu sync.RWMutex
}

// NewWatcher creates a new file watcher for the given directory.
func NewWatcher(rootDir string, onReload func(string) error, debug bool) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher:  fsWatcher,
		rootDir:  rootDir,
		onReload: onReload,
		done:     make(chan bool),
		debug:    debug,
	}

	// Add root directory
	if err := w.addDirectoryRecursive(rootDir); err != nil {
		fsWatcher.Close()
		return nil, err
	}

	return w, nil
}

// addDirectoryRecursive adds a directory and all its subdirectories to the watcher.
func (w *Watcher) addDirectoryRecursive(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories starting with . (hidden dirs like .git)
		// Note: We DO watch directories starting with _ (like _data/) because
		// they may contain external data files referenced by sources
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}

			if err := w.watcher.Add(path); err != nil {
				return err
			}

			if w.debug {
				log.Printf("[Watch] Added directory: %s", path)
			}
		}

		return nil
	})
}

// Start begins watching for file changes.
func (w *Watcher) Start() {
	go func() {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}

				// Respond to write/create events for .md files OR for
				// any file in the explicitly-tracked extra set (typically
				// files included by `LANG include="..."` fences).
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					relPath, err := filepath.Rel(w.rootDir, event.Name)
					if err != nil {
						relPath = event.Name
					}

					interesting := filepath.Ext(event.Name) == ".md"
					if !interesting {
						if absPath, absErr := filepath.Abs(event.Name); absErr == nil {
							w.extraMu.RLock()
							_, interesting = w.extra[absPath]
							w.extraMu.RUnlock()
						}
					}
					if !interesting {
						continue
					}

					if w.debug {
						log.Printf("[Watch] File changed: %s", relPath)
					}

					if err := w.onReload(relPath); err != nil {
						log.Printf("[Watch] Reload failed for %s: %v", relPath, err)
					}
				}

			case err, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[Watch] Error: %v", err)

			case <-w.done:
				return
			}
		}
	}()
}

// Stop stops the watcher.
func (w *Watcher) Stop() error {
	close(w.done)
	return w.watcher.Close()
}

// SetExtraFiles records the absolute paths of non-`.md` files whose
// changes should also trigger reload. The Server calls this after each
// Discover() with the union of every page's IncludedFiles, so docs
// stay in sync with the real source they cite. Each call replaces the
// previous set (idempotent across re-discovery).
//
// The watcher relies on its existing recursive directory adds — as
// long as the included file lives under rootDir, fsnotify already
// emits events for it; SetExtraFiles only flips the "interesting"
// bit so the event loop doesn't filter it out alongside other
// non-.md noise.
func (w *Watcher) SetExtraFiles(paths []string) {
	w.extraMu.Lock()
	defer w.extraMu.Unlock()
	w.extra = make(map[string]struct{}, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		w.extra[abs] = struct{}{}
	}
}
