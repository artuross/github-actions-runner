package actions

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type TokenSource struct {
	clientID   string
	signingKey *rsa.PrivateKey
	tokenURL   string
}

func NewTokenSource(authorizationURL, clientID string, signingKey *rsa.PrivateKey) oauth2.TokenSource {
	return &TokenSource{
		clientID:   clientID,
		signingKey: signingKey,
		tokenURL:   authorizationURL,
	}
}

func (s *TokenSource) Token() (*oauth2.Token, error) {
	now := time.Now().UTC().Add(-30 * time.Second)

	jwtToken := jwt.NewWithClaims(
		jwt.SigningMethodRS256,
		jwt.MapClaims{
			"sub": s.clientID,
			"iss": s.clientID,
			"jti": uuid.New().String(),
			"aud": s.tokenURL,
			"nbf": now.Unix(),
			"iat": now.Unix(),
			"exp": now.Add(time.Minute * 5).Unix(),
		})

	signedToken, err := jwtToken.SignedString(s.signingKey)
	if err != nil {
		return nil, fmt.Errorf("jwt sign token: %w", err)
	}

	oauthToken, err := s.fetchToken(signedToken)
	if err != nil {
		return nil, fmt.Errorf("fetch token: %w", err)
	}

	return oauthToken, nil
}

func (s *TokenSource) fetchToken(signedToken string) (*oauth2.Token, error) {
	request, err := http.NewRequest("POST", s.tokenURL, s.prepareRequestBody(signedToken))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	request.Header.Add("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	request.Header.Add("Accept", "application/json")

	// TODO: use custom HTTP client
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var tokenResponse TokenResponse200
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, fmt.Errorf("unmarshal token response: %w", err)
	}

	tokenType := tokenResponse.TokenType

	// JWT is not accepted
	if tokenType == "JWT" {
		tokenType = "Bearer"
	}

	token := oauth2.Token{
		AccessToken: tokenResponse.AccessToken,
		TokenType:   tokenType,
		ExpiresIn:   int64(tokenResponse.ExpiresIn),
		Expiry:      time.Now().UTC().Add(time.Second * time.Duration(tokenResponse.ExpiresIn)),
	}

	return &token, nil
}

func (s *TokenSource) prepareRequestBody(clientAssertion string) io.Reader {
	data := url.Values{}
	data.Set("client_assertion", clientAssertion)
	data.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	data.Set("grant_type", "client_credentials")

	return bytes.NewBufferString(data.Encode())
}

type TokenResponse200 struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}
