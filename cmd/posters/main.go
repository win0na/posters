package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
	"github.com/win0na/posters/internal/tui"
)

const version = "0.1.0"

func main() {
	configDir := flag.String("config-dir", "", "config directory for state and metadata")
	dryRun := flag.Bool("dry-run", false, "find matches without uploading posters or recording metadata")
	force := flag.Bool("force", false, "include movies already recorded as updated")
	wikiFallback := flag.Bool("wiki-fallback", false, "use Wikipedia theatrical poster when IMP match is missing or ambiguous")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	store, err := openStore(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open config: %v\n", err)
		os.Exit(1)
	}

	client := plex.NewClient(store)
	program := tea.NewProgram(
		tui.NewWithOptions(store, client, tui.Options{Force: *force, DryRun: *dryRun, WikiFallback: *wikiFallback}),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run tui: %v\n", err)
		os.Exit(1)
	}
}

func openStore(dir string) (*config.Store, error) {
	if dir != "" {
		return config.OpenDir(dir)
	}
	return config.Open()
}
