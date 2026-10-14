package scrub

import "testing"

func TestContainsSecret(t *testing.T) {
	hits := []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----\nabc",
		"key = sk-abcdefghijklmnop",
		"curl -H 'Authorization: Bearer abcdefghijklmnopqrstuv'",
		"AKIAABCDEFGHIJKLMNOP",
		"ghp_" + "abcdefghijklmnopqrstuvwxyzabcdef",
		"postgres://user:hunter22@db/app",
		`"api_key": "abcdef0123456789abcdef"`,
		"PASSWORD=correcthorsebatterystaple",
	}
	for _, s := range hits {
		if !ContainsSecret(s) {
			t.Errorf("missed secret in %q", s)
		}
	}
	misses := []string{
		"echo hello",
		`token = os.environ.get("TOKEN")`,
		"password = getpass()",
		"Bearer token goes here",
		"https://example.com/path",
		"secret_name = 'short'",
	}
	for _, s := range misses {
		if ContainsSecret(s) {
			t.Errorf("false positive in %q", s)
		}
	}
}
