// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spezifisch/stmps/commands"
	"github.com/spezifisch/stmps/logger"
	"github.com/spezifisch/stmps/mpvplayer"
	"github.com/spezifisch/stmps/remote"
	"github.com/spezifisch/stmps/subsonic"
	tviewcommand "github.com/spezifisch/tview-command"
)

// struct contains all the updatable elements of the Ui
type Ui struct {
	app   *tview.Application
	pages *tview.Pages

	// keybindings, managed by tview-command
	keyConfig       *tviewcommand.Config
	keyContextStack *tviewcommand.ContextStack

	// command registry, managed by stmps (for now)
	commandRegistry *commands.CommandRegistry

	// top bar
	startStopStatus *tview.TextView
	playerStatus    *tview.TextView

	// bottom bar
	menuWidget *MenuWidget

	// browser page
	browserPage *BrowserPage

	// queue page
	queuePage *QueuePage

	// playlist page
	playlistPage *PlaylistPage

	// search page
	searchPage *SearchPage

	// log page
	logPage *LogPage

	// modals
	addToPlaylistList    *tview.List
	messageBox           *tview.Modal
	helpModal            tview.Primitive
	helpWidget           *HelpWidget
	selectPlaylistModal  tview.Primitive
	selectPlaylistWidget *PlaylistSelectionWidget

	starIdList map[string]struct{}

	eventLoop   *eventLoop
	mpvEvents   chan mpvplayer.UiEvent
	mprisPlayer *remote.MprisPlayer

	playlists  []subsonic.SubsonicPlaylist
	connection *subsonic.SubsonicConnection
	player     *mpvplayer.Player
	logger     *logger.Logger
}

const (
	// page identifiers (use these instead of hardcoding page names for showing/hiding)
	PageBrowser   = "browser"
	PageQueue     = "queue"
	PagePlaylists = "playlists"
	PageSearch    = "search"
	PageLog       = "log"

	PageDeletePlaylist = "deletePlaylist"
	PageNewPlaylist    = "newPlaylist"
	PageAddToPlaylist  = "addToPlaylist"
	PageMessageBox     = "messageBox"
	PageHelpBox        = "helpBox"
	PageSelectPlaylist = "selectPlaylist"
)

func InitGui(
	tvcomConfig *tviewcommand.Config,
	tvcomContextStack *tviewcommand.ContextStack,
	commandRegistry *commands.CommandRegistry,
	indexes *[]subsonic.SubsonicIndex,
	connection *subsonic.SubsonicConnection,
	player *mpvplayer.Player,
	logger *logger.Logger,
	mprisPlayer *remote.MprisPlayer) (ui *Ui) {
	ui = &Ui{
		keyConfig:       tvcomConfig,
		keyContextStack: tvcomContextStack,

		starIdList: map[string]struct{}{},

		eventLoop: nil, // initialized by initEventLoops()
		mpvEvents: make(chan mpvplayer.UiEvent, 5),

		playlists:   []subsonic.SubsonicPlaylist{},
		connection:  connection,
		player:      player,
		logger:      logger,
		mprisPlayer: mprisPlayer,
	}

	logger.Print("tc: Init")
	ui.keyContextStack.Push("Init")
	ui.commandRegistry = commandRegistry

	ui.initEventLoops()

	ui.app = tview.NewApplication()
	ui.pages = tview.NewPages()

	// status text at the top
	statusLeft := fmt.Sprintf("[::b]%s[::-] v%s", clientName, clientVersion)
	ui.startStopStatus = tview.NewTextView().SetText(statusLeft).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true).
		SetScrollable(false)
	ui.startStopStatus.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return action, nil
	})

	statusRight := formatPlayerStatus(0, 0, 0)
	ui.playerStatus = tview.NewTextView().SetText(statusRight).
		SetTextAlign(tview.AlignRight).
		SetDynamicColors(true).
		SetScrollable(false)

	ui.menuWidget = ui.createMenuWidget()
	ui.helpWidget = ui.createHelpWidget()
	ui.selectPlaylistWidget = ui.createPlaylistSelectionWidget()

	// same as 'playlistList' except for the addToPlaylistModal
	// - we need a specific version of this because we need different keybinds
	ui.addToPlaylistList = tview.NewList().ShowSecondaryText(false)

	// message box for small notes
	ui.messageBox = tview.NewModal().
		SetText("hi there").
		SetBackgroundColor(tcell.ColorBlack)
	ui.messageBox.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		ui.pages.HidePage(PageMessageBox)
		return event
	})

	ui.selectPlaylistModal = makeModal(ui.selectPlaylistWidget.Root, 80, 5)

	// help box modal
	ui.helpModal = makeModal(ui.helpWidget.Root, 80, 30)
	ui.helpWidget.Root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Belts and suspenders. After the dialog is shown, this function will
		// _always_ be called. Therefore, check to ensure it's actually visible
		// before triggering on events. Also, don't close on every key, but only
		// ESC, like the help text says.
		if ui.helpWidget.visible && (event.Key() == tcell.KeyEscape) {
			ui.CloseHelp()
		}
		return event
	})

	// top bar: status text
	topBarFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(ui.startStopStatus, 0, 1, false).
		AddItem(ui.playerStatus, 20, 0, false)

	// browser page
	ui.browserPage = ui.createBrowserPage(indexes)

	// queue page
	ui.queuePage = ui.createQueuePage()

	// playlist page
	ui.playlistPage = ui.createPlaylistPage()

	// search page
	ui.searchPage = ui.createSearchPage()

	// log page
	ui.logPage = ui.createLogPage()

	ui.pages.AddPage(PageBrowser, ui.browserPage.Root, true, true).
		AddPage(PageQueue, ui.queuePage.Root, true, false).
		AddPage(PagePlaylists, ui.playlistPage.Root, true, false).
		AddPage(PageSearch, ui.searchPage.Root, true, false).
		AddPage(PageDeletePlaylist, ui.playlistPage.DeletePlaylistModal, true, false).
		AddPage(PageNewPlaylist, ui.playlistPage.NewPlaylistModal, true, false).
		AddPage(PageAddToPlaylist, ui.browserPage.AddToPlaylistModal, true, false).
		AddPage(PageSelectPlaylist, ui.selectPlaylistModal, true, false).
		AddPage(PageMessageBox, ui.messageBox, true, false).
		AddPage(PageHelpBox, ui.helpModal, true, false).
		AddPage(PageLog, ui.logPage.Root, true, false)

	rootFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(topBarFlex, 1, 0, false).
		AddItem(ui.pages, 0, 1, true).
		AddItem(ui.menuWidget.Root, 1, 0, false)

	// add commands
	logger.Print("tc: Adding Ui commands")
	ui.registerCommands(ui.commandRegistry)

	// add main input handler
	logger.Print("tc: Adding input handler")
	rootFlex.SetInputCapture(ui.handlePageInput)

	ui.app.SetRoot(rootFlex, true).
		SetFocus(rootFlex).
		EnableMouse(true)

	if !testMode {
		// this connects to the subsonic server, so exclude it for tests
		ui.playlistPage.UpdatePlaylists()
	}

	return ui
}

func (ui *Ui) Run() error {
	// receive events from mpv wrapper
	ui.player.RegisterEventConsumer(ui)

	// leave init key context
	ui.keyContextStack.PopExpect("Init")

	// run gui/background event handler
	ui.runEventLoops()

	// run mpv event handler
	go ui.player.EventLoop()

	// gui main loop (blocking)
	return ui.app.Run()
}

func (ui *Ui) ShowHelp() {
	ui.keyContextStack.Push("Help")
	activePage := ui.menuWidget.GetActivePage()
	ui.helpWidget.RenderHelp(activePage)

	ui.pages.ShowPage(PageHelpBox)
	ui.pages.SendToFront(PageHelpBox)
	ui.app.SetFocus(ui.helpModal)
	ui.helpWidget.visible = true
}

func (ui *Ui) CloseHelp() {
	ui.keyContextStack.PopExpect("Help")
	ui.helpWidget.visible = false
	ui.pages.HidePage(PageHelpBox)
}

func (ui *Ui) ShowSelectPlaylist() {
	ui.keyContextStack.Push("SelectPlaylist")
	ui.pages.ShowPage(PageSelectPlaylist)
	ui.pages.SendToFront(PageSelectPlaylist)
	ui.app.SetFocus(ui.selectPlaylistModal)
	ui.selectPlaylistWidget.visible = true
}

func (ui *Ui) CloseSelectPlaylist() {
	ui.keyContextStack.PopExpect("SelectPlaylist")
	ui.pages.HidePage(PageSelectPlaylist)
	ui.selectPlaylistWidget.visible = false
}

func (ui *Ui) showMessageBox(text string) {
	ui.pages.ShowPage(PageMessageBox)
	ui.messageBox.SetText(text)
	ui.app.SetFocus(ui.messageBox)
}
