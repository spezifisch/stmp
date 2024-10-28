// Copyright 2023 The STMPS Authors
// SPDX-License-Identifier: GPL-3.0-only

package subsonic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/spezifisch/stmps/logger"
)

type SubsonicConnection struct {
	Username         string
	Password         string
	Host             string
	PlaintextAuth    bool
	Scrobble         bool
	RandomSongNumber uint

	clientName    string
	clientVersion string

	logger         logger.LoggerInterface
	directoryCache map[string]SubsonicResponse
	coverArts      map[string]image.Image
}

func Init(logger logger.LoggerInterface) *SubsonicConnection {
	return &SubsonicConnection{
		clientName:    "example",
		clientVersion: "1.8.0",

		logger:         logger,
		directoryCache: make(map[string]SubsonicResponse),
		coverArts:      make(map[string]image.Image),
	}
}

func (s *SubsonicConnection) SetClientInfo(name, version string) {
	s.clientName = name
	s.clientVersion = version
}

func (s *SubsonicConnection) ClearCache() {
	s.directoryCache = make(map[string]SubsonicResponse)
}

func (s *SubsonicConnection) RemoveCacheEntry(key string) {
	delete(s.directoryCache, key)
}

func defaultQuery(connection *SubsonicConnection) url.Values {
	query := url.Values{}
	if connection.PlaintextAuth {
		query.Set("p", connection.Password)
	} else {
		token, salt := authToken(connection.Password)
		query.Set("t", token)
		query.Set("s", salt)
	}
	query.Set("u", connection.Username)
	query.Set("v", connection.clientVersion)
	query.Set("c", connection.clientName)
	query.Set("f", "json")

	return query
}

type Ider interface {
	ID() string
}

// response structs
type SubsonicError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type SubsonicArtist struct {
	Id         string `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"albumCount"`
}

func (s SubsonicArtist) ID() string {
	return s.Id
}

type SubsonicDirectory struct {
	Id       string           `json:"id"`
	Parent   string           `json:"parent"`
	Name     string           `json:"name"`
	Entities SubsonicEntities `json:"child"`
}

func (s SubsonicDirectory) ID() string {
	return s.Id
}

type SubsonicSongs struct {
	Song SubsonicEntities `json:"song"`
}

type SubsonicResults struct {
	Artist []Artist         `json:"artist"`
	Album  []Album          `json:"album"`
	Song   SubsonicEntities `json:"song"`
}

type ScanStatus struct {
	Scanning bool `json:"scanning"`
	Count    int  `json:"count"`
}

type PlayQueue struct {
	Current  string           `json:"current"`
	Position int              `json:"position"`
	Entries  SubsonicEntities `json:"entry"`
}

type GenreEntries struct {
	Genres []GenreEntry `json:"genre"`
}

type GenreEntry struct {
	SongCount  int    `json:"songCount"`
	AlbumCount int    `json:"albumCount"`
	Name       string `json:"value"`
}

type Artist struct {
	Id         string  `json:"id"`
	Name       string  `json:"name"`
	AlbumCount int     `json:"albumCount"`
	Album      []Album `json:"album"`
}

func (s Artist) ID() string {
	return s.Id
}

type Album struct {
	Id            string           `json:"id"`
	Created       string           `json:"created"`
	ArtistId      string           `json:"artistId"`
	Artist        string           `json:"artist"`
	Artists       []Artist         `json:"artists"`
	DisplayArtist string           `json:"displayArtist"`
	Title         string           `json:"title"`
	Album         string           `json:"album"`
	Name          string           `json:"name"`
	SongCount     int              `json:"songCount"`
	Duration      int              `json:"duration"`
	PlayCount     int              `json:"playCount"`
	Genre         string           `json:"genre"`
	Genres        []Genre          `json:"genres"`
	Year          int              `json:"year"`
	Song          SubsonicEntities `json:"song"`
	CoverArt      string           `json:"coverArt"`
}

func (s Album) ID() string {
	return s.Id
}

type Genre struct {
	Name string `json:"name"`
}

type SubsonicEntity struct {
	Id          string   `json:"id"`
	IsDirectory bool     `json:"isDir"`
	Parent      string   `json:"parent"`
	Title       string   `json:"title"`
	ArtistId    string   `json:"artistId"`
	Artist      string   `json:"artist"`
	Artists     []Artist `json:"artists"`
	Duration    int      `json:"duration"`
	Track       int      `json:"track"`
	DiscNumber  int      `json:"discNumber"`
	Path        string   `json:"path"`
	CoverArtId  string   `json:"coverArt"`
	Year        int      `json:"year"`
}

func (s SubsonicEntity) ID() string {
	return s.Id
}

// Return the title if present, otherwise fallback to the file path
func (e SubsonicEntity) GetSongTitle() string {
	if e.Title != "" {
		return e.Title
	}

	// we get around the weird edge case where a path ends with a '/' by just
	// returning nothing in that instance, which shouldn't happen unless
	// subsonic is being weird
	if e.Path == "" || strings.HasSuffix(e.Path, "/") {
		return ""
	}

	lastSlash := strings.LastIndex(e.Path, "/")

	if lastSlash == -1 {
		return e.Path
	}

	return e.Path[lastSlash+1 : len(e.Path)]
}

// SubsonicEntities is a sortable list of entities.
// Directories are first, then in alphabelical order. Entities are sorted by
// track number, if they have track numbers; otherwise, they're sorted
// alphabetically.
type SubsonicEntities []SubsonicEntity

func (s SubsonicEntities) Len() int      { return len(s) }
func (s SubsonicEntities) Swap(i, j int) { s[j], s[i] = s[i], s[j] }
func (s SubsonicEntities) Less(i, j int) bool {
	// Directories are before tracks, alphabetically
	if s[i].IsDirectory {
		if s[j].IsDirectory {
			return s[i].Title < s[j].Title
		}
		return true
	}
	// Disk and track numbers are only relevant within the same parent
	if s[i].Parent == s[j].Parent {
		// sort first by DiskNumber
		if s[i].DiscNumber == s[j].DiscNumber {
			// Tracks on the same disk are sorted by track
			return s[i].Track < s[j].Track
		}
		return s[i].DiscNumber < s[j].DiscNumber
	}
	// If we get here, the songs are either from different albums, or else
	// they're on the same disk

	return s[i].Title < s[j].Title
}

type SubsonicIndexes struct {
	Index []SubsonicIndex
}

type SubsonicIndex struct {
	Name    string           `json:"name"`
	Artists []SubsonicArtist `json:"artist"`
}

type SubsonicPlaylists struct {
	Playlists []SubsonicPlaylist `json:"playlist"`
}

type SubsonicPlaylist struct {
	Id        SubsonicId       `json:"id"`
	Name      string           `json:"name"`
	SongCount int              `json:"songCount"`
	Entries   SubsonicEntities `json:"entry"`
}

type SubsonicResponse struct {
	Status        string            `json:"status"`
	Version       string            `json:"version"`
	Indexes       SubsonicIndexes   `json:"indexes"`
	Directory     SubsonicDirectory `json:"directory"`
	RandomSongs   SubsonicSongs     `json:"randomSongs"`
	SimilarSongs  SubsonicSongs     `json:"similarSongs"`
	Starred       SubsonicResults   `json:"starred"`
	Playlists     SubsonicPlaylists `json:"playlists"`
	Playlist      SubsonicPlaylist  `json:"playlist"`
	Error         SubsonicError     `json:"error"`
	Artist        Artist            `json:"artist"`
	Album         Album             `json:"album"`
	SearchResults SubsonicResults   `json:"searchResult3"`
	ScanStatus    ScanStatus        `json:"scanStatus"`
	PlayQueue     PlayQueue         `json:"playQueue"`
	Genres        GenreEntries      `json:"genres"`
	SongsByGenre  SubsonicSongs     `json:"songsByGenre"`
}

type responseWrapper struct {
	Response SubsonicResponse `json:"subsonic-response"`
}

type SubsonicId string

func (si *SubsonicId) UnmarshalJSON(b []byte) error {
	if b[0] == '"' {
		return json.Unmarshal(b, (*string)(si))
	}
	var i int
	if err := json.Unmarshal(b, &i); err != nil {
		return err
	}
	s := strconv.Itoa(i)
	*si = SubsonicId(s)
	return nil
}

// requests
func (connection *SubsonicConnection) GetServerInfo() (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	requestUrl := connection.Host + "/rest/ping" + "?" + query.Encode()
	return connection.getResponse("GetServerInfo", requestUrl)
}

func (connection *SubsonicConnection) GetIndexes() (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	requestUrl := connection.Host + "/rest/getIndexes" + "?" + query.Encode()
	return connection.getResponse("GetIndexes", requestUrl)
}

func (connection *SubsonicConnection) GetArtist(id string) (*SubsonicResponse, error) {
	if cachedResponse, present := connection.directoryCache[id]; present {
		return &cachedResponse, nil
	}

	query := defaultQuery(connection)
	query.Set("id", id)
	requestUrl := connection.Host + "/rest/getArtist" + "?" + query.Encode()
	resp, err := connection.getResponse("GetMusicDirectory", requestUrl)
	if err != nil {
		return resp, err
	}

	// on a sucessful request, cache the response
	if resp.Status == "ok" {
		connection.directoryCache[id] = *resp
	}

	sort.Sort(resp.Directory.Entities)

	return resp, nil
}

func (connection *SubsonicConnection) GetAlbum(id string) (*SubsonicResponse, error) {
	if cachedResponse, present := connection.directoryCache[id]; present {
		// This is because Albums that were fetched as Directories aren't populated correctly
		if cachedResponse.Album.Name != "" {
			return &cachedResponse, nil
		}
	}

	query := defaultQuery(connection)
	query.Set("id", id)
	requestUrl := connection.Host + "/rest/getAlbum" + "?" + query.Encode()
	resp, err := connection.getResponse("GetAlbum", requestUrl)
	if err != nil {
		return resp, err
	}

	// on a sucessful request, cache the response
	if resp.Status == "ok" {
		connection.directoryCache[id] = *resp
	}

	sort.Sort(resp.Directory.Entities)

	return resp, nil
}

func (connection *SubsonicConnection) GetMusicDirectory(id string) (*SubsonicResponse, error) {
	if cachedResponse, present := connection.directoryCache[id]; present {
		return &cachedResponse, nil
	}

	query := defaultQuery(connection)
	query.Set("id", id)
	requestUrl := connection.Host + "/rest/getMusicDirectory" + "?" + query.Encode()
	resp, err := connection.getResponse("GetMusicDirectory", requestUrl)
	if err != nil {
		return resp, err
	}

	// on a sucessful request, cache the response
	if resp.Status == "ok" {
		connection.directoryCache[id] = *resp
	}

	sort.Sort(resp.Directory.Entities)

	return resp, nil
}

// GetCoverArt fetches album art from the server, by ID. The results are cached,
// so it is safe to call this function repeatedly. If id is empty, an error
// is returned. If, for some reason, the server response can't be parsed into
// an image, an error is returned. This function can parse GIF, JPEG, and PNG
// images.
func (connection *SubsonicConnection) GetCoverArt(id string) (image.Image, error) {
	if id == "" {
		return nil, fmt.Errorf("GetCoverArt: no ID provided")
	}
	if rv, ok := connection.coverArts[id]; ok {
		return rv, nil
	}
	query := defaultQuery(connection)
	query.Set("id", id)
	query.Set("f", "image/png")
	caller := "GetCoverArt"
	res, err := http.Get(connection.Host + "/rest/getCoverArt" + "?" + query.Encode())
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to make GET request: %v", caller, err)
	}

	if res.Body != nil {
		defer res.Body.Close()
	} else {
		return nil, fmt.Errorf("[%s] response body is nil", caller)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[%s] unexpected status code: %d, status: %s", caller, res.StatusCode, res.Status)
	}

	if len(res.Header["Content-Type"]) == 0 {
		return nil, fmt.Errorf("[%s] unknown image type (no content-type from server)", caller)
	}
	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to read response body: %v", caller, err)
	}
	var art image.Image
	switch res.Header["Content-Type"][0] {
	case "image/png":
		art, err = png.Decode(bytes.NewReader(responseBody))
	case "image/jpeg":
		art, err = jpeg.Decode(bytes.NewReader(responseBody))
	case "image/gif":
		art, err = gif.Decode(bytes.NewReader(responseBody))
	default:
		return nil, fmt.Errorf("[%s] unhandled image type %s: %v", caller, res.Header["Content-Type"][0], err)
	}
	if art != nil {
		// FIXME connection.coverArts shouldn't grow indefinitely. Add some LRU cleanup after loading a few hundred cover arts.
		connection.coverArts[id] = art
	}
	return art, err
}

func (connection *SubsonicConnection) GetRandomSongs(Id string, randomType string) (*SubsonicResponse, error) {
	query := defaultQuery(connection)

	// Set the default size for random/similar songs, clamped to 500
	size := "50"
	if connection.RandomSongNumber > 0 && connection.RandomSongNumber < 500 {
		size = strconv.FormatInt(int64(connection.RandomSongNumber), 10)
	}

	switch randomType {
	case "random":
		query.Set("size", size)
		requestUrl := connection.Host + "/rest/getRandomSongs?" + query.Encode()
		return connection.getResponse("GetRandomSongs", requestUrl)

	case "similar":
		query.Set("id", Id)
		query.Set("count", size)
		requestUrl := connection.Host + "/rest/getSimilarSongs?" + query.Encode()
		return connection.getResponse("GetSimilar", requestUrl)

	default:
		query.Set("size", size)
		requestUrl := connection.Host + "/rest/getRandomSongs?" + query.Encode()
		return connection.getResponse("GetRandomSongs", requestUrl)
	}
}

func (connection *SubsonicConnection) ScrobbleSubmission(id string, isSubmission bool) (resp *SubsonicResponse, err error) {
	query := defaultQuery(connection)
	query.Set("id", id)

	// optional field, false for "now playing", true for "submission"
	query.Set("submission", strconv.FormatBool(isSubmission))

	requestUrl := connection.Host + "/rest/scrobble" + "?" + query.Encode()
	resp, err = connection.getResponse("ScrobbleSubmission", requestUrl)
	return
}

func (connection *SubsonicConnection) GetStarred() (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	requestUrl := connection.Host + "/rest/getStarred" + "?" + query.Encode()
	resp, err := connection.getResponse("GetStarred", requestUrl)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

func (connection *SubsonicConnection) ToggleStar(id string, starredItems map[string]struct{}) (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	query.Set("id", id)

	_, ok := starredItems[id]
	var action = "star"
	// If the key exists, we're unstarring
	if ok {
		action = "unstar"
	}

	requestUrl := connection.Host + "/rest/" + action + "?" + query.Encode()
	resp, err := connection.getResponse("ToggleStar", requestUrl)
	if err != nil {
		if ok {
			delete(starredItems, id)
		} else {
			starredItems[id] = struct{}{}
		}
		return resp, err
	}
	return resp, nil
}

func (connection *SubsonicConnection) GetPlaylists() (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	requestUrl := connection.Host + "/rest/getPlaylists" + "?" + query.Encode()
	resp, err := connection.getResponse("GetPlaylists", requestUrl)
	if err != nil {
		return resp, err
	}

	for i := 0; i < len(resp.Playlists.Playlists); i++ {
		playlist := &resp.Playlists.Playlists[i]

		if playlist.SongCount == 0 {
			continue
		}

		response, err := connection.GetPlaylist(string(playlist.Id))

		if err != nil {
			return nil, err
		}

		playlist.Entries = response.Playlist.Entries
	}

	return resp, nil
}

func (connection *SubsonicConnection) GetPlaylist(id string) (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	query.Set("id", id)

	requestUrl := connection.Host + "/rest/getPlaylist" + "?" + query.Encode()
	return connection.getResponse("GetPlaylist", requestUrl)
}

// CreatePlaylist creates or updates a playlist on the server.
// If id is provided, the existing playlist with that ID is updated with the new song list.
// If name is provided, a new playlist is created with the song list.
// Either id or name _must_ be populated, or the function returns an error.
// If _both_ id and name are poplated, the function returns an error.
// songIds may be nil, in which case the new playlist is created empty, or all
// songs are removed from the existing playlist.
func (connection *SubsonicConnection) CreatePlaylist(id, name string, songIds []string) (*SubsonicResponse, error) {
	if (id == "" && name == "") || (id != "" && name != "") {
		return nil, errors.New("CreatePlaylist: exactly one of id or name must be provided")
	}
	query := defaultQuery(connection)
	if id != "" {
		query.Set("id", id)
	} else {
		query.Set("name", name)
	}
	for _, sid := range songIds {
		query.Add("songId", sid)
	}
	requestUrl := connection.Host + "/rest/createPlaylist" + "?" + query.Encode()
	return connection.getResponse("GetPlaylist", requestUrl)
}

func (connection *SubsonicConnection) getResponse(caller, requestUrl string) (*SubsonicResponse, error) {
	res, err := http.Get(requestUrl)
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to make GET request: %v", caller, err)
	}

	if res.Body != nil {
		defer res.Body.Close()
	} else {
		return nil, fmt.Errorf("[%s] response body is nil", caller)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[%s] unexpected status code: %d, status: %s", caller, res.StatusCode, res.Status)
	}

	responseBody, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, fmt.Errorf("[%s] failed to read response body: %v", caller, readErr)
	}

	var decodedBody responseWrapper
	err = json.Unmarshal(responseBody, &decodedBody)
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to unmarshal response body: %v", caller, err)
	}

	return &decodedBody.Response, nil
}

func (connection *SubsonicConnection) DeletePlaylist(id string) error {
	query := defaultQuery(connection)
	query.Set("id", id)
	requestUrl := connection.Host + "/rest/deletePlaylist" + "?" + query.Encode()
	_, err := http.Get(requestUrl)
	return err
}

func (connection *SubsonicConnection) AddSongToPlaylist(playlistId string, songId string) error {
	query := defaultQuery(connection)
	query.Set("playlistId", playlistId)
	query.Set("songIdToAdd", songId)
	requestUrl := connection.Host + "/rest/updatePlaylist" + "?" + query.Encode()
	_, err := http.Get(requestUrl)
	return err
}

func (connection *SubsonicConnection) RemoveSongFromPlaylist(playlistId string, songIndex int) error {
	query := defaultQuery(connection)
	query.Set("playlistId", playlistId)
	query.Set("songIndexToRemove", strconv.Itoa(songIndex))
	requestUrl := connection.Host + "/rest/updatePlaylist" + "?" + query.Encode()
	_, err := http.Get(requestUrl)
	return err
}

// note that this function does not make a request, it just formats the play url
// to pass to mpv
func (connection *SubsonicConnection) GetPlayUrl(entity *SubsonicEntity) string {
	// we don't want to call stream on a directory
	if entity.IsDirectory {
		return ""
	}

	query := defaultQuery(connection)
	query.Set("id", entity.Id)
	return connection.Host + "/rest/stream" + "?" + query.Encode()
}

// Search uses the Subsonic search3 API to query a server for all songs that have
// ID3 tags that match the query. The query is global, in that it matches in any
// ID3 field.
// https://www.subsonic.org/pages/api.jsp#search3
func (connection *SubsonicConnection) Search(searchTerm string, artistOffset, albumOffset, songOffset int) (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	query.Set("query", searchTerm)
	query.Set("artistOffset", strconv.Itoa(artistOffset))
	query.Set("albumOffset", strconv.Itoa(albumOffset))
	query.Set("songOffset", strconv.Itoa(songOffset))
	requestUrl := connection.Host + "/rest/search3" + "?" + query.Encode()
	res, err := connection.getResponse("Search", requestUrl)
	return res, err
}

// StartScan tells the Subsonic server to initiate a media library scan. Whether
// this is a deep or surface scan is dependent on the server implementation.
// https://subsonic.org/pages/api.jsp#startScan
func (connection *SubsonicConnection) StartScan() error {
	query := defaultQuery(connection)
	requestUrl := fmt.Sprintf("%s/rest/startScan?%s", connection.Host, query.Encode())
	if res, err := connection.getResponse("StartScan", requestUrl); err != nil {
		return err
	} else if !res.ScanStatus.Scanning {
		return fmt.Errorf("server returned false for scan status on scan attempt")
	}
	return nil
}

func (connection *SubsonicConnection) SavePlayQueue(queueIds []string, current string, position int) error {
	query := defaultQuery(connection)
	for _, songId := range queueIds {
		query.Add("id", songId)
	}
	query.Set("current", current)
	query.Set("position", fmt.Sprintf("%d", position))
	requestUrl := fmt.Sprintf("%s/rest/savePlayQueue?%s", connection.Host, query.Encode())
	_, err := connection.getResponse("SavePlayQueue", requestUrl)
	return err
}

func (connection *SubsonicConnection) LoadPlayQueue() (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	requestUrl := fmt.Sprintf("%s/rest/getPlayQueue?%s", connection.Host, query.Encode())
	return connection.getResponse("GetPlayQueue", requestUrl)
}

func (connection *SubsonicConnection) GetGenres() (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	requestUrl := connection.Host + "/rest/getGenres" + "?" + query.Encode()
	resp, err := connection.getResponse("GetGenres", requestUrl)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

func (connection *SubsonicConnection) GetSongsByGenre(genre string, offset int, musicFolderID string) (*SubsonicResponse, error) {
	query := defaultQuery(connection)
	query.Add("genre", genre)
	if offset != 0 {
		query.Add("offset", strconv.Itoa(offset))
	}
	if musicFolderID != "" {
		query.Add("musicFolderId", musicFolderID)
	}
	requestUrl := connection.Host + "/rest/getSongsByGenre" + "?" + query.Encode()
	resp, err := connection.getResponse("GetPlaylists", requestUrl)
	if err != nil {
		return resp, err
	}
	return resp, nil
}
