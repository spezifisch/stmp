// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"text/template"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spezifisch/stmps/logger"
	"github.com/spezifisch/stmps/mpvplayer"
	"github.com/spezifisch/stmps/subsonic"
	"github.com/spf13/viper"
)

// TODO show total # of entries somewhere (top?)

// columns: star, title, artist, duration
const queueDataColumns = 4
const starIcon = "♥"

// data for rendering queue table
type queueData struct {
	tview.TableContentReadOnly

	// our copy of the queue
	playerQueue mpvplayer.PlayerQueue
	// we also need to know which elements are starred
	starIdList map[string]struct{}
}

var _ tview.TableContent = (*queueData)(nil)

type QueuePage struct {
	Root *tview.Flex

	queueList *tview.Table
	queueData queueData

	songInfo *tview.TextView
	coverArt *tview.Image

	// external refs
	ui     *Ui
	logger logger.LoggerInterface

	songInfoTemplate *template.Template

	coverArtCache Cache[image.Image]
}

var STMPS_LOGO image.Image

// init sets up the default image used for songs for which the server provides
// no cover art.
func init() {
	var err error
	STMPS_LOGO, err = png.Decode(bytes.NewReader(_stmps_logo))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
	}
}

func (ui *Ui) createQueuePage() *QueuePage {
	addSongInfo := !viper.GetBool("ui.hide-song-info")

	var songInfoTemplate *template.Template
	if addSongInfo {
		tmpl := template.New("song info").Funcs(template.FuncMap{
			"formatTime": func(i int) string {
				return (time.Duration(i) * time.Second).String()
			},
		})
		var err error
		if songInfoTemplate, err = tmpl.Parse(songInfoTemplateString); err != nil {
			ui.logger.PrintError("createQueuePage", err)
		}
	}

	queuePage := QueuePage{
		ui:               ui,
		logger:           ui.logger,
		songInfoTemplate: songInfoTemplate,
	}

	// main table
	queuePage.queueList = tview.NewTable().
		SetSelectable(true, false). // rows selectable
		SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorLightGray).Foreground(tcell.ColorBlack))
	queuePage.queueList.Box.
		SetTitle(" queue ").
		SetTitleAlign(tview.AlignLeft).
		SetBorder(true)
	queuePage.queueList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyDelete || event.Rune() == 'd' {
			queuePage.handleDeleteFromQueue()
		} else {
			switch event.Rune() {
			case 'y':
				queuePage.handleToggleStar()
			case 'j':
				queuePage.moveSongDown()
			case 'k':
				queuePage.moveSongUp()
			case 's':
				if len(queuePage.queueData.playerQueue) == 0 {
					queuePage.logger.Print("no items in queue to save")
					return nil
				}
				queuePage.ui.ShowSelectPlaylist()
			case 'S':
				queuePage.shuffle()
			case 'l':
				go func() {
					ssr, err := queuePage.ui.connection.LoadPlayQueue()
					if err != nil {
						queuePage.logger.Printf("unable to load play queue from server: %s", err)
						return
					}
					queuePage.queueList.Clear()
					queuePage.queueData.Clear()
					if ssr.PlayQueue.Entries != nil {
						for _, ent := range ssr.PlayQueue.Entries {
							ui.addSongToQueue(&ent)
						}
						ui.queuePage.UpdateQueue()
						if err := ui.player.Play(); err != nil {
							queuePage.logger.Printf("error playing: %s", err)
						}
						if err = ui.player.Seek(ssr.PlayQueue.Position); err != nil {
							queuePage.logger.Printf("unable to seek to position %s: %s", time.Duration(ssr.PlayQueue.Position)*time.Second, err)
						}
						_ = ui.player.Pause()
					}
				}()

			default:
				return event
			}
		}

		return nil
	})

	// Song info
	queuePage.songInfo = tview.NewTextView()
	queuePage.songInfo.SetDynamicColors(true).SetScrollable(true)
	queuePage.songInfo.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})

	queuePage.queueList.SetSelectionChangedFunc(queuePage.changeSelection)

	// private data
	queuePage.queueData = queueData{
		starIdList: ui.starIdList,
	}

	// flex wrapper
	queuePage.Root = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(queuePage.queueList, 0, 2, true)

	if addSongInfo {
		// Song info
		queuePage.songInfo = tview.NewTextView()
		queuePage.songInfo.SetDynamicColors(true).SetScrollable(true)

		queuePage.coverArt = tview.NewImage()
		queuePage.coverArt.SetImage(STMPS_LOGO)

		infoFlex := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(queuePage.songInfo, 0, 1, false).
			AddItem(queuePage.coverArt, 0, 1, false)
		infoFlex.SetBorder(true)
		infoFlex.SetTitle(" song info ")
		queuePage.Root.AddItem(infoFlex, 0, 1, false)

		queuePage.coverArtCache = NewCache(
			// zero value
			STMPS_LOGO,
			// function that loads assets; can be slow
			ui.connection.GetCoverArt,
			// function that gets called when the actual asset is loaded
			func(imgId string, img image.Image) {
				row, _ := queuePage.queueList.GetSelection()
				// If nothing is selected, set the image to the logo
				if row >= len(queuePage.queueData.playerQueue) || row < 0 {
					ui.app.QueueUpdate(func() {
						queuePage.coverArt.SetImage(STMPS_LOGO)
					})
					return
				}
				// If the fetched asset isn't the asset for the current song,
				// just skip it.
				currentSong := queuePage.queueData.playerQueue[row]
				if currentSong.CoverArtId != imgId {
					return
				}
				// Otherwise, the asset is for the current song, so update it
				ui.app.QueueUpdate(func() {
					queuePage.coverArt.SetImage(img)
				})
			},
			// function called to check if asset is invalid:
			// true if it can be purged from the cache, false if it's still needed
			func(assetId string) bool {
				for _, song := range queuePage.queueData.playerQueue {
					if song.CoverArtId == assetId {
						return false
					}
				}
				// Didn't find a song that needs the asset; purge it.
				return true
			},
			// How frequently we check for invalid assets
			time.Minute,
			ui.logger,
		)
	}

	return &queuePage
}

func (q *QueuePage) changeSelection(row, column int) {
	// If the user disabled song info, there's nothing to do
	if q.songInfo == nil {
		return
	}
	q.songInfo.Clear()
	if row >= len(q.queueData.playerQueue) || row < 0 || column < 0 {
		q.coverArt.SetImage(STMPS_LOGO)
		return
	}
	currentSong := q.queueData.playerQueue[row]
	art := STMPS_LOGO
	if currentSong.CoverArtId != "" {
		art = q.coverArtCache.Get(currentSong.CoverArtId)
	}
	q.coverArt.SetImage(art)
	_ = q.songInfoTemplate.Execute(q.songInfo, currentSong)
}

func (q *QueuePage) UpdateQueue() {
	q.updateQueue()
}

func (q *QueuePage) getSelectedItem() (index int, err error) {
	index, _ = q.queueList.GetSelection()
	if index < 0 {
		err = errors.New("invalid index")
		return
	}
	return
}

// button handler
func (q *QueuePage) handleDeleteFromQueue() {
	currentIndex, err := q.getSelectedItem()
	if err != nil {
		return
	}

	// remove the item from the queue
	q.ui.player.DeleteQueueItem(currentIndex)
	q.updateQueue()
}

// button handler
func (q *QueuePage) handleToggleStar() {
	starIdList := q.queueData.starIdList

	currentIndex, err := q.getSelectedItem()
	if err != nil {
		q.logger.PrintError("handleToggleStar", err)
		return
	}

	entity, err := q.ui.player.GetQueueItem(currentIndex)
	if err != nil {
		q.logger.PrintError("handleToggleStar", err)
		return
	}

	// If the song is already in the star list, remove it
	_, remove := starIdList[entity.Id]

	// update on server
	if _, err = q.ui.connection.ToggleStar(entity.Id, starIdList); err != nil {
		q.ui.showMessageBox("ToggleStar failed")
		return // fail, assume not toggled
	}

	if remove {
		delete(starIdList, entity.Id)
	} else {
		starIdList[entity.Id] = struct{}{}
	}

	q.ui.browserPage.UpdateStars()
}

// re-read queue data from mpvplayer which is the authoritative source for the queue
func (q *QueuePage) updateQueue() {
	queueWasEmpty := len(q.queueData.playerQueue) == 0

	// tell tview table to update its data
	q.queueData.playerQueue = q.ui.player.GetQueueCopy()
	q.queueList.SetContent(&q.queueData)

	// by default we're scrolled down after initially adding rows, fix this
	if queueWasEmpty {
		q.queueList.ScrollToBeginning()
	}

	r, c := q.queueList.GetSelection()
	q.changeSelection(r, c)
}

// moveSongUp moves the currently selected song up in the queue
// If the selected song isn't the third or higher, this is a NOP
// and no error is reported.
func (q *QueuePage) moveSongUp() {
	if len(q.queueData.playerQueue) == 0 {
		return
	}

	currentIndex, column := q.queueList.GetSelection()
	if currentIndex < 0 || column < 0 {
		q.logger.Printf("moveSongUp: invalid selection (%d, %d)", currentIndex, column)
		return
	}

	if currentIndex == 0 {
		return
	}

	if currentIndex == 1 {
		// An error here won't affect re-arranging the queue.
		_ = q.ui.player.Stop()
	}

	// remove the item from the queue
	q.ui.player.MoveSongUp(currentIndex)
	q.queueList.Select(currentIndex-1, column)
	q.updateQueue()
}

// moveSongUp moves the currently selected song up in the queue
// If the selected song is not the second-to-the-last or lower, this is a NOP,
// and no error is reported
func (q *QueuePage) moveSongDown() {
	queueLen := len(q.queueData.playerQueue)
	if queueLen == 0 {
		return
	}

	currentIndex, column := q.queueList.GetSelection()
	if currentIndex < 0 || column < 0 {
		q.logger.Printf("moveSongDown: invalid selection (%d, %d)", currentIndex, column)
		return
	}

	if currentIndex == 0 {
		// An error here won't affect re-arranging the queue.
		_ = q.ui.player.Stop()
	}

	if currentIndex > queueLen-2 {
		q.logger.Printf("moveSongDown: can't move last song")
		return
	}

	// remove the item from the queue
	q.ui.player.MoveSongDown(currentIndex)
	q.queueList.Select(currentIndex+1, column)
	q.updateQueue()
}

// saveQueue persists the current queue as a playlist. It presents the user
// with a way of choosing the playlist name, and if a playlist with the
// same name already exists it requires the user to confirm that they
// want to overwrite the existing playlist.
//
// Errors are reported to the user and require confirmation to dismiss,
// and logged.
func (q *QueuePage) saveQueue(playlistName string) {
	// When updating an existing playlist, there are two options:
	// updatePlaylist, and createPlaylist. createPlaylist on an
	// existing playlist is a replace function.
	//
	// updatePlaylist is more surgical: it can selectively add and
	// remove songs, and update playlist attributes. It is more
	// network efficient than using createPlaylist to change an
	// existing playlist.  However, using it here would require
	// a more complex diffing algorithm, and much more code.
	// Consequently, this version of save() uses the more simple
	// brute-force approach of always using createPlaylist().
	songIds := make([]string, len(q.queueData.playerQueue))
	for i, it := range q.queueData.playerQueue {
		songIds[i] = it.Id
	}

	var playlistId string
	for _, p := range q.ui.playlists {
		if p.Name == playlistName {
			playlistId = string(p.Id)
			break
		}
	}
	var response *subsonic.SubsonicResponse
	var err error
	if playlistId == "" {
		q.logger.Printf("Saving %d items to playlist %s", len(q.queueData.playerQueue), playlistName)
		response, err = q.ui.connection.CreatePlaylist("", playlistName, songIds)
	} else {
		q.logger.Printf("Replacing playlist %s with %d", playlistId, len(q.queueData.playerQueue))
		response, err = q.ui.connection.CreatePlaylist(playlistId, "", songIds)
	}
	if err != nil {
		message := fmt.Sprintf("Error saving queue: %s", err)
		q.ui.showMessageBox(message)
		q.logger.Print(message)
	} else {
		if playlistId != "" {
			for i, pl := range q.ui.playlists {
				if string(pl.Id) == playlistId {
					q.ui.playlists[i] = response.Playlist
					break
				}
			}
		} else {
			q.ui.playlistPage.addPlaylist(response.Playlist)
			q.ui.playlists = append(q.ui.playlists, response.Playlist)
		}
		q.ui.playlistPage.handlePlaylistSelected(response.Playlist)
	}
}

// shuffle randomly shuffles entries in the queue, updates it, and moves
// the selected-item to the new first entry.
func (q *QueuePage) shuffle() {
	if len(q.queueData.playerQueue) == 0 {
		return
	}

	// An error here won't affect re-arranging the queue.
	_ = q.ui.player.Stop()
	q.ui.player.Shuffle()

	q.queueList.Select(0, 0)
	q.updateQueue()
}

// queueData methods, used by tview to lazily render the table
func (q *queueData) GetCell(row, column int) *tview.TableCell {
	if row >= len(q.playerQueue) || column >= queueDataColumns || row < 0 || column < 0 {
		return nil
	}
	song := q.playerQueue[row]

	switch column {
	case 0: // star
		text := " "
		color := tcell.ColorDefault
		if _, starred := q.starIdList[song.Id]; starred {
			text = starIcon
			color = tcell.ColorRed
		}
		return &tview.TableCell{
			Text:        text,
			Color:       color,
			Expansion:   0,
			MaxWidth:    1,
			Transparent: true,
		}
	case 1: // title
		return &tview.TableCell{
			Text:        tview.Escape(song.Title),
			Expansion:   1,
			Transparent: true,
		}
	case 2: // artist
		return &tview.TableCell{
			Text:        tview.Escape(song.Artist),
			Expansion:   1,
			Transparent: true,
		}
	case 3: // duration
		min, sec := iSecondsToMinAndSec(song.Duration)
		text := fmt.Sprintf("%3d:%02d", min, sec)
		return &tview.TableCell{
			Text:        text,
			Align:       tview.AlignRight,
			Expansion:   0,
			MaxWidth:    6,
			Transparent: true,
		}
	}

	return nil
}

// Return the total number of rows in the table.
func (q *queueData) GetRowCount() int {
	return len(q.playerQueue)
}

// Return the total number of columns in the table.
func (q *queueData) GetColumnCount() int {
	return queueDataColumns
}

var songInfoTemplateString = `[blue::b]Title:[-:-:-:-] [green::i]{{.Title}}[-:-:-:-] [yellow::i]({{formatTime .Duration}})[-:-:-:-]
[blue::b]Artist:[-:-:-:-] [::i]{{.Artist}}[-:-:-:-]
[blue::b]Album:[-:-:-:-] [::i]{{.GetAlbum}}[-:-:-:-]
[blue::b]Disc:[-:-:-:-] [::i]{{.GetDiscNumber}}[-:-:-:-]  [blue::b]Track:[-:-:-:-] [::i]{{.GetTrackNumber}}[-:-:-:-]
[blue::b]Year:[-:-:-:-] [::i]{{.GetYear}}[-:-:-:-]
`

//go:embed docs/stmps_logo.png
var _stmps_logo []byte
