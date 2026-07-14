package biz

import (
	"context"
	"errors"
	"testing"
)

// fakeMTANodeRepo is an in-memory MTANodeRepo for use case tests.
type fakeMTANodeRepo struct {
	nodes  map[string]*MTANode
	nextID int
}

func newFakeMTANodeRepo() *fakeMTANodeRepo {
	return &fakeMTANodeRepo{nodes: map[string]*MTANode{}}
}

func (f *fakeMTANodeRepo) ListNodes(ctx context.Context) ([]*MTANode, error) {
	out := make([]*MTANode, 0, len(f.nodes))
	for _, n := range f.nodes {
		cp := *n
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeMTANodeRepo) GetNode(ctx context.Context, id string) (*MTANode, error) {
	n, ok := f.nodes[id]
	if !ok {
		return nil, NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	cp := *n
	return &cp, nil
}

func (f *fakeMTANodeRepo) CreateNode(ctx context.Context, n *MTANode) (*MTANode, error) {
	f.nextID++
	cp := *n
	cp.ID = string(rune('a' + f.nextID - 1))
	f.nodes[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (f *fakeMTANodeRepo) UpdateNode(ctx context.Context, n *MTANode) (*MTANode, error) {
	if _, ok := f.nodes[n.ID]; !ok {
		return nil, NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", n.ID)
	}
	cp := *n
	f.nodes[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (f *fakeMTANodeRepo) DeleteNode(ctx context.Context, id string) error {
	if _, ok := f.nodes[id]; !ok {
		return NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	delete(f.nodes, id)
	return nil
}

func (f *fakeMTANodeRepo) SetNodeCertFingerprint(ctx context.Context, id, fp string) error {
	n, ok := f.nodes[id]
	if !ok {
		return NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	cp := *n
	cp.CertFingerprint = fp
	f.nodes[id] = &cp
	return nil
}

func (f *fakeMTANodeRepo) RecordNodeHeartbeat(ctx context.Context, id, version, checksum, kumoState string) error {
	n, ok := f.nodes[id]
	if !ok {
		return NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	cp := *n
	cp.Version, cp.AppliedChecksum = version, checksum
	f.nodes[id] = &cp
	return nil
}

func (f *fakeMTANodeRepo) CreateEnrollToken(ctx context.Context, t *MTANodeEnrollToken) (*MTANodeEnrollToken, error) {
	return t, nil
}

func (f *fakeMTANodeRepo) OpenEnrollTokens(ctx context.Context, nodeID string) ([]*MTANodeEnrollToken, error) {
	return nil, nil
}

func (f *fakeMTANodeRepo) ConsumeEnrollToken(ctx context.Context, id string) error { return nil }

func clusterCtx(perms ...Permission) context.Context {
	strs := make([]string, 0, len(perms))
	for _, p := range perms {
		strs = append(strs, string(p))
	}
	return WithIdentity(context.Background(), &Identity{
		UserID:      "u1",
		Permissions: NewPermissionSet(strs),
		MFAVerified: true,
	})
}

func TestMTANodeUsecaseCRUD(t *testing.T) {
	repo := newFakeMTANodeRepo()
	uc := NewMTANodeUsecase(repo, nil)
	ctx := clusterCtx(PermClusterRead, PermClusterWrite)

	created, err := uc.Create(ctx, &MTANode{Name: "node1", AgentURL: "https://10.0.0.5:8447"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" || created.Status != MTANodeStatusActive {
		t.Fatalf("Create returned %+v", created)
	}

	created.Status = MTANodeStatusDraining
	updated, err := uc.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != MTANodeStatusDraining {
		t.Fatalf("Update status = %s", updated.Status)
	}

	list, err := uc.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v, %v", list, err)
	}

	if err := uc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := uc.Get(ctx, created.ID); err == nil {
		t.Fatalf("Get after delete should fail")
	}
}

func TestMTANodeUsecasePermissions(t *testing.T) {
	repo := newFakeMTANodeRepo()
	uc := NewMTANodeUsecase(repo, nil)

	readOnly := clusterCtx(PermClusterRead)
	if _, err := uc.Create(readOnly, &MTANode{Name: "node1"}); !isForbidden(err) {
		t.Fatalf("Create without cluster:write = %v, want forbidden", err)
	}
	if err := uc.Delete(readOnly, "x"); !isForbidden(err) {
		t.Fatalf("Delete without cluster:write = %v, want forbidden", err)
	}
	if _, err := uc.List(clusterCtx()); !isForbidden(err) {
		t.Fatalf("List without cluster:read = %v, want forbidden", err)
	}
	if _, err := uc.List(context.Background()); err == nil {
		t.Fatalf("List unauthenticated should fail")
	}
}

func TestMTANodeUsecaseValidation(t *testing.T) {
	uc := NewMTANodeUsecase(newFakeMTANodeRepo(), nil)
	ctx := clusterCtx(PermClusterRead, PermClusterWrite)

	if _, err := uc.Create(ctx, &MTANode{Name: ""}); err == nil {
		t.Fatalf("Create with empty name should fail validation")
	}
	if _, err := uc.Update(ctx, &MTANode{Name: "node1"}); err == nil {
		t.Fatalf("Update without id should fail")
	}
	if _, err := uc.Get(ctx, ""); err == nil {
		t.Fatalf("Get without id should fail")
	}
}

func isForbidden(err error) bool {
	var de *DomainError
	if !errors.As(err, &de) {
		return false
	}
	return de.Reason == "PERMISSION_DENIED"
}
