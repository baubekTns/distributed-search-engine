package security

import (
	"context"
	"net"
	"net/url"
	"testing"
)

type fakeResolver struct {
	addresses []net.IPAddr
	err       error
}

func (r fakeResolver) LookupIPAddr(
	ctx context.Context,
	host string,
) ([]net.IPAddr, error) {
	return r.addresses, r.err
}

func TestDestinationValidatorRejectsPrivateAddress(t *testing.T) {
	validator := NewDestinationValidator(fakeResolver{
		addresses: []net.IPAddr{
			{IP: net.ParseIP("192.168.1.10")},
		},
	})

	target, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := validator.Validate(
		context.Background(),
		target,
	); err == nil {
		t.Fatal("expected private destination to be rejected")
	}
}

func TestDestinationValidatorAcceptsPublicAddress(t *testing.T) {
	validator := NewDestinationValidator(fakeResolver{
		addresses: []net.IPAddr{
			{IP: net.ParseIP("93.184.216.34")},
		},
	})

	target, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	ips, err := validator.Validate(
		context.Background(),
		target,
	)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if len(ips) != 1 {
		t.Fatalf("expected one IP, got %d", len(ips))
	}
}
