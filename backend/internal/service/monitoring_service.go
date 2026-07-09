package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListMonitoringAccounts returns all monitoring accounts (no passwords).
func (s *Service) ListMonitoringAccounts(ctx context.Context, req *adminv1.ListMonitoringAccountsRequest) (*adminv1.ListMonitoringAccountsReply, error) {
	if s.monitoring == nil {
		return nil, notImplemented("ListMonitoringAccounts")
	}
	items, err := s.monitoring.ListAccounts(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListMonitoringAccounts", err)
	}
	out := &adminv1.ListMonitoringAccountsReply{}
	for _, a := range items {
		out.Items = append(out.Items, monitoringAccountToProto(a))
	}
	return out, nil
}

// CreateMonitoringAccount adds a mailbox account.
func (s *Service) CreateMonitoringAccount(ctx context.Context, req *adminv1.CreateMonitoringAccountRequest) (*adminv1.MonitoringAccount, error) {
	if s.monitoring == nil {
		return nil, notImplemented("CreateMonitoringAccount")
	}
	out, err := s.monitoring.CreateAccount(ctx, &biz.MonitoringAccount{
		Label:            req.GetLabel(),
		Provider:         req.GetProvider(),
		Email:            req.GetEmail(),
		Protocol:         req.GetProtocol(),
		Host:             req.GetHost(),
		Port:             int(req.GetPort()),
		TLS:              req.GetTls(),
		Username:         req.GetUsername(),
		Password:         req.GetPassword(),
		CheckFolders:     req.GetCheckFolders(),
		FromAddress:      req.GetFromAddress(),
		ScheduleEnabled:  req.GetScheduleEnabled(),
		ScheduleInterval: req.GetScheduleInterval(),
		FetchDelay:       req.GetFetchDelay(),
		Enabled:          req.GetEnabled(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateMonitoringAccount", err)
	}
	return monitoringAccountToProto(out), nil
}

// UpdateMonitoringAccount edits mutable fields (not the password).
func (s *Service) UpdateMonitoringAccount(ctx context.Context, req *adminv1.UpdateMonitoringAccountRequest) (*adminv1.MonitoringAccount, error) {
	if s.monitoring == nil {
		return nil, notImplemented("UpdateMonitoringAccount")
	}
	out, err := s.monitoring.UpdateAccount(ctx, &biz.MonitoringAccount{
		ID:               req.GetId(),
		Label:            req.GetLabel(),
		Provider:         req.GetProvider(),
		Email:            req.GetEmail(),
		Protocol:         req.GetProtocol(),
		Host:             req.GetHost(),
		Port:             int(req.GetPort()),
		TLS:              req.GetTls(),
		Username:         req.GetUsername(),
		CheckFolders:     req.GetCheckFolders(),
		FromAddress:      req.GetFromAddress(),
		ScheduleEnabled:  req.GetScheduleEnabled(),
		ScheduleInterval: req.GetScheduleInterval(),
		FetchDelay:       req.GetFetchDelay(),
		Enabled:          req.GetEnabled(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateMonitoringAccount", err)
	}
	return monitoringAccountToProto(out), nil
}

// SetMonitoringAccountPassword rotates the mailbox password.
func (s *Service) SetMonitoringAccountPassword(ctx context.Context, req *adminv1.SetMonitoringAccountPasswordRequest) (*adminv1.MonitoringAccount, error) {
	if s.monitoring == nil {
		return nil, notImplemented("SetMonitoringAccountPassword")
	}
	if err := s.monitoring.SetAccountPassword(ctx, req.GetId(), req.GetPassword()); err != nil {
		return nil, s.fail(ctx, "SetMonitoringAccountPassword", err)
	}
	out, err := s.monitoring.GetAccount(ctx, req.GetId())
	if err != nil {
		return nil, s.fail(ctx, "SetMonitoringAccountPassword", err)
	}
	return monitoringAccountToProto(out), nil
}

// DeleteMonitoringAccount removes an account and its probes.
func (s *Service) DeleteMonitoringAccount(ctx context.Context, req *adminv1.DeleteMonitoringAccountRequest) (*adminv1.DeleteMonitoringAccountReply, error) {
	if s.monitoring == nil {
		return nil, notImplemented("DeleteMonitoringAccount")
	}
	if err := s.monitoring.DeleteAccount(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteMonitoringAccount", err)
	}
	return &adminv1.DeleteMonitoringAccountReply{Ok: true}, nil
}

// SendMonitoringProbe sends a probe to the account's mailbox now.
func (s *Service) SendMonitoringProbe(ctx context.Context, req *adminv1.SendMonitoringProbeRequest) (*adminv1.MonitoringProbe, error) {
	if s.monitoring == nil {
		return nil, notImplemented("SendMonitoringProbe")
	}
	out, err := s.monitoring.SendProbe(ctx, req.GetAccountId())
	if err != nil {
		return nil, s.fail(ctx, "SendMonitoringProbe", err)
	}
	return monitoringProbeToProto(out), nil
}

// ListMonitoringProbes returns probes for an account, newest first.
func (s *Service) ListMonitoringProbes(ctx context.Context, req *adminv1.ListMonitoringProbesRequest) (*adminv1.ListMonitoringProbesReply, error) {
	if s.monitoring == nil {
		return nil, notImplemented("ListMonitoringProbes")
	}
	page := biz.NormalizePage(int(req.GetPageSize()), req.GetPageToken())
	items, err := s.monitoring.ListProbes(ctx, req.GetAccountId(), page)
	if err != nil {
		return nil, s.fail(ctx, "ListMonitoringProbes", err)
	}
	out := &adminv1.ListMonitoringProbesReply{NextPageToken: page.NextToken(len(items))}
	for _, p := range items {
		out.Items = append(out.Items, monitoringProbeToProto(p))
	}
	return out, nil
}

func monitoringAccountToProto(a *biz.MonitoringAccount) *adminv1.MonitoringAccount {
	p := &adminv1.MonitoringAccount{
		Id:               a.ID,
		Label:            a.Label,
		Provider:         a.Provider,
		Email:            a.Email,
		Protocol:         a.Protocol,
		Host:             a.Host,
		Port:             int32(a.Port),
		Tls:              a.TLS,
		Username:         a.Username,
		CheckFolders:     a.CheckFolders,
		FromAddress:      a.FromAddress,
		ScheduleEnabled:  a.ScheduleEnabled,
		ScheduleInterval: a.ScheduleInterval,
		FetchDelay:       a.FetchDelay,
		Enabled:          a.Enabled,
		HasPassword:      a.HasPassword,
		CreatedAt:        formatTime(a.CreatedAt),
		UpdatedAt:        formatTime(a.UpdatedAt),
	}
	if a.LastProbeAt != nil {
		p.LastProbeAt = formatTime(*a.LastProbeAt)
	}
	return p
}

func monitoringProbeToProto(m *biz.MonitoringProbe) *adminv1.MonitoringProbe {
	p := &adminv1.MonitoringProbe{
		Id:            m.ID,
		AccountId:     m.AccountID,
		ProbeUid:      m.ProbeUID,
		MessageId:     m.MessageID,
		Subject:       m.Subject,
		FromAddr:      m.FromAddr,
		Recipient:     m.Recipient,
		SentAt:        formatTime(m.SentAt),
		SendStatus:    m.SendStatus,
		MailboxStatus: m.MailboxStatus,
		Placement:     m.Placement,
		Analysis:      m.Analysis,
		Error:         m.Error,
		CreatedAt:     formatTime(m.CreatedAt),
		UpdatedAt:     formatTime(m.UpdatedAt),
	}
	if m.FoundAt != nil {
		p.FoundAt = formatTime(*m.FoundAt)
	}
	if m.LatencyMs != nil {
		p.LatencyMs = *m.LatencyMs
	}
	return p
}
