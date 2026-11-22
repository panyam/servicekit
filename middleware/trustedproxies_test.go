package middleware

// Forwarding headers are client-supplied, so they count only from a trusted
// proxy. See https://cwe.mitre.org/data/definitions/348.html and
// https://www.rfc-editor.org/rfc/rfc7239#section-5.4. Cases follow
// oneauth's core.TrustedProxies tests (servicekit#67).

import (
	"crypto/tls"
	"errors"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func mustProxies(t *testing.T, entries ...string) TrustedProxies {
	t.Helper()
	tp, err := ParseTrustedProxies(entries)
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

func TestParseTrustedProxies(t *testing.T) {
	tp := mustProxies(t, " 10.0.0.0/8 ", "", "192.0.2.10", "::1", "::ffff:198.51.100.7", "172.16.5.9/12")
	want := []string{"10.0.0.0/8", "192.0.2.10/32", "::1/128", "198.51.100.7/32", "172.16.0.0/12"}
	if len(tp) != len(want) {
		t.Fatalf("got %v, want %v", tp, want)
	}
	for i, w := range want {
		if tp[i].String() != w {
			t.Errorf("entry %d: got %s, want %s", i, tp[i], w)
		}
	}
}

func TestParseTrustedProxies_Errors(t *testing.T) {
	for _, bad := range []string{"not-an-ip", "10.0.0.0/33", "10.0.0.1:80", "proxy.internal"} {
		if _, err := ParseTrustedProxies([]string{"127.0.0.1", bad}); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestTrustedProxies_EmptyTrustsNobody(t *testing.T) {
	for name, tp := range map[string]TrustedProxies{"nil": nil, "empty": {}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", "203.0.113.50")
		r.Header.Set("X-Real-IP", "203.0.113.51")
		r.Header.Set("X-Forwarded-Proto", "https")
		if got := tp.ClientIP(r); got != "10.0.0.1" {
			t.Errorf("%s: ClientIP = %q, want the peer 10.0.0.1", name, got)
		}
		if tp.IsHTTPS(r) {
			t.Errorf("%s: IsHTTPS = true from headers with no trusted proxies", name)
		}
	}
}

func TestTrustedProxies_Contains(t *testing.T) {
	tp := mustProxies(t, "10.0.0.0/8", "2001:db8::/32")
	for addr, want := range map[string]bool{
		"10.1.2.3":        true,
		"::ffff:10.1.2.3": true,
		"11.0.0.1":        false,
		"2001:db8::5":     true,
		"2001:db9::5":     false,
		"::ffff:11.0.0.1": false,
	} {
		if got := tp.Contains(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Contains(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestTrustedProxies_ClientIP(t *testing.T) {
	tp := mustProxies(t, "10.0.0.0/8", "::1")
	for name, tc := range map[string]struct {
		remote string
		xff    []string
		xri    string
		want   string
	}{
		"untrusted peer, forged xff":            {remote: "198.51.100.99:1", xff: []string{"1.2.3.4"}, want: "198.51.100.99"},
		"untrusted peer, forged x-real-ip":      {remote: "198.51.100.99:1", xri: "1.2.3.4", want: "198.51.100.99"},
		"trusted peer, one hop":                 {remote: "10.0.0.1:1", xff: []string{"203.0.113.50"}, want: "203.0.113.50"},
		"client prepends a forged entry":        {remote: "10.0.0.1:1", xff: []string{"1.2.3.4, 203.0.113.50"}, want: "203.0.113.50"},
		"two trusted hops are skipped":          {remote: "10.0.0.1:1", xff: []string{"1.2.3.4, 203.0.113.50, 10.0.0.9"}, want: "203.0.113.50"},
		"xff split across header lines":         {remote: "10.0.0.1:1", xff: []string{"1.2.3.4", "203.0.113.50, 10.0.0.9"}, want: "203.0.113.50"},
		"every hop trusted: leftmost wins":      {remote: "10.0.0.1:1", xff: []string{"10.5.5.5, 10.0.0.9"}, want: "10.5.5.5"},
		"garbage hop stops the walk":            {remote: "10.0.0.1:1", xff: []string{"203.0.113.50, junk"}, want: "10.0.0.1"},
		"empty entries are skipped":             {remote: "10.0.0.1:1", xff: []string{" , 203.0.113.50 ,"}, want: "203.0.113.50"},
		"x-real-ip only without xff":            {remote: "10.0.0.1:1", xri: "203.0.113.51", want: "203.0.113.51"},
		"xff wins over x-real-ip":               {remote: "10.0.0.1:1", xff: []string{"203.0.113.50"}, xri: "203.0.113.51", want: "203.0.113.50"},
		"bad x-real-ip falls back to peer":      {remote: "10.0.0.1:1", xri: "junk", want: "10.0.0.1"},
		"ipv6 trusted peer":                     {remote: "[::1]:1", xff: []string{"2001:db8::7"}, want: "2001:db8::7"},
		"mapped ipv4 peer is unmapped":          {remote: "[::ffff:10.0.0.1]:1", xff: []string{"::ffff:203.0.113.50"}, want: "203.0.113.50"},
		"peer without a port":                   {remote: "198.51.100.99", want: "198.51.100.99"},
		"unparseable peer is returned verbatim": {remote: "pipe", xff: []string{"1.2.3.4"}, want: "pipe"},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if tc.xri != "" {
				r.Header.Set("X-Real-IP", tc.xri)
			}
			if got := tp.ClientIP(r); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrustedProxies_IsHTTPS(t *testing.T) {
	proxies := mustProxies(t, "10.0.0.0/8")
	for name, tc := range map[string]struct {
		remote    string
		tls       bool
		xfp       []string
		forwarded []string
		trusted   TrustedProxies
		want      bool
	}{
		"direct TLS":                        {remote: "203.0.113.5:4000", tls: true, want: true},
		"direct TLS, untrusted xfp http":    {remote: "203.0.113.5:4000", tls: true, xfp: []string{"http"}, want: true},
		"plain HTTP, no headers":            {remote: "203.0.113.5:4000", want: false},
		"trusted proxy says https":          {remote: "10.0.0.7:4000", xfp: []string{"https"}, trusted: proxies, want: true},
		"trusted proxy says HTTPS":          {remote: "10.0.0.7:4000", xfp: []string{"HTTPS"}, trusted: proxies, want: true},
		"trusted proxy says http":           {remote: "10.0.0.7:4000", xfp: []string{"http"}, trusted: proxies, want: false},
		"trusted proxy, no header":          {remote: "10.0.0.7:4000", trusted: proxies, want: false},
		"untrusted peer says https":         {remote: "203.0.113.5:4000", xfp: []string{"https"}, trusted: proxies, want: false},
		"no proxies configured":             {remote: "10.0.0.7:4000", xfp: []string{"https"}, want: false},
		"xfp list: only first counts":       {remote: "10.0.0.7:4000", xfp: []string{"http, https"}, trusted: proxies, want: false},
		"xfp list: first is https":          {remote: "10.0.0.7:4000", xfp: []string{" https ,http"}, trusted: proxies, want: true},
		"xfp repeated: first header counts": {remote: "10.0.0.7:4000", xfp: []string{"http", "https"}, trusted: proxies, want: false},
		"Forwarded proto=https":             {remote: "10.0.0.7:4000", forwarded: []string{"for=198.51.100.9;proto=https;by=10.0.0.7"}, trusted: proxies, want: true},
		"Forwarded quoted, mixed case":      {remote: "10.0.0.7:4000", forwarded: []string{`For="[2001:db8::1]"; Proto="HTTPS"`}, trusted: proxies, want: true},
		"Forwarded: first element counts":   {remote: "10.0.0.7:4000", forwarded: []string{"proto=http, proto=https"}, trusted: proxies, want: false},
		"Forwarded without proto":           {remote: "10.0.0.7:4000", forwarded: []string{"for=198.51.100.9"}, trusted: proxies, want: false},
		"Forwarded from untrusted peer":     {remote: "203.0.113.5:4000", forwarded: []string{"proto=https"}, trusted: proxies, want: false},
		"xfp wins over Forwarded":           {remote: "10.0.0.7:4000", xfp: []string{"http"}, forwarded: []string{"proto=https"}, trusted: proxies, want: false},
		"IPv4-mapped IPv6 trusted peer":     {remote: "[::ffff:10.0.0.7]:4000", xfp: []string{"https"}, trusted: proxies, want: true},
		"unparseable RemoteAddr":            {remote: "pipe", xfp: []string{"https"}, trusted: proxies, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://svc.example/login", nil)
			r.RemoteAddr = tc.remote
			r.TLS = nil
			if tc.tls {
				r.TLS = &tls.ConnectionState{}
			}
			for _, v := range tc.xfp {
				r.Header.Add("X-Forwarded-Proto", v)
			}
			for _, v := range tc.forwarded {
				r.Header.Add("Forwarded", v)
			}
			if got := tc.trusted.IsHTTPS(r); got != tc.want {
				t.Errorf("IsHTTPS = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckPlaintext(t *testing.T) {
	proxies := mustProxies(t, "10.0.0.0/8")
	for name, tc := range map[string]struct {
		addr    string
		tls     bool
		proxies TrustedProxies
		refuse  bool
	}{
		"all interfaces, bare":    {addr: ":8080", refuse: true},
		"0.0.0.0, bare":           {addr: "0.0.0.0:8080", refuse: true},
		"[::], bare":              {addr: "[::]:8080", refuse: true},
		"public address, bare":    {addr: "203.0.113.5:8080", refuse: true},
		"hostname, bare":          {addr: "svc.example:8080", refuse: true},
		"all interfaces, TLS":     {addr: ":8443", tls: true},
		"all interfaces, proxies": {addr: ":8080", proxies: proxies},
		"127.0.0.1":               {addr: "127.0.0.1:8080"},
		"127.0.0.53":              {addr: "127.0.0.53:8080"},
		"[::1]":                   {addr: "[::1]:8080"},
		"localhost":               {addr: "localhost:8080"},
		"LOCALHOST":               {addr: "LOCALHOST:8080"},
	} {
		t.Run(name, func(t *testing.T) {
			err := CheckPlaintext(tc.addr, tc.tls, tc.proxies)
			if tc.refuse != errors.Is(err, ErrPlaintextCredentials) {
				t.Errorf("CheckPlaintext(%q) = %v, refuse = %v", tc.addr, err, tc.refuse)
			}
			if !tc.refuse && err != nil {
				t.Errorf("CheckPlaintext(%q) = %v, want nil", tc.addr, err)
			}
		})
	}
}

func TestCheckPlaintext_BadAddr(t *testing.T) {
	err := CheckPlaintext("8080", false, nil)
	if err == nil || errors.Is(err, ErrPlaintextCredentials) {
		t.Errorf("CheckPlaintext(\"8080\") = %v, want a parse error", err)
	}
}
