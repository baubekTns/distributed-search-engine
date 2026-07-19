package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var (
	ErrUnsupportedScheme = errors.New("only HTTP and HTTPS are supported")
	ErrMissingHostname   = errors.New("destination has no hostname")
	ErrUnsafeAddress     = errors.New("destination resolves to an unsafe IP address")
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type DestinationValidator struct {
	resolver Resolver
}

func NewDestinationValidator(resolver Resolver) *DestinationValidator {
	return &DestinationValidator{
		resolver: resolver,
	}
}

func (v *DestinationValidator) Validate(
	ctx context.Context,
	targetURL *url.URL,
) ([]net.IP, error) {
	if targetURL == nil {
		return nil, errors.New("destination URL is nil")
	}

	scheme := strings.ToLower(targetURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrUnsupportedScheme
	}

	hostname := targetURL.Hostname()
	if hostname == "" {
		return nil, ErrMissingHostname
	}

	if strings.EqualFold(hostname, "localhost") {
		return nil, ErrUnsafeAddress
	}

	// Handle URLs that contain an IP directly.
	if literalIP := net.ParseIP(hostname); literalIP != nil {
		if !isPublicIP(literalIP) {
			return nil, ErrUnsafeAddress
		}

		return []net.IP{literalIP}, nil
	}

	addresses, err := v.resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("resolve hostname: %w", err)
	}

	if len(addresses) == 0 {
		return nil, errors.New("hostname resolved to no IP addresses")
	}

	publicIPs := make([]net.IP, 0, len(addresses))

	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return nil, fmt.Errorf(
				"%w: %s resolved to %s",
				ErrUnsafeAddress,
				hostname,
				address.IP.String(),
			)
		}

		publicIPs = append(publicIPs, address.IP)
	}

	return publicIPs, nil
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	return !ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}
