package manager

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	log "github.com/sirupsen/logrus"
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
)

const (
	// The JWT issuer
	issuer = "alpha-api-gateway"
	// The JWT audience
	audience = "server-alpha"
	// The JWT expiration time
	expirationTime = 24 * time.Hour
	// The JWT refresh expiration time
	refreshExpirationTime = 7 * 24 * time.Hour
)

type AlphaClaims struct {
	IsRefreshToken bool `json:"irf"`
	jwt.RegisteredClaims
}

type JWTManager interface {
	Generate(username string) (*schema.TokenPairResponse, error)
	Verify(token string) (string, error)
	Refresh(refreshToken string) (*schema.TokenPairResponse, error)
}

type jwtManager struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func NewJWTManager() JWTManager {
	// Get the public and private key from the environment
	publicKeyPem := os.Getenv("JWT_PUBLIC_KEY")
	privateKeyPem := os.Getenv("JWT_PRIVATE_KEY")

	// Load the public and private key
	publicKey := loadPublicKey(publicKeyPem)
	privateKey := loadPrivateKey(privateKeyPem)

	return &jwtManager{
		publicKey:  publicKey,
		privateKey: privateKey,
	}
}

func (j *jwtManager) Generate(username string) (*schema.TokenPairResponse, error) {
	// Generate the JWT and refresh token
	token, err := generateJWT(username, false, j.privateKey)
	if err != nil {
		return nil, err
	}

	refreshToken, err := generateJWT(username, true, j.privateKey)
	if err != nil {
		return nil, err
	}

	return &schema.TokenPairResponse{
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func (j *jwtManager) Verify(token string) (string, error) {
	return verifyToken(token, false, j.publicKey)
}

func (j *jwtManager) Refresh(refreshToken string) (*schema.TokenPairResponse, error) {
	// Verify the refresh token
	username, err := verifyToken(refreshToken, true, j.publicKey)
	if err != nil {
		return nil, err
	}

	// Generate a new JWT and refresh token
	token, err := generateJWT(username, false, j.privateKey)
	if err != nil {
		return nil, err
	}
	newRefreshToken, err := generateJWT(username, true, j.privateKey)
	if err != nil {
		return nil, err
	}

	return &schema.TokenPairResponse{
		Token:        token,
		RefreshToken: newRefreshToken,
	}, nil
}

func verifyToken(token string, shouldBeRefreshToken bool, publicKey ed25519.PublicKey) (string, error) {
	// Parse the token
	claims := &AlphaClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return publicKey, nil
	})
	if err != nil {
		log.Errorf("Received an unverifiable token: %v", err)
		return "", err
	}

	// Check if the token is valid
	if !parsedToken.Valid {
		return "", fmt.Errorf("Received an invalid token")
	}
	if !shouldBeRefreshToken && claims.IsRefreshToken {
		// We don't accept refresh tokens for normal authentication
		return "", fmt.Errorf("Received a refresh token when a normal token was expected")
	}
	// Return the subject of the token
	return claims.Subject, nil
}

func generateJWT(username string, isRefreshToken bool, privateKey ed25519.PrivateKey) (string, error) {
	now := time.Now()
	expiresAt := now.Add(expirationTime)
	if isRefreshToken {
		expiresAt = now.Add(refreshExpirationTime)
	}

	claims := AlphaClaims{
		IsRefreshToken: isRefreshToken,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(privateKey)
}

func loadPublicKey(pemKey string) ed25519.PublicKey {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil || block.Type != "PUBLIC KEY" {
		log.Fatalf("Failed to decode PEM block containing public key")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		log.Fatalf("Failed to parse public key: %v", err)
	}

	ed25519Key, ok := key.(ed25519.PublicKey)
	if !ok {
		log.Fatalf("Failed to cast public key to Ed25519 public key")
	}

	return ed25519Key
}

func loadPrivateKey(path string) ed25519.PrivateKey {
	block, _ := pem.Decode([]byte(path))
	if block == nil || block.Type != "PRIVATE KEY" {
		log.Fatalf("Failed to decode PEM block containing private key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		log.Fatalf("Failed to parse private key: %v", err)
	}

	ed25519Key, ok := key.(ed25519.PrivateKey)
	if !ok {
		log.Fatalf("Failed to cast private key to Ed25519 private key")
	}

	return ed25519Key
}
