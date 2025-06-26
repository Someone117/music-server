#!/bin/bash

set -e

go_files=$(find . -name "*.go" | tr '\n' ' ')

if [ "$1" == "--release" ]; then
    export GIN_MODE=release
else 
    export GIN_MODE=debug
fi

go build -o "music-server" $go_files

./music-server