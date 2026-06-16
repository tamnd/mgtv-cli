package mgtv

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// Client HTTP behaviour is covered in mgtv_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "mgtv" {
		t.Errorf("Scheme = %q, want mgtv", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "mgtv" {
		t.Errorf("Identity.Binary = %q, want mgtv", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in, typ, id string
	}{
		{"867784", "video", "867784"},
		{"https://www.mgtv.com/b/867784/24423183.html", "video", "867784"},
		{"https://www.mgtv.com/b/875592.html", "video", "875592"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType, id, want string
	}{
		{"video", "867784", "https://www.mgtv.com/b/867784.html"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)", tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestClassifyInvalid(t *testing.T) {
	_, _, err := Domain{}.Classify("not-a-clip-id")
	if err == nil {
		t.Error("expected error for invalid input")
	}
}
