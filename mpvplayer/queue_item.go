// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package mpvplayer

import (
	"github.com/spezifisch/stmps/remote"
)

type QueueItem struct {
	Id          string
	Uri         string
	Title       string
	Artist      string
	Duration    int
	Album       string
	TrackNumber int
	CoverArtId  string
	DiscNumber  int
	Year        int
}

var _ remote.TrackInterface = (*QueueItem)(nil)

func (q QueueItem) GetAlbumArtist() string {
	return q.Artist
}

func (q QueueItem) GetArtist() string {
	return q.Artist
}

func (q QueueItem) GetTitle() string {
	return q.Title
}

func (q QueueItem) GetDuration() int {
	return q.Duration
}

func (q QueueItem) IsValid() bool {
	return q.Id != ""
}

func (q QueueItem) GetId() string {
	return q.Id
}

func (q QueueItem) GetUri() string {
	return q.Uri
}

func (q QueueItem) GetAlbum() string {
	return q.Album
}

func (q QueueItem) GetTrackNumber() int {
	return q.TrackNumber
}

func (q QueueItem) GetDiscNumber() int {
	return q.DiscNumber
}

func (q QueueItem) GetYear() int {
	return q.Year
}
