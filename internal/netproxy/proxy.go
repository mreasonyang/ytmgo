// Package netproxy keeps catalogue and media traffic on the user's chosen route.
// Loopback always stays local. Explicit proxy environment settings take priority
// over platform settings; neither proxy configuration nor errors are logged.
package netproxy

import (
	"errors"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/net/http/httpproxy"
)

var errProxy = errors.New("proxy_configuration_unavailable")
var configured = sync.OnceValue(func() func(*http.Request) (*url.URL, error) {
	return resolver(*httpproxy.FromEnvironment(), platformProxy)
})

func Proxy(req *http.Request) (*url.URL, error) { return configured()(req) }

func resolver(config httpproxy.Config, platform func(*url.URL) (*url.URL, error)) func(*http.Request) (*url.URL, error) {
	environment := config.ProxyFunc()
	// Use the standard library's matching rules for NO_PROXY and loopback,
	// including when the selected proxy comes from platform settings.
	bypass := (&httpproxy.Config{HTTPProxy: "http://proxy.invalid:80", HTTPSProxy: "http://proxy.invalid:80", NoProxy: config.NoProxy}).ProxyFunc()
	return func(req *http.Request) (*url.URL, error) {
		if req == nil || req.URL == nil {
			return nil, errProxy
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return nil, nil
		}
		candidate, err := bypass(req.URL)
		if err != nil {
			return nil, errProxy
		}
		if candidate == nil {
			return nil, nil
		}
		if req.URL.Scheme == "https" && config.HTTPSProxy != "" || req.URL.Scheme == "http" && config.HTTPProxy != "" {
			candidate, err = environment(req.URL)
		} else {
			candidate, err = platform(req.URL)
		}
		if err != nil {
			return nil, errProxy
		}
		return candidate, nil
	}
}
