package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// CheckDomainBounceSetup verifies a domain's MX/SPF/DKIM via live DNS.
func (s *Service) CheckDomainBounceSetup(ctx context.Context, req *adminv1.CheckDomainBounceSetupRequest) (*adminv1.DomainBounceCheck, error) {
	if s.domainCheck == nil {
		return nil, notImplemented("CheckDomainBounceSetup")
	}
	res, err := s.domainCheck.Check(ctx, req.GetDomain())
	if err != nil {
		return nil, s.fail(ctx, "CheckDomainBounceSetup", err)
	}
	out := &adminv1.DomainBounceCheck{Domain: res.Domain}
	for _, i := range res.Items {
		out.Items = append(out.Items, &adminv1.DomainCheckItem{
			Name: i.Name, Status: i.Status, Detail: i.Detail, Records: i.Records,
		})
	}
	return out, nil
}
