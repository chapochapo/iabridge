// Package middleware provides HTTP middleware for iabridge.
package middleware

import (
	"net"
	"net/http"
	"strings"
)

// LANOnly returns a middleware that allows only requests originating from
// within the given CIDR range. All other requests receive 403 Forbidden.
//
// The allowed net.IPNet is parsed once at startup from ALLOWED_CIDR in config
// and passed here — no parsing at request time.
//
// IP resolution order:
//  1. RemoteAddr is always extracted first.
//  2. X-Forwarded-For is trusted only when RemoteAddr is loopback or within
//     the allowed CIDR — both indicate a known reverse proxy on the local network.
//     An external client cannot forge RemoteAddr, so this prevents IP spoofing.
//  3. Any request whose resolved IP is outside the allowed CIDR receives 403.
//
// Usage:
//
//	mux.Handle("/config", middleware.LANOnly(allowedNet, configHandler))
func LANOnly(allowed *net.IPNet, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, err := originIP(r, allowed)
		if err != nil || !allowed.Contains(ip) {
			http.Error(w, "403 Forbidden — this page is only accessible from your local network", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originIP extracts the origin IP from a request.
// X-Forwarded-For is only honoured when RemoteAddr is loopback or within trusted,
// preventing external clients from spoofing their source IP.
func originIP(r *http.Request, trusted *net.IPNet) (net.IP, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil, err
	}
	remoteIP := net.ParseIP(host)
	if remoteIP == nil {
		return nil, &net.AddrError{Err: "invalid RemoteAddr", Addr: host}
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if remoteIP.IsLoopback() || trusted.Contains(remoteIP) {
			raw := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
			ip := net.ParseIP(raw)
			if ip == nil {
				return nil, &net.AddrError{Err: "invalid IP in X-Forwarded-For", Addr: raw}
			}
			return ip, nil
		}
		// RemoteAddr is outside trusted range; ignore XFF to prevent spoofing.
	}

	return remoteIP, nil
}
