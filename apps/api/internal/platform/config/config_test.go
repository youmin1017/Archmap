package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAuthGuards(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"defaults are production and secure", nil, ""},
		{"dev login in development", map[string]string{"APP_ENV": "development", "AUTH_DEV_LOGIN": "true", "COOKIE_SECURE": "false"}, ""},
		{"dev login without APP_ENV", map[string]string{"AUTH_DEV_LOGIN": "true"}, "AUTH_DEV_LOGIN"},
		{"dev login in production", map[string]string{"APP_ENV": "production", "AUTH_DEV_LOGIN": "true"}, "AUTH_DEV_LOGIN"},
		{"dev login in test", map[string]string{"APP_ENV": "test", "AUTH_DEV_LOGIN": "true"}, "AUTH_DEV_LOGIN"},
		{"insecure cookies in production", map[string]string{"COOKIE_SECURE": "false"}, "COOKIE_SECURE"},
		{"unknown environment", map[string]string{"APP_ENV": "staging"}, "APP_ENV"},
		{"removed dev header flag", map[string]string{"APP_ENV": "development", "AUTH_DEV_HEADER": "true"}, "AUTH_DEV_HEADER"},
		{"bad local password policy", map[string]string{"AUTH_LOCAL_PASSWORD": "nobody"}, "AUTH_LOCAL_PASSWORD"},
		{"bad idle timeout", map[string]string{"SESSION_IDLE_TIMEOUT": "soon"}, "SESSION_IDLE_TIMEOUT"},
		{"bad trusted proxy", map[string]string{"TRUSTED_PROXIES": "10.0.0.1"}, "TRUSTED_PROXIES"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(func(k string) string { return tc.env[k] })
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error = %v, want mention of %s", err, tc.wantErr)
			}
		})
	}
}

func TestLoadAuthValues(t *testing.T) {
	env := map[string]string{
		"SESSION_IDLE_TIMEOUT": "30m", "SESSION_MAX_AGE": "24h",
		"AUTH_LOCAL_PASSWORD": "maintainers", "TRUSTED_PROXIES": "10.0.0.0/8, 192.168.1.1/32",
	}
	cfg, err := load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Auth
	if cfg.Env != EnvProduction || a.DevLogin || !a.CookieSecure || a.IdleTimeout != 30*time.Minute ||
		a.MaxAge != 24*time.Hour || a.LocalPassword != LocalPasswordMaintainers || len(a.TrustedProxies) != 2 {
		t.Fatalf("cfg = %+v", cfg)
	}
}
