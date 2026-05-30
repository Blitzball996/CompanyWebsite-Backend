package geo

import (
	"path/filepath"
	"testing"
)

func TestLookupKnownIPs(t *testing.T) {
	if err := Init(filepath.Join("data", "ip2region.xdb")); err != nil {
		t.Fatalf("init xdb: %v", err)
	}
	cases := []string{
		"114.114.114.114", // 江苏 南京 (114DNS)
		"223.5.5.5",       // 阿里 DNS 浙江杭州
		"8.8.8.8",         // 美国 Google
		"1.2.3.4",
	}
	for _, ip := range cases {
		r := Lookup(ip)
		t.Logf("%-16s => 国家=%q 省=%q 市=%q ISP=%q", ip, r.Country, r.Province, r.City, r.ISP)
		if r.Country == "" {
			t.Errorf("%s: empty country (lookup likely failed)", ip)
		}
	}
	// private
	p := Lookup("127.0.0.1")
	t.Logf("127.0.0.1 => %+v", p)
}
