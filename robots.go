package kloakd

import (
	"net/url"
	"strings"
)

// robots.txt is respected by default on every route that fetches a site.
//
// Before it asks for a page KLOAKD reads the site's robots.txt. When the
// file disallows the page, or cannot be read, the request stops with HTTP
// 409 ROBOTS_DISALLOWED (*RobotsDisallowedError: Reason, Host, Rule) and
// nothing is fetched. To go past the stop, send the same request again with
// the stopped host in the method's RobotsAcknowledgedHosts option (sent as
// robots_acknowledged_hosts): by naming a host you confirm you have the
// right to access its content despite its robots.txt and accept
// responsibility for doing so under the Terms. The acknowledgement counts
// for that request and those exact hosts only (nothing is remembered); the
// API records it with the Terms version. It takes bare host names (as the
// 409's Host gives them), at most 50; anything else is a 422.
//
// A 503 ROBOTS_CHECK_UNAVAILABLE is not a stop and there is nothing to
// acknowledge: KLOAKD could not read the file (*RobotsCheckUnavailableError,
// retried by the SDK), or every network exit refused to connect to the site
// (*RobotsExitsRefusedError, reason "exits_refused", not retried).

// Codes and reasons of the robots.txt answers (detail.error, detail.reason).
const (
	RobotsDisallowedCode       = "ROBOTS_DISALLOWED"
	RobotsCheckUnavailableCode = "ROBOTS_CHECK_UNAVAILABLE"
	RobotsReasonDisallowed     = "disallowed"
	RobotsReasonUnreadable     = "unreadable"
	RobotsReasonExitsRefused   = "exits_refused"
	// RobotsAcknowledgedHostsField is the request field that acknowledges
	// a stop.
	RobotsAcknowledgedHostsField = "robots_acknowledged_hosts"
	// MaxRobotsAcknowledgedHosts is the most hosts one request may name.
	MaxRobotsAcknowledgedHosts = 50
)

// applyRobotsAck writes robots_acknowledged_hosts into a request body when
// hosts are named; none is sent otherwise.
func applyRobotsAck(body map[string]interface{}, hosts []string) {
	if len(hosts) > 0 {
		body[RobotsAcknowledgedHostsField] = hosts
	}
}

// withRobotsAckQuery adds robots_acknowledged_hosts to a URL's query, once
// per host, for a route that takes it as a query parameter.
func withRobotsAckQuery(reqURL string, hosts []string) string {
	if len(hosts) == 0 {
		return reqURL
	}
	sep := "?"
	if strings.Contains(reqURL, "?") {
		sep = "&"
	}
	return reqURL + sep + url.Values{RobotsAcknowledgedHostsField: hosts}.Encode()
}
