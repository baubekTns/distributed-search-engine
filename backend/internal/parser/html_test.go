package parser

import (
	"strings"
	"testing"
)

func TestHTMLParser(t *testing.T) {
	document := `
		<!doctype html>
		<html>
			<head>
				<title> Test Documentation </title>
				<style>.hidden { display: none; }</style>
			</head>
			<body>
				<h1>Getting Started</h1>
				<p>Welcome to the documentation.</p>

				<a href="/install">Install</a>
				<a href="https://example.com/config">Config</a>
				<a href="https://other.example.com/external">External</a>
				<a href="#section">Fragment</a>

				<script>
					console.log("should not appear");
				</script>
			</body>
		</html>
	`

	htmlParser := NewHTMLParser(100)

	page, err := htmlParser.Parse(
		"https://example.com/docs/start",
		[]byte(document),
	)
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	if page.Title != "Test Documentation" {
		t.Errorf(
			"expected title %q, got %q",
			"Test Documentation",
			page.Title,
		)
	}

	if !strings.Contains(page.Text, "Getting Started") {
		t.Error("expected visible heading in page text")
	}

	if !strings.Contains(
		page.Text,
		"Welcome to the documentation.",
	) {
		t.Error("expected paragraph in page text")
	}

	if strings.Contains(page.Text, "console.log") {
		t.Error("script content should not be included")
	}

	expectedLinks := map[string]bool{
		"https://example.com/install": true,
		"https://example.com/config":  true,
	}

	if len(page.Links) != len(expectedLinks) {
		t.Fatalf(
			"expected %d links, got %d: %#v",
			len(expectedLinks),
			len(page.Links),
			page.Links,
		)
	}

	for _, link := range page.Links {
		if !expectedLinks[link] {
			t.Errorf("unexpected link: %s", link)
		}
	}
}

func TestHTMLParserLimitsLinks(t *testing.T) {
	document := `
		<html>
			<body>
				<a href="/one">One</a>
				<a href="/two">Two</a>
				<a href="/three">Three</a>
			</body>
		</html>
	`

	htmlParser := NewHTMLParser(2)

	page, err := htmlParser.Parse(
		"https://example.com/",
		[]byte(document),
	)
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	if len(page.Links) != 2 {
		t.Fatalf(
			"expected 2 links, got %d",
			len(page.Links),
		)
	}
}
