package acmedns

import (
	"errors"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/acmedns"
	"github.com/go-acme/lego/v4/providers/dns/clouddns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/easydns"
	"github.com/go-acme/lego/v4/providers/dns/gcloud"
	"github.com/go-acme/lego/v4/providers/dns/httpreq"
	"github.com/go-acme/lego/v4/providers/dns/hurricane"
	"github.com/go-acme/lego/v4/providers/dns/pdns"
	"github.com/go-acme/lego/v4/providers/dns/route53"
)

// init registers every supported DNS-01 provider. Each entry's RequiredFields /
// OptionalFields drives the UI form.
func init() {
	must(RegisterProvider(&ProviderInfo{
		Name:           "acmedns",
		Description:    "ACME-DNS external DNS server (RFC 8555 DNS-01 via acme-dns)",
		OptionalFields: []string{"apiBase", "allowList", "storagePath", "storageBaseUrl"},
		Factory:        factoryACMEDNS,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "cloudflare",
		Description:    "Cloudflare DNS API",
		RequiredFields: []string{"dnsApiToken"},
		OptionalFields: []string{"zoneApiToken", "dnsPropagationTimeout", "dnsTTL"},
		Factory:        factoryCloudflare,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "clouddns",
		Description:    "CloudDNS (vshosting) API",
		RequiredFields: []string{"clientId", "email", "password"},
		OptionalFields: []string{"dnsPropagationTimeout", "dnsPollingInterval", "dnsTTL"},
		Factory:        factoryCloudDNS,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "digitalocean",
		Description:    "DigitalOcean DNS API",
		RequiredFields: []string{"authToken"},
		OptionalFields: []string{"baseUrl", "dnsPropagationTimeout", "dnsPollingInterval", "dnsTTL"},
		Factory:        factoryDigitalOcean,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "easydns",
		Description:    "EasyDNS API",
		RequiredFields: []string{"token", "key"},
		OptionalFields: []string{"endpoint", "dnsPropagationTimeout", "dnsPollingInterval", "dnsSequenceInterval", "dnsTTL"},
		Factory:        factoryEasyDNS,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "gcloud",
		Description:    "Google Cloud DNS API",
		RequiredFields: []string{"project"},
		OptionalFields: []string{"serviceAccountKey", "serviceAccountFile", "zoneId", "allowPrivateZone", "impersonateServiceAccount", "dnsPropagationTimeout", "dnsPollingInterval", "dnsTTL", "debug"},
		Factory:        factoryGCloud,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "hurricane",
		Description:    "Hurricane Electric Free DNS",
		RequiredFields: []string{"credentials"},
		OptionalFields: []string{"dnsPropagationTimeout", "dnsPollingInterval", "dnsSequenceInterval"},
		Factory:        factoryHurricane,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "httpreq",
		Description:    "Generic HTTP endpoint for DNS operations (lego httpreq)",
		RequiredFields: []string{"endpoint"},
		OptionalFields: []string{"mode", "username", "password", "dnsPropagationTimeout", "dnsPollingInterval"},
		Factory:        factoryHTTPReq,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "pdns",
		Description:    "PowerDNS Authoritative Server API",
		RequiredFields: []string{"apiKey", "host"},
		OptionalFields: []string{"serverName", "apiVersion", "dnsPropagationTimeout", "dnsPollingInterval", "dnsTTL"},
		Factory:        factoryPowerDNS,
	}))
	must(RegisterProvider(&ProviderInfo{
		Name:           "route53",
		Description:    "Amazon Route53 DNS API",
		RequiredFields: []string{"accessKeyId", "secretAccessKey", "region"},
		OptionalFields: []string{"hostedZoneId", "dnsPropagationTimeout", "dnsTTL"},
		Factory:        factoryRoute53,
	}))
}

// must escalates an init-time registration error (always a programming bug).
func must(err error) {
	if err != nil {
		panic("acmedns: " + err.Error())
	}
}

// --- factories ------------------------------------------------------------
//
// Each factory pulls operator-supplied values from the config map (with the
// same camelCase keys the UI emits), translates them into a lego provider
// config, and constructs a challenge.Provider.

func factoryACMEDNS(c map[string]string) (challenge.Provider, error) {
	cfg := acmedns.NewDefaultConfig()
	if v := getString(c, "apiBase", ""); v != "" {
		cfg.APIBase = v
	}
	cfg.AllowList = getStringSlice(c, "allowList")
	cfg.StoragePath = getString(c, "storagePath", "")
	cfg.StorageBaseURL = getString(c, "storageBaseUrl", "")
	return acmedns.NewDNSProviderConfig(cfg)
}

func factoryCloudflare(c map[string]string) (challenge.Provider, error) {
	cfg := cloudflare.NewDefaultConfig()
	cfg.AuthToken = getString(c, "dnsApiToken", "")
	cfg.ZoneToken = getString(c, "zoneApiToken", "")
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsTTL", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.TTL = int(v)
	}
	return cloudflare.NewDNSProviderConfig(cfg)
}

func factoryCloudDNS(c map[string]string) (challenge.Provider, error) {
	cfg := clouddns.NewDefaultConfig()
	cfg.ClientID = getString(c, "clientId", "")
	cfg.Email = getString(c, "email", "")
	cfg.Password = getString(c, "password", "")
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsPollingInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PollingInterval = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsTTL", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.TTL = int(v)
	}
	return clouddns.NewDNSProviderConfig(cfg)
}

func factoryDigitalOcean(c map[string]string) (challenge.Provider, error) {
	cfg := digitalocean.NewDefaultConfig()
	cfg.AuthToken = getString(c, "authToken", "")
	if v := getString(c, "baseUrl", ""); v != "" {
		cfg.BaseURL = v
	}
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsPollingInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PollingInterval = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsTTL", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.TTL = int(v)
	}
	return digitalocean.NewDNSProviderConfig(cfg)
}

func factoryEasyDNS(c map[string]string) (challenge.Provider, error) {
	cfg := easydns.NewDefaultConfig()
	cfg.Token = getString(c, "token", "")
	cfg.Key = getString(c, "key", "")
	if u := mustParseURL(getString(c, "endpoint", "")); u != nil {
		cfg.Endpoint = u
	}
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsPollingInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PollingInterval = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsSequenceInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.SequenceInterval = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsTTL", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.TTL = int(v)
	}
	return easydns.NewDNSProviderConfig(cfg)
}

func factoryGCloud(c map[string]string) (challenge.Provider, error) {
	project := getString(c, "project", "")
	if project == "" {
		return nil, errors.New("project is required")
	}
	// Service-account inputs use the dedicated constructors; otherwise the
	// env-driven default works for ADC / GKE workload identity.
	saKey := getString(c, "serviceAccountKey", "")
	saFile := getString(c, "serviceAccountFile", "")
	switch {
	case saKey != "":
		return gcloud.NewDNSProviderServiceAccountKey([]byte(saKey))
	case saFile != "":
		return gcloud.NewDNSProviderServiceAccount(saFile)
	}
	cfg := gcloud.NewDefaultConfig()
	cfg.Project = project
	cfg.ZoneID = getString(c, "zoneId", "")
	if v, err := getBool(c, "allowPrivateZone", false); err != nil {
		return nil, err
	} else {
		cfg.AllowPrivateZone = v
	}
	cfg.ImpersonateServiceAccount = getString(c, "impersonateServiceAccount", "")
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsPollingInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PollingInterval = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsTTL", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.TTL = int(v)
	}
	if v, err := getBool(c, "debug", false); err != nil {
		return nil, err
	} else {
		cfg.Debug = v
	}
	return gcloud.NewDNSProviderConfig(cfg)
}

func factoryHurricane(c map[string]string) (challenge.Provider, error) {
	credentials, err := getJSONMap(c, "credentials")
	if err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return nil, errors.New("credentials JSON map is required (zone → token)")
	}
	cfg := hurricane.NewDefaultConfig()
	cfg.Credentials = credentials
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsPollingInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PollingInterval = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsSequenceInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.SequenceInterval = time.Duration(v) * time.Second
	}
	return hurricane.NewDNSProviderConfig(cfg)
}

func factoryHTTPReq(c map[string]string) (challenge.Provider, error) {
	cfg := httpreq.NewDefaultConfig()
	cfg.Endpoint = mustParseURL(getString(c, "endpoint", ""))
	cfg.Mode = getString(c, "mode", "")
	cfg.Username = getString(c, "username", "")
	cfg.Password = getString(c, "password", "")
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsPollingInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PollingInterval = time.Duration(v) * time.Second
	}
	return httpreq.NewDNSProviderConfig(cfg)
}

func factoryPowerDNS(c map[string]string) (challenge.Provider, error) {
	host := mustParseURL(getString(c, "host", ""))
	if host == nil {
		return nil, errors.New("host is required (full URL, e.g. https://pdns.example.com:8081)")
	}
	cfg := pdns.NewDefaultConfig()
	cfg.APIKey = getString(c, "apiKey", "")
	cfg.Host = host
	cfg.ServerName = getString(c, "serverName", "")
	if v, err := getInt32(c, "apiVersion", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.APIVersion = int(v)
	}
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsPollingInterval", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PollingInterval = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsTTL", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.TTL = int(v)
	}
	return pdns.NewDNSProviderConfig(cfg)
}

func factoryRoute53(c map[string]string) (challenge.Provider, error) {
	cfg := route53.NewDefaultConfig()
	cfg.AccessKeyID = getString(c, "accessKeyId", "")
	cfg.SecretAccessKey = getString(c, "secretAccessKey", "")
	cfg.Region = getString(c, "region", "")
	cfg.HostedZoneID = getString(c, "hostedZoneId", "")
	if v, err := getInt32(c, "dnsPropagationTimeout", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.PropagationTimeout = time.Duration(v) * time.Second
	}
	if v, err := getInt32(c, "dnsTTL", 0); err != nil {
		return nil, err
	} else if v > 0 {
		cfg.TTL = int(v)
	}
	return route53.NewDNSProviderConfig(cfg)
}
