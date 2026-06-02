package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Login-firewall rule types and methods. Stored as strings to match the
// ent enum and the proto enum names.
const (
	PolicyTypeBlacklist = "BLACKLIST"
	PolicyTypeWhitelist = "WHITELIST"

	MethodIP     = "IP"
	MethodMAC    = "MAC"
	MethodRegion = "REGION"
	MethodTime   = "TIME"
	MethodDevice = "DEVICE"
)

// TimeWindow is a recurring weekly time restriction for method=TIME rules.
// Empty Days means "every day". Start/End are 24h "HH:MM" in Timezone
// (IANA; empty = UTC). v1 supports same-day ranges only (Start <= End).
type TimeWindow struct {
	Days     []time.Weekday
	Start    string
	End      string
	Timezone string
}

// LoginPolicyRow is the data-layer view of a login_policies row, decoupled
// from the ent generated types. TargetID 0 means a global rule.
type LoginPolicyRow struct {
	ID         uint32
	TargetID   uint32
	Type       string
	Method     string
	Value      string
	TimeWindow *TimeWindow
	Reason     string
	Enabled    bool
	CreatedBy  uint32
	UpdatedBy  uint32
	DeletedBy  uint32
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
}

// LoginPolicyStore is the data-layer contract for login-firewall rules.
type LoginPolicyStore interface {
	List(ctx context.Context, limit, offset int) ([]LoginPolicyRow, error)
	Count(ctx context.Context) (int, error)
	Get(ctx context.Context, id uint32) (*LoginPolicyRow, error)
	Create(ctx context.Context, in LoginPolicyRow) (*LoginPolicyRow, error)
	Update(ctx context.Context, id uint32, in LoginPolicyRow) (*LoginPolicyRow, error)
	Delete(ctx context.Context, id, deletedBy uint32) error
	// ListApplicable returns enabled, non-deleted rules that apply to a
	// login: all global rules (target_id == 0) plus, when userID != nil,
	// rules targeting that user.
	ListApplicable(ctx context.Context, userID *uint32) ([]LoginPolicyRow, error)
}

// ErrWouldLockOutSelf is returned by Create/Update when the resulting rule
// set would block the acting operator's current login and they did not pass
// acknowledge=true.
var ErrWouldLockOutSelf = errors.New("login_policy: rule would block your current login")

// LoginPolicyService validates and persists login-firewall rules and guards
// against an operator locking themselves out.
type LoginPolicyService struct {
	store LoginPolicyStore
	geo   GeoResolver // for the self-lockout REGION check; may be nil
	now   func() time.Time
}

// NewLoginPolicyService constructs the service. geo may be nil (REGION
// self-lockout checks then fail open, consistent with enforcement).
func NewLoginPolicyService(store LoginPolicyStore, geo GeoResolver) *LoginPolicyService {
	return &LoginPolicyService{store: store, geo: geo, now: time.Now}
}

// List returns a page of rules plus the total count.
func (s *LoginPolicyService) List(ctx context.Context, limit, offset int) ([]LoginPolicyRow, int, error) {
	rows, err := s.store.List(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("login_policy: list: %w", err)
	}
	total, err := s.store.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("login_policy: count: %w", err)
	}
	return rows, total, nil
}

// Get returns a single rule by id.
func (s *LoginPolicyService) Get(ctx context.Context, id uint32) (*LoginPolicyRow, error) {
	row, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("login_policy: get: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("login_policy: rule %d not found", id)
	}
	return row, nil
}

// Create validates and persists a new rule. actingUserID/actingIP identify
// the operator for the self-lockout guard and audit fields; acknowledge
// bypasses the guard.
func (s *LoginPolicyService) Create(ctx context.Context, in LoginPolicyRow, actingUserID uint32, actingIP string, acknowledge bool) (*LoginPolicyRow, error) {
	normaliseLoginPolicy(&in)
	if err := validateLoginPolicy(&in); err != nil {
		return nil, err
	}
	if !acknowledge {
		if locked, reason, err := s.wouldLockOutSelf(ctx, in, 0, actingUserID, actingIP); err != nil {
			return nil, err
		} else if locked {
			return nil, fmt.Errorf("%w (%s)", ErrWouldLockOutSelf, reason)
		}
	}
	in.CreatedBy = actingUserID
	in.UpdatedBy = actingUserID
	row, err := s.store.Create(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("login_policy: create: %w", err)
	}
	return row, nil
}

// Update validates and persists changes to an existing rule.
func (s *LoginPolicyService) Update(ctx context.Context, id uint32, in LoginPolicyRow, actingUserID uint32, actingIP string, acknowledge bool) (*LoginPolicyRow, error) {
	normaliseLoginPolicy(&in)
	if err := validateLoginPolicy(&in); err != nil {
		return nil, err
	}
	if !acknowledge {
		if locked, reason, err := s.wouldLockOutSelf(ctx, in, id, actingUserID, actingIP); err != nil {
			return nil, err
		} else if locked {
			return nil, fmt.Errorf("%w (%s)", ErrWouldLockOutSelf, reason)
		}
	}
	in.UpdatedBy = actingUserID
	row, err := s.store.Update(ctx, id, in)
	if err != nil {
		return nil, fmt.Errorf("login_policy: update: %w", err)
	}
	return row, nil
}

// Delete soft-deletes a rule. Removing a rule can never add a block, so no
// self-lockout guard is needed.
func (s *LoginPolicyService) Delete(ctx context.Context, id, actingUserID uint32) error {
	if err := s.store.Delete(ctx, id, actingUserID); err != nil {
		return fmt.Errorf("login_policy: delete: %w", err)
	}
	return nil
}

// wouldLockOutSelf simulates the rule set after writing `candidate` and
// reports whether it would block the acting operator's current login.
// replacingID is the id being updated (0 for create) so the old version is
// excluded from the simulated set.
func (s *LoginPolicyService) wouldLockOutSelf(ctx context.Context, candidate LoginPolicyRow, replacingID, actingUserID uint32, actingIP string) (bool, string, error) {
	uid := actingUserID
	existing, err := s.store.ListApplicable(ctx, &uid)
	if err != nil {
		// Can't evaluate the guard — fail safe by allowing the write
		// rather than blocking legitimate config changes on a DB blip.
		return false, "", nil
	}
	set := make([]LoginPolicyRow, 0, len(existing)+1)
	for _, r := range existing {
		if replacingID != 0 && r.ID == replacingID {
			continue // old version replaced by candidate
		}
		set = append(set, r)
	}
	// Only the acting operator's own login is simulated, so the candidate
	// matters only if it would apply to them and is active.
	if candidate.Enabled && (candidate.TargetID == 0 || candidate.TargetID == actingUserID) {
		set = append(set, candidate)
	}
	res := evaluateRules(set, LoginAttempt{
		UserID: &uid,
		IP:     actingIP,
		Now:    s.now(),
	}, s.geo)
	if !res.Allowed {
		return true, res.Reason, nil
	}
	return false, "", nil
}

// normaliseLoginPolicy trims/uppercases fields so validation and storage see
// canonical values.
func normaliseLoginPolicy(in *LoginPolicyRow) {
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Method = strings.ToUpper(strings.TrimSpace(in.Method))
	in.Value = strings.TrimSpace(in.Value)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Method == MethodRegion {
		in.Value = strings.ToUpper(in.Value)
	}
}

// validateLoginPolicy enforces type/method correctness and per-method value
// shape. MAC/DEVICE are rejected: they can't be observed for a web login.
func validateLoginPolicy(in *LoginPolicyRow) error {
	switch in.Type {
	case PolicyTypeBlacklist, PolicyTypeWhitelist:
	default:
		return fmt.Errorf("login_policy: type must be %s or %s", PolicyTypeBlacklist, PolicyTypeWhitelist)
	}
	switch in.Method {
	case MethodIP:
		if _, err := parseRuleCIDR(in.Value); err != nil {
			return fmt.Errorf("login_policy: invalid IP/CIDR %q: %w", in.Value, err)
		}
	case MethodRegion:
		if !isCountryCode(in.Value) {
			return fmt.Errorf("login_policy: region must be a 2-letter ISO country code, got %q", in.Value)
		}
	case MethodTime:
		if err := validateTimeWindow(in.TimeWindow); err != nil {
			return err
		}
	case MethodMAC, MethodDevice:
		return fmt.Errorf("login_policy: method %s is not supported yet", in.Method)
	default:
		return fmt.Errorf("login_policy: unknown method %q", in.Method)
	}
	return nil
}

// parseRuleCIDR parses a rule value as a CIDR, treating a bare IP as a host
// route (/32 for IPv4, /128 for IPv6).
func parseRuleCIDR(v string) (*net.IPNet, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, errors.New("empty")
	}
	if !strings.Contains(v, "/") {
		ip := net.ParseIP(v)
		if ip == nil {
			return nil, fmt.Errorf("not an IP address")
		}
		if ip.To4() != nil {
			v += "/32"
		} else {
			v += "/128"
		}
	}
	_, ipnet, err := net.ParseCIDR(v)
	if err != nil {
		return nil, err
	}
	return ipnet, nil
}

func isCountryCode(v string) bool {
	if len(v) != 2 {
		return false
	}
	for _, r := range v {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func validateTimeWindow(tw *TimeWindow) error {
	if tw == nil {
		return errors.New("login_policy: time_window is required for method=TIME")
	}
	start, err := parseHHMM(tw.Start)
	if err != nil {
		return fmt.Errorf("login_policy: time_window.start %q: %w", tw.Start, err)
	}
	end, err := parseHHMM(tw.End)
	if err != nil {
		return fmt.Errorf("login_policy: time_window.end %q: %w", tw.End, err)
	}
	if start > end {
		return errors.New("login_policy: time_window.start must be <= end (wrap-past-midnight unsupported)")
	}
	if tw.Timezone != "" {
		if _, err := time.LoadLocation(tw.Timezone); err != nil {
			return fmt.Errorf("login_policy: time_window.timezone %q: %w", tw.Timezone, err)
		}
	}
	for _, d := range tw.Days {
		if d < time.Sunday || d > time.Saturday {
			return fmt.Errorf("login_policy: time_window.days out of range: %d", d)
		}
	}
	return nil
}

// parseHHMM parses "HH:MM" into minutes-since-midnight.
func parseHHMM(v string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(v))
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}
