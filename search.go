// TODO: validate JSON plz
package main

import (
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
	req, err := http.NewRequest(method, url, strings.NewReader(body.Encode()))
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

func generatePath(id int) string {
	return musicDir + "/" + strconv.Itoa(id) + "." + fileExtension
}

func getRelativePath(id int) string {
	return strconv.Itoa(id) + "." + fileExtension
}

func getIntermediateRelativePath(id int, intermediateExtension string) string {
	return strconv.Itoa(id) + "." + intermediateExtension
}

func searchHandler(c *gin.Context) {
	_, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	query := c.Query("query")
	if query == "" {
		c.JSON(400, gin.H{"Error": "No search query provided"})
		return
	}
	searchType := c.Query("type")
	if searchType == "" {
		c.JSON(400, gin.H{"Error": "No search type provided"})
		return
	}
	url := getURL(false)
	result, err := search(url, query, searchType, 0, 20)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Error performing search: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"result": result,
	})
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
		for _, track := range tracks {
			queueDownloads(track.ID, true)
		}
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
		c.JSON(500, gin.H{"Error": "Internal DB error"})
		return
	}

	c.JSON(200, gin.H{
		"playlists": playlist,
	})
}

func artistTracksHandler(c *gin.Context) {
	_, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	ID := c.Query("id")
	artistID, err := strconv.Atoi(ID)
	if err != nil {
		c.JSON(400, gin.H{"Error": "Invalid artist ID"})
		return
	}

	// get Artist from db
	artist := Artist{}
	err = db.Get(&artist, "SELECT * FROM artists WHERE id = ?", artistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal DB error"})
		return
	}

	// see if the timestamp is older than a day
	if artist.LastUpdated < (time.Now().Unix() - 86400) {
		url := getURL(false)
		albums, err := getArtistAlbums(url, artist.ID)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal Service error"})
			return
		}
		tracks := []Track{}
		for _, album := range albums {
			// get tracks for album and add to db
			trackSet, err := getAlbumTracks(url, album.ID)
			if err != nil {
				c.JSON(500, gin.H{"Error": "Internal Service error"})
				return
			}
			tracks = append(tracks, trackSet...)
		}
	}
	// get tracks from db

	tracks := []Track{}
	query, args, err := sqlx.In("SELECT tracks.* FROM tracks where artist_id = ?", artistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal DB error"})
		return
	}
	query = db.Rebind(query)

	err = db.Select(&tracks, query, args...)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal DB error"})
		return
	}

	c.JSON(200, gin.H{
		"tracks":           tracks,
		"number_of_tracks": len(tracks),
	})
}

func artistAlbumsHandler(c *gin.Context) {
	_, err := validateToken(c)
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
		url := getURL(false)
		albums, err = getArtistAlbums(url, artist.ID)

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
	_, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	albumID := c.Query("id")

	// get Album from db
	album := Album{}
	err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", albumID)
	if err != nil {
		fmt.Println("Error DB: %s", err.Error())
		c.JSON(500, gin.H{"Error": "Internal DB error"})
		return
	}

	// if album is not full, get tracks from spotify
	if album.IsFull != 1 {
		url := getURL(false)
		tracks, err := getAlbumTracks(url, album.ID)
		if err != nil {
			fmt.Println("Error Service: %s", err.Error())
			c.JSON(500, gin.H{"Error": "Internal Service error"})
			return
		}
		for _, track := range tracks {
			err = AddTrack(track)
			if err != nil {
				fmt.Println("Error DB: %s", err.Error())
				c.JSON(500, gin.H{"Error": "Internal DB error"})
				return
			}
			for _, artistID := range track.ArtistsIDs {
				err = addArtistAlbum(artistID, track.AlbumID)
				if err != nil {
					fmt.Println("Error DB: %s", err.Error())
					c.JSON(500, gin.H{"Error": "Internal DB error"})
					return
				}
			}
		}
		// set isfull to 1
		_, err = db.Exec("UPDATE albums SET isfull = ? WHERE id = ?", 1, albumID)
		if err != nil {
			fmt.Printf("Error updating album isfull: %v\n", err)
		}

		c.JSON(200, gin.H{
			"tracks":           tracks,
			"number_of_tracks": len(tracks),
		})
	} else {
		tracks := []Track{}
		err = db.Select(&tracks, "SELECT * FROM tracks WHERE album_id = ?", albumID)
		if err != nil {
			fmt.Println("Error DB: %s", err.Error())
			c.JSON(500, gin.H{"Error": "Internal DB error"})
			return
		}
		c.JSON(200, gin.H{
			"tracks":           tracks,
			"number_of_tracks": len(tracks),
		})
	}
}

var streamUrls []string
var apiUrls []string

func getURL(isStreaming bool) string {
	if len(streamUrls) == 0 || len(apiUrls) == 0 {
		resp, err := makeRequest("GET", "https://tidal-uptime.jiffy-puffs-1j.workers.dev/", nil, nil, nil)
		// get for each [streaming] get [url]
		if err != nil {
			var result struct {
				Streaming []struct {
					URL string `json:"url"`
				} `json:"streaming"`
				API []struct {
					URL string `json:"url"`
				} `json:"api"`
			}
			err := json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				fmt.Printf("Error decoding response: %v\n", err)
				return "https://us-west.monochrome.tf"
			}
			for _, stream := range result.Streaming {
				streamUrls = append(streamUrls, stream.URL)
			}
			for _, api := range result.API {
				apiUrls = append(apiUrls, api.URL)
			}
		} else {
			fmt.Println("Error fetching URLs, using default")
			return "https://us-west.monochrome.tf"
		}
	}
	// get randIndex for streamUrls and apiUrls
	if isStreaming {
		randIndex := time.Now().UnixNano() % int64(len(streamUrls))
		return streamUrls[randIndex]
	} else {
		randIndex := time.Now().UnixNano() % int64(len(apiUrls))
		return apiUrls[randIndex]
	}
}
