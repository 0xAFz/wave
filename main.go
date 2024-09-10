package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	spotifyAPIBase    = "https://api.spotify.com/v1"
	spotifyTokenURL   = "https://accounts.spotify.com/api/token"
	telegramAPIBase   = "https://api.telegram.org/bot%s/sendAudio"
	refreshInterval   = 60 * time.Second
	httpClientTimeout = 10 * time.Second
	audioFileName     = "audio.mp3"
	dataFileName      = "data.json"
)

var (
	thumbnailExtensions = [2]string{".jpg", ".webp"}
)

type (
	SpotifyClient struct {
		clientID     string
		clientSecret string
		refreshToken string
		httpClient   *http.Client
	}

	TelegramClient struct {
		botToken   string
		chatID     string
		httpClient *http.Client
	}

	Track struct {
		Name    string   `json:"name"`
		Artists []string `json:"artists"`
	}

	SpotifyTokenResponse struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
	}

	TrackHistory map[string]bool
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("error loading .env file: %w", err)
	}

	spotifyClient, err := newSpotifyClient()
	if err != nil {
		return fmt.Errorf("error creating Spotify client: %w", err)
	}

	telegramClient, err := newTelegramClient()
	if err != nil {
		return fmt.Errorf("error creating Telegram client: %w", err)
	}

	history, err := loadTrackHistory(dataFileName)
	if err != nil {
		return fmt.Errorf("error loading track history: %w", err)
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := processCurrentTrack(spotifyClient, telegramClient, history); err != nil {
			log.Printf("Error processing current track: %v", err)
		}
	}

	return nil
}

func newSpotifyClient() (*SpotifyClient, error) {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	refreshToken := os.Getenv("SPOTIFY_REFRESH_TOKEN")

	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return nil, fmt.Errorf("missing required Spotify environment variables")
	}

	return &SpotifyClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient:   &http.Client{Timeout: httpClientTimeout},
	}, nil
}

func newTelegramClient() (*TelegramClient, error) {
	botToken := os.Getenv("BOT_TOKEN")
	chatID := os.Getenv("CHAT_ID")

	if botToken == "" || chatID == "" {
		return nil, fmt.Errorf("missing required Telegram environment variables")
	}

	return &TelegramClient{
		botToken:   botToken,
		chatID:     chatID,
		httpClient: &http.Client{Timeout: httpClientTimeout},
	}, nil
}

func (c *SpotifyClient) getAccessToken(ctx context.Context) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, body)
	}

	var tokenResp SpotifyTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

func (c *SpotifyClient) getCurrentTrack(ctx context.Context) (*Track, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting access token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spotifyAPIBase+"/me/player/currently-playing", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil // No track currently playing
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, body)
	}

	var response struct {
		Item struct {
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	artists := make([]string, len(response.Item.Artists))
	for i, artist := range response.Item.Artists {
		artists[i] = artist.Name
	}

	return &Track{
		Name:    response.Item.Name,
		Artists: artists,
	}, nil
}

func (t *TelegramClient) sendAudio(ctx context.Context, filePath, title, performer, thumbnail string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening audio file: %w", err)
	}
	defer file.Close()

	body := &strings.Builder{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("audio", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("error creating form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("error copying audio file: %w", err)
	}

	if thumbnail != "" {
		thumbFile, err := os.Open(thumbnail)
		if err != nil {
			return fmt.Errorf("error opening thumbnail file: %w", err)
		}
		defer thumbFile.Close()

		part, err := writer.CreateFormFile("thumbnail", filepath.Base(thumbnail))
		if err != nil {
			return fmt.Errorf("error creating thumbnail form file: %w", err)
		}
		if _, err := io.Copy(part, thumbFile); err != nil {
			return fmt.Errorf("error copying thumbnail file: %w", err)
		}
	}

	writer.WriteField("chat_id", t.chatID)
	writer.WriteField("title", title)
	writer.WriteField("performer", performer)

	if err := writer.Close(); err != nil {
		return fmt.Errorf("error closing multipart writer: %w", err)
	}

	url := fmt.Sprintf(telegramAPIBase, t.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body.String()))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, body)
	}

	return nil
}

func downloadFromYouTube(track *Track) error {
	query := fmt.Sprintf("%s %s", track.Name, strings.Join(track.Artists, " "))
	cmd := exec.Command("yt-dlp", "-x", "--audio-format", "mp3", "--write-thumbnail", "-o", audioFileName, "ytsearch1:"+query)
	return cmd.Run()
}

func getThumbnail(filename string) (string, error) {
	for _, ext := range thumbnailExtensions {
		thumbnail := filename + ext
		if _, err := os.Stat(thumbnail); err == nil {
			return thumbnail, nil
		}
	}
	return "", fmt.Errorf("thumbnail not found")
}

func loadTrackHistory(filename string) (TrackHistory, error) {
	history := make(TrackHistory)

	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return history, nil
		}
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&history); err != nil && err != io.EOF {
		return nil, fmt.Errorf("error decoding JSON: %w", err)
	}

	return history, nil
}

func saveTrackHistory(filename string, history TrackHistory) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(history); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	return nil
}

func processCurrentTrack(spotifyClient *SpotifyClient, telegramClient *TelegramClient, history TrackHistory) error {
	ctx := context.Background()

	track, err := spotifyClient.getCurrentTrack(ctx)
	if err != nil {
		return fmt.Errorf("error getting current track: %w", err)
	}
	if track == nil {
		log.Println("No track currently playing")
		return nil
	}

	trackKey := fmt.Sprintf("%s - %s", track.Name, strings.Join(track.Artists, ", "))
	log.Printf("Current track: %s", trackKey)

	if history[trackKey] {
		log.Printf("Track '%s' already processed. Skipping.", trackKey)
		return nil
	}

	log.Printf("Downloading: %s", trackKey)
	if err := downloadFromYouTube(track); err != nil {
		return fmt.Errorf("error downloading from YouTube: %w", err)
	}

	thumbnail, err := getThumbnail(audioFileName)
	if err != nil {
		log.Printf("Error getting thumbnail: %v", err)
	}

	log.Printf("Uploading to Telegram: %s", trackKey)
	if err := telegramClient.sendAudio(ctx, audioFileName, track.Name, strings.Join(track.Artists, ", "), thumbnail); err != nil {
		return fmt.Errorf("error uploading to Telegram: %w", err)
	}

	history[trackKey] = true
	if err := saveTrackHistory(dataFileName, history); err != nil {
		log.Printf("Error saving track history: %v", err)
	}

	os.Remove(audioFileName)
	if thumbnail != "" {
		os.Remove(thumbnail)
	}

	return nil
}
