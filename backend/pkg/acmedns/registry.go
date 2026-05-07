// Package acmedns is a registry of DNS-01 challenge providers backed
// by go-acme/lego. Each registered provider exposes the metadata the
// admin UI needs to render a credentials form (required + optional
// fields), and a factory that turns a string-keyed config map into a
// challenge.Provider lego can drive.
//
// Layout mirrors github.com/go-tangra/go-tangra-lcm/pkg/dns but is
// flatter: no per-provider sub-packages, just one factory function per
// provider in providers.go. The tradeoff is one big file vs ten little
// ones — operators don't need to grep across packages to find why a
// config key is named what it's named.
package acmedns

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/go-acme/lego/v4/challenge"
)

// ACMEChallenger is the interface lego expects a DNS-01 provider to
// satisfy. We re-export it so callers don't have to depend on lego's
// internal type for a one-method interface.
type ACMEChallenger = challenge.Provider

// ProviderFactory builds a challenge.Provider from a string-keyed
// configuration map. The map shape is provider-specific; ProviderInfo
// describes required and optional keys.
type ProviderFactory func(config map[string]string) (challenge.Provider, error)

// ProviderInfo is the metadata the admin UI surfaces when it renders a
// dynamic config form. Required + Optional are flat lists of camelCase
// keys; the UI shows them as text inputs and the factory parses them.
type ProviderInfo struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	RequiredFields []string        `json:"required_fields"`
	OptionalFields []string        `json:"optional_fields"`
	Factory        ProviderFactory `json:"-"`
}

type registry struct {
	mu        sync.RWMutex
	providers map[string]*ProviderInfo
}

var globalRegistry = &registry{providers: make(map[string]*ProviderInfo)}

// RegisterProvider adds a provider to the global registry. Called from
// init() functions in this package — operators don't need to call it.
// Returns an error if Name is empty, Factory is nil, or the provider
// is already registered (catches accidental duplicate registration
// when a new provider is added in two places).
func RegisterProvider(info *ProviderInfo) error {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	if info == nil {
		return errors.New("acmedns: provider info cannot be nil")
	}
	if info.Name == "" {
		return errors.New("acmedns: provider name cannot be empty")
	}
	if info.Factory == nil {
		return errors.New("acmedns: provider factory function cannot be nil")
	}
	if _, exists := globalRegistry.providers[info.Name]; exists {
		return fmt.Errorf("acmedns: provider %q is already registered", info.Name)
	}

	globalRegistry.providers[info.Name] = info
	return nil
}

// GetProvider creates a configured challenge.Provider by name. The
// caller passes the operator-supplied config; we validate required
// fields up front (clearer error than letting lego complain about a
// missing API token mid-issuance).
func GetProvider(name string, config map[string]string) (ACMEChallenger, error) {
	globalRegistry.mu.RLock()
	info, exists := globalRegistry.providers[name]
	globalRegistry.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("acmedns: provider %q is not registered", name)
	}
	if err := validateRequiredFields(info, config); err != nil {
		return nil, fmt.Errorf("acmedns: %s: %w", name, err)
	}

	provider, err := info.Factory(config)
	if err != nil {
		return nil, fmt.Errorf("acmedns: %s: factory failed: %w", name, err)
	}
	return provider, nil
}

// ListProviders returns the names of every registered provider in a
// stable (alphabetical) order — the UI uses this directly as the
// provider dropdown option list.
func ListProviders() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	names := make([]string, 0, len(globalRegistry.providers))
	for name := range globalRegistry.providers {
		names = append(names, name)
	}
	// Stable order so the UI's dropdown doesn't reshuffle between requests.
	sortStrings(names)
	return names
}

// GetProviderInfo returns a deep copy of the provider's metadata so
// callers can't mutate the registry by mutating the slices.
func GetProviderInfo(name string) (*ProviderInfo, error) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	info, exists := globalRegistry.providers[name]
	if !exists {
		return nil, fmt.Errorf("acmedns: provider %q is not registered", name)
	}
	return cloneInfo(info), nil
}

// GetAllProviderInfo returns deep copies of every provider's metadata,
// keyed by name. Suitable for the UI's "registry" listing endpoint.
func GetAllProviderInfo() map[string]*ProviderInfo {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	out := make(map[string]*ProviderInfo, len(globalRegistry.providers))
	for name, info := range globalRegistry.providers {
		out[name] = cloneInfo(info)
	}
	return out
}

func cloneInfo(in *ProviderInfo) *ProviderInfo {
	return &ProviderInfo{
		Name:           in.Name,
		Description:    in.Description,
		RequiredFields: append([]string(nil), in.RequiredFields...),
		OptionalFields: append([]string(nil), in.OptionalFields...),
	}
}

func validateRequiredFields(info *ProviderInfo, config map[string]string) error {
	missing := make([]string, 0)
	for _, field := range info.RequiredFields {
		if v, ok := config[field]; !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// --- type-safe config getters used by every provider factory --------------

// getString returns the trimmed value or fallback when missing/empty.
func getString(c map[string]string, key, fallback string) string {
	if v, ok := c[key]; ok {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return fallback
}

// getInt32 parses an int32 with a fallback. Returns an error so the
// factory can surface "you set dnsTTL=banana" instead of silently
// using the default.
func getInt32(c map[string]string, key string, fallback int32) (int32, error) {
	v := getString(c, key, "")
	if v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return fallback, fmt.Errorf("invalid integer for %s: %q", key, v)
	}
	return int32(parsed), nil
}

func getBool(c map[string]string, key string, fallback bool) (bool, error) {
	v := getString(c, key, "")
	if v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback, fmt.Errorf("invalid boolean for %s: %q", key, v)
	}
	return parsed, nil
}

// getStringSlice splits a comma-separated config value into a trimmed
// slice. Empty / missing returns nil.
func getStringSlice(c map[string]string, key string) []string {
	v := getString(c, key, "")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// getJSONMap decodes a JSON-string config value into a string→string
// map. Used by hurricane (credentials per zone) and any future provider
// that takes structured credentials.
func getJSONMap(c map[string]string, key string) (map[string]string, error) {
	v := getString(c, key, "")
	if v == "" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, fmt.Errorf("invalid JSON for %s: %w", key, err)
	}
	return out, nil
}

// sortStrings is a tiny dep-free in-place sort. Used so ListProviders
// returns a stable order without pulling in sort.* across the whole
// package — this is hot-path-free code so the n² lookup doesn't matter.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
