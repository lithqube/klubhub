package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func valid() Config {
	return Config{
		DatabaseURL:  "postgres://klubhub_app@db/promoter",
		PublicOrigin: "http://localhost:4400, https://promoter.example/path",
		KEK:          base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))),
		KEKID:        "kek-1",
		AuthProvider: "local",
	}
}

func TestValidConfig(t *testing.T) {
	c := valid()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if o := c.Origins(); len(o) != 2 || o[1] != "https://promoter.example" {
		t.Fatalf("origins %v", o)
	}
}

func TestRejectsInsecureSettings(t *testing.T) {
	cases := map[string]func(*Config){
		"short KEK":         func(c *Config) { c.KEK = base64.StdEncoding.EncodeToString([]byte("short")) },
		"KEK not base64":    func(c *Config) { c.KEK = "***" },
		"no origin":         func(c *Config) { c.PublicOrigin = "not a url" },
		"unknown provider":  func(c *Config) { c.AuthProvider = "none" },
		"zitadel no aud":    func(c *Config) { c.AuthProvider = "zitadel"; c.ZitadelIssuer = "https://auth" },
		"bad previous KEKs": func(c *Config) { c.PreviousKEKs = "missing-colon" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := valid()
			mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatalf("%s must be rejected", name)
			}
		})
	}
}

func TestPreviousKEKs(t *testing.T) {
	c := valid()
	c.PreviousKEKs = "kek-0:" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("o", 32)))
	keks, err := c.KEKs()
	if err != nil || len(keks) != 2 || keks[1].ID() != "kek-0" {
		t.Fatalf("previous KEK not parsed: %v", err)
	}
}
