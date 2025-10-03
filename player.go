package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var hostPasswords = make(map[string]string) // host password (if set)
var clientsMutex sync.Mutex

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
			downloadTracks([]string{track.ID})
		}
		// try again
		err = db.Get(&track, "SELECT * FROM tracks WHERE id = ?", id)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
		if track.IsDownloaded == 0 {
			// if the file exists, set it as downloaded
			trackPath := generatePath(track.ID)
			if _, err := os.Stat(trackPath); err == nil {
				err = SetTrackDownloaded(track.ID, 1)
				if err != nil {
					log.Println("Error setting track as downloaded: ", err.Error())
				}
			} else if !os.IsNotExist(err) {
				// Some other error occurred while checking the file
				log.Println("Error checking file: ", err.Error())
			}
			// if still not downloaded, add to queue
			if track.IsDownloaded == 0 {
				c.JSON(500, gin.H{"Error": "Track not downloaded yet, try again later"})
				return
			}

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

	c.File(generatePath(track.ID))
}

// downloads track
func downloadTracks(data []string) {

	if len(data) == 0 {
		return
	}

	for _, trackID := range data {
		trackPath := generatePath(trackID)
		if _, err := os.Stat(trackPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			// Some other error occurred while checking the file
			log.Println("Error checking file: ", err.Error())
			continue
		}
		track := Track{}
		err := db.Get(&track, "SELECT * FROM tracks WHERE id = ?", trackID)
		if err != nil {
			log.Println("Error getting track from DB: ", err.Error())
			continue
		}
		// get artist
		album := Album{}
		err = db.Get(&album, "SELECT * FROM albums WHERE id = ?", track.Album)
		if err != nil {
			log.Println("Error getting artist from DB: ", err.Error())
			continue
		}

		query := fmt.Sprintf("ytsearch1:%s %s %s", track.Title, album.Title, "song")
		outputFile := generatePath(track.ID)

		// yt-dlp command
		cmd := exec.Command(musicDir+"/yt-dlp",
			query,
			"-x", "--audio-format", "mp3",
			"-o", outputFile,
		)

		fmt.Printf("Downloading: %s -> %s.mp3\n", track.Title, track.ID)
		err = cmd.Run()
		if err != nil {
			log.Printf("Error downloading track %s: %s\n", track.Title, err.Error())
		} else {
			fmt.Printf("Downloaded: %s\n", track.Title)
		}
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
			downloadOnceMutex.Lock()
			delete(downloadingMap, trackID)
			downloadOnceMutex.Unlock()
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

func loadTracksHandler(c *gin.Context) {
	_, err := validateSession(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	ids := c.Query("id")
	split := strings.Split(ids, ",")
	if len(split) == 0 {
		c.JSON(400, gin.H{"Error": "No track IDs provided"})
		return
	}

	queueDownloads(split, true)
	c.JSON(200, gin.H{"Message": "Tracks are being loaded"})
}

var downloadOnceMutex sync.Mutex
var downloadingMap = make(map[string]bool)

func queueDownloads(downloadIDs []string, downloadAsync bool) {
	toDownload := []string{}

	downloadOnceMutex.Lock()
	for _, id := range downloadIDs {
		if downloadingMap[id] {
			// already downloading
			continue
		}
		downloadingMap[id] = true
		toDownload = append(toDownload, id)
	}
	downloadOnceMutex.Unlock()
	if len(toDownload) == 0 {
		return
	}

	if !downloadAsync {
		downloadTracks(toDownload)
	} else {
		go downloadTracks(toDownload)
	}
}
