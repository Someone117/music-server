#!/bin/bash

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <username> <password>"
  exit 1
fi

username="$1"
password="$2"
spotify_client_id="$3"
spotify_client_secret="$4"
database_file="music.db"

sqlite3 "$database_file" "INSERT INTO Users (username, password) VALUES ('$username', '$password');"

if [ $? -eq 0 ]; then
  echo "User '$username' added"
else
  echo "Error"
fi

exit 0