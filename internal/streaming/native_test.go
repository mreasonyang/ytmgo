package streaming

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// These tests never resolve media, run FFmpeg or open an audio device.
func nativeCommand(t *testing.T) func(context.Context, string) *exec.Cmd {
	t.Helper()
	path := os.Getenv("YTMGO_TEST_NATIVE_HOST")
	if path == "" {
		t.Skip("native helper fixture not selected")
	}
	return func(ctx context.Context, mode string) *exec.Cmd { return exec.CommandContext(ctx, path, mode) }
}
func TestNativeScriptIsolationAndCancellation(t *testing.T) {
	command := nativeCommand(t)
	h, err := startHost(command, "eval")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	var result []string
	if h.call(context.Background(), "[typeof process,typeof require,typeof fetch]", &result) != nil || len(result) != 3 || result[0] != "undefined" || result[1] != "undefined" || result[2] != "undefined" {
		t.Fatal("unexpected native script API")
	}
	var value int
	if h.call(context.Background(), "(()=>{globalThis.fixture=5;return 7})()", &value) != nil || value != 7 {
		t.Fatal("native eval failed")
	}
	var missing string
	if h.call(context.Background(), "typeof fixture", &missing) != nil || missing != "undefined" {
		t.Fatal("realm state survived")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	began := time.Now()
	if err := h.call(ctx, "(()=>{while(true){} })()", &value); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("native execution did not cancel")
	}
	h.Close()
	if time.Since(began) > 5*time.Second {
		t.Fatal("native process not reaped")
	}
}
func TestNativeProofDOMHasNoPlaybackAPIs(t *testing.T) {
	if os.Getenv("YTMGO_TEST_PROOF_DOM") != "1" {
		t.Skip("hidden offline DOM fixture not selected")
	}
	h, err := startHost(nativeCommand(t), "dom")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var v []string
	if err := h.call(ctx, "[typeof ytmgoProof.snapshot,typeof ytmgoProof.mint,typeof AudioContext,typeof Audio,document.cookie]", &v); err != nil {
		t.Fatal(err)
	}
	if len(v) != 5 || v[0] != "function" || v[1] != "function" || v[2] != "undefined" || v[3] != "undefined" || v[4] != "" {
		t.Fatal("proof realm contract mismatch")
	}
	var origin string
	if h.call(ctx, "location.origin", &origin) != nil || origin != "https://www.youtube.com" {
		t.Fatal("proof page lost its local document origin")
	}
	var agent string
	if h.call(ctx, "navigator.userAgent", &agent) != nil || agent != proofUserAgent {
		t.Fatal("proof realm agent differs from its challenge requests")
	}
	var isolated bool
	if h.call(ctx, "(async()=>{try{await fetch('https://example.invalid/');return false}catch{return true}})()", &isolated) != nil || !isolated {
		t.Fatal("proof page network not blocked")
	}
}

func TestNativePlayerExtractionAndTransform(t *testing.T) {
	command := nativeCommand(t)
	source := `(function(g){
 g.URL=function(url){this.input={url};this.values={n:'transformed-n'};this.set=function(k,v){this.values[k]=v};this.get=function(k){return this.values[k]};Object.setPrototypeOf(this,{})};
 var f=function(url,sp="",s=""){url=new g.URL(url);url.set("alr","yes");url.set(sp,s);return url};
 var st=function(){return {signatureTimestamp:12345}};
 })({});`
	h, err := startHost(command, "extract")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	var seed extracted
	if h.call(context.Background(), "extractPlayer("+jsJSON(source)+")", &seed) != nil || seed.Timestamp != 12345 || seed.Code == "" {
		t.Fatal("native extraction failed")
	}
	evaluator, err := startHost(command, "eval")
	if err != nil {
		t.Fatal(err)
	}
	defer evaluator.Close()
	var result struct{ Sig, N string }
	if evaluator.call(context.Background(), "(()=>{"+seed.Code+"\n"+transform+"\n"+`return process("input-n","sig","input-s");})()`, &result) != nil || result.Sig != "input-s" || result.N != "transformed-n" {
		t.Fatal("native transform failed")
	}
}
func TestAnonymousWarmOnly(t *testing.T) {
	if os.Getenv("YTMGO_TEST_ANONYMOUS_WARM") != "1" {
		t.Skip("public bootstrap-only validation not selected")
	}
	r, err := New(Options{Command: nativeCommand(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err = r.Configure(ctx, Session{Revision: 1}); err == nil {
		err = r.Warm(ctx)
	}
	if err != nil {
		var detail *Failure
		if errors.As(err, &detail) {
			t.Fatalf("warm failed: category=%s stage=%s step=%s status=%d script=%s", detail.Category, detail.Stage, detail.BootstrapStep, detail.HTTPStatus, detail.ScriptReason)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("warm timeout")
		}
		if errors.Is(err, context.Canceled) {
			t.Fatal("warm cancelled")
		}
		t.Fatal("warm failed without upstream diagnostic text")
	}
	if !r.ready || r.proof == nil || r.seed.Code == "" {
		t.Fatal("warm did not initialize")
	}
	// Mint the session binding as the old warm path did, without requesting any
	// /player endpoint or media bytes. Never print or persist the token.
	var proof string
	if r.proof.call(ctx, "ytmgoProof.mint("+jsJSON(r.binding.Binding)+")", &proof) != nil || len(proof) == 0 || len(proof) > 24<<10 {
		t.Fatal("warm proof mint failed")
	}
}

func TestPublicPlayerExtractionOnly(t *testing.T) {
	if os.Getenv("YTMGO_TEST_ANONYMOUS_WARM") != "1" {
		t.Skip("public script validation not selected")
	}
	command := nativeCommand(t)
	r, err := New(Options{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var script []byte
	fixture := os.Getenv("YTMGO_PUBLIC_SCRIPT_FIXTURE")
	if fixture != "" {
		script, _ = os.ReadFile(fixture)
	}
	if len(script) == 0 {
		api, err := r.request(ctx, "GET", "https://www.youtube.com/iframe_api", nil, nil, "bootstrap", "player_identifier")
		if err != nil {
			t.Fatal("public identifier request failed")
		}
		id := playerIDPattern.FindSubmatch(api)
		if len(id) != 2 {
			t.Fatal("public identifier parse failed")
		}
		script, err = r.request(ctx, "GET", "https://www.youtube.com/s/player/"+string(id[1])+"/player_es6.vflset/en_US/base.js", nil, nil, "bootstrap", "player_script")
		if err != nil {
			t.Fatal("public script request failed")
		}
		// Optional build-only fixture: this is the public, unauthenticated base.js,
		// never an account page, proof token or media URL. No body enters test logs.
		if fixture != "" {
			f, e := os.OpenFile(fixture, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				t.Fatal("public fixture creation failed")
			}
			_, e = f.Write(script)
			f.Close()
			if e != nil {
				t.Fatal("public fixture write failed")
			}
		}
	}
	h, err := startHost(command, "extract")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	var seed extracted
	if err = h.call(ctx, "extractPlayer("+jsJSON(string(script))+")", &seed); err != nil {
		t.Fatalf("public extraction: %s (%d input bytes)", err.Error(), len(script))
	}
	if seed.Timestamp <= 0 || seed.Code == "" {
		t.Fatal("public extraction incomplete")
	}
	evaluator, err := startHost(command, "eval")
	if err != nil {
		t.Fatal("public transform host failed")
	}
	defer evaluator.Close()
	var result struct{ N string }
	// Public script + synthetic n only: no signed URL, player request or media.
	err = evaluator.call(ctx, "(()=>{"+seed.Code+"\n"+transform+"\n"+`return process("synthetic-input-n","","");})()`, &result)
	if err != nil || result.N == "" || strings.HasPrefix(result.N, "enhanced_except_") {
		t.Fatal("public transform execution failed")
	}
	t.Logf("public extraction passed: input=%d output=%d bytes", len(script), len(seed.Code))
}
