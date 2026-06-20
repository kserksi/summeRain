package config

import "testing"

func TestLoadUsesMainlandCompatibleRecaptchaVerifyURL(t *testing.T) {
	t.Setenv("RECAPTCHA_VERIFY_URL", "")

	cfg := Load()

	if cfg.Captcha.Recaptcha.VerifyURL != "https://www.recaptcha.net/recaptcha/api/siteverify" {
		t.Fatalf("VerifyURL = %q, want recaptcha.net mirror", cfg.Captcha.Recaptcha.VerifyURL)
	}
}

func TestLoadDefaultsProviderFromRecaptchaEnabled(t *testing.T) {
	t.Setenv("CAPTCHA_PROVIDER", "")
	t.Setenv("RECAPTCHA_ENABLED", "true")
	if cfg := Load(); cfg.Captcha.Provider != "recaptcha" {
		t.Fatalf("Provider = %q, want recaptcha (derived)", cfg.Captcha.Provider)
	}

	t.Setenv("RECAPTCHA_ENABLED", "false")
	if cfg := Load(); cfg.Captcha.Provider != "none" {
		t.Fatalf("Provider = %q, want none", cfg.Captcha.Provider)
	}

	t.Setenv("CAPTCHA_PROVIDER", "turnstile")
	if cfg := Load(); cfg.Captcha.Provider != "turnstile" {
		t.Fatalf("Provider = %q, want turnstile", cfg.Captcha.Provider)
	}
}
