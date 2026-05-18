package main

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// the HIFI music api implementation

func getStatus(url string) bool {
	resp, err := makeRequest("GET", url, nil, nil, nil)
	if err != nil {
		fmt.Println(err)
	}
	return resp.StatusCode == 200
}

func parseReleaseDate(dateStr string) int {
	if dateStr == "" {
		return 0
	}
	layouts := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, dateStr)
		if err == nil {
			return int(t.Unix())
		}
	}
	fmt.Printf("Error parsing release date: %q\n", dateStr)
	return 0
}

func responsePreview(body []byte) string {
	preview := strings.TrimSpace(string(body))
	if len(preview) > 300 {
		preview = preview[:300] + "..."
	}
	return preview
}

type SearchResult struct {
	ResponseType string     `json:"responseType"`
	Tracks       []Track    `json:"tracks"`
	Artists      []Artist   `json:"artists"`
	Albums       []Album    `json:"albums"`
	Playlists    []Playlist `json:"playlists"`
}

type MediaMetadata struct {
	Tags []string `json:"tags"`
}

type rawArtist struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

type rawAlbumItem struct {
	Item rawTrack `json:"item"`
	Type string   `json:"type"`
}

type rawAlbum struct {
	ID             int            `json:"id"`
	Title          string         `json:"title"`
	NumberOfTracks int            `json:"numberOfTracks"`
	ReleaseDate    string         `json:"releaseDate"`
	Cover          string         `json:"cover"`
	MediaMetadata  MediaMetadata  `json:"mediaMetadata"`
	Artists        []rawArtist    `json:"artists"`
	Items          []rawAlbumItem `json:"items"`
}

type rawTrack struct {
	ID            int           `json:"id"`
	Title         string        `json:"title"`
	Duration      int           `json:"duration"`
	ReplayGain    float64       `json:"replayGain"`
	Peak          float64       `json:"peak"`
	MediaMetadata MediaMetadata `json:"mediaMetadata"`
	Album         rawAlbum      `json:"album"`
	Artists       []rawArtist   `json:"artists"`
}

type searchResponse struct {
	Data struct {
		Items      []rawTrack `json:"items"`
		TotalItems int        `json:"totalNumberOfItems"`
		Artists    *struct {
			Items      []rawArtist `json:"items"`
			TotalItems int         `json:"totalNumberOfItems"`
		} `json:"artists"`
		Albums *struct {
			Items      []rawAlbum `json:"items"`
			TotalItems int        `json:"totalNumberOfItems"`
		} `json:"albums"`
	} `json:"data"`
}

func artistFromRaw(r rawArtist) Artist {
	a := Artist{
		ID:    r.ID,
		Name:  r.Name,
		Image: r.Picture,
	}
	AddArtist(a)
	return a
}

func albumFromRaw(r rawAlbum) Album {
	artistIDs := make([]int, len(r.Artists))
	artistNames := make([]string, len(r.Artists))
	for i, a := range r.Artists {
		artistIDs[i] = a.ID
		artistNames[i] = a.Name
	}
	isFull := 0
	if r.NumberOfTracks > 0 && len(r.Items) > 0 {
		isFull = 1
	}
	a := Album{
		ID:           r.ID,
		Title:        r.Title,
		Image:        r.Cover,
		NumTracks:    r.NumberOfTracks,
		ReleaseDate:  parseReleaseDate(r.ReleaseDate),
		ArtistsIDs:   artistIDs,
		ArtistsNames: artistNames,
		IsFull:       isFull,
	}
	AddAlbum(a)
	return a
}

func trackFromRaw(r rawTrack) Track {
	artistIDs := make([]int, len(r.Artists))
	artistNames := make([]string, len(r.Artists))
	for i, a := range r.Artists {
		artistIDs[i] = a.ID
		artistNames[i] = a.Name
	}
	t := Track{
		ID:            r.ID,
		Title:         r.Title,
		AlbumID:       r.Album.ID,
		AlbumName:     r.Album.Title,
		ArtistsIDs:    artistIDs,
		ArtistsNames:  artistNames,
		Image:         r.Album.Cover,
		ReplayGain:    r.ReplayGain,
		Peak:          r.Peak,
		MediaMetadata: r.MediaMetadata.Tags,
	}
	AddTrack(t)
	return t
}

// itemType: "track" | "artist" | "album"
func search(baseURL string, query string, itemType string, offset int, limit int) (SearchResult, error) {
	var paramKey string
	switch itemType {
	case "track":
		paramKey = "s"
	case "artist":
		paramKey = "a"
	case "album":
		paramKey = "al"
	case "playlist":
		// search db for playlists matching query
		var playlists []Playlist
		err := db.Select(&playlists, "SELECT * FROM playlists WHERE title LIKE ?", "%"+query+"%")
		if err != nil {
			return SearchResult{}, fmt.Errorf("searching playlists in db failed: %w", err)
		}
		return SearchResult{
			ResponseType: itemType,
			Playlists:    playlists,
		}, nil
	default:
		return SearchResult{}, fmt.Errorf("unsupported itemType %q: must be track, artist, or album", itemType)
	}

	queryParams := map[string]string{
		paramKey: query,
		"offset": fmt.Sprintf("%d", offset),
		"limit":  fmt.Sprintf("%d", limit),
	}
	attemptURLs := []string{baseURL}
	seen := map[string]bool{baseURL: true}
	for i := 0; i < len(apiUrls); i++ {
		u := getURL(false)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		attemptURLs = append(attemptURLs, u)
	}

	var resp searchResponse
	var lastErr error
	for _, attemptURL := range attemptURLs {
		raw, err := makeRequest("GET", attemptURL+"/search/", nil, queryParams, nil)
		if err != nil {
			lastErr = fmt.Errorf("search request failed on %s: %w", attemptURL, err)
			continue
		}

		body, err := io.ReadAll(raw.Body)
		raw.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read search response from %s: %w", attemptURL, err)
			continue
		}

		if raw.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("search upstream returned status %d from %s: %s", raw.StatusCode, attemptURL, responsePreview(body))
			continue
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "<") {
			lastErr = fmt.Errorf("search upstream returned HTML instead of JSON from %s: %s", attemptURL, responsePreview(body))
			continue
		}

		if err := json.Unmarshal(body, &resp); err != nil {
			lastErr = fmt.Errorf("failed to parse search response from %s: %w", attemptURL, err)
			continue
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		return SearchResult{}, lastErr
	}

	d := resp.Data

	out := SearchResult{
		ResponseType: itemType,
	}

	switch itemType {
	case "track":
		for _, r := range d.Items {
			for _, a := range r.Artists {
				artist := artistFromRaw(a)
				out.Artists = append(out.Artists, artist)
			}
			t := trackFromRaw(r)
			album := albumFromRaw(r.Album)
			out.Albums = append(out.Albums, album)
			out.Tracks = append(out.Tracks, t)
		}

	case "artist":
		if d.Artists != nil {

			for _, r := range d.Artists.Items {
				a := artistFromRaw(r)
				out.Artists = append(out.Artists, a)
			}
		}

	case "album":
		if d.Albums != nil {
			for _, r := range d.Albums.Items {
				for _, a := range r.Artists {
					artist := artistFromRaw(a)
					out.Artists = append(out.Artists, artist)
				}
				al := albumFromRaw(r)
				out.Albums = append(out.Albums, al)
			}
		}
	}

	return out, nil
}

func getAlbum(baseURL string, id int) (Album, []Track, error) {
	queryParams := map[string]string{
		"id": fmt.Sprintf("%d", id),
	}

	attemptURLs := []string{baseURL}
	seen := map[string]bool{baseURL: true}
	for i := 0; i < len(apiUrls); i++ {
		u := getURL(false)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		attemptURLs = append(attemptURLs, u)
	}

	var raw *http.Response
	var err error
	var lastErr error
	for _, attemptURL := range attemptURLs {
		raw, err = makeRequest("GET", attemptURL+"/album/", nil, queryParams, nil)
		if err != nil {
			lastErr = fmt.Errorf("get album failed on %s: %w", attemptURL, err)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return Album{}, nil, lastErr
	}
	defer raw.Body.Close()

	var resp struct {
		Data struct {
			ID             int           `json:"id"`
			Title          string        `json:"title"`
			NumberOfTracks int           `json:"numberOfTracks"`
			ReleaseDate    string        `json:"releaseDate"`
			Cover          string        `json:"cover"`
			MediaMetadata  MediaMetadata `json:"mediaMetadata"`
			Artists        []rawArtist   `json:"artists"`
			Items          []struct {
				Item rawTrack `json:"item"`
			} `json:"items"`
		} `json:"data"`
	}

	if err := json.NewDecoder(raw.Body).Decode(&resp); err != nil {
		return Album{}, nil, fmt.Errorf("failed to parse album response: %w", err)
	}

	d := resp.Data

	album := albumFromRaw(rawAlbum{
		ID:             d.ID,
		Title:          d.Title,
		NumberOfTracks: d.NumberOfTracks,
		ReleaseDate:    d.ReleaseDate,
		Cover:          d.Cover,
		MediaMetadata:  d.MediaMetadata,
		Artists:        d.Artists,
	})

	var tracks []Track
	trackIDs := make([]int, 0, len(d.Items))
	for _, item := range d.Items {
		t := trackFromRaw(item.Item)
		tracks = append(tracks, t)
		trackIDs = append(trackIDs, t.ID)
	}

	return album, tracks, nil
}

func getArtist(baseURL string, id int) (Artist, error) {
	raw, err := makeRequest("GET", baseURL+"/artist/", nil, map[string]string{
		"id": fmt.Sprintf("%d", id),
	}, nil)
	if err != nil {
		return Artist{}, fmt.Errorf("get artist failed: %w", err)
	}

	var resp struct {
		Artist struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		} `json:"artist"`
	}

	if err := json.NewDecoder(raw.Body).Decode(&resp); err != nil {
		return Artist{}, fmt.Errorf("failed to parse artist response: %w", err)
	}

	a := Artist{
		ID:    resp.Artist.ID,
		Name:  resp.Artist.Name,
		Image: resp.Artist.Picture,
	}
	return a, nil
}

func getArtistAlbums(baseURL string, id int) ([]Album, error) {
	raw, err := makeRequest("GET", baseURL+"/artist/", nil, map[string]string{
		"f":           fmt.Sprintf("%d", id),
		"skip_tracks": "true",
	}, nil)
	if err != nil {
		fmt.Println("trying backup url")
		// remove that URL:
		for i, url := range apiUrls {
			if url == baseURL {
				apiUrls = append(apiUrls[:i], apiUrls[i+1:]...)
				break
			}
		}
		// try 1 more time with new URL:
		baseURL = getURL(false)
		raw, err = makeRequest("GET", baseURL+"/artist/", nil, map[string]string{
			"f":           fmt.Sprintf("%d", id),
			"skip_tracks": "true",
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("get artist albums failed: %w", err)
		}
	}
	defer raw.Body.Close()

	var resp struct {
		Albums struct {
			Items []rawAlbum `json:"items"`
		} `json:"albums"`
	}

	if err := json.NewDecoder(raw.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to parse artist albums response: %w", err)
	}
	fmt.Println("Artist Albums Response:", resp.Albums.Items)

	var albums []Album
	for _, r := range resp.Albums.Items {
		al := albumFromRaw(r)
		if err := addArtistAlbum(id, al.ID); err != nil {
			return nil, fmt.Errorf("failed to save artist album relation: %w", err)
		}
		albums = append(albums, al)
	}
	return albums, nil
}

func getAlbumTracks(baseURL string, id int) ([]Track, error) {
	_, tracks, err := getAlbum(baseURL, id)
	return tracks, err
}

type dashManifest struct {
	Periods []struct {
		AdaptationSets []struct {
			MimeType        string `xml:"mimeType,attr"` // Now exported
			Representations []struct {
				SegmentTemplate struct {
					Initialization  string `xml:"initialization,attr"`
					Media           string `xml:"media,attr"`
					StartNumber     int    `xml:"startNumber,attr"`
					SegmentTimeline struct {
						Segments []struct {
							Duration int `xml:"d,attr"`
							Repeat   int `xml:"r,attr"`
						} `xml:"S"`
					} `xml:"SegmentTimeline"`
				} `xml:"SegmentTemplate"`
			} `xml:"Representation"`
		} `xml:"AdaptationSet"`
	} `xml:"Period"`
}

func buildDashSegmentURLs(decodedManifest []byte) ([]string, string, error) {
	var mpd dashManifest
	if err := xml.Unmarshal(decodedManifest, &mpd); err != nil {
		return nil, "", fmt.Errorf("failed to parse MPD manifest: %w", err)
	}

	if len(mpd.Periods) == 0 || len(mpd.Periods[0].AdaptationSets) == 0 || len(mpd.Periods[0].AdaptationSets[0].Representations) == 0 {
		return nil, "", fmt.Errorf("invalid MPD: missing period/adaptation set/representation")
	}

	tmpl := mpd.Periods[0].AdaptationSets[0].Representations[0].SegmentTemplate
	if tmpl.Initialization == "" || tmpl.Media == "" {
		return nil, "", fmt.Errorf("invalid MPD: missing SegmentTemplate initialization/media")
	}

	if !strings.Contains(tmpl.Media, "$Number$") {
		return nil, "", fmt.Errorf("unsupported MPD media template: expected $Number$")
	}

	startNumber := tmpl.StartNumber
	if startNumber <= 0 {
		startNumber = 1
	}

	segmentCount := 0
	for _, s := range tmpl.SegmentTimeline.Segments {
		repeat := s.Repeat
		if repeat < 0 {
			return nil, "", fmt.Errorf("unsupported MPD SegmentTimeline repeat value: %d", repeat)
		}
		segmentCount += repeat + 1
	}

	if segmentCount == 0 {
		return nil, "", fmt.Errorf("invalid MPD: no media segments in SegmentTimeline")
	}

	urls := make([]string, 0, segmentCount+1)
	urls = append(urls, tmpl.Initialization)
	for i := 0; i < segmentCount; i++ {
		n := startNumber + i
		urls = append(urls, strings.ReplaceAll(tmpl.Media, "$Number$", fmt.Sprintf("%d", n)))
	}
	return urls, mpd.Periods[0].AdaptationSets[0].MimeType, nil
}

type btsManifest struct {
	ManifestMimeType string   `json:"mimeType"`
	Codecs           string   `json:"codecs"` // idk if this is a list
	EncryptionType   string   `json:"encryptionType"`
	URLs             []string `json:"urls"`
}

func buildBTSURLs(decodedManifest []byte) ([]string, string, error) {
	var bts btsManifest
	if err := json.Unmarshal(decodedManifest, &bts); err != nil {
		return nil, "", fmt.Errorf("failed to parse BTS manifest: %w", err)
	}
	return bts.URLs, bts.ManifestMimeType, nil
}

// check between manifestMimeType application/dash+xml and application/vnd.tidal.bts to determine if we need to do segmented downloading or not, and then get the urls for the segments if needed
func decodeManifest(manifest string) ([]string, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(manifest)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode manifest: %w", err)
	}
	// check if it's a dash manifest or not by looking for the <MPD> tag
	if !strings.Contains(string(decoded), "<MPD") {
		urls, manifestMimeType, err := buildBTSURLs(decoded)
		if err != nil {
			return nil, "", fmt.Errorf("failed to build BTS URLs: %w", err)
		}
		return urls, manifestMimeType, nil
	} else {
		urls, manifestMimeType, err := buildDashSegmentURLs(decoded)
		if err != nil {
			return nil, "", fmt.Errorf("failed to build DASH segment URLs: %w", err)
		}
		return urls, manifestMimeType, nil
	}
}

func hiFiTrackURLs(baseURL string, id int) ([]string, string, error) {
	queryParams := map[string]string{
		"id": fmt.Sprintf("%d", id),
	}

	attemptURLs := []string{baseURL}
	seen := map[string]bool{baseURL: true}
	for i := 0; i < len(apiUrls); i++ {
		u := getURL(false)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		attemptURLs = append(attemptURLs, u)
	}

	var lastErr error
	for _, attemptURL := range attemptURLs {
		data, err := makeRequest("GET", attemptURL+"/track/", nil, queryParams, nil)
		if err != nil {
			lastErr = fmt.Errorf("get track failed on %s: %w", attemptURL, err)
			continue
		}

		var resp struct {
			Version string `json:"version"`
			Data    struct {
				TrackID            int     `json:"trackId"`
				AssetPresentation  string  `json:"assetPresentation"`
				AudioMode          string  `json:"audioMode"`
				AudioQuality       string  `json:"audioQuality"`
				ManifestMimeType   string  `json:"manifestMimeType"`
				ManifestHash       string  `json:"manifestHash"`
				Manifest           string  `json:"manifest"`
				AlbumReplayGain    float64 `json:"albumReplayGain"`
				AlbumPeakAmplitude float64 `json:"albumPeakAmplitude"`
				TrackReplayGain    float64 `json:"trackReplayGain"`
				TrackPeakAmplitude float64 `json:"trackPeakAmplitude"`
				BitDepth           int     `json:"bitDepth"`
				SampleRate         int     `json:"sampleRate"`
			} `json:"data"`
		}

		if err := json.NewDecoder(data.Body).Decode(&resp); err != nil {
			data.Body.Close()
			lastErr = fmt.Errorf("failed to parse track response from %s: %w", attemptURL, err)
			continue
		}
		data.Body.Close()

		urls, manifestMimeType, err := decodeManifest(resp.Data.Manifest)
		if err != nil {
			lastErr = fmt.Errorf("failed to decode manifest from %s: %w", attemptURL, err)
			continue
		}
		return urls, manifestMimeType, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("get track failed: no upstream URL available")
	}
	return nil, "", lastErr
}

var activeJobs sync.Map // id -> *TrackState
var downloadSlots = make(chan struct{}, 3)

type DownloadState struct {
	mutex     sync.Mutex
	done      bool
	readers   int
	finalPath string
	tmpPath   string
}

func handleStream() gin.HandlerFunc {
	return func(c *gin.Context) {
		handleDownload(c, true)
	}
}
func preloadHiFiTrack() gin.HandlerFunc {
	return func(c *gin.Context) {
		handleDownload(c, false)
	}
}

func handleDownload(c *gin.Context, stream bool) {
	trackID, _ := strconv.Atoi(c.Query("id"))
	basePath := generatePath(trackID)

	// if already downloaded, serve the file
	if matches, _ := filepath.Glob(basePath + ".*"); len(matches) > 0 {
		for _, m := range matches {
			if !strings.HasSuffix(m, ".tmp") {
				c.File(m)
				return
			}
		}
	}

	actual, loaded := activeJobs.LoadOrStore(trackID, &DownloadState{})
	// wait for the existing job to finish if there is one
	if loaded {
		// stream while we wait
		job := actual.(*DownloadState)
		if stream {
			job.mutex.Lock()
			job.readers++
			job.mutex.Unlock()
			defer func() {
				job.mutex.Lock()
				job.readers--
				if job.readers == 0 && job.done {
					os.Remove(job.tmpPath)
				}
				if job.done {
					activeJobs.Delete(trackID)
				}
				job.mutex.Unlock()
			}()
			for {
				job.mutex.Lock()
				tmp := job.tmpPath
				done := job.done
				job.mutex.Unlock()

				if tmp != "" {
					streamFile(c, job)
					return
				}
				if done {
					c.File(job.finalPath)
				}

				time.Sleep(20 * time.Millisecond)
			}
		}
		return
	}
	job := actual.(*DownloadState)
	go func() {
		_, err := downloadTrack(trackID, job)
		if err != nil {
			fmt.Println("download error:", err)
		}
	}()
	if stream {
		job.mutex.Lock()
		job.readers++
		job.mutex.Unlock()
		defer func() {
			job.mutex.Lock()
			job.readers--
			if job.readers == 0 && job.done {
				os.Remove(job.tmpPath)
			}
			if job.done {
				activeJobs.Delete(trackID)
			}
			job.mutex.Unlock()
		}()
		for {
			job.mutex.Lock()
			tmp := job.tmpPath
			done := job.done
			job.mutex.Unlock()

			if tmp != "" {
				streamFile(c, job)
				return
			}
			if done {
				c.File(job.finalPath)
			}

			time.Sleep(20 * time.Millisecond)
		}
	}
}

func downloadTrack(trackID int, job *DownloadState) (string, error) {
	downloadSlots <- struct{}{}
	defer func() { <-downloadSlots }()

	urls, segmentMimeType, err := hiFiTrackURLs(getURL(false), trackID)
	if err != nil {
		return "", err
	}

	ext := extFromMime(segmentMimeType)
	finalPath := generatePath(trackID) + ext
	tmpPath := finalPath + ".tmp"

	os.MkdirAll(filepath.Dir(tmpPath), 0755)

	// stream while writing
	localFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", err
	}

	// set tmpPath immediately
	job.mutex.Lock()
	job.tmpPath = tmpPath
	job.mutex.Unlock()

	client := &http.Client{Timeout: 20 * time.Second}
	for _, u := range urls {
		segmentErr := error(nil)
		for attempt := 1; attempt <= 3; attempt++ {
			resp, err := client.Get(u)
			if err != nil {
				segmentErr = err
				time.Sleep(time.Duration(attempt*150) * time.Millisecond)
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				segmentErr = fmt.Errorf("segment returned status %d", resp.StatusCode)
				time.Sleep(time.Duration(attempt*150) * time.Millisecond)
				continue
			}

			_, err = io.Copy(localFile, resp.Body)
			resp.Body.Close()
			if err != nil {
				segmentErr = err
				time.Sleep(time.Duration(attempt*150) * time.Millisecond)
				continue
			}

			segmentErr = nil
			break
		}

		if segmentErr != nil {
			return "", fmt.Errorf("failed downloading track %d segment: %w", trackID, segmentErr)
		}

		// flush to disk to ensure streamer sees data
		localFile.Sync()
	}

	localFile.Close()

	// move (atomic)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", err
	}

	job.mutex.Lock()
	job.done = true
	job.finalPath = finalPath
	job.mutex.Unlock()

	return finalPath, nil
}

func streamFile(c *gin.Context, job *DownloadState) {
	job.mutex.Lock()
	path := job.tmpPath
	job.mutex.Unlock()

	f, err := os.Open(path)
	if err != nil {
		c.Status(404)
		return
	}
	defer f.Close()

	c.Header("Content-Type", mimeFromPath(path))

	var offset int64 = 0
	buf := make([]byte, 64*1024)

	for {
		n, err := f.ReadAt(buf, offset)
		if n > 0 {
			c.Writer.Write(buf[:n])
			c.Writer.Flush()
			offset += int64(n)
		}

		if err == io.EOF {
			job.mutex.Lock()
			finished := job.done
			job.mutex.Unlock()

			if finished {
				// check for last remaining bytes
				n, _ = f.ReadAt(buf, offset)
				if n > 0 {
					c.Writer.Write(buf[:n])
				}
				return
			}
			// wait for more segments
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if err != nil {
			break
		}
	}
}

/*
	format

<?xml version='1.0' encoding='UTF-8'?>
<MPD

	xmlns="urn:mpeg:dash:schema:mpd:2011"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xlink="http://www.w3.org/1999/xlink"
	xmlns:cenc="urn:mpeg:cenc:2013" xsi:schemaLocation="urn:mpeg:dash:schema:mpd:2011 DASH-MPD.xsd" profiles="urn:mpeg:dash:profile:isoff-main:2011" type="static" minBufferTime="PT3.993S" mediaPresentationDuration="PT2M42.879S">
	<Period id="0">
		<AdaptationSet id="0" contentType="audio" mimeType="audio/mp4" segmentAlignment="true">
			<Representation id="0" codecs="flac" bandwidth="1667553" audioSamplingRate="44100">
				<SegmentTemplate timescale="44100" initialization="https://sp-ad-cf.audio.tidal.com/mediatracks/GisIAxInOWY0MTk5YzU0ZDJjZDhjNmQ3ZmYwNWFlMWU5ZjMzNGVfNjIubXA0IiAdAACAQCACKhChOXtGEg23OWwnOykkIQDmMgUNAACgQQ/0.mp4?Policy=eyJTdGF0ZW1lbnQiOiBbeyJSZXNvdXJjZSI6Imh0dHBzOi8vc3AtYWQtY2YuYXVkaW8udGlkYWwuY29tL21lZGlhdHJhY2tzL0dpc0lBeEluT1dZME1UazVZelUwWkRKalpEaGpObVEzWm1Zd05XRmxNV1U1WmpNek5HVmZOakl1YlhBMElpQWRBQUNBUUNBQ0toQ2hPWHRHRWcyM09Xd25PeWtrSVFEbU1nVU5BQUNnUVEvKiIsIkNvbmRpdGlvbiI6eyJEYXRlTGVzc1RoYW4iOnsiQVdTOkVwb2NoVGltZSI6MTc2Njg3MDQ3Mn19fV19&amp;Signature=vX~upSjXBUUTJ2gS0dhNukcZ3-CHhSMwj3S23Yus4jZ0EwBSbdBWOlfynrZlNbt6WKtB088jig39wV-xzDELyDygNN3oa8fM5p5UhT7OBJQg6C~iK9DgqexOdkvsxJLmYNPWRc2vBK~LdOlmj5UrlGflhXPtYN~6wh4id70yPrYrVCGtVIOVsfP2jzGD1M3Q2U910cn9gOaMyiOXIKgp9x1xEj8DeoPT6Ocxb4-G~kAS0aL9IHYMv~6F33Zu5J-Ju1HBcw2cGWR0Ufb4s4eJDhMdulGOXKFdNUYdtujdgiqpbIiWj5ViCbxrYkRPTFt9wgq-r2uQXMp8aHg7v0VNWQ__&amp;Key-Pair-Id=K14LZCZ9QUI4JL" media="https://sp-ad-cf.audio.tidal.com/mediatracks/GisIAxInOWY0MTk5YzU0ZDJjZDhjNmQ3ZmYwNWFlMWU5ZjMzNGVfNjIubXA0IiAdAACAQCACKhChOXtGEg23OWwnOykkIQDmMgUNAACgQQ/$Number$.mp4?Policy=eyJTdGF0ZW1lbnQiOiBbeyJSZXNvdXJjZSI6Imh0dHBzOi8vc3AtYWQtY2YuYXVkaW8udGlkYWwuY29tL21lZGlhdHJhY2tzL0dpc0lBeEluT1dZME1UazVZelUwWkRKalpEaGpObVEzWm1Zd05XRmxNV1U1WmpNek5HVmZOakl1YlhBMElpQWRBQUNBUUNBQ0toQ2hPWHRHRWcyM09Xd25PeWtrSVFEbU1nVU5BQUNnUVEvKiIsIkNvbmRpdGlvbiI6eyJEYXRlTGVzc1RoYW4iOnsiQVdTOkVwb2NoVGltZSI6MTc2Njg3MDQ3Mn19fV19&amp;Signature=vX~upSjXBUUTJ2gS0dhNukcZ3-CHhSMwj3S23Yus4jZ0EwBSbdBWOlfynrZlNbt6WKtB088jig39wV-xzDELyDygNN3oa8fM5p5UhT7OBJQg6C~iK9DgqexOdkvsxJLmYNPWRc2vBK~LdOlmj5UrlGflhXPtYN~6wh4id70yPrYrVCGtVIOVsfP2jzGD1M3Q2U910cn9gOaMyiOXIKgp9x1xEj8DeoPT6Ocxb4-G~kAS0aL9IHYMv~6F33Zu5J-Ju1HBcw2cGWR0Ufb4s4eJDhMdulGOXKFdNUYdtujdgiqpbIiWj5ViCbxrYkRPTFt9wgq-r2uQXMp8aHg7v0VNWQ__&amp;Key-Pair-Id=K14LZCZ9QUI4JL" startNumber="1">
					<SegmentTimeline>
						<S d="176128" r="39"/>
						<S d="137860"/>
					</SegmentTimeline>
				</SegmentTemplate>
				<Label>FLAC_HIRES</Label>
			</Representation>
		</AdaptationSet>
	</Period>

</MPD>
*/
