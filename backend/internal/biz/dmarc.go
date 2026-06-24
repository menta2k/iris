package biz

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/xml"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

// DMARC parsing size guards.
const (
	dmarcMaxRawBytes = 25 << 20 // 25 MiB raw message
	dmarcMaxXMLBytes = 50 << 20 // 50 MiB decompressed XML
)

// DMARCReport is one parsed aggregate report's metadata.
type DMARCReport struct {
	OrgName    string
	ReportID   string
	Domain     string
	DateBegin  time.Time
	DateEnd    time.Time
	PolicyP    string
	PolicyPct  int
	ReceivedAt time.Time
}

// DMARCRecord is one <record> row: a source IP's evaluated result for a count of
// messages.
type DMARCRecord struct {
	SourceIP    string
	Count       int
	Disposition string
	DKIMResult  string
	SPFResult   string
	HeaderFrom  string
}

// xmlFeedback mirrors the RFC 7489 aggregate-report schema (subset).
type xmlFeedback struct {
	XMLName  xml.Name `xml:"feedback"`
	Metadata struct {
		OrgName   string `xml:"org_name"`
		ReportID  string `xml:"report_id"`
		DateRange struct {
			Begin int64 `xml:"begin"`
			End   int64 `xml:"end"`
		} `xml:"date_range"`
	} `xml:"report_metadata"`
	Policy struct {
		Domain string `xml:"domain"`
		P      string `xml:"p"`
		Pct    string `xml:"pct"`
	} `xml:"policy_published"`
	Records []struct {
		Row struct {
			SourceIP        string `xml:"source_ip"`
			Count           int    `xml:"count"`
			PolicyEvaluated struct {
				Disposition string `xml:"disposition"`
				DKIM        string `xml:"dkim"`
				SPF         string `xml:"spf"`
			} `xml:"policy_evaluated"`
		} `xml:"row"`
		Identifiers struct {
			HeaderFrom string `xml:"header_from"`
		} `xml:"identifiers"`
	} `xml:"record"`
}

// ParseDMARCReport parses a raw RFC822 message containing a DMARC aggregate
// report (gzip/zip/xml attachment) into its metadata + per-source records.
func ParseDMARCReport(raw []byte) (*DMARCReport, []DMARCRecord, error) {
	if len(raw) == 0 {
		return nil, nil, Invalid("DMARC_EMPTY", "empty message")
	}
	if len(raw) > dmarcMaxRawBytes {
		return nil, nil, Invalid("DMARC_TOO_LARGE", "message exceeds %d bytes", dmarcMaxRawBytes)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, Invalid("DMARC_PARSE", "not a valid message: %v", err)
	}
	xmlBytes, err := extractDMARCXML(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"), msg.Body)
	if err != nil {
		return nil, nil, err
	}
	var fb xmlFeedback
	if err := xml.Unmarshal(xmlBytes, &fb); err != nil {
		return nil, nil, Invalid("DMARC_XML", "invalid aggregate XML: %v", err)
	}
	if fb.Metadata.ReportID == "" && fb.Policy.Domain == "" {
		return nil, nil, Invalid("DMARC_XML", "not a DMARC aggregate report (no report_id/domain)")
	}

	pct := 100
	if v, perr := strconv.Atoi(strings.TrimSpace(fb.Policy.Pct)); perr == nil {
		pct = v
	}
	report := &DMARCReport{
		OrgName:    strings.TrimSpace(fb.Metadata.OrgName),
		ReportID:   strings.TrimSpace(fb.Metadata.ReportID),
		Domain:     strings.ToLower(strings.TrimSpace(fb.Policy.Domain)),
		DateBegin:  time.Unix(fb.Metadata.DateRange.Begin, 0).UTC(),
		DateEnd:    time.Unix(fb.Metadata.DateRange.End, 0).UTC(),
		PolicyP:    strings.ToLower(strings.TrimSpace(fb.Policy.P)),
		PolicyPct:  pct,
		ReceivedAt: time.Now().UTC(),
	}
	records := make([]DMARCRecord, 0, len(fb.Records))
	for _, r := range fb.Records {
		records = append(records, DMARCRecord{
			SourceIP:    strings.TrimSpace(r.Row.SourceIP),
			Count:       r.Row.Count,
			Disposition: strings.ToLower(strings.TrimSpace(r.Row.PolicyEvaluated.Disposition)),
			DKIMResult:  strings.ToLower(strings.TrimSpace(r.Row.PolicyEvaluated.DKIM)),
			SPFResult:   strings.ToLower(strings.TrimSpace(r.Row.PolicyEvaluated.SPF)),
			HeaderFrom:  strings.ToLower(strings.TrimSpace(r.Identifiers.HeaderFrom)),
		})
	}
	return report, records, nil
}

// extractDMARCXML walks a MIME entity (recursing into multipart) and returns the
// decompressed report XML from the first gzip/zip/xml attachment it finds.
func extractDMARCXML(contentType, transferEncoding string, body io.Reader) ([]byte, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// No/blank content type: treat the body as raw XML (some senders do this).
		mediaType = "text/xml"
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return nil, Invalid("DMARC_MIME", "multipart message without boundary")
		}
		mr := multipart.NewReader(body, boundary)
		for {
			part, perr := mr.NextPart()
			if perr == io.EOF {
				break
			}
			if perr != nil {
				return nil, Invalid("DMARC_MIME", "reading parts: %v", perr)
			}
			xmlBytes, found, ferr := dmarcPartXML(part)
			if ferr != nil {
				return nil, ferr
			}
			if found {
				return xmlBytes, nil
			}
		}
		return nil, Invalid("DMARC_NO_REPORT", "no DMARC report attachment found")
	}

	// Single-part entity: decode + decompress directly.
	data, err := decodeTransfer(transferEncoding, body)
	if err != nil {
		return nil, err
	}
	return decompressDMARC(mediaType, "", data)
}

// dmarcPartXML examines one multipart part: recurses if it is itself multipart,
// otherwise tries to decode it as a report attachment. found=false means "not a
// report, keep looking".
func dmarcPartXML(part *multipart.Part) (xmlBytes []byte, found bool, err error) {
	ct := part.Header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(ct)
	if strings.HasPrefix(mediaType, "multipart/") {
		nested, nerr := extractDMARCXML(ct, part.Header.Get("Content-Transfer-Encoding"), part)
		if nerr != nil {
			return nil, false, nil // a non-report nested part is fine; keep scanning siblings
		}
		return nested, true, nil
	}
	filename := params["name"]
	if filename == "" {
		if _, dp, derr := mime.ParseMediaType(part.Header.Get("Content-Disposition")); derr == nil {
			filename = dp["filename"]
		}
	}
	if !isDMARCAttachment(mediaType, filename) {
		return nil, false, nil
	}
	data, derr := decodeTransfer(part.Header.Get("Content-Transfer-Encoding"), part)
	if derr != nil {
		return nil, false, derr
	}
	out, cerr := decompressDMARC(mediaType, filename, data)
	if cerr != nil {
		return nil, false, cerr
	}
	return out, true, nil
}

func isDMARCAttachment(mediaType, filename string) bool {
	switch mediaType {
	case "application/gzip", "application/x-gzip", "application/zip", "application/x-zip-compressed",
		"text/xml", "application/xml":
		return true
	}
	f := strings.ToLower(filename)
	return strings.HasSuffix(f, ".gz") || strings.HasSuffix(f, ".zip") || strings.HasSuffix(f, ".xml")
}

// decodeTransfer decodes a part body per its Content-Transfer-Encoding.
func decodeTransfer(encoding string, body io.Reader) ([]byte, error) {
	var r io.Reader = io.LimitReader(body, dmarcMaxRawBytes)
	if strings.EqualFold(strings.TrimSpace(encoding), "base64") {
		// Go's base64 decoder already skips \r and \n (the only whitespace MIME
		// base64 uses), so the limited reader can be wrapped directly.
		r = base64.NewDecoder(base64.StdEncoding, r)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, Invalid("DMARC_DECODE", "decoding attachment: %v", err)
	}
	return data, nil
}

// decompressDMARC turns a (decoded) attachment into report XML based on its media
// type / filename.
func decompressDMARC(mediaType, filename string, data []byte) ([]byte, error) {
	f := strings.ToLower(filename)
	switch {
	case mediaType == "application/gzip" || mediaType == "application/x-gzip" || strings.HasSuffix(f, ".gz"):
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, Invalid("DMARC_GZIP", "bad gzip: %v", err)
		}
		defer gz.Close()
		return io.ReadAll(io.LimitReader(gz, dmarcMaxXMLBytes))
	case mediaType == "application/zip" || mediaType == "application/x-zip-compressed" || strings.HasSuffix(f, ".zip"):
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, Invalid("DMARC_ZIP", "bad zip: %v", err)
		}
		for _, zf := range zr.File {
			if !strings.HasSuffix(strings.ToLower(zf.Name), ".xml") {
				continue
			}
			rc, oerr := zf.Open()
			if oerr != nil {
				return nil, Invalid("DMARC_ZIP", "open zip entry: %v", oerr)
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, dmarcMaxXMLBytes))
		}
		return nil, Invalid("DMARC_ZIP", "zip has no .xml entry")
	default:
		return data, nil // already XML
	}
}
