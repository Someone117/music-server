package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/gin-gonic/gin"
)

var hostPasswords = make(map[string]string) // host password (if set)
var clientsMutex sync.Mutex

var downloadQueue []string // queue of tracks to download
var downloadQueueMutex sync.Mutex

// Parameters: username, password, hostPw
// Returns: response code 200 if successful
// Sets password for listen along sessions, if password is blank, no listen along allowed
func setHostPasswordHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	newPassword := c.Query("hostPw")

	clientsMutex.Lock()
	hostPasswords[username] = newPassword // Store the new password
	clientsMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Host password updated"})
}

// Parameters: username, password, id, download (bool, optional)
// Returns: the track file
// Returns the track file for the given track ID
func playerHandler(c *gin.Context) {

	_, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	id := c.Query("id")
	track := Track{}

	download := c.Query("download") == "true"
	if download {
		err = db.Get(&track, "SELECT * FROM tracks WHERE id = ?", id)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
		if track.IsDownloaded == 0 {
			downloadQueueMutex.Lock()
			downloadQueue = append(downloadQueue, track.ID)
			downloadQueueMutex.Unlock()
			downloadTracks()
		}
		// try again
		err = db.Get(&track, "SELECT * FROM tracks WHERE id = ?", id)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
		if track.IsDownloaded == 0 {
			downloadQueueMutex.Lock()
			downloadQueue = append(downloadQueue, track.ID)
			downloadQueueMutex.Unlock()
			downloadTracks()
		}
	}

	// check again
	err = db.Get(&track, "SELECT * FROM tracks WHERE id = ?", id)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Error getting track"})
		return
	}

	// can't get
	if track.IsDownloaded == 0 {
		c.JSON(500, gin.H{"Error": "Track not downloaded"})
		return
	}

	c.Writer.WriteHeader(http.StatusOK)
	c.File(generatePath(track.ID))
}

// downloads track
func downloadTracks() {
	downloadQueueMutex.Lock()
	data := downloadQueue
	downloadQueue = []string{}
	downloadQueueMutex.Unlock()

	var urls string
	for _, trackID := range data {
		trackPath := generatePath(trackID)
		if _, err := os.Stat(trackPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			// Some other error occurred while checking the file
			log.Println("Error checking file: ", err.Error())
			continue
		}
		urls += "https://open.spotify.com/track/" + trackID + " "
	}

	// Download the track using spotdl
	cmd := exec.Command(musicDir+"/spotdl", "--format", fileExtension, "--output", "{track-id}", "download", urls)
	cmd.Dir = musicDir
	err := cmd.Run()
	// log error
	if err != nil {
		log.Println("Error downloading track: ", err.Error())
	}

	// Check if the tracks are downloaded
	for _, trackID := range data {
		trackPath := generatePath(trackID)
		if _, err := os.Stat(trackPath); err == nil {
			// Set the track as downloaded
			err = SetTrackDownloaded(trackID, 1)
			if err != nil {
				log.Println("Error setting track as downloaded: ", err.Error())
			}
		} else {
			// you need to redownload the track
			downloadQueueMutex.Lock()
			downloadQueue = append(downloadQueue, trackID)
			downloadQueueMutex.Unlock()
		}
	}
}

// Parameters: username, password, id, timestamp (int), volume (float), paused (bool), nextTrack
// Returns: response code 200 if successful
// Sets currently playing data for the user
func currentlyPlayingHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	version := c.Query("version")
	data := c.Query("data")

	currentlyPlayingMutex.Lock()
	currentlyPlayingMap[username] = Currently_Playing{Version: version, Data: data}
	currentlyPlayingMutex.Unlock()

	c.JSON(200, gin.H{"Message": "Currently playing updated"})
}

// Parameters: username, password, host, hostPw (optional if host is username)
// Returns: currently_playing
// Provides currently playing data for a host
func getCurrentlyPlayingHandler(c *gin.Context) {
	username, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	hostUsername := c.Query("host")

	if hostUsername == username {
		currentlyPlayingMutex.Lock()
		data, ok := currentlyPlayingMap[username]
		currentlyPlayingMutex.Unlock()
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"Error": "Not found"})
			return
		}
		c.JSON(200, gin.H{
			"currently_playing": data,
		})
		return
	}

	hostPassword := c.Query("hostPw")

	check_password, ok := hostPasswords[hostUsername]
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"Error": "Forbidden"})
		return
	}

	if check_password == hostPassword && check_password != "" {
		currentlyPlayingMutex.Lock()
		data, ok := currentlyPlayingMap[username]
		currentlyPlayingMutex.Unlock()
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"Error": "Not found"})
			return
		}
		c.JSON(200, gin.H{
			"currently_playing": data,
		})
		return
	}
	c.JSON(http.StatusForbidden, gin.H{"Error": "Forbidden"})
}
