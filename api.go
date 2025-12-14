package main

import (
	"net/http"
	"strings"
	"syscall"

	"github.com/fvbock/endless"
	"github.com/gin-contrib/pprof"
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
	router.Use(ipWhiteList())
	// Register pprof routes under /debug/pprof
	pprof.Register(router)

	router.GET("/ping", func(c *gin.Context) {
		_, err := validateToken(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// redirect / to /home
	router.GET("/", func(c *gin.Context) {
		c.File("./static/music-client/home.html")
	})
	router.GET("/loginPage", func(c *gin.Context) {
		c.File("./static/music-client/login.html")
	})
	router.GET("/login", loginHandler)
	router.GET("/callback", callbackHandler)

	// search all including spotify
	router.GET("/search", searchHandler)
	router.GET("/getArtistTracks", artistTracksHandler)
	// get one or more from db
	router.GET("/getTracks", trackHandler)
	router.GET("/getAlbums", albumHandler)
	router.GET("/getArtists", artistHandler)
	router.GET("/getPlaylists", playlistHandler)
	router.GET("/getArtistAlbums", artistAlbumsHandler)
	router.GET("/getAlbumTracks", albumTracksHandler)

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
	router.GET("/loadTracks", loadTracksHandler)

	// currently playing
	router.POST("/currentlyPlaying", currentlyPlayingHandler)
	router.POST("/setHostPassword", setHostPasswordHandler)
	router.GET("/getCurrentlyPlaying", getCurrentlyPlayingHandler)

	router.POST("/refreshToken", refreshTokenHandler)
	router.POST("/logout", logoutHandler)

	router.GET("/favicon.ico", func(ctx *gin.Context) {
		ctx.File("./static/music-client/favicon.ico")
	})

	// automatically serve files in static
	router.Static("/static", "./static/music-client")

	// start the server
	// gin.SetMode(gin.ReleaseMode)
	// user certificates for https
	endlessCallback := func() {
		cleanup()
	}

	server := endless.NewServer(":8080", router)
	server.SignalHooks[endless.PRE_SIGNAL][syscall.SIGTERM] = append(
		server.SignalHooks[endless.PRE_SIGNAL][syscall.SIGTERM],
		endlessCallback,
	)
	server.SignalHooks[endless.PRE_SIGNAL][syscall.SIGINT] = append(
		server.SignalHooks[endless.PRE_SIGNAL][syscall.SIGINT],
		endlessCallback,
	)

	server.ListenAndServeTLS("./cert/certificate.pem", "./cert/privatekey.pem")
}
