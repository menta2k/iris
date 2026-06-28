# DMARC aggregate reports

DMARC lets you publish a policy and request **aggregate reports** (`rua=`) from
receivers, who email back daily XML summaries of how your mail authenticated
(SPF/DKIM alignment, pass/fail counts per source IP). Iris captures and parses
those reports so you can monitor authentication and spot spoofing.

UI: **Operations → DMARC Reports** (`operations:read`). The report address is set
in **Global Settings**.

## Setup

1. Set the **DMARC report address** (`dmarc_report_email` global setting), e.g.
   `dmarc@dmarc.example.com`, and point that domain's MX at your KumoMTA.
2. Publish it as the `rua=` in your domains' DMARC DNS records. One address can
   serve all your domains.

The generated policy then accepts mail for the report domain and routes mail
addressed to the exact report address to a **DMARC tracker** queue that XADDs the
raw message onto the `iris.dmarc.events` Redis stream.

## Parsing

The **`dmarc` worker** consumes the stream and parses the aggregate report,
including reports delivered as **zip or gzip attachments** (the common case). The
parsed rows — reporter, date range, source IP, disposition, SPF/DKIM results, and
message counts — are stored and shown on the DMARC Reports page.

## Gotchas

- Keep the DMARC report domain **distinct** from the
  [FBL](feedback-loops.md) domain; sharing one domain collides in the policy's
  `get_listener_domain` precedence and one pipeline can swallow the other.
- A blank "Mail-flow" chart usually means Prometheus cannot scrape KumoMTA
  (unrelated to DMARC capture) — see [dashboard](dashboard.md).

## Related

- [Bounce handling](bounce-handling.md)
- [Feedback loops](feedback-loops.md)
- [DKIM](dkim.md)
