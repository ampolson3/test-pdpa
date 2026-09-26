package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolve(t *testing.T) {
	r, err := Parse("10.0.0.0/8, 192.168.1.5 ,::1")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, peer string
		xff        []string
		want       string
	}{
		{"direct client, header ignored", "203.0.113.7:5000", []string{"1.2.3.4"}, "203.0.113.7"},
		{"load balancer in front", "10.0.0.2:443", []string{"198.51.100.9"}, "198.51.100.9"},
		{"spoofed left entries don't count", "10.0.0.2:443", []string{"6.6.6.6, 198.51.100.9"}, "198.51.100.9"},
		{"chain of trusted proxies", "10.0.0.2:443", []string{"198.51.100.9, 192.168.1.5", "10.1.1.1"}, "198.51.100.9"},
		{"no header behind a proxy", "10.0.0.2:443", nil, "10.0.0.2"},
		{"malformed hop stops the walk", "10.0.0.2:443", []string{"198.51.100.9, junk"}, "10.0.0.2"},
		{"all hops trusted", "10.0.0.2:443", []string{"10.9.9.9"}, "10.9.9.9"},
		{"IPv4-mapped IPv6 peer", "[::ffff:10.0.0.2]:443", []string{"198.51.100.9"}, "198.51.100.9"},
		{"IPv6 client", "[::1]:443", []string{"2001:db8::1"}, "2001:db8::1"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = c.peer
		for _, h := range c.xff {
			req.Header.Add("X-Forwarded-For", h)
		}
		got, ok := r.Resolve(req)
		if !ok || got.String() != c.want {
			t.Errorf("%s: %v %v, want %s", c.name, got, ok, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	none, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	if got, _ := none.Resolve(req); got.String() != "10.0.0.2" {
		t.Errorf("no trusted proxies must use the peer: %v", got)
	}
	if _, err := Parse("10.0.0.0/33"); err == nil {
		t.Error("bad prefix accepted")
	}
	if _, err := Parse("proxy.local"); err == nil {
		t.Error("host name accepted")
	}
}

func TestMiddleware(t *testing.T) {
	r, _ := Parse("10.0.0.0/8")
	var seen string
	h := r.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		a, _ := From(req.Context())
		seen = a.String()
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "198.51.100.9" {
		t.Errorf("from context: %q", seen)
	}
}
