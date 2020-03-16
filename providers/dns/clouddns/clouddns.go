// TODO Comments are lies
// Package clouddns implements a DNS provider for solving the DNS-01 challenge using CloudDNS API.
package clouddns

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-acme/lego/v3/challenge/dns01"
	"github.com/go-acme/lego/v3/platform/config/env"
)

// Config is used to configure the creation of the DNSProvider
type Config struct {
	ClientId  string
	Email     string
	Password  string

	TTL                int
	PropagationTimeout time.Duration
	PollingInterval    time.Duration
}

// NewDefaultConfig returns a default configuration for the DNSProvider
func NewDefaultConfig() *Config {
	return &Config{
		TTL:                env.GetOrDefaultInt("CLOUDDNS_TTL", 300),
		PropagationTimeout: env.GetOrDefaultSecond("CLOUDDNS_PROPAGATION_TIMEOUT", 120*time.Second),
		PollingInterval:    env.GetOrDefaultSecond("CLOUDDNS_POLLING_INTERVAL", 5*time.Second),
	}
}

// DNSProvider is an implementation of the challenge.Provider interface
// that uses CloudDNS API to manage TXT records for a domain.
type DNSProvider struct {
    client      *cloudDnsClient
	config      *Config
}

// NewDNSProvider returns a DNSProvider instance configured for CloudDNS.
// Credentials must be passed in the environment variables:
// CLOUDDNS_CLIENT_ID, CLOUDDNS_EMAIL, CLOUDDNS_PASSWORD.
func NewDNSProvider() (*DNSProvider, error) {
    config, err := NewDNSProviderConfig()
    if err != nil {
		return nil, err
    }
    client := NewCloudDnsClient(config.ClientId, config.Email, config.Password, config.TTL)
    return &DNSProvider{
        client: client,
        config: config,
    }, nil
}

// NewDNSProviderConfig return a DNSProvider instance configured for Digital Ocean.
func NewDNSProviderConfig() (*Config, error) {
	values, err := env.Get("CLOUDDNS_CLIENT_ID", "CLOUDDNS_EMAIL", "CLOUDDNS_PASSWORD")
    // FIXME these errors never get printed
	if err != nil {
		return nil, fmt.Errorf("clouddns: %v", err)
	}
	if values["CLOUDDNS_CLIENT_ID"] == "" {
		return nil, fmt.Errorf("clouddns: clientId missing")
	}

	if values["CLOUDDNS_EMAIL"] == "" {
		return nil, fmt.Errorf("cloudds: email missing")
	}

	if values["CLOUDDNS_PASSWORD"] == "" {
		return nil, fmt.Errorf("clouddns: password missing")
	}

	config := NewDefaultConfig()
	if config == nil {
		return nil, errors.New("clouddns: the configuration of the DNS provider is nil")
	}

	config.ClientId = values["CLOUDDNS_CLIENT_ID"]
	config.Email = values["CLOUDDNS_EMAIL"]
	config.Password = values["CLOUDDNS_PASSWORD"]
    return config, nil
}

// Timeout returns the timeout and interval to use when checking for DNS propagation.
// Adjusting here to cope with spikes in propagation times.
func (d *DNSProvider) Timeout() (timeout, interval time.Duration) {
	return d.config.PropagationTimeout, d.config.PollingInterval
}

// Present creates a TXT record using the specified parameters
func (d *DNSProvider) Present(domain, token, keyAuth string) error {
	fqdn, value := dns01.GetRecord(domain, keyAuth)

	authZone, err := dns01.FindZoneByFqdn(fqdn)
	if err != nil {
		return fmt.Errorf("clouddns: %v", err)
	}

	err = d.client.AddRecord(authZone, fqdn, value)
	if err != nil {
		return fmt.Errorf("clouddns: %v", err)
	}

	return nil
}

// CleanUp removes the TXT record matching the specified parameters
func (d *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	fqdn, _ := dns01.GetRecord(domain, keyAuth)

	authZone, err := dns01.FindZoneByFqdn(fqdn)
	if err != nil {
		return fmt.Errorf("clouddns: %v", err)
	}

	err = d.client.DeleteRecord(authZone, fqdn)
	if err != nil {
		return fmt.Errorf("clouddns: %v", err)
	}

	return nil
}

//func main() {
//    provider, err := NewDNSProvider()
//    if err != nil {
//		fmt.Println(err)
//        os.Exit(1)
//    }
//    fmt.Println("Adding challenge record")
//    provider.Present("lego.rodinnakniha.cz", "testtoken", "keyauth")
//    time.Sleep(time.Second * 20)
//    fmt.Println("Removing challenge record")
//    provider.CleanUp("lego.rodinnakniha.cz", "testtoken", "keyauth")
//}
