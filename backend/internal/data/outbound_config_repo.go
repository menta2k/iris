package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/menta2k/iris/backend/internal/biz"
)

// jsonRoutingCondition is the JSONB shape of a routing rule's match condition.
type jsonRoutingCondition struct {
	Header string `json:"header"`
	Value  string `json:"value"`
}

// marshalRoutingConditions serializes conditions to a JSON array string for the
// match_conditions JSONB column (empty array when none).
func marshalRoutingConditions(conds []biz.RoutingMatchCondition) string {
	arr := make([]jsonRoutingCondition, 0, len(conds))
	for _, c := range conds {
		arr = append(arr, jsonRoutingCondition{Header: c.Header, Value: c.Value})
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// scanRoutingConditions parses the match_conditions JSONB column.
func scanRoutingConditions(raw []byte) []biz.RoutingMatchCondition {
	if len(raw) == 0 {
		return nil
	}
	var arr []jsonRoutingCondition
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := make([]biz.RoutingMatchCondition, 0, len(arr))
	for _, c := range arr {
		out = append(out, biz.RoutingMatchCondition{Header: c.Header, Value: c.Value})
	}
	return out
}

// OutboundConfigRepo persists VMTAs, VMTA groups, and routing rules.
type OutboundConfigRepo struct {
	db *DB
}

// NewOutboundConfigRepo constructs the repository.
func NewOutboundConfigRepo(db *DB) *OutboundConfigRepo { return &OutboundConfigRepo{db: db} }

var _ biz.OutboundConfigRepo = (*OutboundConfigRepo)(nil)

// vmtaSelect is the VMTA projection. IP/EHLO are owned by the VMTA; the listener
// is an OPTIONAL reference, LEFT JOINed only to resolve its display name.
const vmtaSelect = `v.id, v.name, host(v.ip_address), v.ehlo_name,
	coalesce(v.listener_id::text, ''), v.max_connections, v.tls_mode, v.status, v.notes,
	coalesce(l.name, '')`

func scanVMTA(row interface{ Scan(...any) error }) (*biz.VMTA, error) {
	v := &biz.VMTA{}
	if err := row.Scan(&v.ID, &v.Name, &v.IPAddress, &v.EHLOName,
		&v.ListenerID, &v.MaxConnections, &v.TLSMode, &v.Status, &v.Notes, &v.ListenerName); err != nil {
		return nil, err
	}
	return v, nil
}

// CreateVMTA inserts a VMTA (owning its egress IP/EHLO) and returns the stored
// record. The listener association is optional.
func (r *OutboundConfigRepo) CreateVMTA(ctx context.Context, v *biz.VMTA) (*biz.VMTA, error) {
	out, err := scanVMTA(r.db.Pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO vmtas (name, ip_address, ehlo_name, listener_id, max_connections, tls_mode, status, notes)
			VALUES ($1, $2::inet, $3, $4, $5, $6, $7, $8)
			RETURNING id, name, ip_address, ehlo_name, listener_id, max_connections, tls_mode, status, notes
		)
		SELECT `+vmtaSelect+` FROM ins v LEFT JOIN listeners l ON l.id = v.listener_id`,
		v.Name, v.IPAddress, v.EHLOName, nullableUUID(v.ListenerID), v.MaxConnections, v.TLSMode, v.Status, v.Notes))
	if err != nil {
		return nil, mapConstraint(err, "vmta")
	}
	return out, nil
}

// UpdateVMTA updates a VMTA by id and returns the stored record.
func (r *OutboundConfigRepo) UpdateVMTA(ctx context.Context, id string, v *biz.VMTA) (*biz.VMTA, error) {
	out, err := scanVMTA(r.db.Pool.QueryRow(ctx, `
		WITH upd AS (
			UPDATE vmtas SET name = $2, ip_address = $3::inet, ehlo_name = $4,
				listener_id = $5, max_connections = $6, tls_mode = $7, status = $8, notes = $9, updated_at = now()
			WHERE id = $1
			RETURNING id, name, ip_address, ehlo_name, listener_id, max_connections, tls_mode, status, notes
		)
		SELECT `+vmtaSelect+` FROM upd v LEFT JOIN listeners l ON l.id = v.listener_id`,
		id, v.Name, v.IPAddress, v.EHLOName, nullableUUID(v.ListenerID), v.MaxConnections, v.TLSMode, v.Status, v.Notes))
	if err != nil {
		return nil, mapConstraint(err, "vmta")
	}
	return out, nil
}

// ListVMTAs returns VMTAs (with their optional listener name) filtered by status.
func (r *OutboundConfigRepo) ListVMTAs(ctx context.Context, status string, page biz.Page) ([]*biz.VMTA, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+vmtaSelect+`
		FROM vmtas v LEFT JOIN listeners l ON l.id = v.listener_id
		WHERE ($1 = '' OR v.status = $1)
		ORDER BY v.name
		LIMIT $2 OFFSET $3`, status, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query vmtas: %w", err)
	}
	defer rows.Close()
	var out []*biz.VMTA
	for rows.Next() {
		v, err := scanVMTA(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vmta: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VMTAExists reports whether a VMTA with the id exists.
func (r *OutboundConfigRepo) VMTAExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := r.db.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM vmtas WHERE id = $1)`, id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check vmta exists: %w", err)
	}
	return ok, nil
}

// --- Listeners --------------------------------------------------------------

const listenerSelect = `id, name, host(ip_address), port, hostname, tls_enabled,
	tls_cert_path, tls_key_path, require_auth, max_message_size, relay_hosts, status, role`

func scanListener(row interface{ Scan(...any) error }) (*biz.Listener, error) {
	l := &biz.Listener{}
	if err := row.Scan(&l.ID, &l.Name, &l.IPAddress, &l.Port, &l.Hostname, &l.TLSEnabled,
		&l.TLSCertPath, &l.TLSKeyPath, &l.RequireAuth, &l.MaxMessageSize, &l.RelayHosts, &l.Status, &l.Role); err != nil {
		return nil, err
	}
	return l, nil
}

// CreateListener inserts a listener.
func (r *OutboundConfigRepo) CreateListener(ctx context.Context, l *biz.Listener) (*biz.Listener, error) {
	out, err := scanListener(r.db.Pool.QueryRow(ctx, `
		INSERT INTO listeners (name, ip_address, port, hostname, tls_enabled,
			tls_cert_path, tls_key_path, require_auth, max_message_size, relay_hosts, status, role)
		VALUES ($1, $2::inet, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+listenerSelect,
		l.Name, l.IPAddress, l.Port, l.Hostname, l.TLSEnabled, l.TLSCertPath, l.TLSKeyPath,
		l.RequireAuth, l.MaxMessageSize, nonNilStrings(l.RelayHosts), l.Status, l.Role))
	if err != nil {
		return nil, mapConstraint(err, "listener")
	}
	return out, nil
}

// UpdateListener updates a listener by id.
func (r *OutboundConfigRepo) UpdateListener(ctx context.Context, id string, l *biz.Listener) (*biz.Listener, error) {
	out, err := scanListener(r.db.Pool.QueryRow(ctx, `
		UPDATE listeners SET name = $2, ip_address = $3::inet, port = $4, hostname = $5,
			tls_enabled = $6, tls_cert_path = $7, tls_key_path = $8, require_auth = $9,
			max_message_size = $10, relay_hosts = $11, status = $12, role = $13, updated_at = now()
		WHERE id = $1
		RETURNING `+listenerSelect,
		id, l.Name, l.IPAddress, l.Port, l.Hostname, l.TLSEnabled, l.TLSCertPath, l.TLSKeyPath,
		l.RequireAuth, l.MaxMessageSize, nonNilStrings(l.RelayHosts), l.Status, l.Role))
	if err != nil {
		return nil, mapConstraint(err, "listener")
	}
	return out, nil
}

// nonNilStrings returns s, or an empty (non-nil) slice so it encodes as a SQL
// empty array rather than NULL.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ListListeners returns listeners with bounded pagination.
func (r *OutboundConfigRepo) ListListeners(ctx context.Context, page biz.Page) ([]*biz.Listener, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT `+listenerSelect+`
		FROM listeners ORDER BY name LIMIT $1 OFFSET $2`, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query listeners: %w", err)
	}
	defer rows.Close()
	var out []*biz.Listener
	for rows.Next() {
		l, err := scanListener(rows)
		if err != nil {
			return nil, fmt.Errorf("scan listener: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListenerExists reports whether a listener with the id exists.
func (r *OutboundConfigRepo) ListenerExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := r.db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM listeners WHERE id = $1)`, id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check listener exists: %w", err)
	}
	return ok, nil
}

// CreateVMTAGroup inserts a group and its members atomically.
func (r *OutboundConfigRepo) CreateVMTAGroup(ctx context.Context, g *biz.VMTAGroup) (*biz.VMTAGroup, error) {
	out := &biz.VMTAGroup{Members: g.Members}
	err := r.db.InTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO vmta_groups (name, status) VALUES ($1, $2)
			RETURNING id, name, status`, g.Name, g.Status)
		if err := row.Scan(&out.ID, &out.Name, &out.Status); err != nil {
			return mapConstraint(err, "vmta_group")
		}
		for _, m := range g.Members {
			if _, err := tx.Exec(ctx, `
				INSERT INTO vmta_group_members (group_id, vmta_id, weight)
				VALUES ($1, $2, $3)`, out.ID, m.VMTAID, m.Weight); err != nil {
				return mapConstraint(err, "vmta_group_member")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateVMTAGroup updates a group's row and replaces its members atomically.
func (r *OutboundConfigRepo) UpdateVMTAGroup(ctx context.Context, id string, g *biz.VMTAGroup) (*biz.VMTAGroup, error) {
	out := &biz.VMTAGroup{Members: g.Members}
	err := r.db.InTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE vmta_groups SET name = $2, status = $3, updated_at = now()
			WHERE id = $1 RETURNING id, name, status`, id, g.Name, g.Status)
		if err := row.Scan(&out.ID, &out.Name, &out.Status); err != nil {
			return mapConstraint(err, "vmta_group")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM vmta_group_members WHERE group_id = $1`, id); err != nil {
			return mapConstraint(err, "vmta_group_member")
		}
		for _, m := range g.Members {
			if _, err := tx.Exec(ctx, `
				INSERT INTO vmta_group_members (group_id, vmta_id, weight)
				VALUES ($1, $2, $3)`, id, m.VMTAID, m.Weight); err != nil {
				return mapConstraint(err, "vmta_group_member")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListVMTAGroups returns groups with their members.
func (r *OutboundConfigRepo) ListVMTAGroups(ctx context.Context, page biz.Page) ([]*biz.VMTAGroup, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, status FROM vmta_groups
		ORDER BY name LIMIT $1 OFFSET $2`, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query vmta groups: %w", err)
	}
	defer rows.Close()
	var groups []*biz.VMTAGroup
	index := map[string]*biz.VMTAGroup{}
	for rows.Next() {
		g := &biz.VMTAGroup{}
		if err := rows.Scan(&g.ID, &g.Name, &g.Status); err != nil {
			return nil, fmt.Errorf("scan vmta group: %w", err)
		}
		groups = append(groups, g)
		index[g.ID] = g
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}

	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	mrows, err := r.db.Pool.Query(ctx, `
		SELECT group_id, vmta_id, weight FROM vmta_group_members
		WHERE group_id = ANY($1) ORDER BY vmta_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("query group members: %w", err)
	}
	defer mrows.Close()
	for mrows.Next() {
		var groupID string
		var m biz.VMTAGroupMember
		if err := mrows.Scan(&groupID, &m.VMTAID, &m.Weight); err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		if g := index[groupID]; g != nil {
			g.Members = append(g.Members, m)
		}
	}
	return groups, mrows.Err()
}

// CreateRoutingRule inserts a routing rule.
func (r *OutboundConfigRepo) CreateRoutingRule(ctx context.Context, rule *biz.RoutingRule) (*biz.RoutingRule, error) {
	var condsRaw []byte
	row := r.db.Pool.QueryRow(ctx, `
		INSERT INTO routing_rules (name, match_type, match_header, match_value, priority, target_type, target_id, assign_mailclass, status, match_conditions)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)
		RETURNING id, name, match_type, match_header, match_value, priority, coalesce(target_type, ''), coalesce(target_id::text, ''), assign_mailclass, status, match_conditions`,
		rule.Name, rule.MatchType, rule.MatchHeader, rule.MatchValue, rule.Priority, nullableText(rule.TargetType), nullableUUID(rule.TargetID), rule.AssignMailclass, rule.Status, marshalRoutingConditions(rule.Conditions))
	out := &biz.RoutingRule{}
	if err := row.Scan(&out.ID, &out.Name, &out.MatchType, &out.MatchHeader, &out.MatchValue, &out.Priority,
		&out.TargetType, &out.TargetID, &out.AssignMailclass, &out.Status, &condsRaw); err != nil {
		return nil, mapConstraint(err, "routing_rule")
	}
	out.Conditions = scanRoutingConditions(condsRaw)
	return out, nil
}

// UpdateRoutingRule updates a routing rule by id.
func (r *OutboundConfigRepo) UpdateRoutingRule(ctx context.Context, id string, rule *biz.RoutingRule) (*biz.RoutingRule, error) {
	var condsRaw []byte
	row := r.db.Pool.QueryRow(ctx, `
		UPDATE routing_rules SET name = $2, match_type = $3, match_header = $4, match_value = $5,
			priority = $6, target_type = $7, target_id = $8, assign_mailclass = $9, status = $10,
			match_conditions = $11::jsonb, updated_at = now()
		WHERE id = $1
		RETURNING id, name, match_type, match_header, match_value, priority, coalesce(target_type, ''), coalesce(target_id::text, ''), assign_mailclass, status, match_conditions`,
		id, rule.Name, rule.MatchType, rule.MatchHeader, rule.MatchValue, rule.Priority, nullableText(rule.TargetType), nullableUUID(rule.TargetID), rule.AssignMailclass, rule.Status, marshalRoutingConditions(rule.Conditions))
	out := &biz.RoutingRule{}
	if err := row.Scan(&out.ID, &out.Name, &out.MatchType, &out.MatchHeader, &out.MatchValue, &out.Priority,
		&out.TargetType, &out.TargetID, &out.AssignMailclass, &out.Status, &condsRaw); err != nil {
		return nil, mapConstraint(err, "routing_rule")
	}
	out.Conditions = scanRoutingConditions(condsRaw)
	return out, nil
}

// ListRoutingRules returns routing rules filtered by optional match type/value.
func (r *OutboundConfigRepo) ListRoutingRules(ctx context.Context, matchType, matchValue string, page biz.Page) ([]*biz.RoutingRule, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, match_type, match_header, match_value, priority,
		       coalesce(target_type, ''), coalesce(target_id::text, ''), assign_mailclass, status, match_conditions
		FROM routing_rules
		WHERE ($1 = '' OR match_type = $1) AND ($2 = '' OR match_value = $2)
		ORDER BY priority DESC, name
		LIMIT $3 OFFSET $4`, matchType, matchValue, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query routing rules: %w", err)
	}
	defer rows.Close()
	var out []*biz.RoutingRule
	for rows.Next() {
		rule := &biz.RoutingRule{}
		var condsRaw []byte
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.MatchType, &rule.MatchHeader, &rule.MatchValue, &rule.Priority,
			&rule.TargetType, &rule.TargetID, &rule.AssignMailclass, &rule.Status, &condsRaw); err != nil {
			return nil, fmt.Errorf("scan routing rule: %w", err)
		}
		rule.Conditions = scanRoutingConditions(condsRaw)
		out = append(out, rule)
	}
	return out, rows.Err()
}

// TargetExists reports whether the routing target (vmta or vmta_group) exists.
func (r *OutboundConfigRepo) TargetExists(ctx context.Context, targetType, id string) (bool, error) {
	table := "vmtas"
	if targetType == biz.TargetVMTAGroup {
		table = "vmta_groups"
	}
	var ok bool
	// table is selected from a fixed allowlist above, never from user input.
	err := r.db.Pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)`, table), id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check target exists: %w", err)
	}
	return ok, nil
}

// mapConstraint translates known unique/foreign-key violations into typed
// domain errors so the API returns 409/400 rather than 500.
func mapConstraint(err error, entity string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return biz.Conflict("CONFLICT", "%s already exists", entity)
		case "23503": // foreign_key_violation
			return biz.Invalid("INVALID_REFERENCE", "referenced %s does not exist", entity)
		case "23514": // check_violation
			return biz.Invalid("CONSTRAINT_VIOLATION", "%s violates a constraint", entity)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return biz.NotFound("NOT_FOUND", "%s not found", entity)
	}
	return biz.Internal(err, "persist %s", entity)
}
