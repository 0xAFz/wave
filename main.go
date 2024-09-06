package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Song struct {
	Item struct {
		Album struct {
			Artists []struct {
				ExternalUrls struct {
					Spotify string `json:"spotify"`
				} `json:"external_urls"`
				Href string `json:"href"`
				Id   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
				Uri  string `json:"uri"`
			} `json:"artists"`
		} `json:"album"`
		Artists []struct {
			ExternalUrls struct {
				Spotify string `json:"spotify"`
			} `json:"external_urls"`
			Href string `json:"href"`
			Id   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
			Uri  string `json:"uri"`
		} `json:"artists"`
		Name       string `json:"name"`
		Popularity int    `json:"popularity"`
		PreviewUrl string `json:"preview_url"`
		Uri        string `json:"uri"`
	} `json:"item"`
}

type SpotifyRefreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

func main() {
	_ = godotenv.Load()

	for {
		time.Sleep(time.Second * 10)

		song, err := getCurrentPlaying()
		if err != nil {
			log.Println(err)
			continue
		}

		var artists strings.Builder
		for _, artist := range song.Item.Artists {
			artists.WriteString(fmt.Sprintf("%s ", artist.Name))
		}

		fmt.Printf("%s - %s\n", song.Item.Name, artists.String())
	}
}

func getAccessToken() (string, error) {
	form := url.Values{}
	form.Add("grant_type", "refresh_token")
	form.Add("refresh_token", os.Getenv("SPOTIFY_REFRESH_TOKEN"))
	encodedData := form.Encode()

	client := &http.Client{}
	req, _ := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(encodedData))

	spotifyClientID := os.Getenv("SPOTIFY_CLIENT_ID")
	spotifyClientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")

	auth := fmt.Sprintf("%s:%s", spotifyClientID, spotifyClientSecret)
	encodedAuth := b64.StdEncoding.EncodeToString([]byte(auth))

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", encodedAuth))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("get access token request failed with status code: %d", resp.StatusCode)
	}

	var spotifyRefreshToken SpotifyRefreshTokenResponse
	if err := json.Unmarshal([]byte(body), &spotifyRefreshToken); err != nil {
		return "", err
	}

	return spotifyRefreshToken.AccessToken, nil
}

func getCurrentPlaying() (Song, error) {
	accessToken, err := getAccessToken()
	if err != nil {
		return Song{}, fmt.Errorf("failed to get access token with error: %w", err)
	}

	client := &http.Client{}
	req, _ := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/currently-playing", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := client.Do(req)
	if err != nil {
		return Song{}, fmt.Errorf("failed to get current playing with error: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return Song{}, fmt.Errorf("not listening music")
	}
	if resp.StatusCode != 200 {
		return Song{}, fmt.Errorf("get current playing request failed with status code: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	var song Song
	if err := json.Unmarshal([]byte(body), &song); err != nil {
		return Song{}, fmt.Errorf("failed to unmarshal response body with error: %w", err)
	}

	return song, nil
}
