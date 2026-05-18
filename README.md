A simple music server with the option to download.

Currently a work in progress. In the process of migrating to muiscbrainz for the data and then will create plugins for downloading.


Setup:
MUSIC_DIR="path/to/music"
FILE_EXTENSION="mp3"
ENABLE_DOWNLOAD="" # set to "I accept the risks" to use ytdlp
IP="https://your.ip.address:port" # the ip address and port of your server, use HTTPs for Spotify auth to work

add my music-client files to /static/music-client/
(run git clone https://github.com/Someone117/music-client in /static/)

To add a user credentials:
adduser.sh `<username>` `<password>`

To add your own music files:
Name them `<TidalID>`.<FILE_EXTENSION> and put them in /music/

Run:
run.sh (optionally with --release) (will create db, but error out without a user)
