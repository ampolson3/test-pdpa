// Package clientip works out the address of the client behind a request (decisions.md D-23). Only when the
// TCP peer is one of the configured trusted proxies (TRUSTED_PROXIES: IPs or CIDRs, comma separated) is
// X-Forwarded-For read — right to left, skipping trusted hops, taking the first address that isn't one — so a
// client can't choose its own address by sending the header itself. With no trusted proxies the peer is the
// client.
package clientip

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

// Resolver holds the trusted proxy ranges.
type Resolver struct {
	trusted []netip.Prefix
}

// Parse builds a Resolver from a TRUSTED_PROXIES value ("" = trust no proxy).
func Parse(list string) (*Resolver, error) {
	r := &Resolver{}
	for _, s := range strings.Split(list, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q: %w", s, err)
			}
			a = a.Unmap()
			r.trusted = append(r.trusted, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %q: %w", s, err)
		}
		r.trusted = append(r.trusted, p.Masked())
	}
	return r, nil
}

func (r *Resolver) isTrusted(a netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Resolve returns the client address of req, or false when not even the peer address is readable.
func (r *Resolver) Resolve(req *http.Request) (netip.Addr, bool) {
	ap, err := netip.ParseAddrPort(req.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	client := ap.Addr().Unmap()
	if !r.isTrusted(client) {
		return client, true
	}
	var hops []string
	for _, h := range req.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			// A malformed hop was not written by a proxy we trust: stop at the last address we could vouch for.
			return client, true
		}
		client = a.Unmap()
		if !r.isTrusted(client) {
			return client, true
		}
	}
	return client, true // every hop is a trusted proxy: the leftmost is as far as the chain goes
}

type ctxKey struct{}

// Middleware stores the client address in the request context (From) for the rate limiter and the audit.
func (r *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if a, ok := r.Resolve(req); ok {
			req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, a))
		}
		next.ServeHTTP(w, req)
	})
}

// From returns the client address the Middleware resolved.
func From(ctx context.Context) (netip.Addr, bool) {
	a, ok := ctx.Value(ctxKey{}).(netip.Addr)
	return a, ok
}
