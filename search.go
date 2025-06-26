// TODO: validate JSON plz
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
	username, err := validateSession(c)
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
	searchDB := c.Query("db") != "false"

	if !searchAlbums && !searchArtists && !searchTracks && !searchPlaylists {
		c.JSON(400, gin.H{"Error": "No search type provided"})
		return
	}

	var albums []Album
	var artists []Artist
	var tracks []Track
	var playlists []Playlist

	if searchDB {
		if searchAlbums {
			// get albums from db
			err := db.Select(&albums, "SELECT * FROM albums WHERE name LIKE ?", "%"+query+"%")
			if err != nil {
				fmt.Printf("Error getting albums %v", err)
			}
		}
		if searchArtists {
			// get artists from db
			err := db.Select(&artists, "SELECT * FROM artists WHERE name LIKE ?", "%"+query+"%")
			if err != nil {
				// do nothing cuz we need to search spotify
				fmt.Printf("Error getting artists %v", err)
			}
		}
		if searchTracks {
			// get tracks from db
			err := db.Select(&tracks, "SELECT * FROM tracks WHERE title LIKE ?", "%"+query+"%")
			if err != nil {
				fmt.Printf("Error getting tracks %v", err)
				// do nothing cuz we need to search spotify
			}
		}
		if searchPlaylists {
			// get playlists from db
			err := db.Select(&playlists, "SELECT * FROM playlists WHERE title LIKE ?", "%"+query+"%")
			if err != nil {
				c.JSON(500, gin.H{"Error": "Error getting playlists" + err.Error()})
				return
			}
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

		// albums
		if searchAlbums {
			for _, album := range spotifyResponse["albums"].(map[string]any)["items"].([]any) {
				image := album.(map[string]any)["images"].([]any)[0].(map[string]any)["url"].(string)
				smallimage := album.(map[string]any)["images"].([]any)[len(album.(map[string]any)["images"].([]any))-1].(map[string]any)["url"].(string)

				release_date := album.(map[string]any)["release_date"].(string)
				release_date_precision := album.(map[string]any)["release_date_precision"].(string)
				release_date_int, _ := parseReleaseDate(release_date, release_date_precision)

				new_album := Album{
					ID:          album.(map[string]any)["id"].(string),
					Title:       album.(map[string]any)["name"].(string),
					Image:       image,
					SmallImage:  smallimage,
					IsFull:      0,
					ReleaseDate: release_date_int,
				}
				err = AddAlbum(new_album)
				if err != nil {
					c.JSON(500, gin.H{"Error": "Error adding artist" + err.Error()})
					return
				}
				// add albums to search results
				spotify_albums = append(spotify_albums, new_album)

				// get artists for album
				for _, artist := range album.(map[string]any)["artists"].([]any) {
					new_artist := Artist{ID: artist.(map[string]any)["id"].(string), Name: artist.(map[string]any)["name"].(string), Image: ""}
					err = AddArtist(new_artist)
					if err != nil {
						c.JSON(500, gin.H{"Error": "Error adding artist" + err.Error()})
						return
					}
					err = AddAlbumArtist(Album_Artist{Artist_ID: new_artist.ID, Album_ID: new_album.ID})
					if err != nil {
						c.JSON(500, gin.H{"Error": "Error adding album_artist" + err.Error()})
						return
					}
				}
			}
		}

		// artists
		if searchArtists {
			for _, artist := range spotifyResponse["artists"].(map[string]any)["items"].([]any) {
				images := artist.(map[string]any)["images"].([]any)
				var imageURL string
				if len(images) > 0 {
					imageURL = images[0].(map[string]any)["url"].(string)
				} else {
					imageURL = "" // or a placeholder URL
				}

				new_artist := Artist{
					ID:    artist.(map[string]any)["id"].(string),
					Name:  artist.(map[string]any)["name"].(string),
					Image: imageURL,
				}
				err = AddArtist(new_artist)
				if err != nil {
					c.JSON(500, gin.H{"Error": "Error adding artist" + err.Error()})
					return
				}
				// add artists to search results
				spotify_artists = append(spotify_artists, new_artist)
			}
		}

		// tracks
		if searchTracks {
			for _, track := range spotifyResponse["tracks"].(map[string]any)["items"].([]any) {
				trackMap := track.(map[string]any)

				var albumID string
				if albumRaw, ok := trackMap["album"].(map[string]any); ok {
					if id, ok := albumRaw["id"].(string); ok {
						albumID = id
					}
				}

				// get album for track
				album_response := track.(map[string]any)["album"].(map[string]any)
				image := album_response["images"].([]any)[0].(map[string]any)["url"].(string)
				smallimage := album_response["images"].([]any)[len(album_response["images"].([]any))-1].(map[string]any)["url"].(string)

				new_track := Track{
					ID:           trackMap["id"].(string),
					Title:        trackMap["name"].(string),
					Album:        albumID,
					IsDownloaded: 0,
					Image:        image,
					SmallImage:   smallimage,
				} // if the track already exists, don't add it to the search results
				err = AddTrack(new_track)
				if err != nil {
					c.JSON(500, gin.H{"Error": "Error adding track" + err.Error()})
					return
				}
				// add tracks to search results
				spotify_tracks = append(spotify_tracks, new_track)

				// get artists for track
				for _, artist := range track.(map[string]any)["artists"].([]any) {
					// add album
					release_date := album_response["release_date"].(string)
					release_date_precision := album_response["release_date_precision"].(string)
					release_date_int, _ := parseReleaseDate(release_date, release_date_precision)

					new_album := Album{
						ID:          album_response["id"].(string),
						Title:       album_response["name"].(string),
						Image:       album_response["images"].([]any)[0].(map[string]any)["url"].(string),
						SmallImage:  album_response["images"].([]any)[len(album_response["images"].([]any))-1].(map[string]any)["url"].(string),
						IsFull:      0,
						ReleaseDate: release_date_int,
					}
					err = AddAlbum(new_album)
					if err != nil {
						c.JSON(500, gin.H{"Error": "Error adding album" + err.Error()})
						return
					}
					new_artist := Artist{ID: artist.(map[string]any)["id"].(string), Name: artist.(map[string]any)["name"].(string), Image: ""}
					err = AddArtist(new_artist)
					if err != nil {
						c.JSON(500, gin.H{"Error": "Error adding artist" + err.Error()})
						return
					}
					err = AddAlbumArtist(Album_Artist{Artist_ID: new_artist.ID, Album_ID: album_response["id"].(string)})
					if err != nil {
						c.JSON(500, gin.H{"Error": "Error adding album_artist" + err.Error()})
						return
					}
				}

			}
		}

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

// Parameters: username, password
// Returns: playlists
// gets playlists from db for the users account
func playlistsHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	playlists := []Playlist{}

	err = db.Select(&playlists, "SELECT * FROM playlists WHERE username = ?", username)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Error getting playlists" + err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"playlists": playlists,
	})
}

// Parameters: username, password, ids
// Returns: list of strings of artist names (comma separated) and a list of strings of artist ids
// gets album artists from db by album id
func albumArtistsHandler(c *gin.Context) {

	_, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	albumIDs := c.Query("ids")
	albumIDList := strings.Split(albumIDs, ",")
	albumArtistNames := []string{}
	albumArtistIDs := []string{}
	for _, albumID := range albumIDList {
		albumArtists := []Album_Artist{}
		err = db.Select(&albumArtists, "SELECT DISTINCT artist_id FROM album_artists WHERE album_id = ?", albumID)
		if err != nil {
			fmt.Printf("Error: %v", err)
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
		// make album artist names and ids into a list
		var artistIDsBuilder strings.Builder
		for i, albumArtist := range albumArtists {
			if i > 0 {
				artistIDsBuilder.WriteString(",")
			}
			artistIDsBuilder.WriteString(albumArtist.Artist_ID)
		}
		albumArtistIDs = append(albumArtistIDs, artistIDsBuilder.String())

		// get artist names from db
		artists := []Artist{}
		for _, albumArtist := range albumArtists {
			err = db.Select(&artists, "SELECT * FROM artists WHERE id = ?", albumArtist.Artist_ID)
			if err != nil {
				fmt.Printf("Error: %v", err)
				c.JSON(500, gin.H{"Error": "Internal server error"})
				return
			}
		}
		// use a strings.Builder for efficient string concatenation
		var artistNamesBuilder strings.Builder
		for i, artist := range artists {
			if i > 0 {
				artistNamesBuilder.WriteString(",")
			}
			artistNamesBuilder.WriteString(artist.Name)
		}
		albumArtistNames = append(albumArtistNames, artistNamesBuilder.String())
	}
	c.JSON(200, gin.H{
		"artistIDs":    albumArtistIDs,
		"artistsNames": albumArtistNames,
	})

}

// Parameters: username, password, id, download (bool, optinal)
// Returns: track
// gets track from db by track id, downloads track if download is true
func trackHandler(c *gin.Context) {

	_, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	id := c.Query("id")
	track := Track{}
	err = db.Get(&track, "SELECT * FROM tracks WHERE id = ?", id)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Error getting track" + err.Error()})
		return
	}
	if c.Query("download") == "true" {
		downloadQueueMutex.Lock()
		downloadQueue = append(downloadQueue, track.ID)
		downloadQueueMutex.Unlock()
		go downloadTracks()
	}
	c.JSON(200, gin.H{
		"track": track,
	})
}

// Parameters: username, password, id
// Returns: album
// gets album from db by album id
func albumHandler(c *gin.Context) {
	_, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	id := c.Query("id")
	album := Album{}
	err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", id)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Error getting album " + err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"album": album,
	})
}

// Parameters: username, password, id
// Returns: list of artist IDs (strings)
// gets album artists from db by album id
func albumArtistHandler(c *gin.Context) {
	_, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	albumID := c.Query("id")
	albumArtists := []Album_Artist{}
	err = db.Select(&albumArtists, "SELECT DISTINCT artist_id FROM album_artists WHERE album_id = ?", albumID)
	if err != nil {
		fmt.Printf("Error: %v", err)
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	// make into a list of artist IDs
	artistIDs := []string{}
	for _, albumArtist := range albumArtists {
		artistIDs = append(artistIDs, albumArtist.Artist_ID)
	}
	// get artist names from db
	artists := []Artist{}
	for _, artistID := range artistIDs {
		err = db.Select(&artists, "SELECT * FROM artists WHERE id = ?", artistID)
		if err != nil {
			fmt.Printf("Error: %v", err)
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
	}
	// make into a list of artist names
	artistNames := []string{}
	for _, artist := range artists {
		artistNames = append(artistNames, artist.Name)
	}

	c.JSON(200, gin.H{
		"artistIDs":    artistIDs,
		"artistsNames": artistNames,
	})
}

// Parameters: username, password, id
// Returns: artist
// gets artist from db by artist id
func artistHandler(c *gin.Context) {
	_, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	id := c.Query("id")
	artist := Artist{}
	err = db.Get(&artist, "SELECT * FROM artists WHERE id = ?", id)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	c.JSON(200, gin.H{
		"artist": artist,
	})
}

// Parameters: username, password, id
// Returns: playlist, tracks
// gets playlist from db by playlist id and all tracks in the playlist
func playlistHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	playlistID := c.Query("playlistID")
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
		"playlist": playlist,
	})
}

// Parameters: username, password, id
// Returns: albums, spotify_albums, number_of_albums, number_queried
// gets all albums for an artist from db and spotify
func artistAlbumsHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	artistID := c.Query("id")

	albums := []Album{}
	album_artists := []Album_Artist{}

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
		albums, err = getSpoifyAlbumsForArtist(userToken, artistID)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
		c.JSON(200, gin.H{
			"albums":           albums,
			"number_of_albums": len(albums),
		})
		return
	}
	// else get albums from db
	err = db.Select(&album_artists, "SELECT * FROM album_artists WHERE artist_id = ?", artistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	for _, album_artist := range album_artists {
		album := Album{}
		err := db.Get(&album, "SELECT * FROM albums WHERE id = ?", album_artist.Album_ID)
		if err != nil {
			fmt.Printf("Error: %v", err)
			c.JSON(500, gin.H{"Error": "Internal server error 2"})
			return
		}
		albums = append(albums, album)
	}
	c.JSON(200, gin.H{
		"albums":           albums,
		"number_of_albums": len(albums),
	})
}

// Parameters: username, password, id, spotify (optional), db (optional)
// Returns: tracks, spotify_tracks, number_of_tracks, number_queried
// gets all tracks for an album from db and spotify
func albumTracksHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	albumID := c.Query("id")

	// get album from db
	album := Album{}
	err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", albumID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	if album.IsFull != 0 {
		// album is already full, no need to search spotify
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
		return
	}

	userToken, err := getValidToken(username)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	tracks, err := getSpotifyTracksForAlbum(userToken, albumID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	// set album to full
	_, err = db.Exec("UPDATE albums SET isfull = ? WHERE id = ?", 1, albumID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	c.JSON(200, gin.H{
		"tracks":           tracks,
		"number_of_tracks": len(tracks),
	})
}

func getSpoifyAlbumsForArtist(userToken string, artistID string) ([]Album, error) {
	// search spotify
	index := 0
	limit, err := strconv.Atoi(spotify_query_limit)
	if err != nil {
		fmt.Printf("Error converting spotify_query_limit to int: %v\n", err)
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
			smallimage := album.(map[string]any)["images"].([]any)[len(album.(map[string]any)["images"].([]any))-1].(map[string]any)["url"].(string)

			// get release_date and release_date_precision
			release_date := album.(map[string]any)["release_date"].(string)
			release_date_precision := album.(map[string]any)["release_date_precision"].(string)
			release_date_int, _ := parseReleaseDate(release_date, release_date_precision)

			new_album := Album{
				ID:          album.(map[string]any)["id"].(string),
				Title:       album.(map[string]any)["name"].(string),
				Image:       album.(map[string]any)["images"].([]any)[0].(map[string]any)["url"].(string),
				SmallImage:  smallimage,
				IsFull:      0,
				ReleaseDate: release_date_int,
			}
			err = AddAlbum(new_album)
			if err != nil {
				fmt.Printf("Error adding album %v\n", err)
			}
			albumsList = append(albumsList, new_album)
		}
		index += limit
	}
	return albumsList, nil
}

func getSpotifyTracksForAlbum(userToken string, albumID string) ([]Track, error) {
	// search spotify
	index := 0
	limit, err := strconv.Atoi(spotify_query_limit)
	if err != nil {
		fmt.Printf("Error converting spotify_query_limit to int: %v\n", err)
		limit = 50
	}
	// get album
	album := Album{}
	err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", albumID)
	if err != nil {
		return nil, fmt.Errorf("error getting album from db: %v", err)
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
			new_track := Track{ID: track_id, Title: track.(map[string]any)["name"].(string), Album: albumID, IsDownloaded: 0, Image: album.Image, SmallImage: album.SmallImage}
			err = AddTrack(new_track)
			if err != nil {
				fmt.Printf("Error adding album %v\n", err)
			}
			tracksList = append(tracksList, new_track)
		}
		index += limit
	}
	return tracksList, nil
}

// Parameters: username, password, id, spotify (optional)
// Returns: url for the image
// gets image from spotify for an artist if it is not found in the db
func artistImageHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	artistID := c.Query("id")
	artist := Artist{}
	err = db.Get(&artist, "SELECT * FROM artists WHERE id = ?", artistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	if artist.Image == "" {
		get_spotify := c.Query("spotify") == "true"
		if get_spotify {
			userToken, err := getValidToken(username)
			if err != nil {
				c.JSON(500, gin.H{"Error": "Error getting user token" + err.Error()})
				return
			}
			url := "https://api.spotify.com/v1/artists/" + artistID
			headers := map[string]string{"Authorization": "Bearer " + userToken}
			// make request to spotify server
			resp, err := makeRequest(http.MethodGet, url, headers, nil, nil)
			if err != nil {
				c.JSON(500, gin.H{"Error": "Error searching Spotify"})
				return
			}
			// read response
			defer resp.Body.Close()
			var spotifyResponse map[string]any
			err = json.NewDecoder(resp.Body).Decode(&spotifyResponse)
			if err != nil {
				c.JSON(500, gin.H{"Error": "Internal server error"})
				return
			}
			fmt.Println("Spotify response: ", spotifyResponse)
			images, ok := spotifyResponse["images"].([]any)
			if ok && len(images) > 0 {
				imageMap, ok := images[0].(map[string]any)
				if ok {
					url, ok := imageMap["url"].(string)
					if ok {
						artist.Image = url
					}
				}
			}

			_, err = db.Exec("UPDATE artists SET image = ? WHERE id = ?", artist.Image, artistID)
			if err != nil {
				c.JSON(500, gin.H{"Error": "Internal server error"})
				return
			}
			_, err = db.Exec("UPDATE artists SET image = ? WHERE id = ?", artist.Image, artistID)
			if err != nil {
				c.JSON(500, gin.H{"Error": "Internal server error"})
				return
			}
		}
	}
	c.JSON(200, gin.H{
		"image": artist.Image,
	})
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
