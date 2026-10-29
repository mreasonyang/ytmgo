package catalog

import "testing"

func TestLocalizedMetadataTypesDoNotBecomeCreators(t *testing.T) {
	for _, label := range []string{"Playlist", "Artist", "Album", "Song", "播放列表", "播放清单", "艺人", "专辑", "歌曲", "播放清單", "藝人", "專輯", "プレイリスト", "アーティスト", "アルバム", "曲"} {
		t.Run(label, func(t *testing.T) {
			artist, _, _ := parseMetadata([]any{words(label)})
			if artist != "" {
				t.Error("type-only metadata became a creator")
			}
			artist, _, _ = parseMetadata([]any{words(label + " • Actual Creator")})
			if artist != "Actual Creator" {
				t.Error("localized type hid the following creator")
			}
			artist, _, _ = parseMetadata([]any{object{"runs": []any{object{"text": label, "navigationEndpoint": browseEndpoint("UC_creator", "MUSIC_PAGE_TYPE_ARTIST")}}}})
			if artist != label {
				t.Error("explicitly linked creator was discarded as a type label")
			}
		})
	}
}

func TestLocalizedMetadataCountsDoNotBecomeCreators(t *testing.T) {
	for _, label := range []string{
		// Observed in the real Traditional Chinese library after type filtering.
		"25 集", "5.68萬位訂閱者", "127 集", "2,723 集",
		"25 songs", "1 song", "2,723 episodes", "5.68M subscribers", "100K views", "42 plays", "2.7k listeners", "1 video",
		"25 首歌曲", "5.68万位订阅者", "1.2亿次观看", "3,421 次播放", "观看次数：2.3万次",
		"25 首歌", "5.68萬名訂閱者", "1.2億次觀看", "觀看次數：2.3萬次",
		"25 曲", "2,723 エピソード", "5.68万人のチャンネル登録者", "チャンネル登録者数 5.68万人", "1.2万 回視聴", "視聴回数：2,723回", "再生回数 2.7万回",
	} {
		t.Run(label, func(t *testing.T) {
			artist, _, _ := parseMetadata([]any{words("Playlist • " + label)})
			if artist != "" {
				t.Error("numeric statistic became a creator")
			}
			artist, _, _ = parseMetadata([]any{words("Playlist • " + label + " • Actual Creator")})
			if artist != "Actual Creator" {
				t.Error("numeric statistic hid the following creator")
			}
			artist, _, _ = parseMetadata([]any{object{"runs": []any{object{"text": label, "navigationEndpoint": browseEndpoint("UC_creator", "MUSIC_PAGE_TYPE_ARTIST")}}}})
			if artist != label {
				t.Error("explicitly linked creator was discarded as a count")
			}
		})
	}
}

func TestMetadataCountFilterPreservesNonCountCreatorNames(t *testing.T) {
	for _, label := range []string{"The Songs", "The Subscribers", "She Plays", "Ocean Views", "127 集樂隊", "5.68萬個理由", "25 Songs of Summer", "100K Views Band", "2,72 episodes", "3.14", "100回の夏", "チャンネル登録者数の歌"} {
		t.Run(label, func(t *testing.T) {
			artist, _, _ := parseMetadata([]any{words("Playlist • " + label)})
			if artist != label {
				t.Error("non-count creator was discarded")
			}
		})
	}
}
