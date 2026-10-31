package streaming

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type format struct {
	raw           map[string]any
	length        int64
	contentType   string
	duration      time.Duration
	width, height int
}

var audioCodec = regexp.MustCompile(`codecs="(?:mp4a\.40\.2|opus)"`)
var videoCodec = regexp.MustCompile(`^video/mp4;.*codecs="avc1\.[a-fA-F0-9]+"`)

func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		n, e := strconv.ParseFloat(x, 64)
		if e == nil {
			return n
		}
	}
	return 0
}
func integer(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && math.Abs(n) <= 9007199254740991
}
func chooseFormat(data map[string]any, kind string) (format, error) {
	list, _ := valueAt(data, "streamingData", "adaptiveFormats").([]any)
	supported := []map[string]any{}
	candidates, clear := 0, 0
	for _, item := range list {
		f, ok := item.(map[string]any)
		if !ok || !strings.HasPrefix(textAt(f, "mimeType"), kind+"/") {
			continue
		}
		candidates++
		drm, _ := f["drmFamilies"].([]any)
		if len(drm) > 0 || f["drmTrackType"] != nil && f["drmTrackType"] != "" || f["fairPlayKeyUri"] != nil && f["fairPlayKeyUri"] != "" || f["type"] == "FORMAT_STREAM_TYPE_OTF" {
			continue
		}
		clear++
		if textAt(f, "url") == "" && textAt(f, "signatureCipher") == "" && textAt(f, "cipher") == "" {
			continue
		}
		mime := textAt(f, "mimeType")
		w, h := number(f["width"]), number(f["height"])
		if kind == "audio" && audioCodec.MatchString(mime) || kind == "video" && videoCodec.MatchString(mime) && integer(w) && integer(h) && w > 0 && w <= 1920 && h > 0 && h <= 480 {
			supported = append(supported, f)
		}
	}
	if len(supported) == 0 {
		category := "no_" + kind + "_format"
		if candidates > 0 && clear == 0 {
			category = "drm_unsupported"
		} else if textAt(data, "streamingData", "serverAbrStreamingUrl") != "" {
			category = "sabr_unsupported"
		}
		return format{}, failure(category, "decipher")
	}
	bitrate := func(f map[string]any) float64 {
		if f["averageBitrate"] != nil {
			return number(f["averageBitrate"])
		}
		return number(f["bitrate"])
	}
	sort.SliceStable(supported, func(i, j int) bool {
		a, b := supported[i], supported[j]
		if kind == "video" && number(a["height"]) != number(b["height"]) {
			return number(a["height"]) > number(b["height"])
		}
		return bitrate(a) > bitrate(b)
	})
	f := supported[0]
	seconds := number(f["approxDurationMs"]) / 1000
	if seconds == 0 {
		seconds = number(valueAt(data, "videoDetails", "lengthSeconds"))
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 86400 {
		return format{}, errors.New("invalid_duration")
	}
	length := number(f["contentLength"])
	if !integer(length) || length <= 0 || length > 64<<30 {
		return format{}, errors.New("invalid_content_length")
	}
	ct := "audio/mp4"
	if kind == "video" {
		ct = "video/mp4"
	} else if strings.Contains(textAt(f, "mimeType"), "opus") {
		ct = "audio/webm"
	}
	return format{raw: f, length: int64(length), contentType: ct, duration: time.Duration(seconds * float64(time.Second)), width: int(number(f["width"])), height: int(number(f["height"]))}, nil
}
func mediaURL(raw string) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 65536 || strings.ContainsAny(raw, "\r\n\x00'\\") {
		return nil, errors.New("invalid_media_url")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".googlevideo.com") || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return nil, errors.New("invalid_media_url")
	}
	if strings.HasPrefix(u.Query().Get("n"), "enhanced_except_") {
		return nil, errors.New("decipher_failed")
	}
	if u.Query().Get("sabr") == "1" {
		return nil, errors.New("sabr_unsupported")
	}
	return u, nil
}

// Derived from the MIT YouTube.js getNsigProcessorFn. Only the extracted
// transform executes here; all service and URL manipulation remains in Go.
const transform = `function process(n,sp,s){
 const f=exportedVars.nsigFunction;
 const u=f("https://ytjs.googlevideo.com/videoplayback?expire=1234567890&n="+encodeURIComponent(n),sp,s);
 for(const p of Object.getOwnPropertyNames(Object.getPrototypeOf(u))) {
  if(!['constructor','clone','set','get'].includes(p)&&typeof u[p]==='function')u[p]();
 }
 const sig=u.get(sp), value=u.get('n');
 return {sig:sig?decodeURIComponent(sig):undefined,n:value?decodeURIComponent(value):undefined};
}`

func (r *Resolver) decipher(ctx context.Context, f format) (*url.URL, error) {
	raw := textAt(f.raw, "url")
	cipher := textAt(f.raw, "signatureCipher")
	if cipher == "" {
		cipher = textAt(f.raw, "cipher")
	}
	args := url.Values{}
	if raw == "" {
		var err error
		args, err = url.ParseQuery(cipher)
		if err != nil {
			return nil, failure("decipher_failed", "decipher")
		}
		raw = args.Get("url")
	}
	remote, err := mediaURL(raw)
	if err != nil {
		return nil, err
	}
	q := remote.Query()
	if q.Has("pot") {
		return nil, failure("unexpected_authentication", "decipher")
	}
	n, s, sp := q.Get("n"), args.Get("s"), args.Get("sp")
	if n != "" || cipher != "" {
		if len(n) > 4096 || len(s) > 4096 || len(sp) > 128 {
			return nil, failure("decipher_failed", "decipher")
		}
		h, err := r.newHost("eval")
		if err != nil {
			return nil, failure("decipher_failed", "decipher")
		}
		// Each evaluation already uses a fresh realm. Keep no idle interpreter
		// process after the transform has been returned to Go.
		defer h.Close()
		var result struct {
			Sig *string `json:"sig"`
			N   *string `json:"n"`
		}
		code := "(()=>{" + r.seed.Code + "\n" + transform + "\nreturn process(" + jsJSON(n) + "," + jsJSON(sp) + "," + jsJSON(s) + ");})()"
		if err = h.call(ctx, code, &result); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, failure("decipher_failed", "decipher")
		}
		if cipher != "" {
			if result.Sig == nil || len(*result.Sig) > 8192 {
				return nil, failure("decipher_failed", "decipher")
			}
			if sp == "" {
				sp = "signature"
			}
			q.Set(sp, *result.Sig)
		}
		if n != "" {
			if result.N == nil || len(*result.N) > 8192 {
				return nil, failure("decipher_failed", "decipher")
			}
			q.Set("n", *result.N)
		}
	}
	if q.Get("c") == "WEB_REMIX" {
		q.Set("cver", musicVersion)
	}
	remote.RawQuery = q.Encode()
	return mediaURL(remote.String())
}
func parseIntegrity(data []byte) (map[string]any, time.Time, error) {
	var values []any
	if json.Unmarshal(data, &values) != nil || len(values) < 2 {
		return nil, time.Time{}, failure("proof_failed", "proof_init")
	}
	token, ok := values[0].(string)
	if !ok || len(token) == 0 || len(token) > 24<<10 {
		return nil, time.Time{}, failure("proof_failed", "proof_init")
	}
	ttl, ok := values[1].(float64)
	refresh := float64(0)
	if len(values) > 2 && values[2] != nil {
		var valid bool
		refresh, valid = values[2].(float64)
		if !valid {
			ok = false
		}
	}
	if !ok || ttl > 86400 || refresh < 0 || ttl-refresh <= 60 {
		return nil, time.Time{}, failure("proof_expired", "proof_init")
	}
	return map[string]any{"integrityToken": token, "estimatedTtlSecs": ttl, "mintRefreshThreshold": refresh}, time.Now().Add(time.Duration((ttl - refresh) * float64(time.Second))), nil
}
