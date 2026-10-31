//go:build !darwin

package netproxy

import "net/url"

// Other platforms retain standard HTTP(S)_PROXY and NO_PROXY support.
func platformProxy(*url.URL) (*url.URL, error) { return nil, nil }
