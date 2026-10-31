package streaming

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// Preserve the SDK's anonymous session bootstrap as a bounded JSON protocol.
// The generated visitor cookie is unrelated to the user's account cookies.
func (r *Resolver) bootstrap(ctx context.Context) error {
	nonce := make([]byte, 9)
	if _, err := rand.Read(nonce); err != nil {
		return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "session_data"}
	}
	h := http.Header{"Accept-Language": {"en-US"}, "Accept": {"*/*"}, "Referer": {"https://www.youtube.com/sw.js"}, "Cookie": {"PREF=tz=UTC;VISITOR_INFO1_LIVE=" + base64.RawURLEncoding.EncodeToString(nonce)[:11] + ";"}}
	raw, err := r.request(ctx, "GET", "https://www.youtube.com/sw.js_data", nil, h, "bootstrap", "session_data")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(raw), ")]}'\n") {
		return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "session_data"}
	}
	var data any
	if json.Unmarshal(raw[5:], &data) != nil {
		return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "session_data"}
	}
	at := func(v any, index int) any {
		a, ok := v.([]any)
		if !ok || index < 0 || index >= len(a) {
			return nil
		}
		return a[index]
	}
	device := at(at(at(at(data, 0), 2), 0), 0)
	if _, ok := device.([]any); !ok {
		return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "session_data"}
	}
	visitor, _ := at(device, 13).(string)
	if len(visitor) > 4096 || !visitorPattern.MatchString(visitor) {
		return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "session_data"}
	}
	client := map[string]any{"hl": "en", "gl": "US", "screenDensityFloat": 1, "screenHeightPoints": 1440, "screenPixelDensity": 1, "screenWidthPoints": 2560, "visitorData": visitor, "clientName": "WEB_REMIX", "clientVersion": musicVersion, "userAgent": userAgent, "platform": "DESKTOP", "clientFormFactor": "UNKNOWN_FORM_FACTOR", "userInterfaceTheme": "USER_INTERFACE_THEME_LIGHT", "timeZone": "UTC", "originalUrl": "https://www.youtube.com", "utcOffsetMinutes": 0, "memoryTotalKbytes": "8000000", "mainAppWebInfo": map[string]any{"graftUrl": "https://www.youtube.com", "pwaInstallabilityStatus": "PWA_INSTALLABILITY_STATUS_UNKNOWN", "webDisplayMode": "WEB_DISPLAY_MODE_BROWSER", "isWebNativeShareAvailable": true}}
	for key, index := range map[string]int{"remoteHost": 3, "deviceMake": 11, "deviceModel": 12, "osName": 17, "osVersion": 18, "timeZone": 79, "browserName": 86, "browserVersion": 87, "deviceExperimentId": 103, "rolloutToken": 107} {
		if s, ok := at(device, index).(string); ok && len(s) <= 65536 {
			client[key] = s
		}
	}
	if config, ok := at(device, 61).([]any); ok && len(config) > 0 {
		if app, ok := config[len(config)-1].(string); ok && len(app) <= 65536 {
			client["configInfo"] = map[string]any{"appInstallData": app}
		}
	}
	r.sessionClient = client
	return nil
}
