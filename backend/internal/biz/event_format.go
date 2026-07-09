package biz

import (
	"strconv"
	"strings"
	"time"
)

// Payload formats an Event Processor can emit. native is iris's own shape;
// greenarrow_bounce_all reshapes bounce events to GreenArrow Engine's
// "bounce_all" event so existing GreenArrow consumers can ingest iris events.
const (
	FormatNative              = "native"
	FormatGreenArrowBounceAll = "greenarrow_bounce_all"
)

var eventFormats = map[string]struct{}{
	"": {}, FormatNative: {}, FormatGreenArrowBounceAll: {},
}

// ValidEventFormat reports whether a payload format is known.
func ValidEventFormat(f string) bool {
	_, ok := eventFormats[strings.TrimSpace(f)]
	return ok
}

// FormatEvent renders a dispatch event into the wire object for the given format.
// Unknown/empty format falls back to native.
func FormatEvent(format string, ev DispatchEvent) map[string]any {
	if strings.TrimSpace(format) == FormatGreenArrowBounceAll {
		return greenArrowBounceAll(ev)
	}
	return nativeFormat(ev)
}

// nativeFormat is iris's canonical event shape.
func nativeFormat(ev DispatchEvent) map[string]any {
	m := map[string]any{
		"type":        ev.Type,
		"occurred_at": ev.OccurredAt.UTC().Format(time.RFC3339),
	}
	if ev.Mailclass != "" {
		m["mailclass"] = ev.Mailclass
	}
	if len(ev.Data) > 0 {
		m["data"] = ev.Data
	}
	return m
}

// greenArrowBounceAll maps a bounce event to GreenArrow Engine's bounce_all
// event fields. Non-bounce events fall back to native (bounce_all is
// bounce-specific), so a GreenArrow-format processor should subscribe to bounce.
func greenArrowBounceAll(ev DispatchEvent) map[string]any {
	if ev.Type != EventBounce {
		return nativeFormat(ev)
	}
	str := func(k string) string {
		v, _ := ev.Data[k].(string)
		return v
	}
	bounceType := "o" // other
	switch strings.ToLower(str("bounce_type")) {
	case "hard":
		bounceType = "h"
	case "soft":
		bounceType = "s"
	}
	code := 0
	if c, err := strconv.Atoi(str("smtp_status")); err == nil {
		code = c
	}
	egress := str("egress_source")
	msgID := str("message_id")
	return map[string]any{
		"event_type":        "bounce_all",
		"event_time":        ev.OccurredAt.Unix(),
		"server_id":         "",
		"event_unique_id":   msgID,
		"email":             str("recipient"),
		"listid":            "",
		"sendid":            strings.ToLower(msgID),
		"bounce_type":       bounceType,
		"bounce_code":       code,
		"bounce_text":       truncateBytes(str("diagnostic"), 1024),
		"click_tracking_id": "",
		"synchronous":       true,
		"mailclass":         ev.Mailclass,
		"instanceid":        "",
		"mtaid_name":        egress,
		"outmtaid_name":     egress,
		"sender":            str("sender"),
	}
}

func truncateBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
