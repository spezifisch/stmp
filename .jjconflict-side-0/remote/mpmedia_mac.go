// Copyright 2023 The STMPS Authors
// Copyright 2023 Drew Weymouth and contributors, zackslash
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package remote

/**
* This file handles implementation of MacOS native controls via the native 'MediaPlayer' framework
**/

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework MediaPlayer
#include "mpmediabridge.h"
*/
import (
	"C"
)

import (
	"log"
	"unsafe"

	"github.com/spezifisch/stmps/logger"
)

// os_remote_command_callback is called by Objective-C when incoming OS media commands are received.
//
//export os_remote_command_callback
func os_remote_command_callback(command C.Command, value C.double) {
	switch command {
	case C.PLAY:
		mpMediaEventRecipient.OnCommandPlay()
	case C.PAUSE:
		mpMediaEventRecipient.OnCommandPause()
	case C.STOP:
		mpMediaEventRecipient.OnCommandStop()
	case C.TOGGLE:
		mpMediaEventRecipient.OnCommandTogglePlayPause()
	case C.PREVIOUS_TRACK:
		mpMediaEventRecipient.OnCommandPreviousTrack()
	case C.NEXT_TRACK:
		mpMediaEventRecipient.OnCommandNextTrack()
	case C.SEEK:
		mpMediaEventRecipient.OnCommandSeek(float64(value))
	default:
		log.Printf("unknown OS command received: %v", command)
	}
}

// MPMediaHandler is the handler for MacOS media controls and system events.
type MPMediaHandler struct {
	player ControlledPlayer
	logger logger.LoggerInterface
}

// global recipient for Object-C callbacks from command center.
// This is global so that it can be called from 'os_remote_command_callback' to avoid passing Go pointers into C.
var mpMediaEventRecipient *MPMediaHandler

// NewMPMediaHandler creates a new MPMediaHandler instances and sets it as the current recipient
// for incoming system events.
func RegisterMPMediaHandler(player ControlledPlayer, logger_ logger.LoggerInterface) error {
	mp := &MPMediaHandler{
		player: player,
		logger: logger_,
	}

	// register remote commands and set callback target
	mpMediaEventRecipient = mp
	C.register_os_remote_commands()

	mp.player.OnSongChange(func(track TrackInterface) {
		mp.logger.Print("OnSongChange")
		mp.updateMetadata(track)
	})

	mp.player.OnStopped(func() {
		mp.logger.Print("OnStopped")
		C.set_os_playback_state_stopped()
	})

	mp.player.OnSeek(func() {
		mp.logger.Print("OnSeek")
		C.update_os_now_playing_info_position(C.double(mp.player.GetTimePos()))
	})

	mp.player.OnPlaying(func() {
		mp.logger.Print("OnPlaying")
		C.set_os_playback_state_playing()
		C.update_os_now_playing_info_position(C.double(mp.player.GetTimePos()))
	})

	mp.player.OnPaused(func() {
		mp.logger.Print("OnPaused")
		C.set_os_playback_state_paused()
		C.update_os_now_playing_info_position(C.double(mp.player.GetTimePos()))
	})

	return nil
}

func (mp *MPMediaHandler) updateMetadata(track TrackInterface) {
	var title, artist string
	var duration int
	if track != nil && track.IsValid() {
		title = track.GetTitle()
		artist = track.GetArtist()
		duration = track.GetDuration()
	}

	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	cArtist := C.CString(artist)
	defer C.free(unsafe.Pointer(cArtist))

	// HACK because we don't have cover art
	cArtURL := C.CString("https://support.apple.com/library/content/dam/edam/applecare/images/en_US/osx/mac-apple-logo-screen-icon.png")
	defer C.free(unsafe.Pointer(cArtURL))

	cTrackDuration := C.double(duration)

	C.set_os_now_playing_info(cTitle, cArtist, cArtURL, cTrackDuration)
}

/**
* Handle incoming OS commands.
**/

// MPMediaHandler instance received OS command 'pause'
func (mp *MPMediaHandler) OnCommandPause() {
	if mp == nil || mp.player == nil {
		return
	}
	if err := mp.player.Pause(); err != nil {
		mp.logger.PrintError("Pause", err)
	}
}

// MPMediaHandler instance received OS command 'play'
func (mp *MPMediaHandler) OnCommandPlay() {
	if mp == nil || mp.player == nil {
		return
	}
	if err := mp.player.Play(); err != nil {
		mp.logger.PrintError("Play", err)
	}
}

// MPMediaHandler instance received OS command 'stop'
func (mp *MPMediaHandler) OnCommandStop() {
	if mp == nil || mp.player == nil {
		return
	}
	if err := mp.player.Stop(); err != nil {
		mp.logger.PrintError("Stop", err)
	}
}

// MPMediaHandler instance received OS command 'toggle'
func (mp *MPMediaHandler) OnCommandTogglePlayPause() {
	if mp == nil || mp.player == nil {
		return
	}
	if err := mp.player.Pause(); err != nil {
		mp.logger.PrintError("Pause", err)
	}
}

// MPMediaHandler instance received OS command 'next track'
func (mp *MPMediaHandler) OnCommandNextTrack() {
	if mp == nil || mp.player == nil {
		return
	}
	if err := mp.player.NextTrack(); err != nil {
		mp.logger.PrintError("NextTrack", err)
	}
}

// MPMediaHandler instance received OS command 'previous track'
func (mp *MPMediaHandler) OnCommandPreviousTrack() {
	if mp == nil || mp.player == nil {
		return
	}
	if err := mp.player.PreviousTrack(); err != nil {
		mp.logger.PrintError("PreviousTrack", err)
	}
}

// MPMediaHandler instance received OS command to 'seek'
func (mp *MPMediaHandler) OnCommandSeek(positionSeconds float64) {
	if mp == nil || mp.player == nil {
		return
	}
	if err := mp.player.SeekAbsolute(positionSeconds); err != nil {
		mp.logger.PrintError("SeekAbsolute", err)
	}
}
