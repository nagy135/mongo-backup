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
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "backup-ui:", err)
		os.Exit(1)
	}
	if output := final.(model).printedDates; output != "" {
		if _, err := fmt.Fprint(os.Stdout, output); err != nil {
			fmt.Fprintln(os.Stderr, "backup-ui:", err)
			os.Exit(1)
		}
	}
}
