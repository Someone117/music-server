package main

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/jmoiron/sqlx"
)

var users = make(map[string]User)
var db *sqlx.DB

var currentlyPlayingMap = make(map[string]Currently_Playing)
var currentlyPlayingMutex sync.Mutex

type JSONList []string

// JSON marshaling helpers for sqlx (since sqlite stores JSON as TEXT)
func (j JSONList) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONList) Scan(src interface{}) error {
	switch data := src.(type) {
	case []byte:
		return json.Unmarshal(data, j)
	case string:
		return json.Unmarshal([]byte(data), j)
	default:
		return fmt.Errorf("unsupported type for JSONList: %T", src)
	}
}

type User struct {
	Username              string `db:"username"`
	Password              string `db:"password"`
	Spotify_Client_ID     string `db:"spotify_client_id"`
	Spotify_Client_Secret string `db:"spotify_client_secret"`
	Spotify_Token_Refresh string `db:"spotify_token_refresh"`
	Refresh_Token         string `db:"refresh_token"`
}

type Artist struct {
	ID          string   `db:"id"`
	Name        string   `db:"name"`
	Image       string   `db:"image"`
	SmallImage  string   `db:"smallimage"`
	LastUpdated int64    `db:"last_updated"`
}

type Album struct {
	ID           string   `db:"id"`
	Title        string   `db:"title"`
	Image        string   `db:"image"`
	SmallImage   string   `db:"smallimage"`
	NumTracks    int      `db:"numtracks"`
	ReleaseDate  int      `db:"releasedate"`
	ArtistsIDs   JSONList `db:"artists_ids"`
	ArtistsNames JSONList `db:"artists_names"`
	IsFull       int      `db:"isfull"`
}

type Track struct {
	ID           string   `db:"id"`
	Title        string   `db:"title"`
	AlbumID      string   `db:"album_id"`
	AlbumName    string   `db:"album_name"`
	ArtistsIDs   JSONList `db:"artists_ids"`
	ArtistsNames JSONList `db:"artists_names"`
	IsDownloaded int      `db:"is_downloaded"`
	Image        string   `db:"image"`
	SmallImage   string   `db:"smallimage"`
}

type Playlist struct {
	ID       string `db:"id"`
	Title    string `db:"title"`
	Username string `db:"username"`
	Tracks   JSONList `db:"tracks"`
	Flags    string `db:"flags"`
}

type ArtistAlbum struct {
	ArtistID string `db:"artist_id"`
	AlbumID  string `db:"album_id"`
}

type Currently_Playing struct {
	Version string `json:"version"`
	Data    string `json:"data"`
}

func setupDB() {
	// db parameters
	dsn := "file:/home/someone/Documents/code/music-server/music.db?" +
		"_journal_mode=WAL&" +
		"_synchronous=NORMAL&" +
		"_cache_size=-16000&" +
		"_busy_timeout=5000&" +
		"_foreign_keys=on&" +
		"_temp_store=memory"

	// does it exist?
	if _, err := os.Stat("music.db"); os.IsNotExist(err) {
		fmt.Println("DB does not exist, creating...")
		file, err := os.Create("music.db")
		if err != nil {
			log.Fatalf("Error creating DB: %v", err)
		}
		file.Close()
		// create tables
		db, err = sqlx.Open("sqlite3", dsn)
		if err != nil {
			log.Fatalln("failed to open:", err)
		}
		schema := `
		CREATE TABLE users (
			username TEXT PRIMARY KEY UNIQUE,
			password TEXT,
			spotify_client_id TEXT,
			spotify_client_secret TEXT,
			spotify_token_refresh TEXT,
			refresh_token TEXT DEFAULT ''
		);
		CREATE TABLE artists (
			id TEXT PRIMARY KEY UNIQUE,
			name TEXT,
			smallimage TEXT,
			image TEXT,
			last_updated INTEGER
		);
		CREATE TABLE albums (
			id TEXT PRIMARY KEY UNIQUE,
			title TEXT,
			image TEXT,
			smallimage TEXT,
			isfull INTEGER,
			releasedate INTEGER,
			artists_ids TEXT,
			artists_names TEXT
		);
		CREATE TABLE tracks (
			id TEXT PRIMARY KEY UNIQUE,
			title TEXT,
			album_id TEXT,
			album_name TEXT,
			artists_names TEXT,
			artists_ids TEXT,
			is_downloaded INTEGER,
			image TEXT,
			smallimage TEXT
		);
		CREATE TABLE playlists (
			id TEXT PRIMARY KEY UNIQUE,
			title TEXT,
			username TEXT,
			tracks TEXT,
			flags TEXT
		);
		CREATE TABLE artist_albums (
			artist_id TEXT,
			album_id TEXT,
			PRIMARY KEY(artist_id, album_id)
		);
		`
		db.MustExec(schema)
		log.Println("DB created and tables initialized.")
	} else if err != nil {
		log.Fatalf("Error checking DB: %v", err)
	} else {
		// open db
		var err error
		db, err = sqlx.Open("sqlite3", dsn)
		if err != nil {
			log.Fatalln("failed to open:", err)
		}
	}

	db.SetMaxOpenConns(2) // SQLite can only handle one writer at a time
	db.SetMaxIdleConns(2)

	// get usernames from db and make user objects and also add to oauthConfigs
	var dbUsers []User
	err := db.Select(&dbUsers, "SELECT * FROM users")
	if err != nil {
		log.Fatalf("Error getting users from DB: %v", err)
	}

	for _, user := range dbUsers {
		users[user.Username] = user
	}

	loadAllRefreshTokens()

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
	// Artist does not exist, add the artist
	_, err := db.Exec("INSERT OR IGNORE INTO artists (id, name, smallimage, image, last_updated) VALUES (?, ?, ?, ?, ?)", artist.ID, artist.Name, artist.SmallImage, artist.Image, 0)
	return err
}

// AddAlbum adds an album to the database, also returns if it was added
func AddAlbum(album Album) error {
	_, err := db.Exec("INSERT OR IGNORE INTO albums (id, title, image, smallimage, isfull, releasedate, artists_ids, artists_names) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", album.ID, album.Title, album.Image, album.SmallImage, album.IsFull, album.ReleaseDate, album.ArtistsIDs, album.ArtistsNames)
	return err
}

// AddTrack adds a track to the database
func AddTrack(track Track) error {
	_, err := db.Exec("INSERT OR IGNORE INTO tracks (id, title, album_id, album_name, artists_names, artists_ids, is_downloaded, image, smallimage) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", track.ID, track.Title, track.AlbumID, track.AlbumName, track.ArtistsNames, track.ArtistsIDs, track.IsDownloaded, track.Image, track.SmallImage)
	return err
}

func addArtistAlbum(artistID string, albumID string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO artist_albums (artist_id, album_id) VALUES (?, ?)", artistID, albumID)
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
