// Package httpfreshness admits explicitly public, fresh anonymous responses.
package httpfreshness

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func FreshUntil(headers http.Header, requested time.Time, maxAgeLimit time.Duration) time.Time {
	if _, setsCookie := headers["Set-Cookie"]; setsCookie {
		return time.Time{}
	}
	for field := range strings.SplitSeq(strings.ToLower(strings.Join(headers.Values("Vary"), ",")), ",") {
		switch strings.TrimSpace(field) {
		case "", "accept-encoding", "origin":
		default:
			return time.Time{}
		}
	}
	control := strings.ToLower(strings.Join(headers.Values("Cache-Control"), ","))
	if strings.Contains(control, "\"") {
		return time.Time{}
	}
	var public bool
	var maxAge int64 = -1
	for directive := range strings.SplitSeq(control, ",") {
		name, value, _ := strings.Cut(strings.TrimSpace(directive), "=")
		switch strings.TrimSpace(name) {
		case "public":
			public = true
		case "private", "no-store", "no-cache", "must-understand":
			return time.Time{}
		case "max-age":
			n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
			if err != nil || maxAge >= 0 {
				return time.Time{}
			}
			maxAge = int64(n)
		}
	}
	if !public || maxAge <= 0 {
		return time.Time{}
	}
	base := requested
	if age := headers.Get("Age"); age != "" {
		seconds, err := strconv.ParseUint(age, 10, 32)
		if err != nil || int64(seconds) >= maxAge {
			return time.Time{}
		}
		base = base.Add(-time.Duration(seconds) * time.Second)
	}
	if date, err := http.ParseTime(headers.Get("Date")); err == nil && date.Before(base) {
		base = date
	}
	expiry := base.Add(time.Duration(maxAge) * time.Second)
	if limit := requested.Add(maxAgeLimit); expiry.After(limit) {
		return limit
	}
	return expiry
}
