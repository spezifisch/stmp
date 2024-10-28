package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spezifisch/stmps/commands"
	"github.com/spezifisch/stmps/logger"
	"github.com/stretchr/testify/assert"
)

func TestRegisterCommands(t *testing.T) {
	ui := &Ui{}

	// Register the commands
	registry := commands.NewRegistry()
	ui.registerCommands(registry)

	// Command context for testing
	ctx := &commands.CommandContext{}

	// Test 'nop' command
	err := registry.Execute(ctx, "nop")
	assert.NoError(t, err, "Command 'nop' should execute without error")

	// Capture output of the echo command using a TestLogger
	var buf bytes.Buffer
	ctx.Logger = &TestLogger{&buf}

	// Test 'echo' command
	err = registry.Execute(ctx, "echo Hello World")
	assert.NoError(t, err, "Command 'echo' should execute without error")
	assert.Equal(t, "Hello World\n", buf.String(), "Command 'echo' should output the correct string")
}

// TestLogger is a simple implementation of LoggerInterface to capture output for testing
type TestLogger struct {
	buf *bytes.Buffer
}

func (l *TestLogger) Print(s string) {
	l.buf.WriteString(s + "\n")
}

func (l *TestLogger) Printf(s string, as ...interface{}) {
	l.buf.WriteString(fmt.Sprintf(s, as...))
}

func (l *TestLogger) PrintError(source string, err error) {
	l.buf.WriteString(fmt.Sprintf("Error in %s: %v\n", source, err))
}

var _ logger.LoggerInterface = (*TestLogger)(nil)
