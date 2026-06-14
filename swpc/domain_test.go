package swpc

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "swpc" {
		t.Errorf("Scheme = %q, want swpc", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "swpc" {
		t.Errorf("Identity.Binary = %q, want swpc", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in      string
		uriType string
		id      string
	}{
		{"kindex", "dataset", "kindex"},
		{"solarwind", "dataset", "solarwind"},
		{"alerts", "dataset", "alerts"},
		{"flares", "dataset", "flares"},
		{"M2.1", "class", "M2.1"},
		{"X1.5", "class", "X1.5"},
		{"B3", "class", "B3"},
		{"C4.2", "class", "C4.2"},
		{"2024-01-01", "date", "2024-01-01"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) returned error: %v", tc.in, err)
			continue
		}
		if typ != tc.uriType || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)",
				tc.in, typ, id, tc.uriType, tc.id)
		}
	}
}

func TestClassifyError(t *testing.T) {
	_, _, err := Domain{}.Classify("unknown-thing")
	if err == nil {
		t.Error("Classify(unknown-thing) should return error")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("dataset", "kindex")
	want := "https://www.swpc.noaa.gov/products/kindex"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateError(t *testing.T) {
	_, err := Domain{}.Locate("class", "M2.1")
	if err == nil {
		t.Error("Locate(class, M2.1) should return error")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	// Kit domain is registered; check basic domain metadata round trips.
	sw := &SolarWind{Time: "2026-06-14T18:42:00", ProtonSpeed: 469}
	u, err := h.Mint(sw)
	if err != nil {
		t.Fatalf("Mint SolarWind: %v", err)
	}
	if want := "swpc://solarwind/2026-06-14T18:42:00"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}
}
