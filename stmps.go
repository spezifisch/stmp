// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"

	"github.com/spezifisch/stmps/commands"
	"github.com/spezifisch/stmps/logger"
	"github.com/spezifisch/stmps/mpvplayer"
	"github.com/spezifisch/stmps/remote"
	"github.com/spezifisch/stmps/subsonic"
	tviewcommand "github.com/spezifisch/tview-command"
	"github.com/spf13/viper"
)

var osExit = os.Exit  // A variable to allow mocking os.Exit in tests
var headlessMode bool // This can be set to true during tests
var testMode bool     // This can be set to true during tests, too
const DEVELOPMENT = "development"

var Version string = DEVELOPMENT

func readConfig(configFile *string) error {
	required_properties := []string{"auth.username", "auth.password", "server.host"}

	if configFile != nil && *configFile != "" {
		// use custom config file
		viper.SetConfigFile(*configFile)
	} else {
		// lookup default dirs
		viper.SetConfigName("stmp") // TODO this should be stmps
		viper.SetConfigType("toml")
		viper.AddConfigPath("$HOME/.config/stmp") // TODO this should be stmps only
		viper.AddConfigPath("$HOME/.config/stmps")
		viper.AddConfigPath(".")
	}

	// read it
	err := viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("Config file error: %s\n", err)
	}

	// validate
	for _, prop := range required_properties {
		if !viper.IsSet(prop) {
			return fmt.Errorf("Config property %s is required\n", prop)
		}
	}

	return nil
}

// parseConfig takes the first non-flag arguments from flags and parses it
// into the viper config.
func parseConfig() {
	if u, e := url.Parse(flag.Arg(0)); e == nil {
		// If credentials were provided
		if len(u.User.Username()) > 0 {
			viper.Set("auth.username", u.User.Username())
			// If the password wasn't provided, the program will fail as normal
			if p, s := u.User.Password(); s {
				viper.Set("auth.password", p)
			}
		}
		// Blank out the credentials so we can use the URL formatting
		u.User = nil
		viper.Set("server.host", u.String())
	} else {
		fmt.Printf("Invalid server format; must be a valid URL!")
		fmt.Printf("Usage: %s <args> [http[s]://[user:pass@]server:port]\n", os.Args[0])
		osExit(1)
	}
}

// initCommandHandler sets up tview-command as main input handler
func initCommandHandler(logger *logger.Logger) *tviewcommand.Config {
	tviewcommand.SetLogHandler(func(msg string) {
		logger.Print(msg)
	})

	configPath := "HACK.commands.toml"

	// Load the configuration file
	config, err := tviewcommand.LoadConfig(configPath)
	if err != nil || config == nil {
		logger.PrintError("Failed to load command-shortcut config", err)
		return nil
	}

	// Register commands
	//keybinding.RegisterCommands(env)

	return config
}

// return codes:
// 0 - OK
// 1 - generic errors
// 2 - main config errors
// 2 - keybinding config errors
func main() {
	// parse flags and config
	help := flag.Bool("help", false, "Print usage")
	enableMpris := flag.Bool("mpris", false, "Enable MPRIS2")
	list := flag.Bool("list", false, "list server data")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile := flag.String("memprofile", "", "write memory profile to `file`")
	configFile := flag.String("config", "", "use config `file`")
	version := flag.Bool("version", false, "print the stmps version and exit")

	flag.Parse()
	if *help {
		fmt.Printf("USAGE: %s <args> [[user:pass@]server:port]\n", os.Args[0])
		flag.Usage()
		osExit(0)
	}
	if Version == DEVELOPMENT {
		if bi, ok := debug.ReadBuildInfo(); ok {
			Version = bi.Main.Version
		}
	}
	if *version {
		fmt.Printf("stmps %s", Version)
		osExit(0)
	}

	// cpu/memprofile code straight from https://pkg.go.dev/runtime/pprof
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	// config gathering
	if len(flag.Args()) > 0 {
		parseConfig()
	}

	if err := readConfig(configFile); err != nil {
		if configFile == nil {
			fmt.Fprintf(os.Stderr, "Failed to read configuration: configuration file is nil\n")
		} else {
			fmt.Fprintf(os.Stderr, "Failed to read configuration from file '%s': %v\n", *configFile, err)
		}
		osExit(2)
	}

	logger := logger.Init()

	// init tview-command
	tvcomConfig := initCommandHandler(logger)
	if tvcomConfig == nil {
		osExit(3)
	}

	// init the context stack (context-sensitive keybindings)
	tvcomContextStack := tviewcommand.NewContextStack()

	// init command registry (command parser and executor)
	commandRegistry := commands.NewRegistry()

	// init mpv engine
	player, err := mpvplayer.NewPlayer(logger)
	if err != nil {
		fmt.Println("Unable to initialize mpv. Is mpv installed?")
		osExit(1)
	}

	var mprisPlayer *remote.MprisPlayer
	// init mpris2 player control (linux only but fails gracefully on other systems)
	if *enableMpris {
		mprisPlayer, err = remote.RegisterMprisPlayer(player, logger)
		if err != nil {
			fmt.Printf("Unable to register MPRIS with DBUS: %s\n", err)
			fmt.Println("Try running without MPRIS")
			osExit(1)
		}
		defer mprisPlayer.Close()
	}

	// init macos mediaplayer control
	if runtime.GOOS == "darwin" {
		if err = remote.RegisterMPMediaHandler(player, logger); err != nil {
			fmt.Printf("Unable to initialize MediaPlayer bindings: %s\n", err)
			osExit(1)
		} else {
			logger.Print("MacOS MediaPlayer registered")
		}
	}

	if testMode {
		fmt.Println("Running in test mode for testing.")
		osExit(0x23420001)
		return
	}

	connection := subsonic.Init(logger)
	connection.SetClientInfo(clientName, clientVersion)
	connection.Username = viper.GetString("auth.username")
	connection.Password = viper.GetString("auth.password")
	connection.Host = viper.GetString("server.host")
	connection.PlaintextAuth = viper.GetBool("auth.plaintext")
	connection.Scrobble = viper.GetBool("server.scrobble")
	connection.RandomSongNumber = viper.GetUint("client.random-songs")

	indexResponse, err := connection.GetIndexes() // TODO how long does this take?
	if err != nil {
		fmt.Printf("Error fetching playlists from server: %s\n", err)
		osExit(1)
	}

	if *list {
		fmt.Printf("Index response:\n")
		fmt.Printf("  Directory: %s\n", indexResponse.Directory.Name)
		fmt.Printf("  Status: %s\n", indexResponse.Status)
		fmt.Printf("  Error: %s\n", indexResponse.Error.Message)
		fmt.Printf("  Playlist: %s\n", indexResponse.Playlist.Name)
		fmt.Printf("  Playlists: (%d)\n", len(indexResponse.Playlists.Playlists))
		for _, pl := range indexResponse.Playlists.Playlists {
			fmt.Printf("    [%d] %s\n", pl.Entries.Len(), pl.Name)
		}
		fmt.Printf("  Indexes:\n")
		for _, pl := range indexResponse.Indexes.Index {
			fmt.Printf("    %s\n", pl.Name)
		}
		fmt.Printf("Playlist response: (this can take a while)\n")
		playlistResponse, err := connection.GetPlaylists()
		if err != nil {
			fmt.Printf("Error fetching indexes from server: %s\n", err)
			osExit(1)
		}
		fmt.Printf("  Directory: %s\n", playlistResponse.Directory.Name)
		fmt.Printf("  Status: %s\n", playlistResponse.Status)
		fmt.Printf("  Error: %s\n", playlistResponse.Error.Message)
		fmt.Printf("  Playlist: %s\n", playlistResponse.Playlist.Name)
		fmt.Printf("  Playlists: (%d)\n", len(indexResponse.Playlists.Playlists))
		for _, pl := range playlistResponse.Playlists.Playlists {
			fmt.Printf("    [%d] %s\n", pl.Entries.Len(), pl.Name)
		}
		fmt.Printf("  Indexes:\n")
		for _, pl := range playlistResponse.Indexes.Index {
			fmt.Printf("    %s\n", pl.Name)
		}

		osExit(0)
	}

	if headlessMode {
		fmt.Println("Running in headless mode for testing.")
		osExit(0x23420002)
		return
	}

	ui := InitGui(
		tvcomConfig,
		tvcomContextStack,
		commandRegistry,
		&indexResponse.Indexes.Index,
		connection,
		player,
		logger,
		mprisPlayer)

	// run main loop
	if err := ui.Run(); err != nil {
		panic(err)
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}
