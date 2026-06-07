package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name   string
		header map[string]string
		addr   string
		want   string
	}{
		{
			name: "CF-Connecting-IP single IP",
			header: map[string]string{
				"CF-Connecting-IP": "203.0.113.42",
			},
			addr: "127.0.0.1:7844",
			want: "203.0.113.42",
		},
		{
			name: "CF-Connecting-IP takes precedence over X-Forwarded-For",
			header: map[string]string{
				"CF-Connecting-IP": "203.0.113.42",
				"X-Forwarded-For":  "198.51.100.25, 10.0.0.5",
			},
			addr: "127.0.0.1:7844",
			want: "203.0.113.42",
		},
		{
			name: "CF-Connecting-IP when XFF has tunnel IP only",
			header: map[string]string{
				"CF-Connecting-IP": "198.51.100.25",
				"X-Forwarded-For":  "127.0.0.1",
			},
			addr: "127.0.0.1:7844",
			want: "198.51.100.25",
		},
		{
			name: "X-Forwarded-For single IP",
			header: map[string]string{
				"X-Forwarded-For": "203.0.113.50",
			},
			addr: "10.0.0.1:43210",
			want: "203.0.113.50",
		},
		{
			name: "X-Forwarded-For chained IPs",
			header: map[string]string{
				"X-Forwarded-For": "198.51.100.25, 192.0.2.1, 10.0.0.5",
			},
			addr: "10.0.0.1:43210",
			want: "198.51.100.25",
		},
		{
			name: "X-Forwarded-For invalid falls through",
			header: map[string]string{
				"X-Forwarded-For": "not-an-ip",
			},
			addr: "10.0.0.1:43210",
			want: "10.0.0.1",
		},
		{
			name: "X-Real-IP when XFF absent",
			header: map[string]string{
				"X-Real-IP": "198.51.100.80",
			},
			addr: "10.0.0.1:43210",
			want: "198.51.100.80",
		},
		{
			name:   "Forwarded header quoted IP",
			header: map[string]string{
				"Forwarded": "for=\"203.0.113.99\"",
			},
			addr: "10.0.0.1:43210",
			want: "203.0.113.99",
		},
		{
			name:   "Forwarded header unquoted IP",
			header: map[string]string{
				"Forwarded": "for=203.0.113.99;proto=https",
			},
			addr: "10.0.0.1:43210",
			want: "203.0.113.99",
		},
		{
			name: "X-Forwarded-For takes precedence over X-Real-IP",
			header: map[string]string{
				"X-Forwarded-For": "203.0.113.10",
				"X-Real-IP":       "198.51.100.80",
			},
			addr: "10.0.0.1:43210",
			want: "203.0.113.10",
		},
		{
			name: "Both XFF and X-Real-IP invalid fall through",
			header: map[string]string{
				"X-Forwarded-For": "bad-ip",
				"X-Real-IP":       "also-bad",
			},
			addr: "10.0.0.1:43210",
			want: "10.0.0.1",
		},
		{
			name: "No headers falls back to RemoteAddr",
			header: map[string]string{},
			addr:   "203.0.113.5:12345",
			want:   "203.0.113.5",
		},
		{
			name: "No headers RemoteAddr no port",
			header: map[string]string{},
			addr:   "203.0.113.5",
			want:   "203.0.113.5",
		},
		{
			name: "IPv6 in X-Forwarded-For",
			header: map[string]string{
				"X-Forwarded-For": "2001:db8::1",
			},
			addr: "10.0.0.1:43210",
			want: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.addr
			for k, v := range tt.header {
				r.Header.Set(k, v)
			}
			got := clientIP(r)
			if got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
