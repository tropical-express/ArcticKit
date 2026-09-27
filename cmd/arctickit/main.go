package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"arctickit/internal/tui"
)

func main() {
	model := tui.NewModel()

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Println("ArcticKit error:", err)
		os.Exit(1)
	}
}
