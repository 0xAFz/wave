package main

import (
	"bytes"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Song struct {
	Item struct {
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"item"`
}

type SpotifyRefreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

var (
	audioFile = "audio.mp3"
	dataFile  = "data.json"
)

func main() {
	_ = godotenv.Load()

	upload := os.Getenv("UPLOAD")
	chatID := os.Getenv("CHAT_ID")
	botToken := os.Getenv("BOT_TOKEN")
	if upload == "" || chatID == "" || botToken == "" {
		log.Fatal("required environment variables not set")
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := processCurrentPlayingSong(botToken, chatID, upload); err != nil {
			log.Println(err)
		}
	}
}

func processCurrentPlayingSong(botToken, chatID, upload string) error {
	song, err := getCurrentPlaying()
	if err != nil {
		return err
	}

	artists := getArtists(song.Item.Artists)
	songKey := fmt.Sprintf("%s - %s", song.Item.Name, artists)

	log.Println(songKey)

	if upload == "true" {
		dataMap, err := loadData(dataFile)
		if err != nil {
			return fmt.Errorf("failed to load data: %w", err)
		}

		if dataMap[songKey] {
			log.Printf("Song '%s' already exists in the database. Skipping download.", songKey)
			return nil
		}

		log.Printf("Downloading: %s", songKey)
		if err := downloadFromYouTube(song.Item.Name, artists); err != nil {
			return fmt.Errorf("failed to download from YouTube: %w", err)
		}

		thumbnail, err := getThumbnail(audioFile)
		if err != nil {
			log.Printf("failed to get thumbnail: %v", err)
		}

		log.Printf("Uploading to Telegram: %s", songKey)
		if err := sendAudio(botToken, chatID, audioFile, song.Item.Name, artists, thumbnail); err != nil {
			return fmt.Errorf("failed to upload to Telegram: %w", err)
		}

		dataMap[songKey] = true
		if err := saveData(dataFile, dataMap); err != nil {
			log.Printf("failed to save data: %v", err)
		}
	}

	return nil
}

func getArtists(artists []struct {
	Name string `json:"name"`
}) string {
	names := make([]string, len(artists))
	for i, artist := range artists {
		names[i] = artist.Name
	}
	return strings.Join(names, " ")
}

func loadData(filename string) (map[string]bool, error) {
	data := make(map[string]bool)

	file, err := os.Open(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return data, nil
		}
		return nil, err
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&data); err != nil && err != io.EOF {
		return nil, err
	}

	return data, nil
}

func saveData(filename string, data map[string]bool) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(data)
}

func getAccessToken() (string, error) {
	form := url.Values{}
	form.Add("grant_type", "refresh_token")
	form.Add("refresh_token", os.Getenv("SPOTIFY_REFRESH_TOKEN"))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	spotifyClientID := os.Getenv("SPOTIFY_CLIENT_ID")
	spotifyClientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")

	auth := fmt.Sprintf("%s:%s", spotifyClientID, spotifyClientSecret)
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", b64.StdEncoding.EncodeToString([]byte(auth))))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get access token request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp SpotifyRefreshTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}

func getCurrentPlaying() (Song, error) {
	accessToken, err := getAccessToken()
	if err != nil {
		return Song{}, fmt.Errorf("failed to get access token: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/currently-playing", nil)
	if err != nil {
		return Song{}, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := client.Do(req)
	if err != nil {
		return Song{}, fmt.Errorf("failed to get current playing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return Song{}, fmt.Errorf("not listening to music")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Song{}, fmt.Errorf("get current playing request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	var song Song
	if err := json.NewDecoder(resp.Body).Decode(&song); err != nil {
		return Song{}, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return song, nil
}

func downloadFromYouTube(song, artists string) error {
	cmd := exec.Command("yt-dlp", "-x", "--audio-format", "mp3", "--write-thumbnail", "-o", audioFile, "ytsearch1:"+fmt.Sprintf("%s %s", song, artists))
	return cmd.Run()
}

func getThumbnail(filename string) (string, error) {
	thumbnailFiles := []string{filename + ".jpg", filename + ".webp"}
	for _, thumbnail := range thumbnailFiles {
		if _, err := os.Stat(thumbnail); err == nil {
			return thumbnail, nil
		}
	}
	return "", errors.New("thumbnail not found")
}

func sendAudio(token, chatID, filePath, title, performer, thumbnail string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := addFileToWriter(writer, "audio", filePath); err != nil {
		return err
	}
	if thumbnail != "" {
		if err := addFileToWriter(writer, "thumbnail", thumbnail); err != nil {
			return err
		}
	}

	writer.WriteField("chat_id", chatID)
	writer.WriteField("title", title)
	writer.WriteField("performer", performer)
	writer.Close()

	req, err := http.NewRequest("POST", "https://api.telegram.org/bot"+token+"/sendAudio", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := os.Remove(filePath); err != nil {
		return err
	}
	if err := os.Remove(thumbnail); err != nil {
		return err
	}

	return nil
}

func addFileToWriter(writer *multipart.Writer, fieldName, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	part, err := writer.CreateFormFile(fieldName, filePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	return nil
}
