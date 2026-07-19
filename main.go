package main

import (
	"fmt"
	"os"

	"snowtui/config"
	"snowtui/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "No config.yaml found.")
		fmt.Fprintln(os.Stderr, "See config.yaml.example for a starting point.")
		os.Exit(1)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.ActiveInstances()) == 0 {
		fmt.Fprintln(os.Stderr, "No active instances found in configuration.")
		os.Exit(1)
	}

	p := tea.NewProgram(
		ui.NewApp(cfg),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
