package service

import (
	"context"
	"testing"
)

type fakeWebhookStore struct{ last MailWebhookRow }

func (f *fakeWebhookStore) List(context.Context, int, int) ([]MailWebhookRow, uint32, error) {
	return nil, 0, nil
}
func (f *fakeWebhookStore) Get(context.Context, uint32) (*MailWebhookRow, error) { return nil, nil }
func (f *fakeWebhookStore) Create(_ context.Context, in MailWebhookRow) (*MailWebhookRow, error) {
	f.last = in
	in.ID = 1
	return &in, nil
}
func (f *fakeWebhookStore) Update(_ context.Context, id uint32, in MailWebhookRow) (*MailWebhookRow, error) {
	in.ID = id
	return &in, nil
}
func (f *fakeWebhookStore) Delete(context.Context, uint32) error { return nil }

func TestMailWebhookValidation(t *testing.T) {
	svc := NewMailWebhookService(&fakeWebhookStore{})
	ctx := context.Background()
	tests := []struct {
		name    string
		in      MailWebhookRow
		wantErr bool
	}{
		{"exact email ok", MailWebhookRow{Name: "support", Address: "support@kmx.example.com", URL: "https://h/x"}, false},
		{"domain catch-all ok", MailWebhookRow{Name: "dom", Address: "inbound.example.com", URL: "http://h/x"}, false},
		{"bad url scheme", MailWebhookRow{Name: "x", Address: "a@b.com", URL: "ftp://h"}, true},
		{"empty url", MailWebhookRow{Name: "x", Address: "a@b.com", URL: ""}, true},
		{"empty address", MailWebhookRow{Name: "x", Address: "", URL: "https://h"}, true},
		{"address no dot", MailWebhookRow{Name: "x", Address: "support@localhost", URL: "https://h"}, true},
		{"bad name", MailWebhookRow{Name: "bad name!", Address: "a@b.com", URL: "https://h"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			_, err := svc.Create(ctx, &in)
			if tt.wantErr != (err != nil) {
				t.Fatalf("Create err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestMailWebhookNormalisesAddress(t *testing.T) {
	store := &fakeWebhookStore{}
	svc := NewMailWebhookService(store)
	_, err := svc.Create(context.Background(), &MailWebhookRow{
		Name: "support", Address: "  Support@KMX.Jobs.BG ", URL: "https://h/x",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if store.last.Address != "support@kmx.example.com" {
		t.Fatalf("address = %q, want lowercased+trimmed", store.last.Address)
	}
}
