package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

var musicDir string
var fileExtension string
var spotify_query_limit string
var disable_auth bool
var enable_download bool

// Init function to load .env file and setup database connection
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	musicDir = os.Getenv("MUSIC_DIR")
	fileExtension = os.Getenv("FILE_EXTENSION")
	spotify_query_limit := os.Getenv("SPOTIFY_QUERY_LIMIT")
	if spotify_query_limit == "" {
		spotify_query_limit = "50"
	}
	disable_auth = os.Getenv("DISABLE_AUTH") == "true"	

	// Note: DO NOT set to true. Use your own music collection. Setting this to true will use spotdl to download music, read https://github.com/spotDL/spotify-downloader for a notice on the potential consequences of using this feature.
	enable_download = os.Getenv("ENABLE_DOWNLOAD") == "true"
	setupDB()
}

// Main function to start the server and handle token refresh
func main() {
	// setup log file
	ApiSetUp()
}
