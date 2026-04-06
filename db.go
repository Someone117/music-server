package main

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

var users = make(map[string]User)
var db *sqlx.DB

var currentlyPlayingMap = make(map[string]Currently_Playing)
var currentlyPlayingMutex sync.Mutex

type JSONList []string
type JSONIntList []int

// JSON marshaling helpers for sqlx (since sqlite stores JSON as TEXT)
func (j JSONList) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	return json.Marshal(j)
}

func (j JSONIntList) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	return json.Marshal(j)
}

func unmarshalJSONOrEmpty(data []byte, target interface{}) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.EqualFold(trimmed, []byte("null")) {
		return nil
	}
	return json.Unmarshal(trimmed, target)
}

func parseJSONIntList(data []byte) (JSONIntList, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.EqualFold(trimmed, []byte("null")) {
		return JSONIntList{}, nil
	}

	var ints []int
	if err := json.Unmarshal(trimmed, &ints); err == nil {
		return JSONIntList(ints), nil
	}

	// Backward compatibility: older rows may contain userids as JSON strings.
	var stringInts []string
	if err := json.Unmarshal(trimmed, &stringInts); err == nil {
		converted := make(JSONIntList, 0, len(stringInts))
		for i, raw := range stringInts {
			value, convErr := strconv.Atoi(raw)
			if convErr != nil {
				return JSONIntList{}, fmt.Errorf("invalid int string at index %d: %q: %w", i, raw, convErr)
			}
			converted = append(converted, value)
		}
		return converted, nil
	}

	return JSONIntList{}, fmt.Errorf("invalid JSONIntList payload: %s", string(trimmed))
}

func (j *JSONList) Scan(src interface{}) error {
	*j = JSONList{}
	switch data := src.(type) {
	case nil:
		return nil
	case []byte:
		return unmarshalJSONOrEmpty(data, j)
	case string:
		return unmarshalJSONOrEmpty([]byte(data), j)
	default:
		return fmt.Errorf("unsupported type for JSONList: %T", src)
	}
}

func (j *JSONIntList) Scan(src interface{}) error {
	*j = JSONIntList{}
	var raw []byte
	switch data := src.(type) {
	case nil:
		return nil
	case []byte:
		raw = data
	case string:
		raw = []byte(data)
	default:
		return fmt.Errorf("unsupported type for JSONIntList: %T", src)
	}

	parsed, err := parseJSONIntList(raw)
	if err != nil {
		return err
	}
	*j = parsed
	return nil
}

type User struct {
	Username      string      `db:"username"`
	Password      string      `db:"password"`
	Refresh_Token JSONList    `db:"refresh_token"`
	UserIDS       JSONIntList `db:"userids"`
}

type Artist struct {
	ID          int    `db:"id"`
	Name        string `db:"name"`
	Image       string `db:"image"`
	LastUpdated int64  `db:"last_updated"`
}

type Album struct {
	ID           int         `db:"id"`
	Title        string      `db:"title"`
	Image        string      `db:"image"`
	NumTracks    int         `db:"numtracks"`
	ReleaseDate  int         `db:"releasedate"`
	ArtistsIDs   JSONIntList `db:"artists_ids"`
	ArtistsNames JSONList    `db:"artists_names"`
	IsFull       int         `db:"isfull"`
}

type Track struct {
	ID            int         `db:"id"`
	Title         string      `db:"title"`
	AlbumID       int         `db:"album_id"`
	AlbumName     string      `db:"album_name"`
	ArtistsIDs    JSONIntList `db:"artists_ids"`
	ArtistsNames  JSONList    `db:"artists_names"`
	IsDownloaded  int         `db:"is_downloaded"`
	Image         string      `db:"image"`
	ReplayGain    float64     `db:"replay_gain"`
	Peak          float64     `db:"peak"`
	MediaMetadata JSONList    `db:"media_metadata"`
}

type Playlist struct {
	ID       int         `db:"id"`
	Title    string      `db:"title"`
	Username string      `db:"username"`
	Tracks   JSONIntList `db:"tracks"`
	Flags    string      `db:"flags"`
}

type ArtistAlbum struct {
	ArtistID int `db:"artist_id"`
	AlbumID  int `db:"album_id"`
}

type Currently_Playing struct {
	Version string `json:"version"`
	Data    string `json:"data"`
}

func setupDB() {
	// db parameters
	dsn := "file:./music.db?" +
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
			refresh_token TEXT DEFAULT '[]',
			userids TEXT DEFAULT '[]'
		);
		CREATE TABLE artists (
			id INTEGER PRIMARY KEY UNIQUE,
			name TEXT,
			image TEXT,
			last_updated INTEGER
		);
		CREATE TABLE albums (
			id INTEGER PRIMARY KEY UNIQUE,
			title TEXT,
			image TEXT,
			numtracks INTEGER,
			releasedate INTEGER,
			artists_ids TEXT,
			artists_names TEXT,
			isfull INTEGER
		);
		CREATE TABLE tracks (
			id INTEGER PRIMARY KEY UNIQUE,
			title TEXT,
			album_id INTEGER,
			album_name TEXT,
			artists_names TEXT,
			artists_ids TEXT,
			is_downloaded INTEGER,
			image TEXT,
			replay_gain REAL,
			peak REAL,
			media_metadata TEXT
		);
		CREATE TABLE playlists (
			id INTEGER PRIMARY KEY UNIQUE,
			title TEXT,
			username TEXT,
			tracks TEXT,
			flags TEXT
		);
		CREATE TABLE artist_albums (
			artist_id INTEGER,
			album_id INTEGER,
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

	userTokens = make(map[int]*UserToken)
	authTokens = make(map[string]int)
	refreshTokens = make(map[string]int)
	authTokensUserName = make(map[string]string)

	for _, user := range dbUsers {
		users[user.Username] = user
		tokenCount := len(user.UserIDS)
		if len(user.Refresh_Token) < tokenCount {
			log.Printf("Warning: user %s has mismatched userids (%d) and refresh_token (%d), ignoring extras", user.Username, len(user.UserIDS), len(user.Refresh_Token))
			tokenCount = len(user.Refresh_Token)
		}
		for i := 0; i < tokenCount; i++ {
			userTokens[user.UserIDS[i]] = &UserToken{
				RefreshToken:  user.Refresh_Token[i],
				RefreshExpiry: time.Now().Add(time.Hour * 24 * 2),
				Username:      user.Username,
				Token:         "",
				TokenExpiry:   time.Now(),
			}
		}

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
	// Artist does not exist, add the artist
	_, err := db.Exec("INSERT OR IGNORE INTO artists (id, name, image, last_updated) VALUES (?, ?, ?, ?)", artist.ID, artist.Name, artist.Image, time.Now().Unix())
	return err
}

// AddAlbum adds an album to the database, also returns if it was added
func AddAlbum(album Album) error {
	_, err := db.Exec("INSERT OR IGNORE INTO albums (id, title, image, numtracks, releasedate, artists_ids, artists_names, isfull) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", album.ID, album.Title, album.Image, album.NumTracks, album.ReleaseDate, album.ArtistsIDs, album.ArtistsNames, album.IsFull)
	return err
}

// AddTrack adds a track to the database
func AddTrack(track Track) error {
	_, err := db.Exec("INSERT OR IGNORE INTO tracks (id, title, album_id, album_name, artists_ids, artists_names, is_downloaded, image, replay_gain, peak, media_metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", track.ID, track.Title, track.AlbumID, track.AlbumName, track.ArtistsIDs, track.ArtistsNames, track.IsDownloaded, track.Image, track.ReplayGain, track.Peak, track.MediaMetadata)
	return err
}

func addArtistAlbum(artistID int, albumID int) error {
	_, err := db.Exec("INSERT OR IGNORE INTO artist_albums (artist_id, album_id) VALUES (?, ?)", artistID, albumID)
	return err
}

func SetTrackDownloaded(trackID int, isDownloaded int) error {
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
