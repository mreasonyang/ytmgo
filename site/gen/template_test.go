package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// "#top" is the top of the page: the HTML standard scrolls there, smoothly
// with the stylesheet's scroll-behavior, only while no element is called
// "top". Once the sticky nav was, the footer's link scrolled 69 px (the
// nav is always in view), and the nav's logo reloaded the page instead.
func TestTopLinksScrollToThePageTop(t *testing.T) {
	raw, err := os.ReadFile("../template.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if regexp.MustCompile(`\bid="top"`).MatchString(html) {
		t.Fatal(`an element is called "top": #top would scroll to it, not to the page's top`)
	}
	brands := regexp.MustCompile(`<a class="brand" href="([^"]*)"`).FindAllStringSubmatch(html, -1)
	if len(brands) != 2 {
		t.Fatalf("want the nav's and the footer's logo, found %d", len(brands))
	}
	for _, b := range brands {
		if b[1] != "#top" {
			t.Errorf("a logo links to %q: it reloads the page instead of gliding to its top", b[1])
		}
	}
	css, err := os.ReadFile("../assets/site.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "scroll-behavior: smooth") {
		t.Error("the page no longer glides to its links")
	}
}
