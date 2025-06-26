#!/bin/bash

if [ "$#" -ne 4 ]; then
  echo "Usage: $0 <username> <password> <spotify_client_id> <spotify_client_secret>"
  exit 1
fi

username="$1"
password="$2"
spotify_client_id="$3"
spotify_client_secret="$4"
database_file="music.db"

sqlite3 "$database_file" "INSERT INTO Users (username, password, spotify_client_id, spotify_client_secret) VALUES ('$username', '$password', '$spotify_client_id', '$spotify_client_secret');"

if [ $? -eq 0 ]; then
  echo "User '$username' added"
else
  echo "Error"
fi

exit 0