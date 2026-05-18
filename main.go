package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

var musicDir string
var IP string
var instanceName string
var isOwnInstance bool
var getInstancesURL string

// Init function to load .env file and setup database connection
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalln("Error loading .env file: ", err)
	}
	musicDir = os.Getenv("MUSIC_DIR")

	if musicDir == "" {
		log.Fatalln("MUSIC_DIR environment variable not set")
	}

	ownInstance := os.Getenv("OWN_INSTANCE")
	otherInstances := os.Getenv("OTHER_INSTANCES")
	if ownInstance == "" && otherInstances == "" {
		log.Fatalln("OWN_INSTANCE environment variable not set")
	} else {
		isOwnInstance = ownInstance != "" // if it is set, using own instance
		if isOwnInstance {
			instanceName = ownInstance
			getInstancesURL = ""
		} else {
			instanceName = otherInstances
			getInstancesURL = os.Getenv("GET_INSTANCES_URL")
			if getInstancesURL == "" {
				log.Fatalln("GET_INSTANCES_URL environment variable not set")
			}
		}
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
