package privacy

import (
	"testing"
)

func TestResolveProxyExplicitRoute(t *testing.T) {
	routes := []RouteEntry{
		{Service: "sam-gov", Proxy: "socks5h://10.0.0.1:9050"},
	}
	got := ResolveProxy("sam-gov", "", routes)
	if got != "socks5h://10.0.0.1:9050" {
		t.Errorf("expected explicit route proxy, got %q", got)
	}
}

func TestResolveProxyTorExpansion(t *testing.T) {
	routes := []RouteEntry{
		{Service: "sam-gov", Proxy: "tor"},
	}
	got := ResolveProxy("sam-gov", "", routes)
	if got != "socks5h://127.0.0.1:9050" {
		t.Errorf("expected tor expansion, got %q", got)
	}
}

func TestResolveProxyFallsBackToDefault(t *testing.T) {
	routes := []RouteEntry{
		{Service: "other-svc", Proxy: "socks5://1.2.3.4:1080"},
	}
	got := ResolveProxy("sam-gov", "socks5h://proxy.example.com:1080", routes)
	if got != "socks5h://proxy.example.com:1080" {
		t.Errorf("expected default proxy, got %q", got)
	}
}

func TestResolveProxyDirectBypassesDefault(t *testing.T) {
	routes := []RouteEntry{
		{Service: "local-crm", Proxy: "direct"},
	}
	got := ResolveProxy("local-crm", "socks5h://127.0.0.1:9050", routes)
	if got != "" {
		t.Errorf("expected empty (direct), got %q", got)
	}
}

func TestResolveProxyNoRoutesNoDefault(t *testing.T) {
	got := ResolveProxy("any-service", "", nil)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestResolveProxyNoneDefault(t *testing.T) {
	got := ResolveProxy("any-service", "none", nil)
	if got != "" {
		t.Errorf("expected empty for 'none' default, got %q", got)
	}
}

func TestValidateProxyURLValid(t *testing.T) {
	valid := []string{
		"",
		"none",
		"direct",
		"tor",
		"http://proxy.example.com:8080",
		"https://proxy.example.com:443",
		"socks5://127.0.0.1:1080",
		"socks5h://127.0.0.1:9050",
	}
	for _, v := range valid {
		if err := ValidateProxyURL(v); err != nil {
			t.Errorf("expected %q to be valid, got: %v", v, err)
		}
	}
}

func TestValidateProxyURLInvalidScheme(t *testing.T) {
	err := ValidateProxyURL("ftp://proxy.example.com")
	if err == nil {
		t.Fatal("expected error for ftp:// scheme")
	}
}

func TestValidateProxyURLMissingHost(t *testing.T) {
	err := ValidateProxyURL("socks5://")
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestValidateProxyURLMalformed(t *testing.T) {
	err := ValidateProxyURL("://not-a-url")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
}
