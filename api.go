package main

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func checkIP(ip string) bool {
	ipParts := strings.Split(ip, ".")
	return ip != "127.0.0.1" || ipParts[0] != "100"
}

func ipWhiteList() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get IP address from request
		ip := c.ClientIP()
		if !checkIP(ip) {
			// Optionally block the request
			c.AbortWithStatusJSON(403, gin.H{"error": "Forbidden IP"})
			return
		}

		// Set CORS headers
		origin := c.GetHeader("Origin")
		if origin != "" {
			// Only allow specific origins if needed
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		// Handle preflight request
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func ApiSetUp() {
	router := gin.Default()
	router.LoadHTMLGlob("templates/*.html") // or wherever your templates are
	router.Use(ipWhiteList())

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	router.GET("/login", loginHandler)
	router.GET("/callback", callbackHandler)

	// search all including spotify
	router.GET("/search", searchHandler)
	router.GET("/getArtistAlbums", artistAlbumsHandler)
	router.GET("/getAlbumTracks", albumTracksHandler)

	// get one from db
	router.GET("/getTrack", trackHandler)
	router.GET("/getAlbum", albumHandler)
	router.GET("/getArtist", artistHandler)
	router.GET("/getPlaylist", playlistHandler)
	router.GET("/getArtistImage", artistImageHandler)
	router.GET("/getAlbumArtist", albumArtistHandler)
	router.GET("/getAlbumArtists", albumArtistsHandler)

	// get all from db
	router.GET("/getPlaylists", playlistsHandler)

	// playlist management
	router.POST("/createPlaylist", createPlaylistHandler)
	router.POST("/addTrack", addTrackHandler)
	router.DELETE("/removeTrack", removeTrackHandler)
	router.DELETE("/deletePlaylist", deletePlaylistHandler)
	router.POST("/setFlags", setPlaylistFlagsHandler)
	router.POST("/setPlaylistTracks", setPlaylistTracksHandler)
	router.POST("/setPlaylistName", setPlaylistNameHandler)

	// player
	router.GET("/play", playerHandler)
	router.POST("/currentlyPlaying", currentlyPlayingHandler)
	router.POST("/setHostPassword", setHostPasswordHandler)
	router.GET("/getCurrentlyPlaying", getCurrentlyPlayingHandler)

	router.GET("/home", homeHandler)
	router.GET("/loginPage", loginPageHandler)

	// start the server
	// gin.SetMode(gin.ReleaseMode)
	router.Run(":8080")
}
