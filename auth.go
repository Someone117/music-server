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

	"github.com/golang-jwt/jwt/v5"

	"github.com/gin-gonic/gin"
)

var (
	AuthURL     = "https://accounts.spotify.com/authorize"
	TokenURL    = "https://accounts.spotify.com/api/token"
	jwtKey      = []byte("my_secret_key")
	refreshKey  = []byte("my_refresh_key")
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
	refreshTokens = make(map[string]string) // token -> username
	oauthStates   = make(map[string]string) // state -> username
)

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func validateUser(username string, password string) error {
	// check if user exists
	user, ok := users[username]
	if !ok || user.Password != password || password == "" || username == "" {
		return fmt.Errorf("invalid username or password")
	}
	return nil
}

func validateToken(c *gin.Context) (string, error) {
	// check if user exists
	token := c.GetHeader("Authorization")
	if token == "" || len(token) <= len("Bearer ") {
		return "", fmt.Errorf("authorization header missing")
	}
		token = token[len("Bearer "):]

	username, ok := usernames[token]
	if !ok {
		return "", fmt.Errorf("invalid token")
	}
	return username, nil
}

func saveAllRefreshTokens() {
	for token, username := range refreshTokens {
		_, ok := users[username]
		if !ok {
			log.Printf("User %s not found while saving refresh token\n", username)
			continue
		}
		// save to db
		_, err := db.Exec("UPDATE users SET refresh_token = ? WHERE username = ?", token, username)
		if err != nil {
			log.Printf("failed to update refresh token in db for user %s: %v", username, err)
		}
	}
}

func loadAllRefreshTokens() {
	for username, user := range users {
		if user.Refresh_Token == "-1" || user.Refresh_Token == "" {
			continue
		}
		refreshTokens[user.Refresh_Token] = username
	}
}

// Parameters: username, password
// Returns: redirects to /callback
// Logs in the user and redirects to Spotify OAuth
func loginHandler(c *gin.Context) {
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
		parsedURL, err := url.Parse(AuthURL)
		if err != nil {
			c.JSON(500, gin.H{"Error": "Failed to parse URL"})
			return
		}

		// Create a URL query object and add query parameters
		query := parsedURL.Query()
		query.Set("client_id", users[username].Spotify_Client_ID)
		query.Set("response_type", "code")
		query.Set("redirect_uri", IP + "/callback")
		// randomly generate state and store it
		state := generateState(username)
		query.Set("state", state)
		query.Set("scope", "user-library-read playlist-read-private")

		// Reassign the query parameters to the URL
		parsedURL.RawQuery = query.Encode()
	}
	// create jwt tokens
	accessToken, refreshToken, err := createTokens(username)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Failed to create tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func generateState(username string) string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatalf("failed to generate state: %v", err)
	}
	state := hex.EncodeToString(b)
	oauthStates[state] = username
	return state
}

func makeTokenRequest(username string, code string, grant_type string) error {
	body := url.Values{}
	body.Set("grant_type", grant_type)
	if grant_type == "authorization_code" {
		body.Set("code", code)
		body.Set("redirect_uri", IP + "/callback")
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
	username, ok := oauthStates[state]
	if !ok {
		c.JSON(400, gin.H{"Error": "Invalid OAuth state"})
		return
	}

	error_message := c.Query("error")
	if error_message != "" {
		c.JSON(400, gin.H{"Error": error_message})
		return
	}

	code := c.Query("code")

	err := makeTokenRequest(username, code, "authorization_code")
	if err != nil {
		c.JSON(500, gin.H{"Error": "Internal server error"})
		return
	}
	// create jwt tokens
	accessToken, refreshToken, err := createTokens(username)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Failed to create tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Refresh spotify token if expired
func getValidToken(username string) (string, error) {
	mu_usertokens.Lock()
	userToken, exists := userTokens[username]
	mu_usertokens.Unlock()
	if !exists {
		// get refresh token from db
		refresh_token := users[username].Spotify_Token_Refresh
		if refresh_token == "-1" {
			return "", fmt.Errorf("please login first")
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

func refreshTokenHandler(c *gin.Context) {
	refreshToken := c.GetHeader("Authorization")
	if refreshToken == "" || len(refreshToken) <= len("Bearer ") {
		c.JSON(401, gin.H{"Error": "Authorization header missing"})
		return
	}
	refreshToken = refreshToken[len("Bearer "):]
	username, ok := refreshTokens[refreshToken]
	if !ok {
		c.JSON(401, gin.H{"Error": "Invalid refresh token"})
		return
	}

	accessToken, newRefreshToken, err := createTokens(username)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Failed to create tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
	})
}

func logoutHandler(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(401, gin.H{"Error": "Authorization header missing"})
		return
	}
	username, ok := usernames[token]
	if !ok {
		c.JSON(401, gin.H{"Error": "Invalid token"})
		return
	}

	// remove tokens
	delete(usernames, token)

	var refreshTokenToDelete string
	for rt, user := range refreshTokens {
		if user == username {
			refreshTokenToDelete = rt
			break
		}
	}
	if refreshTokenToDelete != "" {
		delete(refreshTokens, refreshTokenToDelete)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Logged out successfully",
	})
}

func createTokens(username string) (string, string, error) {
	// Create access token (short-lived)
	accessToken, err := createToken(username, 15*time.Minute, jwtKey)
	if err != nil {
		return "", "", fmt.Errorf("could not create access token: %v", err)
	}

	// Create refresh token (long-lived)
	refreshToken, err := createToken(username, 7*24*time.Hour, refreshKey)
	if err != nil {
		return "", "", fmt.Errorf("could not create refresh token: %v", err)
	}

	// store tokens
	usernames[accessToken] = username
	for token, user := range refreshTokens {
		if user == username {
			// remove old refresh token
			delete(refreshTokens, token)
			break
		}
	}
	refreshTokens[refreshToken] = username

	return accessToken, refreshToken, nil
}

func createToken(username string, duration time.Duration, key []byte) (string, error) {
	expirationTime := time.Now().Add(duration)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(key)
}
