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
    try:
        ytmusic = YTMusic()

        # Try multiple search strategies
        searches = [
            # Strategy 1: Title + Primary Artist (most specific)
            f"{title} {artists_names}",
            # Strategy 2: Title + Album + Primary Artist
            f"{title} {album_name} {artists_names}",
            # Strategy 3: Title + by + artists
            f"{title} by {artists_names}",
            # Strategy 4: Title - cover/remix/live versions filtered
            f"{title.replace('cover', '').replace('remix', '').replace('live', '')} {artists_names}",
        ]

        best_match = None
        best_score = 0

        for search_query in searches:
            try:
                results = ytmusic.search(search_query.strip(), filter="songs", limit=10)

                if not results:
                    continue

                for result in results:
                    if "videoId" not in result:
                        continue

                    # Skip live versions unless in original title
                    if (
                        "live" in result.get("title", "").lower()
                        and "live" not in title.lower()
                    ):
                        continue
                    if (
                        "acoustic" in result.get("title", "").lower()
                        and "acoustic" not in title.lower()
                    ):
                        continue
                    if (
                        "instrumental" in result.get("title", "").lower()
                        and "instrumental" not in title.lower()
                    ):
                        continue
                    if (
                        "karaoke" in result.get("title", "").lower()
                        and "karaoke" not in title.lower()
                    ):
                        continue
                    if (
                        "cover" in result.get("title", "").lower()
                        and "cover" not in title.lower()
                    ):
                        continue
                    if (
                        "remix" in result.get("title", "").lower()
                        and "remix" not in title.lower()
                    ):
                        continue

                    yt_title = result.get("title", "").lower()
                    yt_artist = (
                        result.get("artists", [{}])[0].get("name", "").lower()
                        if result.get("artists")
                        else ""
                    )
                    yt_album = (
                        result.get("album", {}).get("name", "").lower()
                        if result.get("album")
                        else ""
                    )

                    # Clean titles from common noise
                    if "(" in yt_title and ")" in yt_title:
                        paren_content = yt_title[
                            yt_title.find("(") + 1 : yt_title.find(")")
                        ]
                        if any(
                            skip in paren_content
                            for skip in [
                                "official",
                                "video",
                                "music",
                                "lyrics",
                                "lyric",
                                "hd",
                                "4k",
                            ]
                        ):
                            yt_title = yt_title[: yt_title.find("(")].strip()
                    if "ft" not in title.lower():
                        yt_title = yt_title.split("ft.")[0].strip()
                    if "feat" not in title.lower():
                        yt_title = yt_title.split("feat.")[0].strip()
                    if "." not in title.lower():
                        yt_title = yt_title.split(".")[0].strip()
                    if "-" not in title.lower():
                        yt_title = yt_title.split("-")[0].strip()
                    if "official" not in title.lower():
                        yt_title = yt_title.replace("official", "").strip()
                    if "video" not in title.lower():
                        yt_title = yt_title.replace("video", "").strip()
                    if "music" not in title.lower():
                        yt_title = yt_title.replace("music", "").strip()
                    if "lyrics" not in title.lower():
                        yt_title = yt_title.replace("lyrics", "").strip()
                    if "lyric" not in title.lower():
                        yt_title = yt_title.replace("lyric", "").strip()
                    if "hd" not in title.lower():
                        yt_title = yt_title.replace("hd", "").strip()
                    if "4k" not in title.lower():
                        yt_title = yt_title.replace("4k", "").strip()
                    if "audio" not in title.lower():
                        yt_title = yt_title.replace("audio", "").strip()

                    # Calculate multi-factor score
                    title_score = fuzz.token_set_ratio(title.lower(), yt_title.lower())

                    artist_score = fuzz.ratio(artists_names.lower(), yt_artist.lower())

                    # Album matching (if album name exists)
                    album_score = 0
                    if album_name:
                        album_score = fuzz.ratio(album_name.lower(), yt_album.lower())

                    # Combined scoring
                    combined_score = (
                        title_score * 0.5 + artist_score * 0.4 + album_score * 0.1
                    )

                    # Bonus: exact or near-exact title match
                    if fuzz.ratio(title.lower(), yt_title.lower()) > 95:
                        combined_score += 5

                    # Bonus: primary artist match
                    if fuzz.ratio(artists_names.lower(), yt_artist.lower()) > 90:
                        combined_score += 10

                    if combined_score > best_score:
                        best_score = combined_score
                        best_match = result

            except Exception as e:
                continue

        if best_match and best_score > 60:  # Threshold for acceptable match
            return {
                "success": True,
                "videoId": best_match.get("videoId"),
                "url": f"https://www.youtube.com/watch?v={best_match.get('videoId')}",
                "title": best_match.get("title"),
                "artist": best_match.get("artists", [{}])[0].get("name", ""),
                "album": best_match.get("album", {}).get("name", ""),
                "confidence_score": round(best_score, 2),
                "error": "",
            }
        else:
            if best_match:
                return {
                    "success": False,
                    "videoId": best_match.get("videoId"),
                    "url": f"https://www.youtube.com/watch?v={best_match.get('videoId')}",
                    "title": best_match.get("title"),
                    "artist": best_match.get("artists", [{}])[0].get("name", ""),
                    "album": best_match.get("album", {}).get("name", ""),
                    "confidence_score": round(best_score, 2),
                    "error": "score",
                }
            return {
                "success": False,
                "error": "no match",
                "best_score": round(best_score, 2) if best_match else 0,
            }
    except Exception as e:
        return {"success": False, "error": str(e)}


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
