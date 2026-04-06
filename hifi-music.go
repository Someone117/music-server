package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
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

// --- Raw API response (kept private, used only for parsing) ---

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

	raw, err := makeRequest("GET", baseURL+"/search/", nil, queryParams, nil)
	if err != nil {
		return SearchResult{}, fmt.Errorf("search request failed: %w", err)
	}

	var resp searchResponse
	if err := json.NewDecoder(raw.Body).Decode(&resp); err != nil {
		return SearchResult{}, fmt.Errorf("failed to parse search response: %w", err)
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
	raw, err := makeRequest("GET", baseURL+"/album/", nil, map[string]string{
		"id": fmt.Sprintf("%d", id),
	}, nil)
	if err != nil {
		return Album{}, nil, fmt.Errorf("get album failed: %w", err)
	}

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
		return nil, fmt.Errorf("get artist albums failed: %w", err)
	}

	var resp struct {
		Albums struct {
			Items []rawAlbum `json:"items"`
		} `json:"albums"`
	}

	if err := json.NewDecoder(raw.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to parse artist albums response: %w", err)
	}

	var albums []Album
	for _, r := range resp.Albums.Items {
		al := albumFromRaw(r)
		albums = append(albums, al)
	}
	return albums, nil
}

func getAlbumTracks(baseURL string, id int) ([]Track, error) {
	_, tracks, err := getAlbum(baseURL, id)
	return tracks, err
}

func decodeManifest(manifest string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	return decoded, nil
}

type dashManifest struct {
	Periods []struct {
		AdaptationSets []struct {
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

func buildDashSegmentURLs(decodedManifest []byte) ([]string, error) {
	var mpd dashManifest
	if err := xml.Unmarshal(decodedManifest, &mpd); err != nil {
		return nil, fmt.Errorf("failed to parse MPD manifest: %w", err)
	}

	if len(mpd.Periods) == 0 || len(mpd.Periods[0].AdaptationSets) == 0 || len(mpd.Periods[0].AdaptationSets[0].Representations) == 0 {
		return nil, fmt.Errorf("invalid MPD: missing period/adaptation set/representation")
	}

	tmpl := mpd.Periods[0].AdaptationSets[0].Representations[0].SegmentTemplate
	if tmpl.Initialization == "" || tmpl.Media == "" {
		return nil, fmt.Errorf("invalid MPD: missing SegmentTemplate initialization/media")
	}

	if !strings.Contains(tmpl.Media, "$Number$") {
		return nil, fmt.Errorf("unsupported MPD media template: expected $Number$")
	}

	startNumber := tmpl.StartNumber
	if startNumber <= 0 {
		startNumber = 1
	}

	segmentCount := 0
	for _, s := range tmpl.SegmentTimeline.Segments {
		repeat := s.Repeat
		if repeat < 0 {
			return nil, fmt.Errorf("unsupported MPD SegmentTimeline repeat value: %d", repeat)
		}
		segmentCount += repeat + 1
	}

	if segmentCount == 0 {
		return nil, fmt.Errorf("invalid MPD: no media segments in SegmentTimeline")
	}

	urls := make([]string, 0, segmentCount+1)
	urls = append(urls, tmpl.Initialization)
	for i := 0; i < segmentCount; i++ {
		n := startNumber + i
		urls = append(urls, strings.ReplaceAll(tmpl.Media, "$Number$", fmt.Sprintf("%d", n)))
	}

	return urls, nil
}

func downloadAndJoinURLs(urls []string) ([]byte, error) {
	var out bytes.Buffer
	for _, u := range urls {
		resp, err := makeRequest("GET", u, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to request segment: %w", err)
		}

		segmentBytes, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read segment: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("failed to close segment response body: %w", closeErr)
		}

		if _, err := out.Write(segmentBytes); err != nil {
			return nil, fmt.Errorf("failed to append segment: %w", err)
		}
	}

	return out.Bytes(), nil
}

func getTrackStream(baseURL string, id int, quality string) (string, string, error) {
	raw, err := makeRequest("GET", baseURL+"/track/", nil, map[string]string{
		"id":      fmt.Sprintf("%d", id),
		"quality": quality,
	}, nil)
	if err != nil {
		return "", "", fmt.Errorf("getTrackStream failed: %w", err)
	}

	defer raw.Body.Close()
	body, err := io.ReadAll(raw.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read body: %w", err)
	}

	var resp struct {
		Data struct {
			ManifestMimeType string `json:"manifestMimeType"`
			Manifest         string `json:"manifest"`
			AudioQuality     string `json:"audioQuality"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("failed to parse stream response: %w", err)
	}

	return resp.Data.Manifest, resp.Data.ManifestMimeType, nil
}

func getStreamURL(baseURL string, id int, quality string) (string, error) {
	manifest, mimeType, err := getTrackStream(baseURL, id, quality)
	if err != nil {
		return "", err
	}

	decoded, err := decodeManifest(manifest)
	if err != nil {
		return "", err
	}

	switch mimeType {
	case "application/vnd.tidal.bts":
		var bts struct {
			URLs []string `json:"urls"`
		}
		if err := json.Unmarshal(decoded, &bts); err != nil {
			return "", fmt.Errorf("failed to parse BTS manifest: %w", err)
		}
		if len(bts.URLs) == 0 {
			return "", fmt.Errorf("no URLs in manifest")
		}
		return bts.URLs[0], nil

	case "application/dash+xml":
		urls, err := buildDashSegmentURLs(decoded)
		if err != nil {
			return "", err
		}
		if len(urls) == 0 {
			return "", fmt.Errorf("no URLs in DASH manifest")
		}
		return urls[0], nil

	default:
		return "", fmt.Errorf("unknown manifest type: %s", mimeType)
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