package streaming

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type scriptHost interface {
	call(context.Context, string, any) error
	Close()
}
type Resolver struct {
	gate          chan struct{}
	life          context.Context
	cancel        context.CancelFunc
	stateMu       sync.Mutex
	active        context.CancelFunc
	closed        bool
	closeOnce     sync.Once
	client        *http.Client
	newHost       func(string) (scriptHost, error)
	session       Session
	sessionClient map[string]any
	idleTimer     *time.Timer
	ready         bool
	seed          extracted
	binding       proofBinding
	expires       time.Time
	proof         scriptHost
	leaseMu       sync.Mutex
	leases        map[*Lease]struct{}
	slots         chan struct{}
}

func New(options Options) (*Resolver, error) {
	if options.Command == nil {
		return nil, errors.New("runtime_missing")
	}
	life, cancel := context.WithCancel(context.Background())
	r := &Resolver{gate: make(chan struct{}, 1), life: life, cancel: cancel, client: transportClient(options.Client), leases: map[*Lease]struct{}{}, slots: make(chan struct{}, 4)}
	r.newHost = func(mode string) (scriptHost, error) { return startHost(options.Command, mode) }
	return r, nil
}
func (r *Resolver) enter(ctx context.Context) (context.Context, func(), error) {
	select {
	case r.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-r.life.Done():
		return nil, nil, errors.New("runtime_closed")
	}
	r.stateMu.Lock()
	if r.closed {
		r.stateMu.Unlock()
		<-r.gate
		return nil, nil, errors.New("runtime_closed")
	}
	op, cancel := context.WithTimeout(ctx, 45*time.Second)
	r.active = cancel
	r.stateMu.Unlock()
	stop := context.AfterFunc(r.life, cancel)
	return op, func() {
		stop()
		cancel()
		r.stateMu.Lock()
		r.active = nil
		r.stateMu.Unlock()
		<-r.gate
	}, nil
}
func (r *Resolver) Configure(ctx context.Context, s Session) error {
	if err := s.Validate(); err != nil {
		return err
	}
	r.stateMu.Lock()
	if r.active != nil {
		r.active()
	}
	r.stateMu.Unlock()
	ctx, done, err := r.enter(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Revision < r.session.Revision {
		return errors.New("invalid_account_session")
	}
	if s.Revision == r.session.Revision && (s.LoggedIn != r.session.LoggedIn || s.AccountIndex != r.session.AccountIndex || s.OnBehalfOfUser != r.session.OnBehalfOfUser) {
		return errors.New("unexpected_authentication")
	}
	if s == r.session && r.ready {
		return nil
	}
	if s.Revision != r.session.Revision {
		r.revokeAll()
	}
	r.resetHosts()
	r.seed = extracted{}
	r.binding = proofBinding{}
	r.sessionClient = nil
	r.session = s
	return nil
}
func (r *Resolver) resetHosts() {
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	if r.proof != nil {
		r.proof.Close()
	}
	r.proof = nil
	r.ready = false
	r.expires = time.Time{}
}
func (r *Resolver) Close() {
	r.closeOnce.Do(func() {
		r.stateMu.Lock()
		r.closed = true
		r.cancel()
		if r.active != nil {
			r.active()
		}
		r.stateMu.Unlock()
		r.gate <- struct{}{}
		defer func() { <-r.gate }()
		r.resetHosts()
		r.revokeAll()
		r.session = Session{}
		r.seed = extracted{}
		r.client.CloseIdleConnections()
	})
}
func (r *Resolver) revokeAll() {
	r.leaseMu.Lock()
	all := make([]*Lease, 0, len(r.leases))
	for l := range r.leases {
		all = append(all, l)
	}
	r.leaseMu.Unlock()
	for _, l := range all {
		l.Close()
	}
}

var playerIDPattern = regexp.MustCompile(`player\\?/([A-Za-z0-9_-]{1,128})\\?/`)

func (r *Resolver) warm(ctx context.Context) error {
	if r.session.Revision == 0 {
		return failure("init_failed", "sdk_create")
	}
	if r.ready && time.Until(r.expires) > time.Minute {
		return nil
	}
	r.resetHosts()
	if r.sessionClient == nil {
		if err := r.bootstrap(ctx); err != nil {
			return err
		}
	}
	if r.seed.Code == "" {
		data, err := r.request(ctx, "GET", "https://www.youtube.com/iframe_api", nil, nil, "bootstrap", "player_identifier")
		if err != nil {
			return err
		}
		match := playerIDPattern.FindSubmatch(data)
		if len(match) != 2 {
			return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_identifier"}
		}
		script, err := r.request(ctx, "GET", "https://www.youtube.com/s/player/"+string(match[1])+"/player_es6.vflset/en_US/base.js", nil, nil, "bootstrap", "player_script")
		if err != nil {
			return err
		}
		h, err := r.newHost("extract")
		if err != nil {
			return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_script", ScriptReason: "host_start"}
		}
		var seed extracted
		err = h.call(ctx, "extractPlayer("+jsJSON(string(script))+")", &seed)
		h.Close()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err.Error() == "script_memory_limit" || err.Error() == "script_output_limit" {
				return &Failure{Category: "cache_limit", Stage: "bootstrap", BootstrapStep: "player_script"}
			}
			if err.Error() == "script_timeout" {
				return &Failure{Category: "request_timeout", Stage: "bootstrap", BootstrapStep: "player_script"}
			}
			reason := "script_failed"
			switch err.Error() {
			case "script_stack_limit", "player_timestamp_missing", "player_function_missing", "script_reference_error", "script_type_error", "script_syntax_error", "script_internal_error", "script_range_error":
				reason = err.Error()
			}
			return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_script", ScriptReason: reason}
		}
		if seed.Timestamp <= 0 || seed.Timestamp > 1000000 || len(seed.Code) == 0 || len(seed.Code) > 8<<20 {
			return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_script", ScriptReason: "invalid_extraction"}
		}
		r.seed = seed
	}
	if r.session.LoggedIn {
		response, err := r.api(ctx, "browse", map[string]any{"browseId": "FEmusic_library_landing"}, "account_validation")
		if err != nil {
			return err
		}
		if !acceptedAccount(response) {
			return failure("login_required", "account_validation")
		}
	}
	if r.session.MusicPremium {
		r.expires = time.Now().Add(6 * time.Hour)
	} else {
		req, _ := http.NewRequest("GET", musicOrigin+"/", nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Accept-Language", "en-US,en;q=0.7")
		r.session.authorize(req, false)
		page, err := r.request(ctx, "GET", musicOrigin+"/", nil, req.Header, "proof_config_request", "")
		if err != nil {
			return err
		}
		binding, err := parseBinding(page, r.session)
		if err != nil {
			return failure("proof_request_rejected", "proof_config_parse")
		}
		r.binding = binding
		if err = r.warmProof(ctx); err != nil {
			r.resetHosts()
			return err
		}
	}
	if ctx.Err() != nil {
		r.resetHosts()
		return ctx.Err()
	}
	r.ready = true
	// The timer only closes an idle generation, never an in-flight operation.
	expiry := r.expires
	r.idleTimer = time.AfterFunc(max(time.Until(expiry.Add(-time.Minute)), 0), func() {
		select {
		case r.gate <- struct{}{}:
		case <-r.life.Done():
			return
		}
		defer func() { <-r.gate }()
		if r.expires == expiry {
			r.resetHosts()
		}
	})
	return nil
}
func acceptedAccount(v map[string]any) bool {
	if v["error"] != nil || valueAt(v, "responseContext", "errors") != nil || valueAt(v, "responseContext", "mainAppWebResponseContext", "loggedOut") == true {
		return false
	}
	if _, ok := v["contents"].(map[string]any); !ok {
		return false
	}
	services, _ := valueAt(v, "responseContext", "serviceTrackingParams").([]any)
	found := map[string]bool{}
	for _, svc := range services {
		service := textAt(svc, "service")
		params, _ := valueAt(svc, "params").([]any)
		for _, p := range params {
			key := textAt(p, "key")
			if service == "GFEEDBACK" && key == "logged_in" || service == "CSI" && key == "yt_li" {
				if textAt(p, "value") != "1" {
					return false
				}
				found[service] = true
			}
		}
	}
	return found["GFEEDBACK"] && found["CSI"]
}
func (r *Resolver) Warm(ctx context.Context) error {
	ctx, done, err := r.enter(ctx)
	if err != nil {
		return err
	}
	defer done()
	return r.warm(ctx)
}
func (r *Resolver) Resolve(ctx context.Context, id, kind string) (Media, error) {
	if !videoID.MatchString(id) || (kind != "audio" && kind != "video") {
		return Media{}, errors.New("invalid_video_id")
	}
	ctx, done, err := r.enter(ctx)
	if err != nil {
		return Media{}, err
	}
	defer done()
	if err = r.warm(ctx); err != nil {
		return Media{}, err
	}
	r.leaseMu.Lock()
	full := len(r.leases) >= 4
	r.leaseMu.Unlock()
	if full {
		return Media{}, errors.New("capacity_limit")
	}
	response, err := r.api(ctx, "player", map[string]any{"videoId": id, "racyCheckOk": true, "contentCheckOk": true, "playbackContext": map[string]any{"contentPlaybackContext": map[string]any{"vis": 0, "splay": false, "lactMilliseconds": "-1", "signatureTimestamp": r.seed.Timestamp}}}, "player_request")
	if err != nil {
		return Media{}, err
	}
	if status := textAt(response, "playabilityStatus", "status"); status != "OK" {
		if status == "LOGIN_REQUIRED" {
			return Media{}, failure("login_required", "player_request")
		}
		return Media{}, failure("playability_unavailable", "player_request")
	}
	selected, err := chooseFormat(response, kind)
	if err != nil {
		return Media{}, err
	}
	remote, err := r.decipher(ctx, selected)
	if err != nil {
		return Media{}, err
	}
	if r.proof != nil {
		binding := r.binding.Binding
		if r.binding.VideoBound {
			binding = id
		}
		var token string
		if err = r.proof.call(ctx, "ytmgoProof.mint("+jsJSON(binding)+")", &token); err != nil {
			r.resetHosts()
			return Media{}, failure("proof_failed", "proof_mint")
		}
		if token == "" || len(token) > 24<<10 {
			return Media{}, failure("proof_failed", "proof_mint")
		}
		q := remote.Query()
		q.Set("pot", token)
		remote.RawQuery = q.Encode()
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return Media{}, errors.New("stream_resolution_failed")
	}
	q := remote.Query()
	q.Set("cpn", base64.RawURLEncoding.EncodeToString(nonce))
	remote.RawQuery = q.Encode()
	if ctx.Err() != nil {
		return Media{}, ctx.Err()
	}
	lease, err := r.publish(remote, selected)
	if err != nil {
		return Media{}, err
	}
	if ctx.Err() != nil {
		lease.Close()
		return Media{}, ctx.Err()
	}
	return Media{Lease: lease, Duration: selected.duration, Width: selected.width, Height: selected.height}, nil
}
func (r *Resolver) warmProof(ctx context.Context) error {
	body, err := r.request(ctx, "GET", "https://www.youtube.com/", nil, http.Header{"Accept": {"text/html"}, "Accept-Language": {"en-US,en;q=0.7"}}, "proof_init", "")
	if err != nil {
		return err
	}
	proofFailure := func(reason string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Failure{Category: "proof_failed", Stage: "proof_init", ScriptReason: reason}
	}
	p, err := parseProofPage(body)
	if err != nil {
		return proofFailure("page_parse")
	}
	script, err := r.request(ctx, "GET", p.interpreter, nil, nil, "proof_init", "")
	if err != nil {
		return err
	}
	p.Script = string(script)
	r.proof, err = r.newHost("dom")
	if err != nil {
		return proofFailure("host_start")
	}
	var headers map[string]string
	if r.proof.call(ctx, "ytmgoProof.headers()", &headers) != nil {
		return proofFailure("headers_call")
	}
	h := http.Header{}
	for k, v := range headers {
		switch strings.ToLower(k) {
		case "content-type", "x-goog-api-key", "x-user-agent", "user-agent":
			if len(v) > 1024 || strings.ContainsAny(v, "\r\n") {
				return proofFailure("headers_invalid")
			}
			h.Set(k, v)
		default:
			return proofFailure("headers_invalid")
		}
	}
	if h.Get("Content-Type") != "application/json+protobuf" || h.Get("X-Goog-Api-Key") == "" {
		return proofFailure("headers_missing")
	}
	var snapshot string
	if err := r.proof.call(ctx, "ytmgoProof.snapshot("+jsJSON(p)+")", &snapshot); err != nil {
		switch err.Error() {
		case "proof_global_conflict", "proof_interpreter_failed", "proof_interpreter_missing", "proof_program_failed", "proof_snapshot_failed", "proof_snapshot_invalid", "proof_signals_missing":
			return proofFailure(err.Error())
		}
		return proofFailure("snapshot")
	}
	if len(snapshot) == 0 || len(snapshot) > 128<<10 {
		return proofFailure("snapshot")
	}
	// This is the public request identifier used by the MIT upstream example.
	request := []byte(jsJSON([]string{"O43z0dpjhgX20SCx4KAo", snapshot}))
	reply, err := r.request(ctx, "POST", "https://jnn-pa.googleapis.com/$rpc/google.internal.waa.v1.Waa/GenerateIT", request, h, "proof_init", "")
	if err != nil {
		return err
	}
	integrity, expires, err := parseIntegrity(reply)
	if err != nil {
		return err
	}
	var ok bool
	if r.proof.call(ctx, "ytmgoProof.integrity("+jsJSON(integrity)+")", &ok) != nil || !ok {
		return proofFailure("minter")
	}
	r.expires = expires
	return nil
}
