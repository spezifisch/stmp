// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"time"

	"github.com/spezifisch/stmps/mpvplayer"
)

type eventLoop struct {
	// scrobbles are handled by background loop
	scrobbleNowPlaying      chan string
	scrobbleSubmissionTimer *time.Timer
}

func (ui *Ui) initEventLoops() {
	el := &eventLoop{
		scrobbleNowPlaying: make(chan string, 5),
	}
	ui.eventLoop = el

	// create reused timer to scrobble after delay
	el.scrobbleSubmissionTimer = time.NewTimer(0)
	if !el.scrobbleSubmissionTimer.Stop() {
		<-el.scrobbleSubmissionTimer.C
	}
}

func (ui *Ui) runEventLoops() {
	go ui.guiEventLoop()
	go ui.backgroundEventLoop()
}

// handle ui updates
func (ui *Ui) guiEventLoop() {
	ui.addStarredToList()
	events := 0.0
	fpsTimer := time.NewTimer(0)

	for {
		events++

		select {
		case <-fpsTimer.C:
			fpsTimer.Reset(10 * time.Second)
			// ui.logger.Printf("guiEventLoop: %f events per second", events/10.0)
			events = 0

		case msg := <-ui.logger.Prints:
			// handle log page output
			ui.logPage.Print(msg)

		case mpvEvent := <-ui.mpvEvents:
			events++

			// handle events from mpv wrapper
			switch mpvEvent.Type {
			case mpvplayer.EventStatus:
				if mpvEvent.Data == nil {
					continue
				}
				statusData := mpvEvent.Data.(mpvplayer.StatusData) // TODO is this safe to access? maybe we need a copy

				ui.app.QueueUpdateDraw(func() {
					ui.playerStatus.SetText(formatPlayerStatus(statusData.Volume, statusData.Position, statusData.Duration))
					if ui.queuePage.lyrics != nil {
						cl := ui.queuePage.currentLyrics.Lines
						lcl := len(cl)
						if lcl == 0 {
							ui.queuePage.lyrics.SetText("\n[::i]No lyrics[-:-:-]")
						} else {
							// We only get an update every second or so, and Position is truncated
							// to seconds. Make sure that, by the time our tick comes, we're already showing
							// the lyric that's being sung. Do this by pretending that we're a half-second
							// in the future
							p := statusData.Position*1000 + 500
							_, _, _, fh := ui.queuePage.lyrics.GetInnerRect()
							for i := 0; i < lcl-1; i++ {
								if p >= cl[i].Start && p < cl[i+1].Start {
									txt := ""
									if i > 0 {
										txt = cl[i-1].Value + "\n"
									}
									txt += "[::b]" + cl[i].Value + "[-:-:-]\n"
									for k := i + 1; k < lcl && k-i < fh; k++ {
										txt += cl[k].Value + "\n"
									}
									ui.queuePage.lyrics.SetText(txt)
									break
								}
							}
						}
					}
				})

			case mpvplayer.EventStopped:
				ui.logger.Print("mpvEvent: stopped")
				ui.app.QueueUpdateDraw(func() {
					ui.startStopStatus.SetText("[red::b]Stopped[::-]")
					if ui.queuePage.lyrics != nil {
						ui.queuePage.lyrics.SetText("")
					}
					ui.queuePage.updateQueue()
				})

			case mpvplayer.EventPlaying:
				ui.logger.Print("mpvEvent: playing")
				statusText := "[green::b]Playing[::-]"

				var currentSong mpvplayer.QueueItem
				if mpvEvent.Data != nil {
					currentSong = mpvEvent.Data.(mpvplayer.QueueItem) // TODO is this safe to access? maybe we need a copy
					statusText += formatSongForStatusBar(&currentSong)

					// Update MprisPlayer with new track info
					if ui.mprisPlayer != nil {
						ui.mprisPlayer.OnSongChange(currentSong)
					}

					if ui.connection.Scrobble {
						// scrobble "now playing" event (delegate to background event loop)
						ui.eventLoop.scrobbleNowPlaying <- currentSong.Id

						// scrobble "submission" after song has been playing a bit
						// see: https://www.last.fm/api/scrobbling
						// A track should only be scrobbled when the following conditions have been met:
						// The track must be longer than 30 seconds. And the track has been played for
						// at least half its duration, or for 4 minutes (whichever occurs earlier.)
						if currentSong.Duration > 30 {
							scrobbleDelay := currentSong.Duration / 2
							if scrobbleDelay > 240 {
								scrobbleDelay = 240
							}
							scrobbleDuration := time.Duration(scrobbleDelay) * time.Second

							ui.eventLoop.scrobbleSubmissionTimer.Reset(scrobbleDuration)
							ui.logger.Printf("scrobbler: timer started, %v", scrobbleDuration)
						} else {
							ui.logger.Printf("scrobbler: track too short")
						}
					}
				}

				ui.app.QueueUpdateDraw(func() {
					ui.startStopStatus.SetText(statusText)
					ui.queuePage.updateQueue()
					if ui.queuePage.lyrics != nil {
						if len(ui.queuePage.currentLyrics.Lines) == 0 {
							ui.queuePage.lyrics.SetText("\n[::i]No lyrics[-:-:-]")
						} else {
							ui.queuePage.lyrics.SetText("")
						}
					}
				})

			case mpvplayer.EventPaused:
				ui.logger.Print("mpvEvent: paused")
				statusText := "[yellow::b]Paused[::-]"

				var currentSong mpvplayer.QueueItem
				if mpvEvent.Data != nil {
					currentSong = mpvEvent.Data.(mpvplayer.QueueItem) // TODO is this safe to access? maybe we need a copy
					statusText += formatSongForStatusBar(&currentSong)
				}

				ui.app.QueueUpdateDraw(func() {
					ui.startStopStatus.SetText(statusText)
				})

			case mpvplayer.EventUnpaused:
				ui.logger.Print("mpvEvent: unpaused")
				statusText := "[green::b]Playing[::-]"

				var currentSong mpvplayer.QueueItem
				if mpvEvent.Data != nil {
					currentSong = mpvEvent.Data.(mpvplayer.QueueItem) // TODO is this safe to access? maybe we need a copy
					statusText += formatSongForStatusBar(&currentSong)
				}

				ui.app.QueueUpdateDraw(func() {
					ui.startStopStatus.SetText(statusText)
				})

			default:
				ui.logger.Printf("guiEventLoop: unhandled mpvEvent %v", mpvEvent)
			}
		}
	}
}

// loop for blocking background tasks that would otherwise block the ui
func (ui *Ui) backgroundEventLoop() {
	for {
		select {
		case songId := <-ui.eventLoop.scrobbleNowPlaying:
			// scrobble now playing
			if _, err := ui.connection.ScrobbleSubmission(songId, false); err != nil {
				ui.logger.PrintError("scrobble nowplaying", err)
			}

		case <-ui.eventLoop.scrobbleSubmissionTimer.C:
			// scrobble submission delay elapsed
			if currentSong, err := ui.player.GetPlayingTrack(); err != nil {
				// user paused/stopped
				ui.logger.Printf("not scrobbling: %v", err)
			} else {
				// it's still playing
				ui.logger.Printf("scrobbling: %s", currentSong.Id)
				if _, err := ui.connection.ScrobbleSubmission(currentSong.Id, true); err != nil {
					ui.logger.PrintError("scrobble submission", err)
				}
			}
		}
	}
}

func (ui *Ui) addStarredToList() {
	response, err := ui.connection.GetStarred()
	if err != nil {
		ui.logger.PrintError("addStarredToList", err)
	}

	for _, e := range response.Starred.Song {
		// We're storing empty struct as values as we only want the indexes
		// It's faster having direct index access instead of looping through array values
		ui.starIdList[e.Id] = struct{}{}
	}
	for _, e := range response.Starred.Album {
		ui.starIdList[e.Id] = struct{}{}
	}
	for _, e := range response.Starred.Artist {
		ui.starIdList[e.Id] = struct{}{}
	}
}
