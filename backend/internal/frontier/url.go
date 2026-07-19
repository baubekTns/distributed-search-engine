package frontier

import (
	"errors"
	"net"
	"net/url"
	"path"
	"strings"
)

var (
	ErrEmptyURL          = errors.New("URL cannot be empty")
	ErrUnsupportedScheme = errors.New("only HTTP and HTTPS URLs are supported")
	ErrMissingHostname   = errors.New("URL must contain a hostname")
	ErrUserInfo          = errors.New("URLs containing usernames or passwords are not allowed")
)

func NormalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	if rawURL == "" {
		return "", ErrEmptyURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.New("invalid URL")
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrUnsupportedScheme
	}

	if parsed.Hostname() == "" {
		return "", ErrMissingHostname
	}

	if parsed.User != nil {
		return "", ErrUserInfo
	}

	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()

	switch {
	case parsed.Scheme == "http" && port == "80":
		parsed.Host = hostname
	case parsed.Scheme == "https" && port == "443":
		parsed.Host = hostname
	case port != "":
		parsed.Host = net.JoinHostPort(hostname, port)
	default:
		parsed.Host = hostname
	}

	parsed.Fragment = ""

	if parsed.Path == "" {
		parsed.Path = "/"
	} else {
		parsed.Path = path.Clean(parsed.Path)

		if !strings.HasPrefix(parsed.Path, "/") {
			parsed.Path = "/" + parsed.Path
		}
	}

	// url.Values.Encode sorts query keys, producing a more consistent URL.
	parsed.RawQuery = parsed.Query().Encode()

	return parsed.String(), nil
}
