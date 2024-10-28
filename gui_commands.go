package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spezifisch/stmps/commands"
)

// Register all commands to the registry and include the context handling
func (ui *Ui) registerCommands(registry *commands.CommandRegistry) {
	// NOP
	registry.Register("nop", func(ctx *commands.CommandContext, args []string) error {
		return nil
	})

	// ECHO
	registry.Register("echo", func(ctx *commands.CommandContext, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no arguments provided")
		}

		// Join the arguments and output the result
		output := strings.Join(args, " ")
		ctx.Logger.Print(output)
		return nil
	})

	// ...
	registry.Register("show-page", func(ctx *commands.CommandContext, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("missing page argument")
		}
		ui.ShowPage(args[0])
		return nil
	})

	registry.Register("quit", func(ctx *commands.CommandContext, args []string) error {
		ui.Quit()
		return nil
	})

	registry.Register("add-random-songs", func(ctx *commands.CommandContext, args []string) error {
		randomType := "random"
		if len(args) > 0 {
			randomType = args[0]
		}
		ui.handleAddRandomSongs("", randomType)
		return nil
	})

	registry.Register("clear-queue", func(ctx *commands.CommandContext, args []string) error {
		ui.player.ClearQueue()
		ui.queuePage.UpdateQueue()
		return nil
	})

	registry.Register("pause-playback", func(ctx *commands.CommandContext, args []string) error {
		if err := ui.player.Pause(); err != nil {
			return err
		}
		return nil
	})

	registry.Register("stop-playback", func(ctx *commands.CommandContext, args []string) error {
		if err := ui.player.Stop(); err != nil {
			return err
		}
		return nil
	})

	registry.Register("adjust-volume", func(ctx *commands.CommandContext, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("missing volume argument")
		}
		volume, err := strconv.Atoi(args[0])
		if err != nil {
			return err
		}
		if err := ui.player.AdjustVolume(volume); err != nil {
			return err
		}
		return nil
	})

	registry.Register("seek", func(ctx *commands.CommandContext, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("missing seek time argument")
		}
		seekTime, err := strconv.Atoi(args[0])
		if err != nil {
			return err
		}
		if err := ui.player.Seek(seekTime); err != nil {
			return err
		}
		return nil
	})

	registry.Register("next-track", func(ctx *commands.CommandContext, args []string) error {
		if err := ui.player.PlayNextTrack(); err != nil {
			return err
		}
		ui.queuePage.UpdateQueue()
		return nil
	})

	registry.Register("start-scan", func(ctx *commands.CommandContext, args []string) error {
		if err := ui.connection.StartScan(); err != nil {
			return err
		}
		return nil
	})

	registry.Register("debug-message", func(ctx *commands.CommandContext, args []string) error {
		ui.logger.Print("test debug message")
		ui.showMessageBox("foo bar")
		return nil
	})
}
