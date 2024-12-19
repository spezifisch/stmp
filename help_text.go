// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package main

const helpPlayback = `
p      play/pause
P      stop
>      next song
-/=(+) volume down/volume up
,/.    seek -10/+10 seconds
r      add 50 random songs to queue
c      start server library sCan
`

const helpPageBrowser = `
artist tab
  R     refresh the list
  /     Search artists
  a     Add all artist songs to queue
  n     Continue search forward
  N     Continue search backwards
song tab
  ENTER play song (clears current queue)
  a     add album or song to queue
  A     add song to playlist
  y     toggle star on song/album
  R     refresh the list
ESC   Close search
`

const helpPageQueue = `
d/DEL remove currently selected song from the queue
D     remove all songs from queue
y     toggle star on song
k     move selected song up in queue
j     move selected song down in queue
s     save queue as a playlist
S     shuffle the current queue
l     load last queue from server
`

const helpPagePlaylists = `
n     new playlist
d     delete playlist
a     add playlist or song to queue
`

const helpSearchPage = `
artist, album, or song column
  Down/Up navigate within the column
  Left    previous column
  Right   next column
  Enter/a recursively add item to quue
  /       start search
search field
  Enter   search for text
  Esc     cancel search

Note: unlike browser, columns navigate
 search results, not selected items.
`
