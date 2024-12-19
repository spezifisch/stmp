// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/spezifisch/stmps/mpvplayer"
	"github.com/spezifisch/stmps/subsonic"
)

func (ui *Ui) handlePageInput(event *tcell.EventKey) *tcell.EventKey {
	// we don't want any of these firing if we're trying to add a new playlist
	focused := ui.app.GetFocus()
	if ui.playlistPage.IsNewPlaylistInputFocused(focused) || ui.browserPage.IsSearchFocused(focused) || focused == ui.searchPage.searchField || ui.selectPlaylistWidget.visible {
		return event
	}

	switch event.Rune() {
	case '1':
		ui.ShowPage(PageBrowser)

	case '2':
		ui.ShowPage(PageQueue)

	case '3':
		ui.ShowPage(PagePlaylists)

	case '4':
		ui.ShowPage(PageSearch)

	case '5':
		ui.ShowPage(PageLog)

	case '?':
		ui.ShowHelp()

	case 'Q':
		ui.Quit()

	case 'r':
		// add random songs to queue
		ui.handleAddRandomSongs("", "random")

	case 'D':
		// clear queue and stop playing
		ui.player.ClearQueue()
		ui.queuePage.UpdateQueue()

	case 'p':
		// toggle playing/pause
		err := ui.player.Pause()
		if err != nil {
			ui.logger.PrintError("handlePageInput: Pause", err)
		}

	case 'P':
		// stop playing without changes to queue
		ui.logger.Print("key stop")
		err := ui.player.Stop()
		if err != nil {
			ui.logger.PrintError("handlePageInput: Stop", err)
		}

	case 'X':
		// debug stuff
		ui.logger.Print("test")
		//ui.player.Test()
		ui.showMessageBox("foo bar")

	case '-':
		// volume-
		if err := ui.player.AdjustVolume(-5); err != nil {
			ui.logger.PrintError("handlePageInput: AdjustVolume-", err)
		}

	case '+', '=':
		// volume+
		if err := ui.player.AdjustVolume(5); err != nil {
			ui.logger.PrintError("handlePageInput: AdjustVolume+", err)
		}

	case '.':
		// <<
		if err := ui.player.Seek(10); err != nil {
			ui.logger.PrintError("handlePageInput: Seek+", err)
		}

	case ',':
		// >>
		if err := ui.player.Seek(-10); err != nil {
			ui.logger.PrintError("handlePageInput: Seek-", err)
		}

	case '>':
		// skip to next track
		if err := ui.player.PlayNextTrack(); err != nil {
			ui.logger.PrintError("handlePageInput: Next", err)
		}
		ui.queuePage.UpdateQueue()

	case 'c':
		if err := ui.connection.StartScan(); err != nil {
			ui.logger.PrintError("startScan:", err)
		}

	default:
		return event
	}

	return nil
}

func (ui *Ui) ShowPage(name string) {
	ui.pages.SwitchToPage(name)
	ui.menuWidget.SetActivePage(name)
	_, prim := ui.pages.GetFrontPage()
	ui.app.SetFocus(prim)
}

func (ui *Ui) Quit() {
	if len(ui.queuePage.queueData.playerQueue) > 0 {
		ids := make([]string, len(ui.queuePage.queueData.playerQueue))
		for i, it := range ui.queuePage.queueData.playerQueue {
			ids[i] = it.Id
		}
		// stmps always only ever plays the first song in the queue
		pos := ui.player.GetTimePos()
		if err := ui.connection.SavePlayQueue(ids, ids[0], int(pos)); err != nil {
			log.Printf("error stashing play queue: %s", err)
		}
	} else {
		// The only way to purge a saved play queue is to force an error by providing
		// bad data. Therefore, we ignore errors.
		_ = ui.connection.SavePlayQueue([]string{"XXX"}, "XXX", 0)
	}
	ui.player.Quit()
	ui.app.Stop()
}

func (ui *Ui) handleAddRandomSongs(Id string, randomType string) {
	ui.addRandomSongsToQueue(Id, randomType)
	ui.queuePage.UpdateQueue()
}

func (ui *Ui) addRandomSongsToQueue(Id string, randomType string) {
	response, err := ui.connection.GetRandomSongs(Id, randomType)
	if err != nil {
		ui.logger.Printf("addRandomSongsToQueue %s", err.Error())
	}
	switch randomType {
	case "random":
		for _, e := range response.RandomSongs.Song {
			ui.addSongToQueue(&e)
		}
	case "similar":
		for _, e := range response.SimilarSongs.Song {
			ui.addSongToQueue(&e)
		}
	}
}

// make sure to call ui.QueuePage.UpdateQueue() after this
func (ui *Ui) addSongToQueue(entity *subsonic.SubsonicEntity) {
	uri := ui.connection.GetPlayUrl(entity)

	response, err := ui.connection.GetAlbum(entity.Parent)
	album := ""
	if err != nil {
		ui.logger.PrintError("addSongToQueue", err)
	} else {
		switch {
		case response.Album.Name != "":
			album = response.Album.Name
		case response.Album.Title != "":
			album = response.Album.Title
		case response.Album.Album != "":
			album = response.Album.Album
		}
	}

	queueItem := &mpvplayer.QueueItem{
		Id:          entity.Id,
		Uri:         uri,
		Title:       entity.GetSongTitle(),
		Artist:      entity.Artist,
		Duration:    entity.Duration,
		Album:       album,
		TrackNumber: entity.Track,
		CoverArtId:  entity.CoverArtId,
		DiscNumber:  entity.DiscNumber,
	}
	ui.player.AddToQueue(queueItem)
}

func makeSongHandler(entity *subsonic.SubsonicEntity, ui *Ui, fallbackArtist string) func() {
	// make copy of values so this function can be used inside a loop iterating over entities
	id := entity.Id
	// TODO: Why aren't we doing all of this _inside_ the returned func?
	uri := ui.connection.GetPlayUrl(entity)
	title := entity.Title
	artist := stringOr(entity.Artist, fallbackArtist)
	duration := entity.Duration
	track := entity.Track
	coverArtId := entity.CoverArtId
	disc := entity.DiscNumber

	response, err := ui.connection.GetAlbum(entity.Parent)
	album := ""
	if err != nil {
		ui.logger.PrintError("makeSongHandler", err)
	} else {
		switch {
		case response.Album.Name != "":
			album = response.Album.Name
		case response.Album.Title != "":
			album = response.Album.Title
		case response.Album.Album != "":
			album = response.Album.Album
		}
	}

	return func() {
		if err := ui.player.PlayUri(id, uri, title, artist, album, duration, track, disc, coverArtId); err != nil {
			ui.logger.PrintError("SongHandler Play", err)
			return
		}
		ui.queuePage.UpdateQueue()
	}
}
