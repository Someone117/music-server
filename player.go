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
	"time"

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
			err = queueDownloads([]string{track.ID}, false)
			if err != nil {
				c.JSON(500, gin.H{"Error": err.Error()})
				return
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

// downloads track
func downloadTracks(data []string) {

	if len(data) == 0 {
		return
	}
	track := Track{}

	for _, trackID := range data {
		trackPath := generatePath(trackID)
		if _, err := os.Stat(trackPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			// Some other error occurred while checking the file
			log.Println("Error checking file: ", err.Error())
			continue
		}
		err := db.Get(&track, "SELECT * FROM tracks WHERE id = ?", trackID)
		if err != nil {
			log.Println("Error getting track from DB: ", err.Error())
			continue
		}
		artist_names := strings.Join(track.ArtistsNames, " ")

		outputFile := track.ID + "." + strings.TrimPrefix(fileExtension, ".")

		// search for the audio url
		matchResult, err := searchAudio(track.Title, track.AlbumName, artist_names)
		if err != nil {
			log.Printf("Error searching for track %s: %s\n", track.Title, err.Error())
			continue
		}
		if !matchResult.Success {
			log.Printf("No match found for track %s: %s\n", track.Title, matchResult.Error)
			continue
		}
		audioURL := matchResult.URL

		if matchResult.ConfidenceScore < 0.8 {
			log.Printf("%s: %s %s by %s in %s, Confidence: %.2f\n",
				matchResult.URL, track.Title, matchResult.Title, matchResult.Artist, matchResult.Album, matchResult.ConfidenceScore)
		}
		err = downloadAudio(audioURL, outputFile)
		if err != nil {
			log.Printf("Error downloading track %s: %s\n", track.Title, err.Error())
		}
		fmt.Printf("Downloaded track %s\n", track.Title)
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
			// Set ID3 tags using id3v2 package
			tag, err := id3v2.Open(trackPath, id3v2.Options{Parse: true})
			if err != nil {
				log.Printf("Error opening track %s for tagging: %s\n", trackID, err.Error())
			} else {
				tag.SetTitle(track.Title)
				tag.SetArtist(strings.Join(track.ArtistsNames, "/"))
				tag.SetAlbum(track.AlbumName)
				// Add album art if available
				if track.Image != "" {
					// it's a url, download it
					resp, err := http.Get(track.Image)
					if err == nil {
						defer resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							imageData, err := io.ReadAll(resp.Body)
							if err == nil {
								pic := id3v2.PictureFrame{
									Encoding:    id3v2.EncodingUTF8,
									MimeType:    "image/jpeg",
									PictureType: id3v2.PTFrontCover,
									Description: "Cover",
									Picture:     imageData,
								}
								tag.AddAttachedPicture(pic)
							}
						}
					}
				}

				err = tag.Save()
				if err != nil {
					log.Printf("Error saving ID3 tags for track %s: %s\n", trackID, err.Error())
				}
				tag.Close()
			}
		}
	}
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

	err = queueDownloads(split, true)
	if err != nil {
		c.JSON(500, gin.H{"Error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"Message": "Tracks are being loaded"})
}

var downloadOnceMutex sync.Mutex
var downloadingMap = make(map[string]bool)

func queueDownloads(downloadIDs []string, downloadAsync bool) error {
	if !enable_download {
		return fmt.Errorf("downloads disabled by the server administrator")
	}
	toDownload := []string{}

	downloadOnceMutex.Lock()
	for _, id := range downloadIDs {
		for downloadingMap[id] {
			// wait for download to complete
			downloadOnceMutex.Unlock()
			// Small sleep to avoid busy waiting
			time.Sleep(100 * time.Millisecond)
			downloadOnceMutex.Lock()
		}
		downloadingMap[id] = true
		toDownload = append(toDownload, id)
	}
	downloadOnceMutex.Unlock()
	if len(toDownload) == 0 {
		return nil
	}

	if !downloadAsync {
		downloadTracks(toDownload)
	} else {
		go downloadTracks(toDownload)
	}
	return nil;
}
