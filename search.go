// TODO: validate JSON plz
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func makeRequest(method string, url string, headers map[string]string, params map[string]string, body url.Values) (*http.Response, error) {
	// create request
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body.Encode()))
	if err != nil {
		return nil, err
	}

	// add headers
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	// add query params
	if params != nil {
		q := req.URL.Query()
		for key, value := range params {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	// make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func generatePath(id string) string {
	return musicDir + "/" + id + "." + fileExtension
}

// Parameters: username, password, query, albums (optional), artists (optional), tracks (optional), playlists (optional), spotify (optional), db (optional)
// Returns: albums, artists, tracks, playlists spotify_albums, spotify_artists, spotify_tracks
// searches for albums, artists, tracks, and playlists in the database and spotify
func searchHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	// get query and username and password
	query := c.Query("query")

	if query == "" {
		c.JSON(400, gin.H{"Error": "No query provided"})
		return
	}

	searchAlbums := c.Query("albums") == "true"
	searchArtists := c.Query("artists") == "true"
	searchTracks := c.Query("tracks") == "true"
	searchPlaylists := c.Query("playlists") == "true"
	searchSpotify := c.Query("spotify") == "true"

	if !searchAlbums && !searchArtists && !searchTracks && !searchPlaylists {
		c.JSON(400, gin.H{"Error": "No search type provided"})
		return
	}

	var albums []Album
	var artists []Artist
	var tracks []Track
	var playlists []Playlist

	if searchAlbums {
		// get albums from db
		var temp_albums []Album
		err := db.Select(&temp_albums, "SELECT * FROM albums WHERE title LIKE ?", "%"+query+"%")
		if err != nil {
			fmt.Printf("Error getting albums %v", err)
		}
		for _, album := range temp_albums {
			if strings.EqualFold(album.Title, query) {
				// album found
				albums = append([]Album{album}, albums...)
			}
		}
	}
	if searchArtists {
		// get artists from db
		var temp_artists []Artist
		err := db.Select(&temp_artists, "SELECT * FROM artists WHERE name LIKE ?", "%"+query+"%")
		if err != nil {
			// do nothing cuz we need to search spotify
			fmt.Printf("Error getting artists %v", err)
		}
		for _, artist := range temp_artists {
			if strings.EqualFold(artist.Name, query) {
				// artist found
				artists = append([]Artist{artist}, artists...)
			}
		}
	}
	if searchTracks {
		// get tracks from db
		var temp_tracks []Track
		err := db.Select(&temp_tracks, "SELECT * FROM tracks WHERE title LIKE ?", "%"+query+"%")
		if err != nil {
			fmt.Printf("Error getting tracks %v", err)
			// do nothing cuz we need to search spotify
		}
		for _, track := range temp_tracks {
			if strings.EqualFold(track.Title, query) {
				// track found
				tracks = append([]Track{track}, tracks...)
			}
		}
	}
	if searchPlaylists {
		// get playlists from db
		var temp_playlists []Playlist
		err := db.Select(&temp_playlists, "SELECT * FROM playlists WHERE title LIKE ? AND username = ?", "%"+query+"%", username)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting playlists" + err.Error()})
			return
		}
		for _, playlist := range temp_playlists {
			if strings.EqualFold(playlist.Title, query) {
				// playlist found
				playlists = append([]Playlist{playlist}, playlists...)
			}
		}
	}

	if searchSpotify && (!searchAlbums && !searchArtists && !searchTracks) {
		searchSpotify = false
	} else if searchSpotify {
		if searchAlbums && len(albums) >= max_db_to_fetch {
			searchAlbums = false
		}
		if searchArtists && len(artists) >= max_db_to_fetch {
			searchArtists = false
		}
		if searchTracks && len(tracks) >= max_db_to_fetch {
			searchTracks = false
		}
	}

	spotify_albums := []Album{}
	spotify_artists := []Artist{}
	spotify_tracks := []Track{}
	if searchSpotify {
		// search spotify
		userToken, err := getValidToken(username)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting user token" + err.Error()})
			return
		}
		url := "https://api.spotify.com/v1/search"
		headers := map[string]string{"Authorization": "Bearer " + userToken}
		searchTypes := []string{}
		if searchAlbums {
			searchTypes = append(searchTypes, "album")
		}
		if searchArtists {
			searchTypes = append(searchTypes, "artist")
		}
		if searchTracks {
			searchTypes = append(searchTypes, "track")
		}
		if searchPlaylists {
			searchTypes = append(searchTypes, "playlist")
		}
		params := map[string]string{"q": query, "type": strings.Join(searchTypes, ","), "limit": spotify_query_limit}
		// make request to spotify server
		resp, err := makeRequest(http.MethodGet, url, headers, params, nil)

		if err != nil {
			c.JSON(500, gin.H{"Error": "Error searching Spotify" + err.Error()})
			return
		}

		// read response
		defer resp.Body.Close()
		var spotifyResponse map[string]any
		err = json.NewDecoder(resp.Body).Decode(&spotifyResponse)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error decoding Spotify response" + err.Error()})
			return
		}
		if resp.StatusCode != 200 {
			// return response body and status code if status code is not 200
			c.JSON(500, gin.H{"Error": "Error searching Spotify, status code not 200", "response": spotifyResponse, "status": resp.StatusCode})
			return
		}

		new_mini_artists := []string{}

		// tracks
		if searchTracks {
			items := spotifyResponse["tracks"].(map[string]any)["items"].([]interface{})
			for _, item := range items {
				itemMap := item.(map[string]any)
				if itemMap == nil {
					continue
				} else if itemMap["id"] == nil {
					continue
				}
				for _, dbTrack := range tracks {
					if dbTrack.ID == itemMap["id"].(string) {
						// track found in db, skip
						continue
					}
				}
				new_track, new_album, artistIDs, err := trackDataHandler(itemMap, userToken)
				if err != nil {
					c.JSON(500, gin.H{"Error": "Error getting track data" + err.Error()})
					return
				}
				AddTrack(new_track)
				AddAlbum(new_album)
				new_mini_artists = append(new_mini_artists, artistIDs...)
				spotify_tracks = append(spotify_tracks, new_track)
				spotify_albums = append(spotify_albums, new_album)
			}
		}

		// albums
		if searchAlbums {
			items := spotifyResponse["albums"].(map[string]any)["items"].([]interface{})
			for _, item := range items {
				itemMap := item.(map[string]any)
				if itemMap == nil {
					continue
				} else if itemMap["id"] == nil {
					continue
				}
				for _, dbAlbum := range albums {
					if dbAlbum.ID == itemMap["id"].(string) {
						// album found in db, skip
						continue
					}
				}
				// only proceed if album name matches query
				if itemMap["name"] != nil || !strings.Contains(strings.ToLower(itemMap["name"].(string)), strings.ToLower(query)) {
					continue
				}
				new_album, artistIDs, err := albumDataHandler(itemMap, userToken)
				if err != nil {
					c.JSON(500, gin.H{"Error": "Error getting album data" + err.Error()})
					return
				}
				AddAlbum(new_album)
				new_mini_artists = append(new_mini_artists, artistIDs...)
				spotify_albums = append(spotify_albums, new_album)
			}
		}

		// artists
		if searchArtists {
			items := spotifyResponse["artists"].(map[string]any)["items"].([]interface{})
			for _, item := range items {
				itemMap := item.(map[string]any)
				if itemMap == nil {
					continue
				} else if itemMap["id"] == nil {
					continue
				}
				// only proceed if artist name matches query
				for _, dbArtist := range artists {
					if dbArtist.ID == itemMap["id"].(string) {
						// artist found in db, skip
						continue
					}
				}
				new_artist, err := artistDataHandler(itemMap)
				if err != nil {
					c.JSON(500, gin.H{"Error": "Error getting artist data" + err.Error()})
					return
				}
				AddArtist(new_artist)

			}
		}

		// mini_artists
		mini_artists, err := miniArtistDataHandler(new_mini_artists, userToken)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting mini artist data" + err.Error()})
			return
		}
		for _, artist := range mini_artists {
			AddArtist(artist)
		}
		spotify_artists = append(spotify_artists, mini_artists...)

	}

	c.JSON(200, gin.H{
		"albums":          albums,
		"artists":         artists,
		"tracks":          tracks,
		"playlists":       playlists,
		"spotify_albums":  spotify_albums,
		"spotify_artists": spotify_artists,
		"spotify_tracks":  spotify_tracks,
	})
}

func trackDataHandler(track map[string]any, userToken string) (Track, Album, []string, error) {
	// given a track, get the data, and get the album as a string and the artists as a list of strings
	var trackData Track
	// get artists for track
	artistsNames := []string{}
	artistsIDs := []string{}
	album := track["album"]
	artists := track["artists"]
	for _, artist := range artists.([]any) {
		artistMap := artist.(map[string]any)
		artistsNames = append(artistsNames, artistMap["name"].(string))
		artistsIDs = append(artistsIDs, artistMap["id"].(string))
	}
	trackData = Track{
		ID:           track["id"].(string),
		Title:        track["name"].(string),
		AlbumID:      track["album"].(map[string]any)["id"].(string),
		AlbumName:    track["album"].(map[string]any)["name"].(string),
		ArtistsIDs:   artistsIDs,
		ArtistsNames: artistsNames,
		IsDownloaded: 0,
		Image:        track["album"].(map[string]any)["images"].([]any)[0].(map[string]any)["url"].(string),
		SmallImage:   track["album"].(map[string]any)["images"].([]any)[len(track["album"].(map[string]any)["images"].([]any))-1].(map[string]any)["url"].(string),
	}
	newAlbum, newIDs, err := albumDataHandler(album.(map[string]any), userToken)
	if err != nil {
		return trackData, Album{}, nil, err
	}
	artistsIDs = append(artistsIDs, newIDs...)
	return trackData, newAlbum, artistsIDs, nil
}

func miniArtistDataHandler(artistIDs []string, userToken string) ([]Artist, error) {
	// bulk-select existing artists by IDs, return them and identify missing IDs for further lookup
	if len(artistIDs) == 0 {
		return []Artist{}, nil
	}

	var existingArtists []Artist
	var newArtists []Artist

	query, args, err := sqlx.In("SELECT * FROM artists WHERE id IN (?)", artistIDs)
	if err != nil {
		return nil, err
	}
	query = db.Rebind(query)

	err = db.Select(&existingArtists, query, args...)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// build a map of existing IDs to detect missing ones
	existingMap := make(map[string]struct{}, len(existingArtists))
	for _, a := range existingArtists {
		existingMap[a.ID] = struct{}{}
	}

	var missingArtistIDs []string
	seen := make(map[string]struct{}, len(artistIDs))
	for _, id := range artistIDs {
		if _, ok := existingMap[id]; ok {
			continue
		}
		if _, s := seen[id]; s {
			continue
		}
		seen[id] = struct{}{}
		missingArtistIDs = append(missingArtistIDs, id)
	}

	if len(missingArtistIDs) > 0 {
		newArtists, err = searchSpotifyForArtists(missingArtistIDs, userToken)
		if err != nil {
			return nil, err
		}
	}

	return newArtists, nil
}

func searchSpotifyForArtists(artistIDs []string, userToken string) ([]Artist, error) {
	var newArtists []Artist
	// TODO: bulk get artists from Spotify for missingArtistIDs and insert into DB, then append to existingArtists
	// leaving this as a TODO to preserve existing behavior while fixing compilation/runtime errors
	url := "https://api.spotify.com/v1/artists"
	headers := map[string]string{"Authorization": "Bearer " + userToken}
	params := map[string]string{"ids": strings.Join(artistIDs, ",")}
	// make request to spotify server
	resp, err := makeRequest(http.MethodGet, url, headers, params, nil)
	if err != nil {
		return nil, fmt.Errorf("error searching Spotify: %v", err)
	}
	defer resp.Body.Close()
	var spotifyResponse map[string]any
	err = json.NewDecoder(resp.Body).Decode(&spotifyResponse)
	if err != nil {
		return nil, fmt.Errorf("error decoding Spotify response: %v", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error searching Spotify, status code not 200, status: %d, response: %v", resp.StatusCode, spotifyResponse)
	}
	for _, artist := range spotifyResponse["artists"].([]any) {
		artistMap := artist.(map[string]any)
		newArtist, err := artistDataHandler(artistMap)
		if err != nil {
			fmt.Printf("Error adding artist %v\n", err)
			continue
		}
		newArtists = append(newArtists, newArtist)
	}
	return newArtists, nil
}

func artistDataHandler(artist map[string]any) (Artist, error) {
	// given an artist map, extract the data and return an Artist struct
	images := artist["images"].([]any)

	var image string
	var smallImage string
	if len(images) > 0 {
		image = images[0].(map[string]any)["url"].(string)
		if len(images) > 1 {
			smallImage = images[len(images)-1].(map[string]any)["url"].(string)
		}
	}

	new_artist := Artist{
		ID:          artist["id"].(string),
		Name:        artist["name"].(string),
		Image:       image,
		SmallImage:  smallImage,
		LastUpdated: 0, // set to as far in the past as possible to show unset
	}
	return new_artist, nil
}

func albumDataHandler(albumMap map[string]any, userToken string) (Album, []string, error) {
	// given an album, get the data, and get the artists as a list of strings
	// get artists for album
	artists := albumMap["artists"].([]any)
	artistIDStrings := make([]string, len(artists))
	for i, artist := range artists {
		artistIDStrings[i] = artist.(map[string]any)["id"].(string)
	}
	date, err := parseReleaseDate(albumMap["release_date"].(string), albumMap["release_date_precision"].(string))
	if err != nil {
		return Album{}, nil, err
	}
	var image string
	if len(albumMap["images"].([]any)) == 0 {
		image = ""
	} else {
		image = albumMap["images"].([]any)[0].(map[string]any)["url"].(string)
	}

	var smallImage string
	if len(albumMap["images"].([]any)) > 2 {
		smallImage = albumMap["images"].([]any)[len(albumMap["images"].([]any))-1].(map[string]any)["url"].(string)
	} else {
		smallImage = image
	}
	newAlbum := Album{
		ID:          albumMap["id"].(string),
		Title:       albumMap["name"].(string),
		Image:       image,
		SmallImage:  smallImage,
		IsFull:      0,
		ReleaseDate: date,
		ArtistsIDs:  artistIDStrings,
		ArtistsNames: func() []string {
			names := make([]string, len(artists))
			for i, artist := range artists {
				names[i] = artist.(map[string]any)["name"].(string)
			}
			return names
		}(),
	}
	for _, artistID := range artistIDStrings {
		err = addArtistAlbum(artistID, newAlbum.ID)
		if err != nil {
			fmt.Printf("Error adding artist_album %v\n", err)
		}
	}
	return newAlbum, artistIDStrings, nil
}

func trackHandler(c *gin.Context) {

	_, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	ids := c.Query("ids")
	idsList := strings.Split(ids, ",")
	tracks := []Track{}
	for _, id := range idsList {
		if id != "" {
			var track Track
			err = db.Get(&track, "SELECT * FROM tracks WHERE id = ?", id)
			if err != nil {
				c.JSON(500, gin.H{"Error": "Error getting track" + err.Error()})
				return
			}
			tracks = append(tracks, track)
		}
	}
	if c.Query("download") == "true" {
		queueDownloads(idsList, true)
	}
	c.JSON(200, gin.H{
		"tracks": tracks,
	})
}

// will update album if isfull = 0
func albumHandler(c *gin.Context) {
	_, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	ids := c.Query("ids")
	idsList := strings.Split(ids, ",")
	albums := []Album{}
	for _, id := range idsList {
		var album Album
		err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", id)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting album " + err.Error()})
			return
		}
		albums = append(albums, album)
	}
	c.JSON(200, gin.H{
		"albums": albums,
	})
}

// will update artist if last_updated is older than a day
func artistHandler(c *gin.Context) {
	_, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	ids := c.Query("ids")
	idsList := strings.Split(ids, ",")
	artists := []Artist{}
	for _, id := range idsList {
		var artist Artist
		err = db.Get(&artist, "SELECT * FROM artists WHERE id = ?", id)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting artist " + err.Error()})
			return
		}
		artists = append(artists, artist)
	}
	c.JSON(200, gin.H{
		"artists": artists,
	})
}

// if id = "", get all playlists for user
func playlistHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	playlistID := c.Query("id")
	if playlistID == "" {
		playlists := []Playlist{}
		// just get all playlists for user
		db.Select(&playlists, "SELECT * FROM playlists WHERE username = ?", username)
		c.JSON(200, gin.H{
			"playlists": playlists,
		})
		return
	}

	isOwner, err := isPlaylistOwner(username, playlistID)
	if err != nil {
		c.JSON(403, gin.H{"Error": err.Error()})
		return
	}
	if !isOwner {
		c.JSON(403, gin.H{"Error": "You are not the owner of the playlist"})
		return
	}

	playlist := Playlist{}
	err = db.Get(&playlist, "SELECT * FROM playlists WHERE id = ?", playlistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	c.JSON(200, gin.H{
		"playlists": playlist,
	})
}

func getSpotifyAlbumsForArtist(userToken string, artistID string) ([]Album, error) {
	// search spotify
	index := 0
	limit, err := strconv.Atoi(spotify_query_limit)
	if err != nil {
		// fmt.Printf("Error converting spotify_query_limit to int: %v\n", err)
		limit = 50
	}
	number_of_albums := 0.0
	albumsList := []Album{}
	url := "https://api.spotify.com/v1/artists/" + artistID + "/albums"
	headers := map[string]string{"Authorization": "Bearer " + userToken}
	for number_of_albums == 0 || index < int(number_of_albums) {
		params := map[string]string{"limit": spotify_query_limit, "offset": strconv.Itoa(index), "include_groups": "album,single"}
		// make request to spotify server
		resp, err := makeRequest(http.MethodGet, url, headers, params, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var spotifyResponse map[string]any
		err = json.NewDecoder(resp.Body).Decode(&spotifyResponse)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("error searching Spotify, status code not 200")
		}

		number_of_albums = spotifyResponse["total"].(float64) // tells if we need to ask for more albums

		// new albums
		for _, album := range spotifyResponse["items"].([]any) {
			albumMap := album.(map[string]any)
			new_album, _, err := albumDataHandler(albumMap, userToken)
			if err != nil {
				fmt.Printf("Error handling album data: %v\n", err)
				continue
			}

			err = AddAlbum(new_album)
			if err != nil {
				fmt.Printf("Error adding album %v\n", err)
			}
			albumsList = append(albumsList, new_album)
		}
		index += limit
	}
	// update artist last updated time and the albums
	_, err = db.Exec("UPDATE artists SET last_updated = ? WHERE id = ?", time.Now().Unix(), artistID)
	if err != nil {
		fmt.Printf("Error updating artist last updated time: %v\n", err)
	}

	return albumsList, nil
}

func getSpotifyTracksForAlbum(userToken string, albumID string, image string, smallImage string) ([]Track, error) {
	// search spotify
	index := 0
	limit, err := strconv.Atoi(spotify_query_limit)
	if err != nil {
		// fmt.Printf("Error converting spotify_query_limit to int: %v\n", err)
		limit = 50
	}
	number_of_tracks := 0.0
	tracksList := []Track{}
	url := "https://api.spotify.com/v1/albums/" + albumID + "/tracks"
	headers := map[string]string{"Authorization": "Bearer " + userToken}
	for number_of_tracks == 0 || index < int(number_of_tracks) {
		params := map[string]string{"limit": spotify_query_limit, "offset": strconv.Itoa(index)}
		// make request to spotify server
		resp, err := makeRequest(http.MethodGet, url, headers, params, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var spotifyResponse map[string]any
		err = json.NewDecoder(resp.Body).Decode(&spotifyResponse)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("error searching Spotify, status code not 200")
		}

		number_of_tracks = spotifyResponse["total"].(float64) // tells if we need to ask for more tracks

		// new tracks
		for _, track := range spotifyResponse["items"].([]any) {
			track_id := track.(map[string]any)["id"].(string)
			new_track := Track{
				ID:           track_id,
				Title:        track.(map[string]any)["name"].(string),
				AlbumID:      albumID,
				IsDownloaded: 0,
				Image:        image,
				SmallImage:   smallImage,
				ArtistsIDs: func() []string {
					ids := make([]string, len(track.(map[string]any)["artists"].([]any)))
					for i, artist := range track.(map[string]any)["artists"].([]any) {
						ids[i] = artist.(map[string]any)["id"].(string)
					}
					return ids
				}(),
				ArtistsNames: func() []string {
					names := make([]string, len(track.(map[string]any)["artists"].([]any)))
					for i, artist := range track.(map[string]any)["artists"].([]any) {
						names[i] = artist.(map[string]any)["name"].(string)
					}
					return names
				}(),
			}
			err = AddTrack(new_track)
			if err != nil {
				fmt.Printf("Error adding track %v\n", err)
			}
			tracksList = append(tracksList, new_track)
		}
		index += limit
	}
	// update album last updated time and set to full
	_, err = db.Exec("UPDATE albums SET isfull = ? WHERE id = ?", 1, albumID)
	if err != nil {
		fmt.Printf("Error updating album isfull: %v\n", err)
	}
	return tracksList, nil
}

func parseReleaseDate(release_date string, release_date_precision string) (int, error) {
	parts := strings.Split(release_date, "-")
	var year, month, day int
	var err error

	switch release_date_precision {
	case "year":
		year, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid year: %v", err)
		}
		return year * 10000, nil

	case "month":
		if len(parts) < 2 {
			return 0, fmt.Errorf("month precision but no month part")
		}
		year, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid year: %v", err)
		}
		month, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid month: %v", err)
		}
		return year*10000 + month*100, nil

	case "day":
		if len(parts) < 3 {
			return 0, fmt.Errorf("day precision but incomplete date")
		}
		year, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid year: %v", err)
		}
		month, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid month: %v", err)
		}
		day, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, fmt.Errorf("invalid day: %v", err)
		}
		return year*10000 + month*100 + day, nil

	default:
		return 0, fmt.Errorf("unsupported precision: %s", release_date_precision)
	}
}

func artistTracksHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	artistID := c.Query("id")

	// get Artist from db
	artist := Artist{}
	err = db.Get(&artist, "SELECT * FROM artists WHERE id = ?", artistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	// see if the timestamp is older than a day
	if artist.LastUpdated < (time.Now().Unix() - 86400) {
		// search spotify to update
		userToken, err := getValidToken(username)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting user token" + err.Error()})
			return
		}
		_, err = getSpotifyAlbumsForArtist(userToken, artistID)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
	}
	// get tracks from db

	tracks := []Track{}
	query, args, err := sqlx.In("SELECT tracks.* FROM tracks where artist_id = ?", artistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	query = db.Rebind(query)

	err = db.Select(&tracks, query, args...)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	c.JSON(200, gin.H{
		"tracks":           tracks,
		"number_of_tracks": len(tracks),
	})
}

func artistAlbumsHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	artistID := c.Query("id")
	// get artist from db
	artist := Artist{}
	err = db.Get(&artist, "SELECT * FROM artists WHERE id = ?", artistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": err.Error()})
		return
	}
	albums := []Album{}

	// see if the timestamp is older than a day
	if artist.LastUpdated < (time.Now().Unix() - 86400) {
		userToken, err := getValidToken(username)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting user token" + err.Error()})
			return
		}

		albums, err = getSpotifyAlbumsForArtist(userToken, artistID)

		if err != nil {
			c.JSON(500, gin.H{"Error": err.Error()})
			return
		}
	} else {
		albumIDs := []string{}
		// get album ids from artist_albums
		err = db.Select(&albumIDs, "SELECT album_id FROM artist_albums WHERE artist_id = ?", artistID)
		if err != nil {
			c.JSON(500, gin.H{"Error": err.Error()})
			return
		}
		if len(albumIDs) == 0 {
			c.JSON(200, gin.H{
				"albums": albums,
			})
			return
		}
		for _, id := range albumIDs {
			var album Album
			err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", id)
			if err != nil {
				c.JSON(500, gin.H{"Error": err.Error()})
				return
			}
			albums = append(albums, album)
		}
	}
	c.JSON(200, gin.H{
		"albums": albums,
	})
}

func albumTracksHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	albumID := c.Query("id")

	// get Album from db
	album := Album{}
	err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", albumID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	// if album is not full, get tracks from spotify
	if album.IsFull == 0 {
		userToken, err := getValidToken(username)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Error getting user token" + err.Error()})
			return
		}
		_, err = getSpotifyTracksForAlbum(userToken, albumID, album.Image, album.SmallImage)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
	}
	// set isfull to 1
	_, err = db.Exec("UPDATE albums SET isfull = ? WHERE id = ?", 1, albumID)
	if err != nil {
		fmt.Printf("Error updating album isfull: %v\n", err)
	}

	// get tracks from db
	tracks := []Track{}
	err = db.Select(&tracks, "SELECT * FROM tracks WHERE album_id = ?", albumID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	c.JSON(200, gin.H{
		"tracks":           tracks,
		"number_of_tracks": len(tracks),
	})
}
