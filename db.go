package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

var users = make(map[string]User)
var db *sqlx.DB

var currentlyPlayingMap = make(map[string]Currently_Playing)
var currentlyPlayingMutex sync.Mutex

type Album_Artist struct {
	Artist_ID string `db:"artist_id"`
	Album_ID  string `db:"album_id"`
	ROWID     int    `db:"ROWID"`
}

type User struct {
	Username              string `db:"username"`
	Password              string `db:"password"`
	Spotify_Client_ID     string `db:"spotify_client_id"`
	Spotify_Client_Secret string `db:"spotify_client_secret"`
	Spotify_Token_Refresh string `db:"spotify_token_refresh"`
}

type Artist struct {
	ID          string `db:"id"`
	Name        string `db:"name"`
	Image       string `db:"image"`
	LastUpdated int64  `db:"last_updated"`
}

type Album struct {
	ID          string `db:"id"`
	Title       string `db:"name"`
	Image       string `db:"image"`
	SmallImage  string `db:"smallimage"`
	IsFull      int    `db:"isfull"`
	ReleaseDate int    `db:"releasedate"`
}

type Track struct {
	ID           string `db:"id"`
	Title        string `db:"title"`
	Album        string `db:"album_id"`
	IsDownloaded int    `db:"is_downloaded"`
	Image        string `db:"image"`
	SmallImage   string `db:"smallimage"`
}

type Playlist struct {
	ID       string `db:"id"`
	Title    string `db:"title"`
	Username string `db:"username"`
	Tracks   string `db:"tracks"`
	Flags    string `db:"flags"`
}

type Currently_Playing struct {
	Version string `json:"version"`
	Data    string `json:"data"`
}

func setupDB() {
	// Initialize DB - Use SQLite file here
	dsn := "file:/home/someone/Documents/code/music-server/music.db?cache=shared&mode=rwc"
	var err error
	db, err = sqlx.Connect("sqlite3", dsn)
	if err != nil {
		log.Fatalf("Error connecting to DB: %v", err)
	}

	db.MustExec(`
        PRAGMA cache_size = -64000;      -- ~64MB
        PRAGMA temp_store = MEMORY;
        PRAGMA mmap_size = 5000000000;  -- 5GB if OS allows
        PRAGMA optimize;
    `)

	log.Println("SQLite PRAGMAs applied!")

	// get usernames from db and make user objects and also add to oauthConfigs
	var dbUsers []User
	err = db.Select(&dbUsers, "SELECT * FROM users")
	if err != nil {
		log.Fatalf("Error getting users from DB: %v", err)
	}

	for _, user := range dbUsers {
		users[user.Username] = user
	}

	// get number of tracks in db
	var tracks []Track
	err = db.Select(&tracks, "SELECT * FROM tracks")
	if err != nil {
		log.Fatalf("Error getting tracks from DB: %v", err)
	}
	log.Printf("Loaded db with %d user(s) and %d track(s)", len(users), len(tracks))
}

// AddArtist adds an artist to the database, also returns if it was added
func AddArtist(artist Artist) error {
	// Check if the artist already exists
	var existingArtist Artist
	err := db.Get(&existingArtist, "SELECT * FROM artists WHERE id = ?", artist.ID)
	if err == nil {
		// Artist exists, update the last_updated field
		_, err = db.Exec("UPDATE artists SET last_updated = ? WHERE id = ?", time.Now().Unix(), artist.ID)
		return err
	}

	// Artist does not exist, add the artist
	_, err = db.Exec("INSERT INTO artists (id, name, image, last_updated) VALUES (?, ?, ?, ?)", artist.ID, artist.Name, artist.Image, time.Now().Unix())
	return err
}

// AddAlbum adds an album to the database, also returns if it was added
func AddAlbum(album Album) error {
	_, err := db.Exec("INSERT OR IGNORE INTO albums (id, name, image, smallimage, isfull, releasedate) VALUES (?, ?, ?, ?, ?, ?)", album.ID, album.Title, album.Image, album.SmallImage, album.IsFull, album.ReleaseDate)
	// fmt.Printf("Error adding album: %v\n", err)
	return err
}

func AddAlbumArtist(artist_album Album_Artist) error {
	_, err := db.Exec("INSERT OR IGNORE INTO album_artists (artist_id, album_id) VALUES (?, ?)", artist_album.Artist_ID, artist_album.Album_ID)
	return err
}

// AddTrack adds a track to the database
func AddTrack(track Track) error {
	_, err := db.Exec("INSERT OR IGNORE INTO tracks (id, title, album_id, is_downloaded, image, smallimage) VALUES (?, ?, ?, ?, ?, ?)", track.ID, track.Title, track.Album, track.IsDownloaded, track.Image, track.SmallImage)
	// fmt.Printf("Error adding track: %v\n", err)
	return err
}

func SetTrackDownloaded(trackID string, isDownloaded int) error {
	if isDownloaded != 0 && isDownloaded != 1 {
		return fmt.Errorf("isDownloaded must be 0 or 1")
	}
	_, err := db.Exec("UPDATE tracks SET is_downloaded = ? WHERE id = ?", isDownloaded, trackID)
	return err
}

// func getUser(username string) (User, error) {
// 	var user User
// 	err := db.Get(&user, "SELECT * FROM users WHERE username = ?", username)
// 	return user, err
// }
