package netproxy

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

// CFNetwork applies the current system proxy settings and exception rules.
// The API is loaded through the existing purego dependency, without a helper
// process, shell, new runtime, or access to saved proxy passwords.
type proxyAPI struct {
	settings func() uintptr
	proxies  func(uintptr, uintptr) uintptr
	url      func(uintptr, string, int64, uint32, uintptr) uintptr
	string   func(uintptr, string, uint32) uintptr
	text     func(uintptr, *byte, int64, uint32) bool
	count    func(uintptr) int64
	item     func(uintptr, int64) uintptr
	get      func(uintptr, uintptr) uintptr
	number   func(uintptr, int64, *int64) bool
	release  func(uintptr)
}

var nativeAPI = sync.OnceValue(func() *proxyAPI {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil
	}
	network, err := purego.Dlopen("/System/Library/Frameworks/CFNetwork.framework/CFNetwork", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil
	}
	a := &proxyAPI{}
	for _, binding := range []struct {
		fn   any
		lib  uintptr
		name string
	}{
		{&a.settings, network, "CFNetworkCopySystemProxySettings"}, {&a.proxies, network, "CFNetworkCopyProxiesForURL"},
		{&a.url, cf, "CFURLCreateWithBytes"}, {&a.string, cf, "CFStringCreateWithCString"}, {&a.text, cf, "CFStringGetCString"},
		{&a.count, cf, "CFArrayGetCount"}, {&a.item, cf, "CFArrayGetValueAtIndex"}, {&a.get, cf, "CFDictionaryGetValue"},
		{&a.number, cf, "CFNumberGetValue"}, {&a.release, cf, "CFRelease"},
	} {
		address, err := purego.Dlsym(binding.lib, binding.name)
		if err != nil {
			return nil
		}
		purego.RegisterFunc(binding.fn, address)
	}
	return a
})

func platformProxy(target *url.URL) (*url.URL, error) {
	a := nativeAPI()
	if a == nil {
		return nil, errProxy
	}
	const utf8 = 0x08000100
	// Proxy selection needs only the origin, never signed media URLs or queries.
	origin := (&url.URL{Scheme: target.Scheme, Host: target.Host, Path: "/"}).String()
	u := a.url(0, origin, int64(len(origin)), utf8, 0)
	if u == 0 {
		return nil, errProxy
	}
	defer a.release(u)
	settings := a.settings()
	if settings == 0 {
		return nil, errProxy
	}
	defer a.release(settings)
	proxies := a.proxies(u, settings)
	if proxies == 0 {
		return nil, errProxy
	}
	defer a.release(proxies)
	if a.count(proxies) == 0 {
		return nil, nil
	}
	entry := a.item(proxies, 0)
	value := func(key string) uintptr {
		k := a.string(0, key, utf8)
		if k == 0 {
			return 0
		}
		defer a.release(k)
		return a.get(entry, k)
	}
	text := func(v uintptr) string {
		var buffer [4096]byte
		if v == 0 || !a.text(v, &buffer[0], int64(len(buffer)), utf8) {
			return ""
		}
		return strings.TrimRight(string(buffer[:]), "\x00")
	}
	kind := text(value("kCFProxyTypeKey"))
	if kind == "kCFProxyTypeNone" {
		return nil, nil
	}
	scheme := "http"
	switch kind {
	case "kCFProxyTypeHTTP", "kCFProxyTypeHTTPS": // HTTPS uses HTTP CONNECT.
	case "kCFProxyTypeSOCKS":
		scheme = "socks5"
	default:
		return nil, errProxy // Do not silently bypass unsupported PAC routes.
	}
	host := text(value("kCFProxyHostNameKey"))
	var port int64
	p := value("kCFProxyPortNumberKey")
	if host == "" || strings.ContainsAny(host, "/@\r\n\x00") || p == 0 || !a.number(p, 4, &port) || port < 1 || port > 65535 {
		return nil, errProxy
	}
	return &url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.FormatInt(port, 10))}, nil
}
