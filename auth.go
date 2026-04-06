package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	AuthURL  = "https://accounts.spotify.com/authorize"
	TokenURL = "https://accounts.spotify.com/api/token"
)

type UserToken struct {
	Token         string
	TokenExpiry   time.Time
	RefreshToken  string
	RefreshExpiry time.Time
	Username      string
}
type SpotifyToken struct {
	SpotifyToken        string
	SpotifyTokenExpiry  time.Time
	SpotifyRefreshToken string
}

var (
	userTokens         = make(map[int]*UserToken) // device/user id -> tokens etc
	mu_usertokens      sync.Mutex
	authTokensUserName = make(map[string]string) // token -> username for our tokens
	authIDsUserName    = make(map[string]int)    // username -> device/user id for our tokens
	authTokens         = make(map[string]int)    // token -> device/user id for our tokens
	refreshTokens      = make(map[string]int)    // token -> device/user id for our tokens
)

func validateUser(username string, password string) error {
	// check if user exists
	user, ok := users[username]
	if !ok || user.Password != password || password == "" || username == "" {
		return fmt.Errorf("invalid username or password")
	}
	return nil
}

func validateToken(c *gin.Context) (string, error) {
	accessToken := c.GetHeader("Authorization")
	// check if token is valid
	if accessToken == "" || len(accessToken) <= len("Bearer ") {
		return "", fmt.Errorf("Authorization header missing")
	}
	accessToken = accessToken[len("Bearer "):]
	
	mu_usertokens.Lock()
	defer mu_usertokens.Unlock()
	// Use the authTokensUserName map that already exists!
	username, ok := authTokensUserName[accessToken]
	if !ok {
		return "", fmt.Errorf("invalid access token")
	}

	return username, nil
}

func saveAllRefreshTokens() {
	// fix
	userRefreshTokens := make(map[string]JSONList) // username -> refresh tokens
	userIDs := make(map[string]JSONIntList)        // username -> user ids

	mu_usertokens.Lock()
	for userID, token := range userTokens {
		userRefreshTokens[token.Username] = append(userRefreshTokens[token.Username], token.RefreshToken)
		userIDs[token.Username] = append(userIDs[token.Username], userID)
	}
	mu_usertokens.Unlock()

	for username, refreshTokens := range userRefreshTokens {
		_, err := db.Exec("UPDATE users SET refresh_token = ?, userids = ? WHERE username = ?", refreshTokens, userIDs[username], username)
		if err != nil {
			log.Printf("failed to save refresh tokens for user %s: %v", username, err)
		}
	}
}

func generateUniqueUserID() (int, error) {
	for {
		userID, err := rand.Int(rand.Reader, big.NewInt(999999999))
		if err != nil {
			return 0, err
		}
		if _, exists := authIDsUserName[userID.String()]; !exists {
			return int(userID.Int64()), nil
		}
	}
}

func loginHandler(c *gin.Context) {
	username := c.Query("username")
	password := c.Query("password")
	userID := c.Query("user_id")
	fmt.Printf("Login attempted: %s, %s, %s\n", username, password, userID)

	err := validateUser(username, password)
	if err != nil {
		c.JSON(401, gin.H{"Error": err.Error()})
		return
	}
	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		userIDInt, err = generateUniqueUserID()
		if err != nil {
			c.JSON(500, gin.H{"Error": "Failed to generate user ID"})
			return
		}
	}
	accessToken, refreshToken, err := createTokens(username, userIDInt)
	if err != nil {
		c.JSON(500, gin.H{"Error": "Failed to create tokens"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user_id":       userIDInt,
	})
}

func generateState() string {
	stateRand, err := generateSecureRandomString(16)
	if err != nil {
		return "randomstatelollollol"
	}
	return stateRand
}

// our tokens
func refreshTokenHandler(c *gin.Context) {
	refreshToken := c.GetHeader("Authorization")
	if refreshToken == "" || len(refreshToken) <= len("Bearer ") {
		c.JSON(401, gin.H{"Error": "Authorization header missing"})
		return
	}
	refreshToken = refreshToken[len("Bearer "):]
	mu_usertokens.Lock()
	userID, ok := refreshTokens[refreshToken]
	userToken, ok2 := userTokens[userID]
	defer mu_usertokens.Unlock()
	if !ok || !ok2 || userToken.RefreshToken != refreshToken {
		c.JSON(401, gin.H{"Error": "Invalid refresh token"})
		return
	}
	if time.Now().After(userToken.RefreshExpiry) {
		c.Redirect(302, "/login")
		return
	}

	accessToken, newRefreshToken, err := createTokens(userToken.Username, userID)
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
	mu_usertokens.Lock()
	defer mu_usertokens.Unlock()
	username, ok := authTokens[token]
	if !ok {
		c.JSON(401, gin.H{"Error": "Invalid token"})
		return
	}

	// remove tokens
	delete(authTokens, token)
	delete(userTokens, username)

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

func createTokens(username string, userID int) (string, string, error) {
	// Create access token (short-lived)
	accessToken, err := generateSecureRandomString(16)
	if err != nil {
		return "", "", fmt.Errorf("could not create access token: %v", err)
	}

	// Create refresh token (long-lived)
	refreshToken, err := generateSecureRandomString(16)
	if err != nil {
		return "", "", fmt.Errorf("could not create refresh token: %v", err)
	}

	mu_usertokens.Lock()
	defer mu_usertokens.Unlock()

	// do we have a token for this session
	tokens, exists := userTokens[userID]
	if !exists {
		// make a new one
		userTokens[userID] = &UserToken{
			Token:         accessToken,
			RefreshToken:  refreshToken,
			TokenExpiry:   time.Now().Add(time.Minute * 15),
			RefreshExpiry: time.Now().Add(time.Hour * 24 * 2),
			Username:      username,
		}
	} else {
		delete(authTokens, tokens.Token)
		delete(refreshTokens, tokens.RefreshToken)
		delete(authTokensUserName, tokens.Token)

		// else update existing
		tokens.Token = accessToken
		tokens.RefreshToken = refreshToken
		tokens.TokenExpiry = time.Now().Add(time.Minute * 15)
		tokens.RefreshExpiry = time.Now().Add(time.Hour * 24 * 2)
	}

	// add new tokens to maps
	authTokensUserName[accessToken] = username
	refreshTokens[refreshToken] = userID
	authTokens[accessToken] = userID

	return accessToken, refreshToken, nil
}

func generateSecureRandomString(length int) (string, error) {

	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
