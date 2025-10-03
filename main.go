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
var disable_auth_warnings bool

// Init function to load .env file and setup database connection
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file: ", err)
	}
	musicDir = os.Getenv("MUSIC_DIR")
	fileExtension = os.Getenv("FILE_EXTENSION")
	spotify_query_limit := os.Getenv("SPOTIFY_QUERY_LIMIT")
	if spotify_query_limit == "" {
		spotify_query_limit = "50"
	}
	disable_auth = os.Getenv("DISABLE_AUTH") == "true"	
	disable_auth_warnings = os.Getenv("DISABLE_AUTH_WARNINGS") == "true"

	// Note: DO NOT set to true. Use your own music collection. Setting this to true will use yt-dlp to download music.
	setupDB()
}

// Main function to start the server and handle token refresh
func main() {
	// setup log file
	ApiSetUp()
}
