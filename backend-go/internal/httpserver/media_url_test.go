package httpserver

import "testing"

func TestSecurePublicMediaURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "public host", in: "http://im.xlcsh.top/images/result.png", want: "https://im.xlcsh.top/images/result.png"},
		{name: "https unchanged", in: "https://cdn.example.com/result.png", want: "https://cdn.example.com/result.png"},
		{name: "localhost unchanged", in: "http://localhost:3100/assets/result.png", want: "http://localhost:3100/assets/result.png"},
		{name: "loopback unchanged", in: "http://127.0.0.1:3100/assets/result.png", want: "http://127.0.0.1:3100/assets/result.png"},
		{name: "private address unchanged", in: "http://192.168.1.20/assets/result.png", want: "http://192.168.1.20/assets/result.png"},
		{name: "data url unchanged", in: "data:image/png;base64,abc", want: "data:image/png;base64,abc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := securePublicMediaURL(test.in); got != test.want {
				t.Fatalf("securePublicMediaURL(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}
