package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)
var musicDir string
var fileExtension string
var spotify_query_limit string
var enable_download bool
var max_db_to_fetch int = 20
var cookie_path string
var IP string

// Init function to load .env file and setup database connection
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalln("Error loading .env file: ", err)
	}
	musicDir = os.Getenv("MUSIC_DIR")
	fileExtension = os.Getenv("FILE_EXTENSION")
	spotify_query_limit := os.Getenv("SPOTIFY_QUERY_LIMIT")
	if spotify_query_limit == "" {
		spotify_query_limit = "50"
	}

	max_db_to_fetch_str := os.Getenv("MAX_DB_TO_FETCH")
	if max_db_to_fetch_str == "" {
		max_db_to_fetch = 20
	} else {
		max_db_to_fetch, err = strconv.Atoi(max_db_to_fetch_str)
		if err != nil {
			log.Printf("Error parsing MAX_DB_TO_FETCH: %v\n", err)
		}
	}

	cookie_path = os.Getenv("COOKIE_PATH") // path to browser cookies for youtube login
	
	enable_download_str := os.Getenv("ENABLE_DOWNLOAD")
	if enable_download_str == "I accept the risks" {
		enable_download = true
		fmt.Println("Downloads enabled")
	} else {
		enable_download = false
	}

	IP = os.Getenv("IP")
	if IP == "" {
		log.Fatalln("IP environment variable not set")
	}
	setupDB()

}

// Main function to start the server and handle token refresh
func main() {
	ApiSetUp()
}
func cleanup() {
	// Close the database connection when the application exits
	fmt.Println("\nCleaning up...")
	// save the current refresh tokens
	saveAllRefreshTokens()
	if db != nil {
		db.Close()
	}
}