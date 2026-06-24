package biz

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"strings"
	"testing"
)

const sampleDMARCXML = `<?xml version="1.0" encoding="UTF-8"?>
<feedback>
  <report_metadata>
    <org_name>google.com</org_name>
    <report_id>1234567890</report_id>
    <date_range><begin>1700000000</begin><end>1700086400</end></date_range>
  </report_metadata>
  <policy_published><domain>Example.com</domain><p>reject</p><pct>100</pct></policy_published>
  <record>
    <row>
      <source_ip>1.2.3.4</source_ip>
      <count>10</count>
      <policy_evaluated><disposition>none</disposition><dkim>pass</dkim><spf>pass</spf></policy_evaluated>
    </row>
    <identifiers><header_from>example.com</header_from></identifiers>
  </record>
  <record>
    <row>
      <source_ip>5.6.7.8</source_ip>
      <count>3</count>
      <policy_evaluated><disposition>quarantine</disposition><dkim>fail</dkim><spf>fail</spf></policy_evaluated>
    </row>
    <identifiers><header_from>example.com</header_from></identifiers>
  </record>
</feedback>`

func gzipB64(s string) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(s))
	_ = gz.Close()
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func dmarcMIME(attachCT, filename, b64 string) []byte {
	var b strings.Builder
	b.WriteString("From: noreply@google.com\r\n")
	b.WriteString("To: dmarc@kmx.jobs.bg\r\n")
	b.WriteString("Subject: Report Domain: example.com\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"BOUND\"\r\n\r\n")
	b.WriteString("--BOUND\r\n")
	b.WriteString("Content-Type: text/plain\r\n\r\nDMARC aggregate report attached.\r\n")
	b.WriteString("--BOUND\r\n")
	b.WriteString("Content-Type: " + attachCT + "; name=\"" + filename + "\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + filename + "\"\r\n\r\n")
	b.WriteString(b64 + "\r\n")
	b.WriteString("--BOUND--\r\n")
	return []byte(b.String())
}

func TestParseDMARCReportGzip(t *testing.T) {
	raw := dmarcMIME("application/gzip", "report.xml.gz", gzipB64(sampleDMARCXML))
	report, records, err := ParseDMARCReport(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.OrgName != "google.com" || report.ReportID != "1234567890" {
		t.Fatalf("metadata: %+v", report)
	}
	if report.Domain != "example.com" || report.PolicyP != "reject" || report.PolicyPct != 100 {
		t.Fatalf("policy: %+v", report)
	}
	if report.DateBegin.Unix() != 1700000000 || report.DateEnd.Unix() != 1700086400 {
		t.Fatalf("date range: %v..%v", report.DateBegin, report.DateEnd)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}
	if records[0].SourceIP != "1.2.3.4" || records[0].Count != 10 ||
		records[0].Disposition != "none" || records[0].DKIMResult != "pass" || records[0].SPFResult != "pass" {
		t.Fatalf("record 0: %+v", records[0])
	}
	if records[1].Disposition != "quarantine" || records[1].DKIMResult != "fail" {
		t.Fatalf("record 1: %+v", records[1])
	}
}

func TestParseDMARCReportRawXML(t *testing.T) {
	// Some senders attach uncompressed XML.
	raw := dmarcMIME("text/xml", "report.xml", base64.StdEncoding.EncodeToString([]byte(sampleDMARCXML)))
	report, records, err := ParseDMARCReport(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Domain != "example.com" || len(records) != 2 {
		t.Fatalf("unexpected: %+v records=%d", report, len(records))
	}
}

func TestParseDMARCReportRejectsJunk(t *testing.T) {
	if _, _, err := ParseDMARCReport([]byte("not a mime message at all")); err == nil {
		t.Fatal("expected error for non-report input")
	}
	if _, _, err := ParseDMARCReport(nil); err == nil {
		t.Fatal("expected error for empty input")
	}
}
