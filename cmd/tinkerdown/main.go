// Command tinkerdown is the CLI tool for creating and serving interactive documentation.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/cmd/tinkerdown/commands"
)

// version is set via ldflags during build: -ldflags="-X main.version=1.0.0"
var version = "dev"

func main() {
	// Propagate the binary's release version to the library so
	// `LANG include="..."` blocks can construct GitHub source links
	// at the matching tag — pages don't need to hardcode `source_ref`
	// in frontmatter for releases. "dev" builds (no ldflags) leave
	// DefaultSourceRef empty, and the link footer falls back to
	// "main" so links still resolve in development.
	if version != "" && version != "dev" {
		tinkerdown.DefaultSourceRef = version
	}
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		return 1
	}

	command := os.Args[1]
	args := os.Args[2:]

	var err error
	switch command {
	case "serve":
		err = commands.ServeCommand(args)
	case "validate":
		err = commands.ValidateCommand(args)
	case "fix":
		err = commands.FixCommand(args)
	case "new":
		// Parse --template and --list flags from args
		templateName := ""
		listTemplates := false
		filteredArgs := make([]string, 0, len(args))
		skipNext := false
		for i, arg := range args {
			if skipNext {
				skipNext = false
				continue
			}
			if arg == "--list" || arg == "-l" {
				listTemplates = true
			} else if val, ok := strings.CutPrefix(arg, "--template="); ok {
				templateName = val
			} else if val, ok := strings.CutPrefix(arg, "-t="); ok {
				templateName = val
			} else if arg == "--template" || arg == "-t" {
				// Handle space-separated: -t todo or --template todo
				if i+1 < len(args) {
					templateName = args[i+1]
					skipNext = true
				}
			} else {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		if listTemplates {
			commands.ListTemplates()
		} else {
			err = commands.NewCommand(filteredArgs, templateName)
		}
	case "blocks":
		err = commands.BlocksCommand(args)
	case "cli":
		err = commands.CLICommand(args)
	case "build":
		err = commands.BuildCommand(args)
	case "version":
		fmt.Printf("tinkerdown version %s\n", version)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage(os.Stderr)
		return 1
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "tinkerdown - Interactive documentation made easy")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  tinkerdown serve [directory]     Start development server")
	fmt.Fprintln(w, "  tinkerdown build <file|dir>      Build standalone executable")
	fmt.Fprintln(w, "  tinkerdown validate [directory]  Validate markdown files and check policy")
	fmt.Fprintln(w, "  tinkerdown fix [directory]       Auto-fix common issues")
	fmt.Fprintln(w, "  tinkerdown blocks [directory]    Inspect code blocks")
	fmt.Fprintln(w, "  tinkerdown new <name>            Create new app from template")
	fmt.Fprintln(w, "  tinkerdown new --list            List available templates")
	fmt.Fprintln(w, "  tinkerdown cli <path> <action> <source>  CLI mode for CRUD operations")
	fmt.Fprintln(w, "  tinkerdown version               Show version")
	fmt.Fprintln(w, "  tinkerdown help                  Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  tinkerdown serve                 # Serve current directory")
	fmt.Fprintln(w, "  tinkerdown serve ./tutorials     # Serve tutorials directory")
	fmt.Fprintln(w, "  tinkerdown serve --watch         # Serve with live reload")
	fmt.Fprintln(w, "  tinkerdown build app.md -o myapp # Build single-file app")
	fmt.Fprintln(w, "  tinkerdown build ./docs -o docs  # Build directory into binary")
	fmt.Fprintln(w, "  tinkerdown build app.md --target=linux/amd64  # Cross-compile")
	fmt.Fprintln(w, "  tinkerdown validate              # Validate current directory")
	fmt.Fprintln(w, "  tinkerdown validate examples/    # Validate specific directory")
	fmt.Fprintln(w, "  tinkerdown validate --summary    # Emit approved operations as JSON")
	fmt.Fprintln(w, "  tinkerdown fix                   # Auto-fix issues in current directory")
	fmt.Fprintln(w, "  tinkerdown fix --dry-run         # Preview fixes without applying")
	fmt.Fprintln(w, "  tinkerdown blocks examples/      # Inspect blocks in examples/")
	fmt.Fprintln(w, "  tinkerdown blocks . --verbose    # Show detailed block info")
	fmt.Fprintln(w, "  tinkerdown new my-app            # Create new app (basic template)")
	fmt.Fprintln(w, "  tinkerdown new my-app --template=todo  # Use todo template")
	fmt.Fprintln(w, "  tinkerdown new --list            # List all available templates")
	fmt.Fprintln(w, "  tinkerdown cli app.md list tasks # List items from source")
	fmt.Fprintln(w, "  tinkerdown cli . add tasks --text=\"New task\"  # Add item")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Documentation: https://github.com/livetemplate/tinkerdown")
}
