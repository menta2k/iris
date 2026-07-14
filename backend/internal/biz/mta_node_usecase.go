package biz

import "context"

// NodeIPResolver returns a node's assignable IP addresses: the local host's
// for the co-located node, or the remote node's (via its agent). Satisfied by
// data.FileKumoMTA.
type NodeIPResolver interface {
	NodeIPs(ctx context.Context, node *MTANode) ([]string, error)
}

// MTANodeUsecase is the operator-facing CRUD for the KumoMTA cluster node
// registry. Reads require cluster:read, mutations cluster:write. Enrollment
// (tokens, CSR signing) lives in the cluster enrollment use case.
type MTANodeUsecase struct {
	repo    MTANodeRepo
	auditor *Auditor
	ips     NodeIPResolver // optional; nil disables the IP picker
}

// NewMTANodeUsecase constructs the use case.
func NewMTANodeUsecase(repo MTANodeRepo, auditor *Auditor) *MTANodeUsecase {
	return &MTANodeUsecase{repo: repo, auditor: auditor}
}

// WithIPResolver wires the IP resolver used by NodeIPs.
func (uc *MTANodeUsecase) WithIPResolver(r NodeIPResolver) *MTANodeUsecase {
	uc.ips = r
	return uc
}

// NodeIPs returns the assignable IPs for the node with the given id, for the
// UI's listener/VMTA IP pickers. An empty id (or the sentinel "local") returns
// the co-located host's IPs, so the picker works even before any node is
// registered.
func (uc *MTANodeUsecase) NodeIPs(ctx context.Context, id string) ([]string, error) {
	if _, err := RequirePermission(ctx, PermClusterRead); err != nil {
		return nil, err
	}
	if uc.ips == nil {
		return nil, FailedPrecondition("NODE_IPS_UNAVAILABLE", "node IP discovery is not available in this deployment")
	}
	var node *MTANode
	if id != "" && id != "local" {
		n, err := uc.repo.GetNode(ctx, id)
		if err != nil {
			return nil, err
		}
		node = n
	}
	// node==nil ⇒ the local co-located node.
	return uc.ips.NodeIPs(ctx, node)
}

// List returns all registered nodes.
func (uc *MTANodeUsecase) List(ctx context.Context) ([]*MTANode, error) {
	if _, err := RequirePermission(ctx, PermClusterRead); err != nil {
		return nil, err
	}
	return uc.repo.ListNodes(ctx)
}

// Get returns one node by id.
func (uc *MTANodeUsecase) Get(ctx context.Context, id string) (*MTANode, error) {
	if _, err := RequirePermission(ctx, PermClusterRead); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("MTA_NODE_ID_REQUIRED", "id is required")
	}
	return uc.repo.GetNode(ctx, id)
}

// Create registers a node.
func (uc *MTANodeUsecase) Create(ctx context.Context, n *MTANode) (*MTANode, error) {
	if _, err := RequirePermission(ctx, PermClusterWrite); err != nil {
		return nil, err
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateNode(ctx, n)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "cluster.node.create", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "agent_url": out.AgentURL, "status": out.Status,
	})
	return out, nil
}

// Update edits operator-owned fields by id. Agent-reported fields (version,
// applied checksum, last seen) and the pinned certificate are not editable.
func (uc *MTANodeUsecase) Update(ctx context.Context, n *MTANode) (*MTANode, error) {
	if _, err := RequirePermission(ctx, PermClusterWrite); err != nil {
		return nil, err
	}
	if n.ID == "" {
		return nil, Invalid("MTA_NODE_ID_REQUIRED", "id is required")
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateNode(ctx, n)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "cluster.node.update", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "agent_url": out.AgentURL, "status": out.Status,
	})
	return out, nil
}

// Delete removes a node from the registry. VMTA ownership constraints are
// enforced by the database once vmtas.node_id lands (phase 2).
func (uc *MTANodeUsecase) Delete(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermClusterWrite); err != nil {
		return err
	}
	if id == "" {
		return Invalid("MTA_NODE_ID_REQUIRED", "id is required")
	}
	node, err := uc.repo.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if err := uc.repo.DeleteNode(ctx, id); err != nil {
		return err
	}
	uc.audit(ctx, "cluster.node.delete", id, AuditSuccess, map[string]any{"name": node.Name})
	return nil
}

func (uc *MTANodeUsecase) audit(ctx context.Context, op, id string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, "mta_node", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
