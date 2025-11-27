// todo: virtual scrolling so it does not lag to hell

var url = "https://100.79.93.20:8080";

var userPlaylists = new Map();
var currentlyPlaying = null;
var currentTrack = null;
var currentPlaylistId = -1;
var playlistSongIndex = -1;
var queue = [];
let queueIndex = 0;


const audio1 = document.getElementById('audio1');
// const audio2 = document.getElementById('audio2');
// const audio3 = document.getElementById('audio3');
let currentAudio = audio1;
// let nextAudio = audio2;
// let lastAudio = audio3;

const currentTime = document.getElementById('currentTime');
const duration = document.getElementById('duration');

let playInterval;

let liked = false;
let shuffled = false;
let looped = false;
let isPlaying = false;

let playListShuffle = false;
let playlistLoop = false;
let playlistPublic = false;

const popupOverlay = document.getElementById('popupOverlay');

var accessToken = null;
var refreshToken = null;

document.addEventListener('DOMContentLoaded', function () {
    accessToken = localStorage.getItem("access_token");
    refreshToken = localStorage.getItem("refresh_token");

    // // Your code here
    // // make a request to /ping to check if the user is logged in
    // makeRequest(url + "/ping", {}).then(function (data) {
    //     if (data["status"] !== "ok") {
    //         // redirect to login page
    //         window.location.href = "/loginPage";
    //     }
    // });

    // Close popup when clicking outside the popup container
    popupOverlay.addEventListener('click', (e) => {
        if (e.target === popupOverlay) {
            popupOverlay.classList.remove('active');
        }
    });

    const playlistOverlay = document.getElementById('playlistOverlay');
    // Close playlist popup when clicking outside the playlist container
    playlistOverlay.addEventListener('click', (e) => {
        if (e.target === playlistOverlay) {
            playlistOverlay.classList.remove('active');
        }
    });

    // Add click effects for all action buttons
    const popupButtons = document.querySelectorAll('.popup-button');
    popupButtons.forEach(button => {
        button.addEventListener('click', function () {
            // Flash effect
            this.style.backgroundColor = 'rgba(29, 185, 84, 0.3)';
            setTimeout(() => {
                this.style.backgroundColor = '';
            }, 200);
        });
    });

    let searchBar = document.getElementById("search-bar");
    searchBar.addEventListener("keydown", function (event) {
        if (event.key === "Enter") {
            event.preventDefault();
            let searchTab = document.getElementById("search-tab");
            searchTab.innerHTML = "";
            searchDB(false, false);
        }
    });

    searchBar.addEventListener("focus", function (event) {
        search();
    });

    searchBar.addEventListener("input", function (event) {
        // if after 1000 ms there is no input, do the search
        clearTimeout(this.searchTimeout);
        this.searchTimeout = setTimeout(() => {
            let searchTab = document.getElementById("search-tab");
            searchTab.innerHTML = "";
            searchDB(false, false);
        }, 1000);
    });

    const playlistInput = document.getElementById("playlist-name-input");
    playlistInput.value = "";

    const createPlaylistButton = document.getElementById("create-playlist-btn");

    createPlaylistButton.addEventListener("click", async function () {
        const playlistName = playlistInput.value.trim();
        if (!playlistName || playlistName.length > 50) return;

        createPlaylist(playlistName)
    });
    goHome();
    document.getElementById('search-bar').value = '';

});


async function getArtist(artistId, oneArtist = true) {
    if (artistId == null || artistId == undefined || artistId == "" || artistId == "-1") {
        return null;
    }
    const parameters = {
        "ids": artistId
    };

    const data = await makeRequest(url + "/getArtists", parameters);
    if (oneArtist) {
        const item = data["artists"][0];
        return new Artist(item["ID"], item["Name"], item["Image"], item["SmallImage"]);
    }
    const items = data["artists"];
    return items.map(item => new Artist(item["ID"], item["Name"], item["Image"], item["SmallImage"]));
}

class AlbumArtist {
    constructor(artistId, albumId) {
        this.artistId = artistId;
        this.albumId = albumId;
    }
}

class Artist {
    constructor(id, name, image, smallimage) {
        this.id = id;
        this.name = name;
        this.image = image;
        this.smallimage = smallimage;
    }
}

class Album {
    constructor(id, title, image, smallimage, releasedate, artist_ids, artists_names) {
        this.id = id;
        this.title = title;
        this.image = image;
        this.smallimage = smallimage;
        this.releasedate = releasedate;
        this.artistsIDs = artist_ids;
        this.artistsNames = artists_names;
    }
}

class Track {
    constructor(id, title, album, isDownloaded, image, smallimage, albumName, albumID, artistsIDs, artistsNames) {
        this.id = id;
        this.title = title;
        this.isDownloaded = isDownloaded;
        this.image = image;
        this.smallimage = smallimage;
        this.albumName = albumName;
        this.albumID = albumID;
        this.artistsIDs = artistsIDs;
        this.artistsNames = artistsNames;
    }
}

class Playlist {
    constructor(id, title, username, tracks, flags) {
        this.id = id;
        this.title = title;
        this.username = username;
        this.tracks = tracks;
        this.flags = flags;
    }
}

class CurrentlyPlaying {
    constructor(version, data) {
        this.version = version;
        this.data = data;
    }
}

async function makeRequest(newurl, parameters, method = 'GET') {
    // Build the query string
    const query = new URLSearchParams(parameters).toString();
    const fullUrl = `${newurl}?${query}`;

    try {
        const response = await fetch(fullUrl, {
            method: method,
            headers: {
                'Authorization': `Bearer ${accessToken}`
            }
        });
        if (response.status === 401) {
            // Token might be expired, try to refresh
            let new_response = await fetch(url + "/refreshToken", {
                method: 'POST',
                headers: {
                    'Authorization': `Bearer ${refreshToken}`
                }
            });
            if (!new_response.status == 401) {
                window.location.href = "/loginPage";
            } else {
                const new_data = await new_response.json();
                accessToken = new_data.access_token;
                refreshToken = new_data.refresh_token;
                localStorage.setItem("access_token", accessToken);
                localStorage.setItem("refresh_token", refreshToken);
                // Retry the request
                let response3 = await fetch(fullUrl, {
                    method: method,
                    headers: {
                        'Authorization': `Bearer ${accessToken}`
                    }
                });
                if (response3.status === 401) {
                    window.location.href = "/loginPage";
                }
                if (!response.ok) {
                    throw new Error(`HTTP error! Status: ${response.status}`);
                }
                const data = await response3.json(); // or response.text(), etc.
                return data;
            }
        }
        if (!response.ok) {
            throw new Error(`HTTP error! Status: ${response.status}`);
        }
        const data = await response.json(); // or response.text(), etc.
        return data;
    } catch (error) {
        console.error("Fetch error:", error);
        throw error;
    }
}

async function getPlaylists() {
    const parameters = {};

    const data = await makeRequest(url + "/getPlaylists", parameters);
    const item = data["playlists"];

    userPlaylists = new Map();
    if (!item) {
        return;
    }
    item.forEach(p => {
        userPlaylists.set(p["ID"], new Playlist(p["ID"], p["Title"], p["Username"], p["Tracks"], p["Flags"]));
    });
}

async function getArtistAlbums(artistId) {
    const parameters = {
        "id": artistId
    };

    const data = await makeRequest(url + "/getArtistAlbums", parameters);
    const item = data["albums"];

    const albumsList = item.map(a =>
        new Album(a["ID"], a["Title"], a["Image"], a["SmallImage"], a["ReleaseDate"], a["ArtistsIDs"], a["ArtistsNames"])
    );

    return albumsList;
}

async function getAlbumTracks(album) {
    const parameters = {
        "id": album
    };

    const data = await makeRequest(url + "/getAlbumTracks", parameters);
    const item = data["tracks"];

    let albumTracks = item.map(t =>
        new Track(t["ID"], t["Title"], t["Album"], t["IsDownloaded"], t["Image"], t["SmallImage"], t["AlbumName"], t["AlbumID"], t["ArtistsIDs"], t["ArtistsNames"])
    );

    return albumTracks;
}

async function getAlbum(albumId) {
    if (albumId == null || albumId == undefined || albumId == "" || albumId == "-1") {
        return null;
    }
    let parameters = {
        "ids": albumId,
    };

    let data = await makeRequest(url + "/getAlbums", parameters);
    let item = data["albums"];
    if (item.length == 0) {
        return null;
    } else if (item.length == 1) {
        item = item[0];
        return new Album(item["ID"], item["Title"], item["Image"], item["SmallImage"], item["ReleaseDate"], item["ArtistsIDs"], item["ArtistsNames"]);
    } else {
        output = [];
        for (let i = 0; i < item.length; i++) {
            output = new Album(item[i]["ID"], item[i]["Title"], item[i]["Image"], item[i]["SmallImage"], item[i]["ReleaseDate"], item[i]["ArtistsIDs"], item[i]["ArtistsNames"]);
        }
        return output;
    }
}

async function getTracks(trackIds, download = false) {
    if (trackIds == null || trackIds == undefined || trackIds == "") {
        return [];
    }
    const parameters = {
        "ids": trackIds,
        "download": "" + download
    };

    const data = await makeRequest(url + "/getTracks", parameters);
    let item = data["tracks"];
    if (item.length == 0) {
        return null;
    } else if (item.length == 1) {
        item = item[0];
        return new Track(item["ID"], item["Title"], item["Album"], item["IsDownloaded"], item["Image"], item["SmallImage"], item["AlbumName"], item["AlbumID"], item["ArtistsIDs"], item["ArtistsNames"]);
    }
    return item.map(t => new Track(t["ID"], t["Title"], t["Album"], t["IsDownloaded"], t["Image"], t["SmallImage"], t["AlbumName"], t["AlbumID"], t["ArtistsIDs"], t["ArtistsNames"]));
}

async function getPlaylistImage(playlistId) {
    // Get first 4 tracks and make a collage
    const playlist = userPlaylists.get(playlistId);
    if (!playlist) {
        let img = document.createElement("img");
        img.src = "./static/testimage.png";
        img.alt = "Song image";
        img.classList.add("song-img");
        img.onclick = () => showPlaylist(playlistId);
        return img;
    }
    let playlistTracks = playlist.tracks; // Comma-separated list of track IDs
    if (playlistTracks === null || playlistTracks === undefined) {
        let img = document.createElement("img");
        img.src = "./static/testimage.png";
        img.alt = "Song image";
        img.classList.add("song-img");
        img.onclick = () => showPlaylist(playlistId);
        return img;
    }
    let tracksList = playlistTracks.split(",");
    if (tracksList.length > 4) {
        tracksList = tracksList.slice(0, 4);
    } else if (tracksList.length === 0) {
        let img = document.createElement("img");
        img.src = "./static/testimage.png";
        img.alt = "Song image";
        img.classList.add("song-img");
        img.onclick = () => showPlaylist(playlistId);
        return img;
    } else {
        // Use the first image
        if (tracksList[0] == "" || tracksList[0] == "-1") {
            // use no image
            let img = document.createElement("img");
            img.src = "./static/testimage.png";
            img.alt = "Song image";
            img.classList.add("song-img");
            img.onclick = () => showPlaylist(playlistId);
            return img;
        }
        // use the first image
        let track = await getTracks(tracksList[0])
        if (track != null) {
            let img = document.createElement("img");
            img.src = track.smallimage;
            img.alt = "Song image";
            img.classList.add("song-img");
            img.onclick = () => showPlaylist(playlistId);
            return img;
        } else {
            let img = document.createElement("img");
            img.src = "./static/testimage.png";
            img.alt = "Song image";
            img.classList.add("song-img");
            img.onclick = () => showPlaylist(playlistId);
            return img;
        }
    }

    // Create a 2x2 grid for the images
    let container = document.createElement("div");
    container.classList.add("song-img", "imageGrid");
    container.onclick = () => playPlaylist(playlistId, null);

    await Promise.all(
        tracksList.map(async trackId => {
            let track = await getTracks(trackId);
            if (track != null) {
                let img = document.createElement("img");
                img.src = track.smallimage; // match your Track constructor property name
                img.alt = "Track image";
                container.appendChild(img);
            }
        })
    );

    return container;
}


async function makeItemCard(item, playlistId = null, showType = true, isqueue = false, isalbum = false) {
    if (item instanceof Track) {
        let top = document.createElement("div");
        top.classList.add("song-item", "track-item");
        top.setAttribute("data-id", item.id);

        let image = document.createElement("img");
        image.src = item.smallimage || './static/testimage.png';
        image.alt = "Song image";
        image.classList.add("song-img");
        if (isqueue) {
            image.onclick = () => playSongInQueue(item.id);
        } else if (isalbum) {
            image.onclick = () => playSongInAlbum(item.id, playlistId);
        } else {
            image.onclick = () => playSongInPlaylist(playlistId, item.id);
        }

        let title = document.createElement("div");
        title.classList.add("song-title");
        title.textContent = item.title;

        let artist = document.createElement("div");
        artist.classList.add("song-artist");
        if (showType) {
            artist.textContent = `${item.artistsNames.join(", ")}: Track`;
        } else {
            artist.textContent = `${item.artistsNames.join(", ")}`;
        }
        let details = document.createElement("div");
        details.classList.add("song-details");
        details.appendChild(title);
        details.appendChild(artist);

        let options = document.createElement("div");
        options.classList.add("song-options");
        options.textContent = "⋮";
        options.onclick = () => songOptions(item);

        top.appendChild(image);
        top.appendChild(details);
        top.appendChild(options);

        return top;

    } else if (item instanceof Album) {
        let top = document.createElement("div");
        top.classList.add("song-item", "album-item");
        top.setAttribute("data-id", item.id);

        let image = document.createElement("img");
        image.src = item.smallimage || './static/testimage.png';
        image.alt = "Album image";
        image.classList.add("song-img");
        image.onclick = () => goToAlbum(item.id);

        let title = document.createElement("div");
        title.classList.add("song-title");
        title.textContent = item.title;

        let artist = document.createElement("div");
        artist.classList.add("song-artist");

        if (showType) {
            artist.textContent = `${item.artistsNames.join(", ")}: Track`;
        } else {
            artist.textContent = `${item.artistsNames.join(", ")}`;
        }
        let details = document.createElement("div");
        details.classList.add("song-details");
        details.appendChild(title);
        details.appendChild(artist);

        let options = document.createElement("div");
        options.classList.add("song-options");
        options.textContent = "⋮";
        options.onclick = () => albumOptions(item);

        top.appendChild(image);
        top.appendChild(details);
        top.appendChild(options);

        return top;

    } else if (item instanceof Artist) {
        let top = document.createElement("div");
        top.classList.add("song-item", "artist-item");
        top.setAttribute("data-id", item.id);

        let image = document.createElement("img");
        if (item.image && item.image.trim() !== "") {
            image.src = item.image;
        } else {
            image.src = item.smallimage || './static/testimage.png';
        }

        image.alt = "Artist image";
        image.classList.add("song-img");
        image.onclick = () => goToArtist(item.id);

        let title = document.createElement("div");
        title.classList.add("song-title");
        title.textContent = item.name;
        title.onclick = () => goToArtist(item.id);

        let artist = document.createElement("div");
        artist.classList.add("song-artist");
        artist.textContent = "Artist";
        artist.onclick = () => goToArtist(item.id);

        let details = document.createElement("div");
        details.classList.add("song-details");
        details.appendChild(title);
        details.appendChild(artist);

        let options = document.createElement("div");
        options.classList.add("song-options");
        options.textContent = "⋮";
        options.onclick = () => artistOptions(item);

        top.appendChild(image);
        top.appendChild(details);
        top.appendChild(options);

        return top;

    } else if (item instanceof Playlist) {
        const image = getPlaylistImage(item.id);
        image.onclick = () => showPlaylist(item.id);

        let top = document.createElement("div");
        top.classList.add("song-item", "playlist-item");
        top.setAttribute("data-id", item.id);

        let title = document.createElement("div");
        title.classList = ["song-title"];
        title.innerHTML = item.title;

        let artist = document.createElement("div");
        artist.classList = ["song-artist"];
        artist.innerHTML = item.username + ": Playlist";

        let details = document.createElement("div");
        details.classList = ["song-details"];
        details.appendChild(title);
        details.appendChild(artist);

        let options = document.createElement("div");
        options.classList = ["song-options"];
        options.innerHTML = "⋮";

        options.onclick = () => playlistOptions(item);
        top.appendChild(image);
        top.appendChild(details);
        top.appendChild(options);
        return top;
    }
}

// Show playlist
async function showPlaylist(playlistId) {
    let searchBar = document.getElementById('search-bar');
    searchBar.value = '';
    searchBar.classList.remove('active');

    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });
    const playlist = userPlaylists.get(playlistId);
    const playlistName = playlist.title;

    document.getElementById('playlist-tab').classList.add('active');
    document.getElementById('playlist-name').textContent = playlistName;

    document.getElementById('playlist-tab').setAttribute('data-id', playlistId);

    fillPlaylistPage(playlist);
}

// Settings
function settings() {
    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });
    document.getElementById('settings-tab').classList.add('active');
}

// Search
async function search() {
    //TODO: if a playlist is pulled up, search the playlist

    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });


    let searchTab = document.getElementById('search-tab');
    searchTab.innerHTML = "";

    document.getElementById('search-tab').classList.add('active');
}

// Go back to home
async function goHome() {
    let searchBar = document.getElementById('search-bar');
    searchBar.value = '';
    searchBar.classList.remove('active');

    searchContainer = document.getElementById('search-container');
    searchContainer.classList.remove('active');

    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });
    document.getElementById('home-tab').classList.add('active');

    // show playlists
    getPlaylists().then(function (response) {
        const playlistGrid = document.getElementById('playlist-grid');
        playlistGrid.innerHTML = ''; // Clear previous playlists
        for (const [key, value] of userPlaylists) {
            // add to div
            const playlistItem = document.createElement('div');
            playlistItem.className = 'playlist-card';

            const imgWrapper = document.createElement('div');
            getPlaylistImage(key).then(imgNode => {
                imgWrapper.appendChild(imgNode);
            });
            let title = document.createElement('div');
            title.className = 'playlist-title';
            title.textContent = value.title;
            let username = document.createElement('div');
            username.className = 'playlist-username';
            username.textContent = value.username;
            playlistItem.appendChild(imgWrapper);
            playlistItem.appendChild(title);
            playlistItem.appendChild(username);
            playlistItem.onclick = function () {
                showPlaylist(value.id);
            }
            playlistGrid.appendChild(playlistItem);
        }
    });
}

// Show full player
function showFullPlayer() {
    document.getElementById('full-player').classList.add('active');
}

// Hide full player
function hideFullPlayer() {
    document.getElementById('full-player').classList.remove('active');
}

// Show Next Songs
function nextSongs() {
    const nextSongsElement = document.getElementById('next-songs');
    nextSongsElement.style.display = 'block';
    nextSongsElement.style.position = 'fixed';
    nextSongsElement.style.bottom = '-100%';
    nextSongsElement.style.left = '0';
    nextSongsElement.style.width = '100%';
    nextSongsElement.style.height = '100%';
    nextSongsElement.style.backgroundColor = '#121212';
    nextSongsElement.style.color = 'white';
    nextSongsElement.style.overflowY = 'scroll';
    nextSongsElement.style.zIndex = '1000';
    nextSongsElement.style.padding = '20px';
    nextSongsElement.style.boxShadow = '0 -4px 10px rgba(0, 0, 0, 0.5)';
    nextSongsElement.style.transition = 'bottom 0.3s ease-in-out';
    setTimeout(() => {
        nextSongsElement.style.bottom = '0';
    }, 0);
    // fill song list with the queue after the currently playing song
    const songList = document.getElementById('upnext-songs');
    songList.innerHTML = ''; // Clear previous songs

    const partofqueue = queue.slice(queueIndex).join(","); // from current song to end
    getTracks(partofqueue).then(async function (tracks) {
        for (const track of tracks) {
            songList.appendChild(await makeItemCard(track, null, false, true, false));
        }
    });
}

// Hide Next Songs
function hideNextSongs() {
    const nextSongsElement = document.getElementById('next-songs');
    nextSongsElement.style.bottom = '-100%';
    setTimeout(() => {
        nextSongsElement.style.display = 'none';
    }, 300);
}

async function searchDB(onlydb = false, onlySpotify = false) {
    // check the scrolling of the search area, if we need to scroll more, load more
    let searchTab = document.getElementById("search-tab");

    // get query from document
    let query = document.getElementById("search-bar").value; //TODO get the search bar name
    if (query == "") {
        return;
    }
    let parameters = {
        "query": query,
        "db": "true",
        "spotify": "true",
        "albums": "true",
        "artists": "true",
        "tracks": "true",
        "playlists": "true",
    };
    let spotifyResults = [];
    makeRequest(url + "/search", parameters).then(async function (data) {
        // tracks
        item = data["tracks"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Track(item["ID"], item["Title"], item["Album"], item["IsDownloaded"], item["Image"], item["SmallImage"], item["AlbumName"], item["AlbumID"], item["ArtistsIDs"], item["ArtistsNames"]));
            });
        }
        // albums
        item = data["albums"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Album(item["ID"], item["Title"], item["Image"], item["SmallImage"], item["ReleaseDate"], item["ArtistsIDs"], item["ArtistsNames"]));
            });
        }
        // artists
        item = data["artists"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Artist(item["ID"], item["Name"], item["Image"], item["SmallImage"]));
            });
        }
        // playlists
        item = data["playlists"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Playlist(item["ID"], item["Title"], item["Username"], item["Tracks"], item["Flags"]));
            });
        }
        // spotify tracks
        item = data["spotify_tracks"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Track(item["ID"], item["Title"], item["Album"], item["IsDownloaded"], item["Image"], item["SmallImage"], item["AlbumName"], item["AlbumID"], item["ArtistsIDs"], item["ArtistsNames"]));
            });
        }
        // albums
        item = data["spotify_albums"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Album(item["ID"], item["Title"], item["Image"], item["SmallImage"], item["ReleaseDate"], item["ArtistsIDs"], item["ArtistsNames"]));
            });
        }
        // artists
        item = data["spotify_artists"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Artist(item["ID"], item["Name"], item["Image"], item["SmallImage"]));
            });
        }
        // playlists
        item = data["spotify_playlists"];
        if (item != null) {
            item.forEach(function (item) {
                spotifyResults.push(new Playlist(item["ID"], item["Title"], item["Username"], item["Tracks"], item["Flags"]));
            });
        }

        let index = 0;
        for (const item of spotifyResults) {
            if (item instanceof Track) {
                searchTab.appendChild(await makeItemCard(item, null));
                index++;
            } else if (item instanceof Album) {
                searchTab.appendChild(await makeItemCard(item, null));
                index++;
            } else if (item instanceof Artist) {
                searchTab.appendChild(await makeItemCard(item));
            }
        }
    });
}


async function fillPlaylistPage(playlist) {
    let flagsList = playlist.flags.split(",");
    let shuffleInternal = false;
    let repeatInternal = false;
    let publicInternal = false;
    let mixArtists = [];
    let container = document.getElementById("playlist-songs");
    document.getElementById("playlist-play-button").onclick = () => playPlaylist(playlist.id);
    container.innerHTML = "";
    // read version number
    let version = flagsList[0];
    shuffleInternal = flagsList[1] == "1";
    repeatInternal = flagsList[2] == "1";
    publicInternal = flagsList[3] == "1";
    if (version == "001") {
        // make request to get all the artists
        getTracks(playlist.tracks).then(async function (tracks) {
            if (tracks instanceof Array) {
                for (const track of tracks) {
                    container.appendChild(await makeItemCard(track, playlist.id, false));
                }
            } else {
                container.appendChild(await makeItemCard(tracks, playlist.id, false));
            }
        });
        // setplaylistFlags(shuffleInternal, repeatInternal, publicInternal); ToDo: this
        // } else if (version == "002") {
        //     // rest of them are a list of artist ids for the mix
        //     for (let i = 3; i < flagsList.length; i++) {
        //         mixArtists = flagsList[i];
        //     }


        //     // Todo: get the artists

        //     // display the artists in a list like the others
        //     // then make it so that when you play the playlist, it plays random tracks from the artists
        //     // get the albums of the artists
        //     mixArtists.forEach(async function (id) {
        //         container.innerHTML += await makeItemCard(item, null, null);
        //     });
        //     // setplaylistFlags(shuffleInternal, repeatInternal, publicInternal); ToDo: this

    } else {
        getTracks(playlist.tracks).then(async function (tracks) {
            if (tracks instanceof Array) {
                for (const track of tracks) {
                    container.appendChild(await makeItemCard(track, playlist.id, false));
                }
            } else {
                container.appendChild(await makeItemCard(tracks, playlist.id, false));
            }
        });
    }
}

function setCurrentlyPlaying(item) { // actually do it in the server
    // set the currently playing track
    currentlyPlaying = item;
    // set tab title to song title - artist names
    document.title = item.title + " - " + item.artistsNames.join(", ");
    // set icon to album art
    let link = document.querySelector("link[rel~='icon']");
    if (!link) {
        link = document.createElement('link');
        link.rel = 'icon';
        document.getElementsByTagName('head')[0].appendChild(link);
    }
    link.href = item.smallimage || './static/testimage.png';

    // make a request to the server to set the currently playing track
    // let parameters = {
    //     "version": item.version,
    //     "data": item.data
    // };
    // makeRequest(url + "/setCurrentlyPlaying", parameters);

    // set the currently playing track in the UI
    let playlistAddButton = document.getElementById("playlist-add-button");
    playlistAddButton.onclick = () => addToPlaylist(item.id);
    // get the artists of the album
    // set in mini player
    let miniImage = document.getElementById("mini-img");
    miniImage.src = item.image || './static/testimage.png';
    let miniTitle = document.getElementById("mini-title");
    miniTitle.innerHTML = item.title;
    let miniArtist = document.getElementById("mini-artist");
    miniArtist.innerHTML = item.artistsNames.join(", ");

    // big player
    let bigImage = document.getElementById("full-img");
    bigImage.src = item.image;
    let bigTitle = document.getElementById("full-title");
    bigTitle.innerHTML = item.title;
    let bigArtist = document.getElementById("full-artist");
    bigArtist.innerHTML = item.artistsNames.join(", ");

    // set the current time
    let currentTime = document.getElementById("current-time");
    // ToDo: do this
}

async function songOptions(track) {
    // show the song options
    popupOverlay.classList.add('active');
    let top = document.createElement("div");
    top.classList.add("song-item", "track-item");

    let image = document.createElement("img");
    image.src = track.smallimage || './static/testimage.png';
    image.alt = "Song image";
    image.classList.add("song-img");

    let title = document.createElement("div");
    title.classList.add("song-title");
    title.textContent = track.title;

    let artist = document.createElement("div");
    artist.classList.add("song-artist");
    artist.textContent = `${track.artistsNames} : ${track.albumTitle}`;

    let details = document.createElement("div");
    details.classList.add("song-details");
    details.appendChild(title);
    details.appendChild(artist);


    top.appendChild(image);
    top.appendChild(details);

    let songinfo = document.getElementById("popupSongInfo");
    songinfo.innerHTML = "";
    songinfo.appendChild(top);
    // set data of the buttons
    let goToAlbumButton = document.getElementById("popup-button-album");
    goToAlbumButton.onclick = () => goToAlbum(track.album);
    goToAlbumButton.classList.remove("disabled");

    let goToArtistButton = document.getElementById("popup-button-artist");
    // get track artists
    goToArtistButton.onclick = () => goToArtist(track.artistsIDs[0]);
    goToArtistButton.classList.remove("disabled");
    let addToPlaylistButton = document.getElementById("popup-button-playlist");
    addToPlaylistButton.onclick = () => addToPlaylist(track.id);
    addToPlaylistButton.classList.remove("disabled");
    let addToQueue = document.getElementById("popup-button-queue");
    addToQueue.onclick = () => addToQueue(track.id);
    addToQueue.classList.remove("disabled");

    let shareSpotifyButton = document.getElementById("popup-button-share-spotify");
    shareSpotifyButton.onclick = () => shareSpotify(track.id);
    shareSpotifyButton.classList.remove("disabled");
    let shareYtButton = document.getElementById("popup-button-link");
    shareYtButton.onclick = () => shareYT(track.id);
    shareYtButton.classList.remove("disabled");
}

function albumOptions(album) {
    let items = document.getElementsByClassName("popup-button");
    for (let item of items) {
        item.classList.remove("disabled");
    }
    let goToAlbum = document.getElementById("popup-button-album");
    goToAlbum.classList.add("disabled");

    // show the album options
    popupOverlay.classList.add('active');
    let top = document.createElement("div");
    top.classList.add("song-item", "track-item");

    let image = document.createElement("img");
    image.src = album.smallimage || './static/testimage.png';
    image.alt = "Song image";
    image.classList.add("song-img");

    let title = document.createElement("div");
    title.classList.add("song-title");
    title.textContent = album.title;

    let artist = document.createElement("div");
    artist.classList.add("song-artist");
    artist.textContent = `${album.artistsNames}`;

    let details = document.createElement("div");
    details.classList.add("song-details");
    details.appendChild(title);
    details.appendChild(artist);


    top.appendChild(image);
    top.appendChild(details);

    let addToPlaylistButton = document.getElementById("popup-button-playlist");
    addToPlaylistButton.onclick = () => addToPlaylist(album.id, "album");


    let songinfo = document.getElementById("popupSongInfo");
    songinfo.innerHTML = "";
    songinfo.appendChild(top);
}

function artistOptions(artist) {
    // show the artist options
    let items = document.getElementsByClassName("popup-button");
    for (let item of items) {
        item.classList.remove("disabled");
    }
    let goToAlbum = document.getElementById("popup-button-album");
    goToAlbum.classList.add("disabled");
    let goToArtist = document.getElementById("popup-button-artist");
    goToArtist.classList.add("disabled");

    let addToPlaylistButton = document.getElementById("popup-button-playlist");
    addToPlaylistButton.onclick = () => addToPlaylist(artist.id, "artist");

    // show the artist options
    popupOverlay.classList.add('active');
    let top = document.createElement("div");
    top.classList.add("song-item", "track-item");

    let image = document.createElement("img");
    image.src = artist.image || './static/testimage.png';
    image.alt = "Song image";
    image.classList.add("song-img");

    let title = document.createElement("div");
    title.classList.add("song-title");
    title.textContent = artist.name;

    let details = document.createElement("div");
    details.classList.add("song-details");
    details.appendChild(title);

    top.appendChild(image);
    top.appendChild(details);


    let songinfo = document.getElementById("popupSongInfo");
    songinfo.innerHTML = "";
    songinfo.appendChild(top);
}

async function goToAlbum(albumId) {
    const album = await getAlbum(albumId);
    const albumTracks = await getAlbumTracks(albumId);
    await getAlbum(albumId).then(async function (album) {

        document.querySelectorAll('.tab-content').forEach(tab => {
            tab.classList.remove('active');
        });
        let searchBar = document.getElementById('search-bar');
        searchBar.value = '';
        searchBar.classList.remove('active');

        searchContainer = document.getElementById('search-container');
        searchContainer.classList.remove('active');
        popupOverlay.classList.remove('active');
        let albumTab = document.getElementById("album-tab");
        albumTab.setAttribute("data-id", albumId);
        albumTab.classList.add('active');
        let albumTitle = document.getElementById("album-title");
        albumTitle.innerHTML = album.title;
        let albumSubtitle = document.getElementById("album-subtitle");
        albumSubtitle.innerHTML = parseReleaseDate(album.releasedate);
        let albumImage = document.getElementById("album-image");
        albumImage.src = album.image;

        // get all tracks and make track cards
        let albumTracksContainer = document.getElementById("album-songs");
        albumTracksContainer.innerHTML = "";
        for (const track of albumTracks) {
            let trackCard = await makeItemCard(track, albumId, false, false, true);
            albumTracksContainer.appendChild(trackCard);
        }
    });
}

function parseReleaseDate(releaseDate) {
    // turn releaseDate into a string
    let strreleaseDate = releaseDate.toString();

    let year = strreleaseDate.substring(0, 4);
    if (year == "0000") {
        return "Unknown";
    }
    let month = strreleaseDate.substring(4, 6);
    if (month == "00") {
        return `${year}`;
    }
    switch (month) {
        case "01":
            month = "Jan";
            break;
        case "02":
            month = "Feb";
            break;
        case "03":
            month = "Mar";
            break;
        case "04":
            month = "Apr";
            break;
        case "05":
            month = "May";
            break;
        case "06":
            month = "Jun";
            break;
        case "07":
            month = "Jul";
            break;
        case "08":
            month = "Aug";
            break;
        case "09":
            month = "Sep";
            break;
        case "10":
            month = "Oct";
            break;
        case "11":
            month = "Nov";
            break;
        case "12":
            month = "Dec";
            break;
        default:
            month = "Unknown";
            break;
    }
    let day = strreleaseDate.substring(6, 8);
    if (day.charAt(0) == "0") {
        day = day.substring(1);
    }
    if (day == "0") {
        return `${month} ${year}`;
    }
    return `${month} ${day} ${year}`;
}

async function goToArtist(artistId) {
    getArtist(artistId).then(function (artist) {
        getArtistAlbums(artistId).then(function (artistAlbums) {
            document.querySelectorAll('.tab-content').forEach(tab => {
                tab.classList.remove('active');
            });
            let searchBar = document.getElementById('search-bar');
            searchBar.value = '';
            searchBar.classList.remove('active');

            searchContainer = document.getElementById('search-container');
            searchContainer.classList.remove('active');
            popupOverlay.classList.remove('active');
            let artistTab = document.getElementById("artist-tab");
            artistTab.setAttribute("data-id", artistId);
            artistTab.classList.add('active');
            let artistName = document.getElementById("artist-name");
            artistName.innerHTML = artist.name;
            let artistImageDiv = document.getElementById("artist-image");
            artistImageDiv.src = artist.image || artist.smallimage || './static/testimage.png';

            artistTab.setAttribute("data-id", artistId);


            // get all tracks and make track cards
            let artistAlbumsContainer = document.getElementById("artist-albums");
            artistAlbumsContainer.innerHTML = "";
            // sort by release date
            artistAlbums.sort((a, b) => new Date(b.releaseDate) - new Date(a.releaseDate));
            artistAlbums.forEach(async function (album) {
                let albumCard = await makeItemCard(album, null, false);
                artistAlbumsContainer.appendChild(albumCard);
            });
        });

    });
}

function addToPlaylist(trackId, type = "track") {
    playlistOverlay.classList.add('active');
    // remove the previous overlay
    popupOverlay.classList.remove('active');
    // clear the input field
    const playlistInput = document.getElementById("playlist-name-input");
    playlistInput.value = "";
    const playlistContainer = document.getElementById("playlist-container");
    playlistContainer.innerHTML = ""; // clear the container
    playlistContainer.setAttribute("data-track-id", trackId);
    playlistContainer.setAttribute("data-type", type);

    // get playlists
    getPlaylists().then(async function () {
        // loop through userPlaylists and create cards for each playlist
        for (const value of userPlaylists.values()) {
            let top = document.createElement("div");
            top.classList.add("song-item", "playlist-item", "max-width");

            let image = await getPlaylistImage(value.id);

            let title = document.createElement("div");
            title.classList.add("song-title");
            title.textContent = value.title;

            let username = document.createElement("div");
            username.classList.add("song-artist");
            username.textContent = `Owner: ${value.username}`;


            const checkbox = document.createElement("div");
            checkbox.className = "playlist-checkbox";


            // SVG checkmark
            const svgNS = "http://www.w3.org/2000/svg";
            const svg = document.createElementNS(svgNS, "svg");
            svg.setAttribute("viewBox", "0 0 24 24");
            svg.setAttribute("width", "24");
            svg.setAttribute("height", "24");
            svg.setAttribute("fill", "white");

            const path = document.createElementNS(svgNS, "path");
            path.setAttribute("d", "M9,20.42L2.79,14.21L5.62,11.38L9,14.77L18.88,4.88L21.71,7.71L9,20.42Z");

            svg.appendChild(path);
            checkbox.appendChild(svg);


            checkbox.addEventListener("click", () => {
                checkbox.classList.toggle("checked");
                if (checkbox.classList.contains("checked")) {
                    if (type === "artist") {
                        // add all tracks from artist to playlist
                        getArtistAlbums(trackId).then(function (albums) {
                            albums.forEach(function (album) {
                                getAlbumTracks(album.id).then(async function (tracks) {
                                    for (const track of tracks) {
                                        addTrackToPlaylist(value.id, track.id);
                                    }
                                });
                            });
                        });
                    } else if (type === "album") {
                        // add all tracks from album to playlist
                        getAlbumTracks(trackId).then(async function (tracks) {
                            for (const track of tracks) {
                                addTrackToPlaylist(value.id, track.id);
                            }
                        });
                    } else {
                        // add single track to playlist
                        addTrackToPlaylist(value.id, trackId);
                    }
                } else {
                    // remove track from playlist
                    console.log(`Removing track ${trackId} from playlist ${value.id}`);
                }
            });


            let details = document.createElement("div");
            details.classList.add("song-details");
            details.appendChild(title);
            details.appendChild(username);

            top.appendChild(image);
            top.appendChild(details);
            top.appendChild(checkbox);

            top.setAttribute("data-id", value.id);
            playlistContainer.appendChild(top);
        }

    });
}

function addTrackToPlaylist(playlistId, trackId) {
    parameters = {
        "playlistID": playlistId,
        "trackIDs": trackId
    };
    makeRequest(url + "/addTrack", parameters, 'POST')
        .then(function () {
            let playlist = userPlaylists.get(playlistId)
            playlist.tracks += "," + playlistId
            userPlaylists.set(playlistId, playlist)
        });

    console.log(`Adding track ${trackId} to playlist ${playlistId}`);
}

function createPlaylist(playlistName) {
    parameters = {
        "playlistName": playlistName,
    };

    makeRequest(url + "/createPlaylist", parameters, 'POST').then(function (data) {
        if (data["Error"] == null) {
            let playlistId = data["playlistID"];
            const playlistContainer = document.getElementById("playlist-container");
            let trackId = playlistContainer.getAttribute("data-track-id");


            getPlaylists().then(function () {
                const value = userPlaylists.get(playlistId);
                // create a new card for the playlist

                let top = document.createElement("div");
                top.classList.add("song-item", "playlist-item", "max-width");

                let image = getPlaylistImage(value.id) || './static/testimage.png';

                let title = document.createElement("div");
                title.classList.add("song-title");
                title.textContent = value.title;

                let username = document.createElement("div");
                username.classList.add("song-artist");
                username.textContent = `Owner: ${value.username}`;


                const checkbox = document.createElement("div");
                checkbox.className = "playlist-checkbox";


                // SVG checkmark
                const svgNS = "http://www.w3.org/2000/svg";
                const svg = document.createElementNS(svgNS, "svg");
                svg.setAttribute("viewBox", "0 0 24 24");
                svg.setAttribute("width", "24");
                svg.setAttribute("height", "24");
                svg.setAttribute("fill", "white");

                const path = document.createElementNS(svgNS, "path");
                path.setAttribute("d", "M9,20.42L2.79,14.21L5.62,11.38L9,14.77L18.88,4.88L21.71,7.71L9,20.42Z");

                svg.appendChild(path);
                checkbox.appendChild(svg);


                checkbox.addEventListener("click", () => {
                    checkbox.classList.toggle("checked");
                    if (checkbox.classList.contains("checked")) {
                        addTrackToPlaylist(value.id, trackId);
                    } else {
                        // remove track from playlist
                        console.log(`Removing track ${trackId} from playlist ${value.id}`);
                    }
                });


                let details = document.createElement("div");
                details.classList.add("song-details");
                details.appendChild(title);
                details.appendChild(username);

                top.appendChild(image);
                top.appendChild(details);
                top.appendChild(checkbox);

                top.setAttribute("data-id", value.id);
                playlistContainer.appendChild(top);


            });


        } else {
            alert("Error creating playlist: " + data["message"]);
        }
    });
    playlistInput.value = ""; // clear the input field

}

function preloadSongs(trackIds) {
    // Preload the audio track
    if (trackIds == null || trackIds.length === 0) {
        return;
    }
    try {
        makeRequest(url + "/loadTracks", {
            "id": trackIds.join(","),
        });
    } catch {
        // This won't catch async errors from makeRequest
        return;
    }
}

async function makeRequestForAudio(url, parameters, method = 'GET') {
    // Build the query string
    const query = new URLSearchParams(parameters).toString();
    const fullUrl = `${url}?${query}`;
    const token = localStorage.getItem("access_token");

    try {
        const response = await fetch(fullUrl, {
            method: method,
            headers: {
                "Authorization": `Bearer ${token}`
            }
        });
        if (!response.ok) {
            throw new Error(`HTTP error! Status: ${response.status}`);
        }
        return response.blob(); // convert the response to a Blob
    } catch (error) {
        console.error("Fetch error:", error);
        throw error;
    }
}

function loadAudio(trackId, audioElement) {
    makeRequestForAudio(url + "/play", {
        "id": trackId,
        "download": true,
    }).then(blob => {
        const audioURL = URL.createObjectURL(blob); // create a blob URL
        audioElement.src = audioURL; // set the audio element's source to the blob URL
    })
        .catch(err => {
            console.error("Failed to load audio:", err);
        });
}

function shuffleArray(array) {
    let currentIndex = array.length;

    // While there remain elements to shuffle...
    while (currentIndex != 0) {

        // Pick a remaining element...
        let randomIndex = Math.floor(Math.random() * currentIndex);
        currentIndex--;

        // And swap it with the current element.
        [array[currentIndex], array[randomIndex]] = [
            array[randomIndex], array[currentIndex]];
    }
}

function loop() {
    const loopButton = document.getElementById('playlist-loop-button');
    playlistLoop = !playlistLoop;
    if (playlistLoop) {
        loopButton.innerHTML = '<title>repeat</title><path d="M17,17H7V14L3,18L7,22V19H19V13H17M7,7H17V10L21,6L17,2V5H5V11H7V7Z" />';
    } else {
        loopButton.innerHTML = '<title>repeat-off</title><path d="M2,5.27L3.28,4L20,20.72L18.73,22L15.73,19H7V22L3,18L7,14V17H13.73L7,10.27V11H5V8.27L2,5.27M17,13H19V17.18L17,15.18V13M17,5V2L21,6L17,10V7H8.82L6.82,5H17Z" />';
    }
}

function publicPlaylist() {
    const publicButton = document.getElementById('playlist-public-button');
    playlistPublic = !playlistPublic;
    if (playlistPublic) {
        publicButton.innerHTML = '<title>public</title><path d="M17.9,17.39C17.64,16.59 16.89,16 16,16H15V13A1,1 0 0,0 14,12H8V10H10A1,1 0 0,0 11,9V7H13A2,2 0 0,0 15,5V4.59C17.93,5.77 20,8.64 20,12C20,14.08 19.2,15.97 17.9,17.39M11,19.93C7.05,19.44 4,16.08 4,12C4,11.38 4.08,10.78 4.21,10.21L9,15V16A2,2 0 0,0 11,18M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2Z"/>';
    } else {
        publicButton.innerHTML = '<title>public-disabled</title><path d="M22,5.27L20.5,6.75C21.46,8.28 22,10.07 22,12A10,10 0 0,1 12,22C10.08,22 8.28,21.46 6.75,20.5L5.27,22L4,20.72L20.72,4L22,5.27M17.9,17.39C19.2,15.97 20,14.08 20,12C20,10.63 19.66,9.34 19.05,8.22L14.83,12.44C14.94,12.6 15,12.79 15,13V16H16C16.89,16 17.64,16.59 17.9,17.39M11,19.93V18C10.5,18 10.07,17.83 9.73,17.54L8.22,19.05C9.07,19.5 10,19.8 11,19.93M15,4.59V5A2,2 0 0,1 13,7H11V9A1,1 0 0,1 10,10H8V12H10.18L8.09,14.09L4.21,10.21C4.08,10.78 4,11.38 4,12C4,13.74 4.56,15.36 5.5,16.67L4.08,18.1C2.77,16.41 2,14.3 2,12A10,10 0 0,1 12,2C14.3,2 16.41,2.77 18.1,4.08L16.67,5.5C16.16,5.14 15.6,4.83 15,4.59Z"/>';
    }
}

function like() {
    const likeButton = document.getElementById('like-button');
    liked = !liked;
    if (liked) {
        likeButton.innerHTML = '<title>unlike</title><path d="M23,10C23,8.89 22.1,8 21,8H14.68L15.64,3.43C15.66,3.33 15.67,3.22 15.67,3.11C15.67,2.7 15.5,2.32 15.23,2.05L14.17,1L7.59,7.58C7.22,7.95 7,8.45 7,9V19A2,2 0 0,0 9,21H18C18.83,21 19.54,20.5 19.84,19.78L22.86,12.73C22.95,12.5 23,12.26 23,12V10M1,21H5V9H1V21Z" />';
    } else {
        likeButton.innerHTML = '<title>like</title><path d="M5,9V21H1V9H5M9,21A2,2 0 0,1 7,19V9C7,8.45 7.22,7.95 7.59,7.59L14.17,1L15.23,2.06C15.5,2.33 15.67,2.7 15.67,3.11L15.64,3.43L14.69,8H21C22.11,8 23,8.9 23,10V12C23,12.26 22.95,12.5 22.86,12.73L19.84,19.78C19.54,20.5 18.83,21 18,21H9M9,19H18.03L21,12V10H12.21L13.34,4.68L9,9.03V19Z" />';
    }
}

function skipToNextSong() {
    if (queue.length == 0) {
        return;
    }
    // clear current handlers
    // currentAudio.onended = null;
    // currentAudio.onloadedmetadata = null;
    currentAudio.currentTime = 0;


    queueIndex = (queueIndex + 1) % queue.length;

    // swap the audio elements
    // let temp = lastAudio;
    // lastAudio = currentAudio;
    // currentAudio = nextAudio;
    // nextAudio = temp;
    // lastAudio.pause();

    // load the next audio
    // loadAudio(queue[queueIndex + 1 >= queue.length ? 0 : queueIndex + 1],
    // nextAudio);
    loadAudio(queue[queueIndex], currentAudio);


    playQueue();
}

function skipToPreviousSong() {
    if (queue.length == 0) {
        return;
    }
    if (currentAudio.currentTime > 3) {
        // restart the current song
        currentAudio.currentTime = 0;
        currentTime.textContent = formatTime(0);
    } else {
        // currentAudio.onended = null;
        // currentAudio.onloadedmetadata = null;
        // swap the audio elements
        // let temp = nextAudio;
        // nextAudio = currentAudio;
        // currentAudio = lastAudio;
        // lastAudio = temp;

        // nextAudio.pause();

        // load the last audio
        // loadAudio(queue[queueIndex - 1 < 0 ? queue.length - 1 : queueIndex - 1], lastAudio);
        queueIndex = (queueIndex - 1 + queue.length) % queue.length;
        loadAudio(queue[queueIndex], currentAudio);

        // go to the previous song
        playQueue();
    }
}

function play(override = null) {
    const playButton = document.getElementsByClassName('play-button');

    if (override != null) {
        isPlaying = override;
    } else {
        isPlaying = !isPlaying;
    }
    if (isPlaying) {
        Array.from(playButton).forEach(element => {
            element.innerHTML = '<title>pause</title><path d="M13,16V8H15V16H13M9,16V8H11V16H9M12,2A10,10 0 0,1 22,12A10,10 0 0,1 12,22A10,10 0 0,1 2,12A10,10 0 0,1 12,2M12,4A8,8 0 0,0 4,12A8,8 0 0,0 12,20A8,8 0 0,0 20,12A8,8 0 0,0 12,4Z" />';

        });
        playInterval = setInterval(updateRunningTime, 500);

        // play the current audio
        if (!currentAudio.ended) {
            if (currentAudio.paused) {
                currentAudio.play();
            }
        }
    } else {
        Array.from(playButton).forEach(element => {
            element.innerHTML = '<title>play</title><path d="M10,16.5V7.5L16,12M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2Z" />';

        });
        clearInterval(playInterval);
        // pause the current audio
        if (!currentAudio.paused) {
            currentAudio.pause();
        }
    }
}

function playPlaylist(playlistId = null, itemToPlay = null, shuffle = false) {
    if (playlistId == null) {
        playlistId = document.getElementById("playlist-tab").getAttribute("data-id");
    }

    let playlist = userPlaylists.get(playlistId);
    if (playlist == null) {
        return;
    }
    let trackIds = playlist.tracks.split(",");
    if (trackIds.length == 0) {
        return;
    }

    currentPlaylistId = playlistId;

    queue = trackIds;

    if (itemToPlay != null) {
        queueIndex = trackIds.indexOf(itemToPlay.id);
    } else {
        queueIndex = 0;
    }
    if (shuffle) {
        shuffleArray(queue);
        queueIndex = 0;
    }

    let lastQueueIndex = queueIndex - 1 < 0 ? queue.length - 1 : queueIndex - 1;

    // loadAudio(queue[lastQueueIndex], lastAudio);
    loadAudio(queue[queueIndex], currentAudio);
    // loadAudio(queue[queueIndex + 1], nextAudio);
    playQueue();
}

function playQueue() {
    if (queue.length == 0) {
        return;
    }
    let trackId = queue[queueIndex];
    // preload 4 next songs
    if (queueIndex + 4 > queue.length) {
        preloadSongs(queue.slice(queueIndex, queue.length));
    } else {
        preloadSongs(queue.slice(queueIndex, queueIndex + 4));
    }

    playSong(trackId);
}

function formatTime(seconds) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${secs < 10 ? '0' : ''}${secs}`;
}

function playSongInQueue(trackId) {
    if (queue.length == 0) {
        return;
    }
    let index = queue.indexOf(trackId);
    if (index === -1) {
        return;
    }

    queueIndex = index;

    let lastQueueIndex = queueIndex - 1 < 0 ? queue.length - 1 : queueIndex - 1;

    // loadAudio(queue[lastQueueIndex], lastAudio);
    loadAudio(queue[queueIndex], currentAudio);
    // loadAudio(queue[queueIndex + 1], nextAudio);

    playQueue();
}

function playAlbum() {
    let albumId = document.getElementById("album-tab").getAttribute("data-id");
    getAlbumTracks(albumId).then(function (tracks) {
        let trackIds = tracks.map(track => track.id);
        if (trackIds.length == 0) {
            return;
        }
        currentPlaylistId = null;
        queue = trackIds;
        queueIndex = 0;

        let lastQueueIndex = queueIndex - 1 < 0 ? queue.length - 1 : queueIndex - 1;

        // loadAudio(queue[lastQueueIndex], lastAudio);
        loadAudio(queue[queueIndex], currentAudio);
        // loadAudio(queue[queueIndex + 1], nextAudio);
        playQueue();
    });
}

function playSongInAlbum(trackId, albumId) {
    if (albumId == null) {
        albumId = document.getElementById("album-tab").getAttribute("data-id");
    }
    if (albumId == null) {
        return;
    }
    getAlbumTracks(albumId).then(function (tracks) {
        if (tracks.length == 0) {
            return;
        }
        let trackIds = tracks.map(track => track.id);
        if (trackIds.length == 0) {
            return;
        }
        currentPlaylistId = null;
        queue = trackIds;

        queueIndex = trackIds.indexOf(trackId);

        let lastQueueIndex = queueIndex - 1 < 0 ? queue.length - 1 : queueIndex - 1;
        // loadAudio(queue[lastQueueIndex], lastAudio);
        loadAudio(queue[queueIndex], currentAudio);
        // loadAudio(queue[queueIndex + 1], nextAudio);
        playQueue();
    });
}

function playSongInPlaylist(playlistId, trackId) {
    if (playlistId == null) {
        playlistId = document.getElementById("playlist-tab").getAttribute("data-id");
    }
    let playlist = userPlaylists.get(playlistId);
    if (playlist == null) {
        return;
    }
    let trackIds = playlist.tracks.split(",");
    if (trackIds.length == 0) {
        return;
    }

    currentPlaylistId = playlistId;


    if (trackIds != null) {
        queueIndex = trackIds.indexOf(trackId);
    } else {
        queueIndex = 0;
    }

    queue = trackIds.slice(queueIndex, trackIds.length).concat(trackIds.slice(0, queueIndex));

    playSongInQueue(trackId);
}

function playSong(trackId) {
    // set the duration etc.

    currentAudio.onended = skipToNextSong;
    currentAudio.paused = true;
    currentAudio.onloadedmetadata = updateDuration;


    // get track info
    getTracks(trackId).then(async function (track) {
        if (track == null) {
            return;
        }
        setCurrentlyPlaying(track);
        setCurrentlyPlayingInDevice(track);
    });

    // wait for load
    currentAudio.oncanplaythrough = () => {
        currentAudio.play().catch(error => { console.error("Play error:", error) });
        play(true);
    };
}

function playArtist(artistId) {
    if (artistId == null) {
        artistId = document.getElementById("artist-tab").getAttribute("data-id");
    }
    // get all tracks 
    getArtistAlbums(artistId).then(async function (albums) {
        // for each album, get the tracks
        let trackIds = [];
        for (const album of albums) {
            const albumTracks = await getAlbumTracks(album.id);
            for (const track of albumTracks) {
                trackIds.push(track.id);
            }
        }
        if (trackIds.length == 0) {
            return;
        }
        // shuffle the tracks
        shuffleArray(trackIds);
        queue = trackIds;
        queueIndex = 0;

        let lastQueueIndex = queueIndex - 1 < 0 ? queue.length - 1 : queueIndex - 1;

        // loadAudio(queue[lastQueueIndex], lastAudio);
        loadAudio(queue[queueIndex], currentAudio);
        // loadAudio(queue[queueIndex + 1], nextAudio);
        playQueue();
    });

}

// set now playing info in device (like in android etc)
function setCurrentlyPlayingInDevice(track) {
    if (track == null) {
        return;
    }
    if (track.artistsNames == "") {
        track.artistsNames = "Unknown Artist";
    }
    if ('mediaSession' in navigator) {
        navigator.mediaSession.metadata = new MediaMetadata({
            title: track.title,
            artist: track.artistsNames,
            artwork: [
                { src: track.smallimage || './static/testimage.png', sizes: '96x96', type: 'image/png' },
                { src: track.image || './static/testimage.png', sizes: '128x128', type: 'image/png' },
                { src: track.image || './static/testimage.png', sizes: '192x192', type: 'image/png' },
                { src: track.image || './static/testimage.png', sizes: '256x256', type: 'image/png' },
                { src: track.image || './static/testimage.png', sizes: '384x384', type: 'image/png' },
                { src: track.image || './static/testimage.png', sizes: '512x512', type: 'image/png' },
            ]
        });

        navigator.mediaSession.setActionHandler('play', () => { play(true); });
        navigator.mediaSession.setActionHandler('pause', () => { play(false); });
        navigator.mediaSession.setActionHandler('previoustrack', () => { skipToPreviousSong(); });
        navigator.mediaSession.setActionHandler('nexttrack', () => { skipToNextSong(); });
    }
}




// From https://github.com/codewithsadee/music-player/blob/master/assets/js/script.js


const playerDuration = document.querySelector("[data-duration]");
const playerSeekRange = document.querySelector("[data-seek]");

const getTimecode = function (duration) {
    const minutes = Math.floor(duration / 60);
    const seconds = Math.ceil(duration - (minutes * 60));
    const timecode = `${minutes}:${seconds < 10 ? "0" : ""}${seconds}`;
    return timecode;
}

const updateDuration = function () {
    playerSeekRange.max = Math.ceil(currentAudio.duration);
    playerDuration.textContent = getTimecode(Number(playerSeekRange.max));
}

const playerRunningTime = document.querySelector("[data-running-time");

const updateRunningTime = function () {
    playerSeekRange.value = currentAudio.currentTime;
    playerRunningTime.textContent = getTimecode(currentAudio.currentTime);

    updateRangeFill();
}

const ranges = document.querySelectorAll("[data-range]");
const rangeFill = document.querySelector("[data-range-fill]");

const updateRangeFill = function () {
    let element = this || ranges[0];

    const rangeValue = (element.value / element.max) * 100;
    if (rangeFill) {
        rangeFill.style.width = `${rangeValue}%`;
    }
}

for (const range of ranges) {
    range.addEventListener("input", updateRangeFill);
}

const seek = function () {
    currentAudio.currentTime = playerSeekRange.value;
    playerRunningTime.textContent = getTimecode(playerSeekRange.value);
}

playerSeekRange.addEventListener("input", seek);
