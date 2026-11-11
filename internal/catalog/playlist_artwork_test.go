package catalog

import (
	"net/url"
	"strings"
	"testing"
)

const publicPlaylistCoverFixturePath = "https://i.ytimg.com/pl_c/PLSyntheticCover_0123456789abcdefXYZ/studio_square_thumbnail.jpg"

// Identity and query values are fictional; the thumbnail shape matches the
// anonymously observed studio playlist-thumbnail format.
func TestPublicPlaylistArtworkPreservesRenditionOptions(t *testing.T) {
	for _, query := range []string{
		"sqp=c3ludGhldGljLXJlbmRpdGlvbg&rs=U1lOVEhFVElDX1NJR05BVFVSRQ",
		"rs=U1lOVEhFVElDX1NJR05BVFVSRQ&sqp=c3ludGhldGljLXJlbmRpdGlvbg",
		"sqp=c3ludGhldGljLXJlbmRpdGlvbg%3D%3D&rs=U1lOVEhFVElDX1NJR05BVFVSRQ%3D",
	} {
		raw := publicPlaylistCoverFixturePath + "?" + query
		if safeArtwork(raw) != raw {
			t.Fatal("public playlist artwork lost required rendition options")
		}
		item := Item{Title: "Weekend discoveries", Artist: "Fixture Curator", Kind: "playlist", BrowseID: "VLPLSyntheticCover_0123456789abcdefXYZ", Artwork: raw}
		if item.LibraryMetadata().Artwork != raw {
			t.Fatal("public playlist artwork disappeared from saved display metadata")
		}
	}
	if safeArtwork(publicPlaylistCoverFixturePath) != publicPlaylistCoverFixturePath {
		t.Fatal("queryless CDN compatibility changed")
	}
}

func TestPublicPlaylistArtworkQueryBoundary(t *testing.T) {
	const options = "?sqp=c3ludGhldGlj&rs=c2lnbmF0dXJl"
	for name, raw := range map[string]string{
		"missing-signature":   publicPlaylistCoverFixturePath + "?sqp=c3ludGhldGlj",
		"missing-rendition":   publicPlaylistCoverFixturePath + "?rs=c2lnbmF0dXJl",
		"empty-signature":     publicPlaylistCoverFixturePath + "?sqp=c3ludGhldGlj&rs=",
		"duplicate-signature": publicPlaylistCoverFixturePath + options + "&rs=b3RoZXI",
		"duplicate-rendition": publicPlaylistCoverFixturePath + options + "&sqp=b3RoZXI",
		"extra-option":        publicPlaylistCoverFixturePath + options + "&account=synthetic",
		"invalid-value":       publicPlaylistCoverFixturePath + "?sqp=bad%2Fvalue&rs=c2lnbmF0dXJl",
		"too-much-padding":    publicPlaylistCoverFixturePath + "?sqp=bad%3D%3D%3D&rs=c2lnbmF0dXJl",
		"lookalike-host":      strings.Replace(publicPlaylistCoverFixturePath, "i.ytimg.com", "i.ytimg.com.evil.invalid", 1) + options,
		"other-cdn-host":      strings.Replace(publicPlaylistCoverFixturePath, "i.ytimg.com", "other.ytimg.com", 1) + options,
		"encoded-path":        strings.Replace(publicPlaylistCoverFixturePath, "/pl_c/", "/%70l_c/", 1) + options,
		"encoded-separator":   strings.Replace(publicPlaylistCoverFixturePath, "/pl_c/", "/pl_c%2F", 1) + options,
		"other-filename":      strings.Replace(publicPlaylistCoverFixturePath, "studio_square_thumbnail.jpg", "other.jpg", 1) + options,
		"fragment":            publicPlaylistCoverFixturePath + options + "#fixture",
		"port":                strings.Replace(publicPlaylistCoverFixturePath, "i.ytimg.com", "i.ytimg.com:443", 1) + options,
		"userinfo":            strings.Replace(publicPlaylistCoverFixturePath, "https://", "https://synthetic@", 1) + options,
		"http":                strings.Replace(publicPlaylistCoverFixturePath, "https://", "http://", 1) + options,
	} {
		t.Run(name, func(t *testing.T) {
			if safeArtwork(raw) != "" {
				t.Fatal("unsupported playlist artwork query passed admission")
			}
		})
	}
	// Existing video thumbnails still remove sizing/tracking options.
	video := "https://i.ytimg.com/vi/abc123DEF_-/hqdefault.jpg" + options
	parsed, _ := url.Parse(safeArtwork(video))
	if parsed == nil || parsed.RawQuery != "" || parsed.Path != "/vi/abc123DEF_-/hqdefault.jpg" {
		t.Fatal("ordinary video artwork canonicalization changed")
	}
}

func TestPlaylistCardParserSelectsLargestPublicCover(t *testing.T) {
	large := publicPlaylistCoverFixturePath + "?sqp=c3ludGhldGljLTU0NA&rs=c2lnbmF0dXJl"
	renderer := object{"musicTwoRowItemRenderer": object{
		"title": words("Weekend discoveries"), "subtitle": words("Fixture Curator"),
		"navigationEndpoint": browseEndpoint("VLPLSyntheticCover_0123456789abcdefXYZ", "MUSIC_PAGE_TYPE_PLAYLIST"),
		"thumbnailRenderer": object{"musicThumbnailRenderer": object{"thumbnail": object{"thumbnails": []any{
			object{"url": publicPlaylistCoverFixturePath + "?sqp=c3ludGhldGljLTYw&rs=c2lnbmF0dXJl", "width": float64(60), "height": float64(60)},
			object{"url": large, "width": float64(544), "height": float64(544)},
		}}}},
	}}
	page, _, err := parsePage(catalogResponse(renderer))
	if err != nil || len(page.Items) != 1 || page.Items[0].Artwork != large || page.Items[0].Kind != "playlist" {
		t.Fatal("visible playlist renderer lost its largest public cover")
	}
}
