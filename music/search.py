#!/usr/bin/env python3
import sys
import json
from ytmusicapi import YTMusic
from fuzzywuzzy import fuzz
from fuzzywuzzy import process

def match_song(title, album_name, artists_names):
    """
    Match a Spotify song to YouTube Music with high accuracy.
    
    Args:
        title: Song title from Spotify
        album_name: Album name from Spotify
        artists_names: List or comma-separated string of artist names
    
    Returns:
        dict with matched song info or error
    """
    
    ytmusic = YTMusic()

    
    # Try multiple search strategies
    searches = [
        # Strategy 1: Title + Primary Artist (most specific)
        f"{title} {artists_names}",
        # Strategy 2: Title + Album + Primary Artist
        f"{title} {album_name} {artists_names}",
        # Strategy 3: Just title (broader)
        title,
        # Strategy 4: Title + by + artists
        f"{title} by {artists_names}",
    ]
    
    best_match = None
    best_score = 0
    
    for search_query in searches:
        try:
            results = ytmusic.search(search_query.strip(), filter="songs", limit=10)
            
            if not results:
                continue
            
            for result in results:
                if 'videoId' not in result:
                    continue
                
                # Skip live versions unless in original title
                if "live" in result.get("title", "").lower() and "live" not in title.lower():
                    continue
                
                # Skip remixes/covers unless specified
                if any(skip in result.get("title", "").lower() 
                       for skip in ["remix", "cover", "acoustic", "karaoke"]):
                    if not any(skip in title.lower() for skip in ["remix", "cover", "acoustic"]):
                        continue
                
                yt_title = result.get("title", "")
                yt_artist = result.get("artists", [{}])[0].get("name", "") if result.get("artists") else ""
                yt_album = result.get("album", {}).get("name", "") if result.get("album") else ""
                

                # Calculate multi-factor score
                title_score = fuzz.token_set_ratio(title.lower(), yt_title.lower())

                artist_score = fuzz.ratio(artists_names.lower(), yt_artist.lower()) 
                
                # Album matching (if album name exists)
                album_score = 0
                if album_name:
                    album_score = fuzz.ratio(album_name.lower(), yt_album.lower())
                
                # Combined scoring
                combined_score = (
                    title_score * 0.5 +
                    artist_score * 0.4 +
                    album_score * 0.1
                )
                
                # Bonus: exact or near-exact title match
                if fuzz.ratio(title.lower(), yt_title.lower()) > 95:
                    combined_score += 5
                
                # Bonus: primary artist match
                if fuzz.ratio(artists_names.lower(), yt_artist.lower()) > 90:
                    combined_score += 10
                
                # Bonus: our artist in yt artist string 
                # (ex: Artist feat. Someone or Artist & Someone)
                if artists_names.lower() in yt_artist.lower():
                    combined_score += 5
                
                
                if combined_score > best_score:
                    best_score = combined_score
                    best_match = result
        
        except Exception as e:
            continue
    
    if best_match and best_score >= 60:  # Threshold for acceptable match
        return {
            "success": True,
            "videoId": best_match.get("videoId"),
            "url": f"https://www.youtube.com/watch?v={best_match.get('videoId')}",
            "title": best_match.get("title"),
            "artist": best_match.get("artists", [{}])[0].get("name", ""),
            "album": best_match.get("album", {}).get("name", ""),
            "confidence_score": round(best_score, 2),
        }
    else:
        return {
            "success": False,
            "error": "No suitable match found",
            "best_score": round(best_score, 2) if best_match else 0
        }

# Command-line interface
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(json.dumps({"error": "Usage: script.py <Title> [Album] [Artists]"}))
        sys.exit(1)
    
    title = sys.argv[1]
    album = sys.argv[2] if len(sys.argv) > 2 else ""
    artists = sys.argv[3] if len(sys.argv) > 3 else ""
    
    result = match_song(title, album, artists)
    print(json.dumps(result, indent=2))