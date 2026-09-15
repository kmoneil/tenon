package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	rules := []Rule{
		{ID: "BB-001", Area: "BB", Withdrawn: true},
		{ID: "AA-001", Area: "AA", Outline: true},
	}
	data, err := encodeManifest(rules)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "rules": [
    {"id":"BB-001","area":"BB","withdrawn":true,"outline":false},
    {"id":"AA-001","area":"AA","withdrawn":false,"outline":true}
  ]
}
`
	if string(data) != want {
		t.Errorf("encodeManifest wrote\n%s\nwant\n%s", data, want)
	}
	back, err := decodeManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, rules) {
		t.Errorf("decodeManifest = %+v, want %+v", back, rules)
	}
}

func TestManifestDeterministic(t *testing.T) {
	var first []byte
	for i := range 50 {
		rules, err := parseSpec(sampleSpec)
		if err != nil {
			t.Fatal(err)
		}
		data, err := encodeManifest(rules)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = data
		} else if !bytes.Equal(data, first) {
			t.Fatalf("run %d produced\n%s\nbut run 0 produced\n%s", i, data, first)
		}
	}
}

func TestDecodeManifestRejects(t *testing.T) {
	rule := `{"id":"AA-001","area":"AA","withdrawn":false,"outline":false}`
	tests := []struct {
		name, data, want string
	}{
		{"not JSON", `rules`, "invalid character"},
		{"unknown field", `{"rules":[{"id":"AA-001","area":"AA","withdrawn":false,"outline":false,"note":""}]}`, "unknown field"},
		{"trailing data", `{"rules":[]} {}`, "unexpected data"},
		{"malformed identifier", `{"rules":[{"id":"AA-1","area":"AA","withdrawn":false,"outline":false}]}`, "malformed rule identifier"},
		{"area disagrees with identifier", `{"rules":[{"id":"AA-001","area":"BB","withdrawn":false,"outline":false}]}`, "has area"},
		{"rule listed twice", `{"rules":[` + rule + `,` + rule + `]}`, "listed twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeManifest([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeManifest(%s): error %v, want one containing %q", tt.data, err, tt.want)
			}
		})
	}
}

func TestCheckPermanence(t *testing.T) {
	one := Rule{ID: "AA-001", Area: "AA"}
	two := Rule{ID: "AA-002", Area: "AA"}
	twoWithdrawn := Rule{ID: "AA-002", Area: "AA", Withdrawn: true}
	tests := []struct {
		name     string
		old, cur []Rule
		want     string // a substring of the error, or "" for no error
	}{
		{"unchanged", []Rule{one, two}, []Rule{one, two}, ""},
		{"rule added", []Rule{one}, []Rule{one, two}, ""},
		{"rule withdrawn", []Rule{one, two}, []Rule{one, twoWithdrawn}, ""},
		{"rule removed", []Rule{one, two}, []Rule{one}, "rule AA-002 has disappeared"},
		{"withdrawn rule reinstated", []Rule{one, twoWithdrawn}, []Rule{one, two}, "rule AA-002 was withdrawn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPermanence(tt.old, tt.cur)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestDescribeChanges(t *testing.T) {
	old := []Rule{{ID: "AA-001", Area: "AA", Outline: true}}
	cur := []Rule{{ID: "AA-001", Area: "AA"}, {ID: "AA-002", Area: "AA"}}
	want := "  changed AA-001: withdrawn false -> false, outline true -> false\n  added AA-002"
	if got := describeChanges(old, cur); got != want {
		t.Errorf("describeChanges = %q, want %q", got, want)
	}
	if got := describeChanges(cur, cur); !strings.Contains(got, "formatting or order") {
		t.Errorf("describeChanges of identical lists = %q", got)
	}
}
