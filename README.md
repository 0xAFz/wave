# Wave
Shows Your Current Listening Music From Spotify

## Running Steps

### Requirements
- [Docker](https://www.docker.com/) and [Docker Compose](https://docs.docker.com/compose/) plugin

To run this app, we have several steps that we need to do in order:

### 1. Get your Spotify `Client id` and `Client secret` 
Visit your [Spotify developers dashboard](https://developer.spotify.com/dashboard/applications) then select or create your app. Note down your Client ID, Client Secret, and Redirect URI in a convenient location to use in Step 2.

### 2. Get your access code
Visit the following URL after replacing `$CLIENT_ID`, `$SCOPE`, and `$REDIRECT_URI` with the information you noted in Step 1.

```bash
https://accounts.spotify.com/authorize?response_type=code&client_id=$CLIENT_ID&scope=user-read-currently-playing&redirect_uri=http://localhost:8081
```

### 3. Get `Code` from the redirect URL
I was redirected to the following URL because my redirect URI was set to http://localhost:8081. In place of `$CODE` there was a very long string of characters. Copy that string and note it down for use in Step 4.

```bash
http://localhost:8081/?code=$CODE
```

### 4. Get the refresh token
Running the following CURL command will result in a JSON string that contains the refresh token, in addition to other useful data. Again, either replace or export the following variables in your shell `$CILENT_ID`, `$CLIENT_SECRET`, `$CODE`, and `$REDIRECT_URI`.

```bash
curl -d client_id=$CLIENT_ID -d client_secret=$CLIENT_SECRET -d grant_type=authorization_code -d code=$CODE -d redirect_uri=$REDIRECT_URI https://accounts.spotify.com/api/token
```

The result will be a JSON string similar to the following. Take the `refresh_token` and save that in a safe, private place. This token will last for a very long time and can be used to generate a fresh `access_token` whenever it is needed.

### 5. Setup `.env`
Put your `client_id`, `client_secret` and `refresh_token` in `.env`
```bash
cp .env.example .env

vim .env
```
### Run the app
Now you can run wave with docker compose
```bash
docker compose up -d
```
### Show current listening music
You can see current listening music from container logs
```bash
docker logs -f wave
```