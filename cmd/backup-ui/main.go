package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	m := newModel(ctx, configFromEnv(), runCommand)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithoutSignalHandler())
	go func() {
		<-ctx.Done()
		p.Send(interruptMsg{})
	}()
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "backup-ui:", err)
		os.Exit(1)
	}
}
