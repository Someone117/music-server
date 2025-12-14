package main

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var playlistLock sync.Mutex

func isPlaylistOwner(username string, playlistID string) (bool, error) {
	var owner string
	err := db.QueryRow("SELECT username FROM playlists WHERE id = ?", playlistID).Scan(&owner)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // Playlist not found
		}
		return false, err // Other DB error
	}
	return owner == username, nil
}

// Parameters: username, password, playlistName
// Returns: playlist ID for the newly created playlist
// Creates a new playlist in the database ( TODO: and on Spotify in the future)
func createPlaylistHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	playlistName := c.Query("playlistName")
	if playlistName == "" {
		c.JSON(400, gin.H{"Error": "No playlist name provided"})
		return
	}

	// Create playlist in database
	_, err = db.Exec("INSERT INTO playlists (id, title, username, tracks, flags) VALUES (?, ?, ?, ?, ?)", playlistName+"_"+username, playlistName, username, JSONList{}, 0)
	if err != nil {
		fmt.Printf("Error creating playlist: %v\n", err)
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}

	c.JSON(200, gin.H{"playlistID": playlistName})
	// create playlist on Spotify
}

// Parameters: username, password, playlistID, trackID
// Returns: response code 200 if successful
// Adds a track to a playlist in the database (TODO: and on Spotify in the future)
func addTrackHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}
	
	playlistLock.Lock()
	defer playlistLock.Unlock()


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

	trackIDs := c.Query("trackIDs")
	if trackIDs == "" {
		c.JSON(400, gin.H{"Error": "No track IDs provided"})
		return
	}
	listOfIds := strings.Split(trackIDs, ",")

	// get the playlist object from the list
	var playlist Playlist
	err = db.QueryRow("SELECT * FROM playlists WHERE id = ?", playlistID).Scan(&playlist.ID, &playlist.Title, &playlist.Username, &playlist.Tracks, &playlist.Flags)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}

	// prepare existingTracks slice, trimming trailing commas and handling empty playlists
	// Append only unique track IDs to the existingTracks slice
	for _, trackID := range listOfIds {
		trackID = strings.TrimSpace(trackID)
		if trackID == "" {
			continue
		}
		if !slices.Contains(playlist.Tracks, trackID) {
			playlist.Tracks = append(playlist.Tracks, trackID)
		}
	}

	// Join tracks back together without a trailing comma
	// update playlist obj
	_, err = db.Exec("UPDATE playlists SET tracks = ? WHERE id = ?", playlist.Tracks, playlistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}

	c.JSON(200, gin.H{"Message": "Track added"})
}

func setPlaylistTracksHandler(c *gin.Context) {
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	playlistLock.Lock()
	defer playlistLock.Unlock()

	playlistID := c.Query("playlistID")
	isOwner, err := isPlaylistOwner(username, playlistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	if !isOwner {
		c.JSON(403, gin.H{"Error": "You are not the owner of the playlist"})
		return
	}

	trackIDs := c.Query("trackIDs")
	if trackIDs == "" {
		c.JSON(400, gin.H{"Error": "No track IDs provided"})
		return
	}

	listofIds := strings.Split(trackIDs, ",")
	var tracks JSONList
	// Then, make one request to check if all tracks exist
	for _, trackID := range listofIds {
		if trackID == "" {
			continue
		}
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM tracks WHERE id = ?)", trackID).Scan(&exists)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Database error"})
			return
		}
		if !exists {
			c.JSON(404, gin.H{"Error": "Track not found: " + trackID})
			return
		}
		// get playlist obj
		tracks = append(tracks, trackID)

	}
	// remove trailing comma
	// update playlist obj
	_, err = db.Exec("UPDATE playlists SET tracks = ? WHERE id = ?", tracks, playlistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}

	c.JSON(200, gin.H{"Message": "Track added"})
}

func setPlaylistNameHandler(c *gin.Context) {
	username, err := validateToken(c)
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
	playlistName := c.Query("playlistName")
	if playlistName == "" {
		c.JSON(400, gin.H{"Error": "No playlist name provided"})
		return
	}

	_, err = db.Exec("UPDATE playlists SET title = ? WHERE id = ?", playlistName, playlistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}

	c.JSON(200, gin.H{"Message": "Playlist name updated"})
}

func setPlaylistFlagsHandler(c *gin.Context) {
	username, err := validateToken(c)
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

	flags := c.Query("flags")
	// turn flags into int
	flags_int, err := strconv.ParseInt(flags, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"Error": "Invalid flags value"})
		return
	}

	_, err = db.Exec("UPDATE playlists SET flags = ? WHERE id = ?", flags_int, playlistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}

	c.JSON(200, gin.H{"Message": "Flags updated"})
}

// Parameters: username, password, playlistID
// Returns: response code 200 if successful
// Removes a track from a playlist in the database (TODO: and on Spotify in the future)
func removeTrackHandler(c *gin.Context) {
	username, err := validateToken(c)
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

	// Check if track exists in the database
	trackID := c.Query("trackID")
	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM tracks WHERE id = ?)", trackID).Scan(&exists)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}
	if !exists {
		c.JSON(404, gin.H{"Error": "Track not found"})
		return
	}

	// If the track is in the playlist, remove it
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM playlist_tracks WHERE playlist_id = ? AND track_id = ?)", playlistID, trackID).Scan(&exists)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}
	if !exists {
		c.JSON(400, gin.H{"Error": "Track not in playlist"})
		return
	}

	_, err = db.Exec("DELETE FROM playlist_tracks WHERE playlist_id = ? AND track_id = ?", playlistID, trackID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}

	c.JSON(200, gin.H{"Message": "Track removed"})
	// do the thing on spotify
}

// Parameters: username, password, playlistID
// Returns: response code 200 if successful
// Deletes a playlist from the user's account (TODO: and on Spotify in the future)
func deletePlaylistHandler(c *gin.Context) {
	username, err := validateToken(c)
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

	_, err = db.Exec("DELETE FROM playlists WHERE id = ?", playlistID)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Database error"})
		return
	}
	c.JSON(200, gin.H{"Message": "Playlist deleted"})
}
