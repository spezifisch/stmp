package commands

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterAndExecuteCommand(t *testing.T) {
	registry := NewRegistry()
	ctx := &CommandContext{}

	// Track if the command was called
	wasCalledA := false
	wasCalledB := false

	// Register a simple command that logs the first argument
	registry.Register("log", func(ctx *CommandContext, args []string) error {
		if len(args) > 0 {
			wasCalledA = true
			return nil
		}
		wasCalledB = true
		return fmt.Errorf("missing argument")
	})

	// Test executing a valid command
	err := registry.Execute(ctx, "log 'test message'")
	assert.NoError(t, err, "Command 'log' with argument should execute without error")
	assert.True(t, wasCalledA, "Command 'log' success case should have been called")
	assert.False(t, wasCalledB, "Command 'log' failure case should not have been called")

	wasCalledA = false
	wasCalledB = false
	err = registry.Execute(ctx, "log")
	assert.Error(t, err, "Command 'log' without argument should execute with error")
	assert.False(t, wasCalledA, "Command 'log' success case should not have been called")
	assert.True(t, wasCalledB, "Command 'log' failure case should have been called")
}

func TestExecuteNonExistentCommand(t *testing.T) {
	registry := NewRegistry()
	ctx := &CommandContext{}

	// Test executing a command that does not exist
	err := registry.Execute(ctx, "nonexistent")
	assert.Error(t, err, "Should return error when executing a non-existent command")
	assert.Contains(t, err.Error(), "Command 'nonexistent' not found", "Error message should indicate missing command")
}

func TestCommandWithArguments(t *testing.T) {
	registry := NewRegistry()
	ctx := &CommandContext{}

	// Register a command that expects an argument
	registry.Register("log", func(ctx *CommandContext, args []string) error {
		if len(args) > 0 && args[0] == "hello" {
			return nil
		}
		return fmt.Errorf("wrong argument")
	})

	// Test command with correct argument
	err := registry.Execute(ctx, "log 'hello'")
	assert.NoError(t, err, "Command with correct argument should execute without error")

	// Test command with wrong argument
	err = registry.Execute(ctx, "log 'wrong'")
	assert.Error(t, err, "Command with wrong argument should return an error")
}

func TestCommandChaining(t *testing.T) {
	registry := NewRegistry()
	ctx := &CommandContext{}

	// Register a couple of commands
	registry.Register("first", func(ctx *CommandContext, args []string) error {
		return nil
	})
	registry.Register("second", func(ctx *CommandContext, args []string) error {
		return nil
	})

	// Test valid command chaining
	err := registry.ExecuteChain(ctx, "first; second")
	assert.NoError(t, err, "Command chain should execute all commands without error")

	// Test chaining with an invalid command
	err = registry.ExecuteChain(ctx, "first; nonexistent; second")
	assert.Error(t, err, "Command chain should return error if one command is invalid")

	// Test valid command with arguments in chaining
	registry.Register("log", func(ctx *CommandContext, args []string) error {
		if len(args) > 0 && args[0] == "message" {
			return nil
		}
		return fmt.Errorf("unexpected argument")
	})

	err = registry.ExecuteChain(ctx, "log 'message'; first")
	assert.NoError(t, err, "Command chain with arguments should execute without error")

	// Test chaining commands with mixed valid and invalid arguments
	err = registry.ExecuteChain(ctx, "log 'message'; log 'wrong'; first")
	assert.Error(t, err, "Command chain with one invalid argument should return error")
}

func TestParseCommandLine(t *testing.T) {
	// Test parsing command with no arguments
	result := parseCommandChain("log")
	assert.Equal(t, [][]string{{"log"}}, result, "Command with no arguments should return single element slice")

	// Test parsing command with a quoted argument
	result = parseCommandChain("log 'hello world'")
	assert.Equal(t, [][]string{{"log", "hello world"}}, result, "Command with quoted argument should return correctly split parts")

	// Test parsing command with multiple arguments
	result = parseCommandChain("add 'file.txt' 'destination'")
	assert.Equal(t, [][]string{{"add", "file.txt", "destination"}}, result, "Command with multiple quoted arguments should return correctly split parts")

	// Test command chain separated by semicolons
	result = parseCommandChain("log 'message'; first; second")
	assert.Equal(t, [][]string{{"log", "message"}, {"first"}, {"second"}}, result, "Command chain should return correctly split commands and arguments")
}

func TestParseCommandChain(t *testing.T) {
	// Test parsing a chain of commands
	result := parseCommandChain("log 'message'; first; second")
	expected := [][]string{
		{"log", "message"},
		{"first"},
		{"second"},
	}
	assert.Equal(t, expected, result, "Command chain should return correctly split commands and arguments")

	// Test parsing a chain with no arguments
	result = parseCommandChain("first; second")
	expected = [][]string{
		{"first"},
		{"second"},
	}
	assert.Equal(t, expected, result, "Command chain without arguments should return correctly split commands")

	// Test parsing with multiple quoted arguments
	result = parseCommandChain("add 'file.txt' 'destination'; move 'file.txt'")
	expected = [][]string{
		{"add", "file.txt", "destination"},
		{"move", "file.txt"},
	}
	assert.Equal(t, expected, result, "Command chain with multiple arguments should return correctly parsed commands")
}
