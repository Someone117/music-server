package main

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
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
