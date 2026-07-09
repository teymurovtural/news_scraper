package netguard

import (
	"context"
	"net"
	"strings"
	"testing"
)

// TestIsDisallowedIP_RecognizesInternalRanges — private/loopback/link-local
// IP-lərin düzgün tanındığını, public IP-lərin isə keçdiyini yoxlayır.
func TestIsDisallowedIP_RecognizesInternalRanges(t *testing.T) {
	cases := []struct {
		ip         string
		disallowed bool
	}{
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // loopback (IPv6)
		{"10.0.0.5", true},        // RFC1918 private
		{"172.16.0.5", true},      // RFC1918 private
		{"192.168.1.1", true},     // RFC1918 private
		{"169.254.169.254", true}, // link-local (bulud metadata)
		{"0.0.0.0", true},         // unspecified
		{"8.8.8.8", false},        // public (Google DNS)
		{"1.1.1.1", false},        // public (Cloudflare DNS)
		{"93.184.216.34", false},  // public (example.com-a bənzər)
	}

	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("test datası yanlışdır, parse olunmadı: %s", c.ip)
		}
		got := IsDisallowedIP(ip)
		if got != c.disallowed {
			t.Errorf("IsDisallowedIP(%s) = %v, gözlənilən %v", c.ip, got, c.disallowed)
		}
	}
}

// TestSafeDialContext_RejectsLoopbackLiteral — addr birbaşa loopback IP
// literalı olduqda (DNS-ə ehtiyac olmadan) SafeDialContext-in bağlantını
// rədd etdiyini yoxlayır.
func TestSafeDialContext_RejectsLoopbackLiteral(t *testing.T) {
	_, err := SafeDialContext(context.Background(), "tcp", "127.0.0.1:5434")
	if err == nil {
		t.Fatal("loopback IP-yə qoşulma rədd edilməli idi, amma xəta qayıtmadı")
	}
	if !strings.Contains(err.Error(), "rədd edildi") {
		t.Errorf("gözlənilməyən xəta mesajı: %v", err)
	}
}

// TestSafeDialContext_RejectsCloudMetadataLiteral — bulud metadata ünvanının
// (169.254.169.254) da rədd edildiyini yoxlayır — SSRF-lə həssas cloud
// mühitlərində ən çox hədəflənən ünvandır.
func TestSafeDialContext_RejectsCloudMetadataLiteral(t *testing.T) {
	_, err := SafeDialContext(context.Background(), "tcp", "169.254.169.254:80")
	if err == nil {
		t.Fatal("bulud metadata ünvanına qoşulma rədd edilməli idi, amma xəta qayıtmadı")
	}
}

// TestSafeDialContext_InvalidAddr_ReturnsError — host:port formatında
// olmayan bir ünvan verilsə, DNS/dial cəhdinə keçmədən aydın xəta qaytarır.
func TestSafeDialContext_InvalidAddr_ReturnsError(t *testing.T) {
	_, err := SafeDialContext(context.Background(), "tcp", "not-a-valid-addr")
	if err == nil {
		t.Fatal("yanlış formatlı ünvan üçün xəta gözlənilirdi")
	}
}
