A simple music server with the option to download from yt-dlp

Setup:
MUSIC_DIR="path/to/music"
FILE_EXTENSION="mp3"
SPOTIFY_QUERY_LIMIT="50"
ENABLE_DOWNLOAD="" # set to "I accept the risks" to use ytdlp
COOKIE_PATH="" # for yt-dlp to download content that requires a user to sign in (explicit songs)

yt-dlp (named that way) in /music/


To Run:
create a venv named .venv in /music/ and install ytmusicapi and fuzzywuzzy in it

run.sh (optionally with --release) (will create db, but error out without a user)

To add a user with Spotify credentials:
adduser.sh <username> <password> <spotify_client_id> <spotify_client_secret>

To add your own music files:
Name them <spotifyID>.<FILE_EXTENSION> and put them in /music/

ex: https://open.spotify.com/track/5vNgP5RRg6o6BwDNeZNqiJ as an mp3 file becomes: /music/5vNgP5RRg6o6BwDNeZNqiJ.mp3

You can use /music/search.py <title> <album_name> <artists_names> to get a json with the best link for a song.
(yes, confidence can be over 100%, but generally over 0.8-0.7 confidence is really good, most songs will give 100%+)
