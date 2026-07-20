package parser

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/net/html"

	"github.com/baubekTns/distributed-search-engine/backend/internal/frontier"
)

type HTMLParser struct {
	maxLinks int
}

func NewHTMLParser(maxLinks int) *HTMLParser {
	if maxLinks <= 0 {
		maxLinks = 100
	}

	return &HTMLParser{
		maxLinks: maxLinks,
	}
}

func (p *HTMLParser) Parse(
	pageURL string,
	body []byte,
) (Page, error) {
	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return Page{}, fmt.Errorf("parse page URL: %w", err)
	}

	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return Page{}, fmt.Errorf("parse HTML document: %w", err)
	}

	result := Page{
		URL:   pageURL,
		Title: extractTitle(document),
		Text:  extractVisibleText(document),
		Links: extractLinks(document, baseURL, p.maxLinks),
	}

	return result, nil
}

func extractTitle(node *html.Node) string {
	if node.Type == html.ElementNode &&
		strings.EqualFold(node.Data, "title") {

		if node.FirstChild != nil {
			return cleanWhitespace(node.FirstChild.Data)
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if title := extractTitle(child); title != "" {
			return title
		}
	}

	return ""
}

func extractVisibleText(root *html.Node) string {
	var builder strings.Builder

	var walk func(node *html.Node, ignored bool)

	walk = func(node *html.Node, ignored bool) {
		if node.Type == html.ElementNode && isIgnoredElement(node.Data) {
			ignored = true
		}

		if node.Type == html.TextNode && !ignored {
			text := cleanWhitespace(node.Data)

			if text != "" {
				if builder.Len() > 0 {
					builder.WriteByte(' ')
				}

				builder.WriteString(text)
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, ignored)
		}
	}

	walk(root, false)

	return cleanWhitespace(builder.String())
}

func extractLinks(
	root *html.Node,
	baseURL *url.URL,
	maxLinks int,
) []string {
	links := make([]string, 0)
	seen := make(map[string]struct{})

	var walk func(node *html.Node)

	walk = func(node *html.Node) {
		if len(links) >= maxLinks {
			return
		}

		if node.Type == html.ElementNode &&
			strings.EqualFold(node.Data, "a") {

			href := getAttribute(node, "href")

			if href != "" {
				normalized, ok := resolveAndNormalizeLink(
					baseURL,
					href,
				)

				if ok {
					if _, exists := seen[normalized]; !exists {
						seen[normalized] = struct{}{}
						links = append(links, normalized)
					}
				}
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)

			if len(links) >= maxLinks {
				return
			}
		}
	}

	walk(root)

	return links
}

func resolveAndNormalizeLink(
	baseURL *url.URL,
	href string,
) (string, bool) {
	href = strings.TrimSpace(href)

	if href == "" ||
		strings.HasPrefix(href, "#") {
		return "", false
	}

	reference, err := url.Parse(href)
	if err != nil {
		return "", false
	}

	resolved := baseURL.ResolveReference(reference)

	if !strings.EqualFold(resolved.Hostname(), baseURL.Hostname()) {
		return "", false
	}

	normalized, err := frontier.NormalizeURL(resolved.String())
	if err != nil {
		return "", false
	}

	return normalized, true
}

func getAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}

	return ""
}

func isIgnoredElement(name string) bool {
	switch strings.ToLower(name) {
	case "script",
		"style",
		"noscript",
		"svg",
		"canvas",
		"template":
		return true

	default:
		return false
	}
}

func cleanWhitespace(value string) string {
	return strings.Join(
		strings.FieldsFunc(value, unicode.IsSpace),
		" ",
	)
}

func SupportsContentType(contentType string) bool {
	switch contentType {
	case "text/html", "application/xhtml+xml":
		return true

	default:
		return false
	}
}
