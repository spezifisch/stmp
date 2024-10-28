package commands

import (
	"fmt"
	"strings"
)

// CommandFunc defines the signature of a callback function implementing a command.
type CommandFunc func(ctx *CommandContext, args []string) error

// CommandRegistry holds the list of available commands.
type CommandRegistry struct {
	commands map[string]CommandFunc
}

// NewRegistry creates a new CommandRegistry.
func NewRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]CommandFunc),
	}
}

// Register adds a command with arguments support to the registry.
func (r *CommandRegistry) Register(name string, fn CommandFunc) {
	r.commands[name] = fn
}

// Get returns the command function and a boolean indicating if the command exists.
func (r *CommandRegistry) Get(commandName string) (CommandFunc, bool) {
	cmd, exists := r.commands[commandName]
	return cmd, exists
}

// CommandExists is a small wrapper function to extract the "exists" boolean.
func (r *CommandRegistry) CommandExists(commandName string) bool {
	_, exists := r.Get(commandName)
	return exists
}

// Execute parses and runs a command chain, supporting arguments and chaining.
func (r *CommandRegistry) Execute(ctx *CommandContext, commandStr string) error {
	// Split the input into chains of commands
	commandChains := parseCommandChain(commandStr)

	// Iterate over each command in the chain
	for _, chain := range commandChains {
		// Ensure the chain has at least one command
		if len(chain) == 0 {
			continue
		}

		// The first element is the command name, the rest are arguments
		commandName := chain[0]
		args := chain[1:]

		if cmd, exists := r.commands[commandName]; exists {
			// Execute the command with arguments
			err := cmd(ctx, args)
			if err != nil {
				return fmt.Errorf("Error executing command '%s': %v", commandName, err)
			}
		} else {
			return fmt.Errorf("Command '%s' not found", commandName)
		}
	}

	return nil
}

// ExecuteChain allows executing multiple commands separated by ';'
func (r *CommandRegistry) ExecuteChain(ctx *CommandContext, commandChain string) error {
	commands := strings.Split(commandChain, ";")
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if err := r.Execute(ctx, cmd); err != nil {
			return err
		}
	}
	return nil
}

// parseCommandChain splits a command string into parts.
func parseCommandChain(input string) [][]string {
	var commands [][]string
	var currentCommand []string
	var current strings.Builder
	var inQuotes, escapeNext bool

	for _, char := range input {
		switch {
		case escapeNext:
			current.WriteRune(char)
			escapeNext = false
		case char == '\\':
			escapeNext = true
		case char == '\'':
			inQuotes = !inQuotes
		case char == ';' && !inQuotes:
			if current.Len() > 0 {
				currentCommand = append(currentCommand, current.String())
				current.Reset()
			}
			if len(currentCommand) > 0 {
				commands = append(commands, currentCommand)
				currentCommand = nil
			}
		case char == ' ' && !inQuotes:
			if current.Len() > 0 {
				currentCommand = append(currentCommand, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}
	if current.Len() > 0 {
		currentCommand = append(currentCommand, current.String())
	}
	if len(currentCommand) > 0 {
		commands = append(commands, currentCommand)
	}

	return commands
}

// List returns a slice of all registered commands.
func (r *CommandRegistry) List() []string {
	keys := make([]string, 0, len(r.commands))
	for k := range r.commands {
		keys = append(keys, k)
	}
	return keys
}
