package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/oshokin/id3v2"
)

var hostPasswords = make(map[string]string) // host password (if set)
var clientsMutex sync.Mutex

// Parameters: username, password, hostPw
// Returns: response code 200 if successful
// Sets password for listen along sessions, if password is blank, no listen along allowed
func setHostPasswordHandler(c *gin.Context) {
	username, err := validateToken(c)
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

	_, err := validateToken(c)
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
			channel := queueDownloads(track.ID, false)
			if channel != nil {
				<-channel
			}
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
			channel := queueDownloads(track.ID, false)
			if channel != nil {
				<-channel
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

func downloadAudio(url string, outputPath string) error {
	outputPath = fmt.Sprintf("%s/%s", musicDir, outputPath)
	cmd := exec.Command(musicDir+"/yt-dlp", "-x", "--audio-format", "mp3", "-o", outputPath, url)
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()
	if err != nil {
		cmd2 := exec.Command(musicDir+"/yt-dlp", "-x", "--cookies-from-browser", cookie_path, "--audio-format", "mp3", "-o", outputPath, url)
		cmd2.Stdout = nil
		cmd2.Stderr = nil
		return cmd2.Run()
	}
	return nil
}

type MatchResult struct {
	Success         bool    `json:"success"`
	VideoID         string  `json:"videoId"`
	URL             string  `json:"url"`
	Title           string  `json:"title"`
	Artist          string  `json:"artist"`
	Album           string  `json:"album"`
	ConfidenceScore float64 `json:"confidence_score"`
	Error           string  `json:"error"`
	BestScore       float64 `json:"best_score"`
}

func searchAudio(title, album, artists string) (*MatchResult, error) {
	// Build query from Spotify song data
	cmd := exec.Command(musicDir+"/.venv/bin/python", musicDir+"/search.py", title, album, artists)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("command execution error: %v", err)
	}

	result := string(out)
	var matchResult MatchResult
	err = json.Unmarshal([]byte(result), &matchResult)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w\nRaw output: %s", err, result)
	}

	return &matchResult, nil
}

func applyID3Tags(trackPath string, track Track) {
	tag, err := id3v2.Open(trackPath, id3v2.Options{Parse: true})
	if err != nil {
		log.Printf("Error opening track for tagging: %v\n", err)
		return
	}
	defer tag.Close()

	tag.SetTitle(track.Title)
	tag.SetAlbum(track.AlbumName)

	tag.SetArtist(strings.Join(track.ArtistsNames, " / "))

	if track.Image != "" {
		err := downloadAndAttachCover(tag, track.Image)
		if err != nil {
			log.Printf("Warning: Could not attach cover art: %v", err)
		}
	}

	if err := tag.Save(); err != nil {
		log.Printf("Error saving ID3 tags: %v", err)
	}
}

func downloadAndAttachCover(tag *id3v2.Tag, imageURL string) error {
	resp, err := http.Get(imageURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	pic := id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF8,
		MimeType:    "image/jpeg",
		PictureType: id3v2.PTFrontCover,
		Description: "Front Cover",
		Picture:     imageData,
	}
	tag.AddAttachedPicture(pic)
	return nil
}

// downloads track
func downloadTrack(trackID string) {
	if len(trackID) == 0 {
		return
	}

	var track Track
	trackPath := generatePath(trackID)

	if _, err := os.Stat(trackPath); err == nil {
		// we have the track already
		return
	}

	if err := db.Get(&track, "SELECT * FROM tracks WHERE id = ?", trackID); err != nil {
		// db err when getting the track
		log.Printf("DB Error [%s]: %v\n", trackID, err)
		return
	}

	artistNames := strings.Join(track.ArtistsNames, " ")
	match, err := searchAudio(track.Title, track.AlbumName, artistNames)
	if err != nil {
		// we can't find the audio
		log.Printf("Search failed for %s, error: %s\n", track.Title, err.Error())
		return
	}
	if !match.Success {
		if match.Error == "score" {
			log.Printf("Search didn't produce a high score: %s\n", track.Title)
		} else {
			log.Printf("Search failed for %s, \n", track.Title)
			return
		}
	}

	if err := downloadAudio(match.URL, getRelativePath(trackID)); err != nil {
		// can't download
		log.Printf("Download failed: %v\n", err)
		return
	}

	applyID3Tags(trackPath, track)

	SetTrackDownloaded(trackID, 1)

	fmt.Printf("Successfully processed %s\n", track.Title)
}

// Parameters: username, password, id, timestamp (int), volume (float), paused (bool), nextTrack
// Returns: response code 200 if successful
// Sets currently playing data for the user
func currentlyPlayingHandler(c *gin.Context) {
	username, err := validateToken(c)
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
	username, err := validateToken(c)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	hostUsername := c.Query("host")

	if hostUsername == username {
		currentlyPlayingMutex.Lock()
		data, ok := currentlyPlayingMap[hostUsername]
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
		data, ok := currentlyPlayingMap[hostUsername]
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
	_, err := validateToken(c)
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

	channels := make([]<-chan struct{}, 0)
	for _, id := range split {
		var track Track
		err := db.Get(&track, "SELECT * FROM tracks WHERE id = ?", id)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Internal server error"})
			return
		}
		if track.IsDownloaded == 1 {
			continue
		}
		channel := queueDownloads(track.ID, true)
		if channel != nil {
			channels = append(channels, channel)
		}
	}
	go func() {
		for _, ch := range channels {
			<-ch
		}
	}()
	c.JSON(200, gin.H{"Message": "Tracks are being loaded"})
}

var (
	downloadingMap = make(map[string]chan struct{}) // Map of channels
	mapMutex       sync.Mutex
)

func queueDownloads(downloadID string, downloadAsync bool) <-chan struct{} {
	if !enable_download {
		return nil
	}
	mapMutex.Lock()
	defer mapMutex.Unlock()

	ch, exists := downloadingMap[downloadID]
	if exists {
		return ch
	}
	ch = make(chan struct{})
	downloadingMap[downloadID] = ch

	go func() {
		downloadTrack(downloadID)
		mapMutex.Lock()
		close(ch)
		delete(downloadingMap, downloadID)
		mapMutex.Unlock()
	}()
	return ch
}
