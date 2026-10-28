package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestBootstrapReplyMetadataSurvivesErrorAndPlayerState(t *testing.T) {
	data := []byte(`{"id":1,"ok":false,"category":"bootstrap_redirect","stage":"bootstrap","bootstrapStep":"session_data","httpStatus":302,"redirect":"google_challenge","mime":"html"}`)
	reply, err := parseWorkerReply(data)
	if err != nil {
		t.Fatal("valid bootstrap metadata rejected")
	}
	want := FailureDetails{Stage: "bootstrap", BootstrapStep: "session_data", HTTPStatus: 302, Redirect: "google_challenge", MIME: "html"}
	err = fmt.Errorf("wrapped fixture: %w", resolverFailureError(reply.Category, reply.details()))
	if DiagnosticDetails(err) != want || safeError(err) != "bootstrap_redirect" || strings.Contains(err.Error(), "google_challenge") {
		t.Fatal("closed metadata was lost or added to the error string")
	}
	p := &Player{gen: 1}
	p.fail(1, err)
	if state := p.Snapshot(); state.Error != "bootstrap_redirect" || state.ErrorDetails != want {
		t.Fatal("player state lost bootstrap metadata")
	}
	// A new unrelated failure must not retain the previous HTTP response.
	p.fail(1, errors.New("audio_stream_failed"))
	if p.Snapshot().ErrorDetails != (FailureDetails{}) {
		t.Fatal("old bootstrap metadata survived a new failure")
	}
	if !errors.Is(resolverFailureError("request_timeout", FailureDetails{Stage: "bootstrap"}), context.DeadlineExceeded) {
		t.Fatal("structured metadata changed timeout identity")
	}
}

func TestBootstrapReplyMetadataRejectsUntrustedIPC(t *testing.T) {
	base := map[string]any{"id": 1, "ok": false, "category": "init_failed", "stage": "bootstrap"}
	cases := []map[string]any{
		{"bootstrapStep": "private-value"}, {"bootstrapStep": ""}, {"bootstrapStep": nil}, {"bootstrapStep": 1},
		{"httpStatus": 0}, {"httpStatus": 99}, {"httpStatus": 600}, {"httpStatus": 302.5}, {"httpStatus": "302"}, {"httpStatus": nil},
		{"redirect": "https://example.invalid/private"}, {"redirect": ""}, {"redirect": nil}, {"redirect": true},
		{"mime": "text/html; private"}, {"mime": ""}, {"mime": nil}, {"mime": []string{"html"}},
		{"stage": "player_request", "httpStatus": 403}, {"stage": "private-stage", "bootstrapStep": "other"},
		{"ok": true, "bootstrapStep": "session_data"}, {"unexpectedPrivateField": "private-value"},
	}
	for number, changes := range cases {
		t.Run(fmt.Sprint(number), func(t *testing.T) {
			input := make(map[string]any, len(base)+len(changes))
			for key, value := range base {
				input[key] = value
			}
			for key, value := range changes {
				input[key] = value
			}
			data, _ := json.Marshal(input)
			if _, err := parseWorkerReply(data); err == nil || err.Error() != "invalid_resolver_reply" {
				t.Fatal("malformed metadata accepted or untrusted input escaped into error")
			}
		})
	}
	for _, input := range []string{
		`{"id":1,"ok":false,"category":"init_failed","stage":"bootstrap","mime":"html"}`,
		`{"id":1,"ok":false,"category":"init_failed","stage":"bootstrap","httpStatus":200}`,
		`{"id":1,"ok":false,"category":"init_failed","stage":"bootstrap","redirect":"other"}`,
		`{"id":1,"ok":false,"category":"init_failed","stage":"sdk_create"}`,
		`{"id":1,"ok":true,"safe":{},"media":{}}`,
	} {
		if _, err := parseWorkerReply([]byte(input)); err != nil {
			t.Fatal("current optional or non-bootstrap reply rejected")
		}
	}
}

func TestBootstrapCategoriesAreNotReducedToGenericFailure(t *testing.T) {
	for _, category := range []string{"bootstrap_redirect", "bootstrap_network_failed", "bootstrap_network_timeout", "bootstrap_tls_failed"} {
		err := resolverFailureError(category, FailureDetails{Stage: "bootstrap", BootstrapStep: "other"})
		if safeError(err) != category {
			t.Fatal("bootstrap category became generic")
		}
	}
}

func TestExplicitLoginRequirementSurvivesIntoPlayerState(t *testing.T) {
	p := &Player{gen: 1}
	p.fail(1, resolverFailureError("login_required", FailureDetails{Stage: "player_request"}))
	if p.Snapshot().Error != "login_required" {
		t.Fatal("explicit service sign-in requirement was reduced to a generic playback failure")
	}
}
