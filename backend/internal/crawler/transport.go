package crawler

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/baubekTns/distributed-search-engine/backend/internal/security"
)

func NewSafeTransport(
	validator *security.DestinationValidator,
) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,

		DialContext: func(
			ctx context.Context,
			network string,
			address string,
		) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("split destination address: %w", err)
			}

			targetURL := &url.URL{
				Scheme: "http",
				Host:   net.JoinHostPort(host, port),
			}

			ips, err := validator.Validate(ctx, targetURL)
			if err != nil {
				return nil, err
			}

			var lastErr error

			for _, ip := range ips {
				safeAddress := net.JoinHostPort(ip.String(), port)

				connection, dialErr := dialer.DialContext(
					ctx,
					network,
					safeAddress,
				)
				if dialErr == nil {
					return connection, nil
				}

				lastErr = dialErr
			}

			return nil, fmt.Errorf(
				"failed to connect to validated destination: %w",
				lastErr,
			)
		},

		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},

		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}
