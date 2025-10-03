package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	redirectURL = "http://localhost:8080/callback"
	oauthState  = "random_state_string"
	AuthURL     = "https://accounts.spotify.com/authorize"
	TokenURL    = "https://accounts.spotify.com/api/token"
)

// UserToken holds the token and expiry for a user
type UserToken struct {
	Token        string
	TokenExpiry  time.Time
	RefreshToken string
}

var (
	userTokens    = make(map[string]*UserToken)
	mu_usertokens sync.Mutex
	usernames     = make(map[string]string)
)

func validateUser(username string, password string) error {
	// check if user exists
	user, ok := users[username]
	if !ok || user.Password != password || password == "" || username == "" {
		return fmt.Errorf("invalid username or password")
	}
	return nil
}

func validateSession(c *gin.Context) (string, error) {
	// check if user exists
	sessionID, err := c.Cookie("session_id")
	if disable_auth {
		if !disable_auth_warnings {
			fmt.Println("AUTHENTICATION DISABLED - USING TEST USER")
			fmt.Println("PLEASE DO NOT USE IN PRODUCTION")
			fmt.Println("THIS IS FOR TESTING PURPOSES ONLY")
			fmt.Println("WARNING: DO NOT USE IN PRODUCTION")
		}
		return "test", nil
	}
	if err != nil {
		return "", fmt.Errorf("session ID not found")
	}
	username, ok := usernames[sessionID]
	if username == "test" {
		if !disable_auth_warnings {
			fmt.Println("TEST USER DETECTED DURING PRODUCTION")
			fmt.Println("THIS SHOULD NOT HAPPEN")
			fmt.Println("PLEASE USE A REAL USER")
			fmt.Println("ALSO PLEASE SECURE THE SERVER BY DISABLING THE TEST USER")
		}
		return "", fmt.Errorf("invalid username or password")
	}
	if !ok {
		return "", fmt.Errorf("invalid username or password")
	}
	return username, nil
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatalf("failed to generate session ID: %v", err)
	}
	return hex.EncodeToString(b)
}

func loginPageHandler(c *gin.Context) {
	// Render the login page
	c.HTML(http.StatusOK, "login.html", gin.H{
		"title": "Login",
	})
}

func homeHandler(c *gin.Context) {
	// Check if the user is logged in
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		c.Redirect(http.StatusFound, "/loginPage")
		return
	}
	_, exists := usernames[sessionID]
	if !exists {
		c.Redirect(http.StatusFound, "/loginPage")
		return
	}

	c.HTML(http.StatusOK, "home.html", gin.H{
		"title": "Home",
	})
}

// Parameters: username, password
// Returns: redirects to /callback
// Logs in the user and redirects to Spotify OAuth
func loginHandler(c *gin.Context) {
	sessionID := generateSessionID()
	username := c.Query("username")
	password := c.Query("password")
	// check if user exists
	err := validateUser(username, password)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}

	_, err = getValidToken(username)
	if err != nil {
		usernames[sessionID] = username

		parsedURL, err := url.Parse(AuthURL)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Failed to parse URL"})
			return
		}

		// Create a URL query object and add query parameters
		query := parsedURL.Query()
		query.Set("client_id", users[username].Spotify_Client_ID)
		query.Set("response_type", "code")
		query.Set("redirect_uri", redirectURL)
		query.Set("state", oauthState)
		query.Set("scope", "user-library-read playlist-read-private")

		// Reassign the query parameters to the URL
		parsedURL.RawQuery = query.Encode()

		c.SetCookie("session_id", sessionID, 3600, "/", "", false, true)
		c.Redirect(303, parsedURL.String())
		return
	}
	usernames[sessionID] = username
	// redirect to home with username and password details
	c.SetCookie("session_id", sessionID, 3600, "/", "", false, true)
	c.Redirect(303, "/home")
}

func makeTokenRequest(username string, code string, grant_type string) error {
	body := url.Values{}
	body.Set("grant_type", grant_type)
	if grant_type == "authorization_code" {
		body.Set("code", code)
		body.Set("redirect_uri", redirectURL)
	} else if grant_type == "refresh_token" {
		body.Set("refresh_token", code)
	} else {
		return fmt.Errorf("invalid grant type")
	}

	authHeader := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", users[username].Spotify_Client_ID, users[username].Spotify_Client_Secret)))
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Basic %s", authHeader),
		"Content-Type":  "application/x-www-form-urlencoded",
	}

	resp, err := makeRequest(http.MethodPost, TokenURL, headers, nil, body)
	if err != nil {
		return fmt.Errorf("fiailed to get token: %v", err)
	}

	defer resp.Body.Close()
	var spotifyResponse map[string]any
	err = json.NewDecoder(resp.Body).Decode(&spotifyResponse)
	if err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}
	// if there is no refresh_token, use the old one
	if _, ok := spotifyResponse["refresh_token"]; !ok {
		spotifyResponse["refresh_token"] = users[username].Spotify_Token_Refresh
	}

	mu_usertokens.Lock()
	userTokens[username] = &UserToken{
		Token:        spotifyResponse["access_token"].(string),
		TokenExpiry:  time.Now().Add(time.Duration(spotifyResponse["expires_in"].(float64)) * time.Second),
		RefreshToken: spotifyResponse["refresh_token"].(string),
	}
	mu_usertokens.Unlock()

	// set token in db
	_, err = db.Exec("UPDATE users SET spotify_token_refresh = ? WHERE username = ?", spotifyResponse["refresh_token"].(string), username)
	if err != nil {
		log.Printf("failed to update token in db: %v", err)
	}
	return nil
}

// Parameters: N/A
// Returns: N/A
// Spotify OAuth redirects the user here
// REQUIRES NO AUTHENTICATION
func callbackHandler(c *gin.Context) {
	state := c.Query("state")
	if state != oauthState {
		c.JSON(400, gin.H{"Error": "Invalid OAuth state"})
		return
	}

	sessionID, err := c.Cookie("session_id")
	if err != nil {
		c.JSON(400, gin.H{"Error": "Session ID not found"})
		return
	}

	error_message := c.Query("error")
	if error_message != "" {
		c.JSON(400, gin.H{"Error": error_message})
		return
	}

	username := usernames[sessionID]
	code := c.Query("code")

	err = makeTokenRequest(username, code, "authorization_code")
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}

	c.Redirect(303, "/home")
}

// Refresh token if expired
func getValidToken(username string) (string, error) {
	mu_usertokens.Lock()
	userToken, exists := userTokens[username]
	mu_usertokens.Unlock()
	if !exists {
		// get refresh token from db
		refresh_token := users[username].Spotify_Token_Refresh
		if refresh_token == "-1" {
			return "", fmt.Errorf("flease login first")
		}

		err := makeTokenRequest(username, refresh_token, "refresh_token")
		if err != nil {
			return "", fmt.Errorf("failed to get token: %v", err)
		}
		mu_usertokens.Lock()
		userToken, exists = userTokens[username]
		mu_usertokens.Unlock()
		if !exists {
			return "", fmt.Errorf("failed to get token")
		}
		return userToken.Token, nil
	}

	if time.Now().After(userToken.TokenExpiry) {
		err := makeTokenRequest(username, userToken.RefreshToken, "refresh_token")
		if err != nil {
			return "", fmt.Errorf("failed to refresh token: %v", err)
		}
	}

	return userToken.Token, nil
}
