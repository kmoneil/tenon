package tenon_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// TestConformance_DI018_HostForms holds the forms other packages render in to
// the display form and the JSON projection: log/slog logs values, types,
// constraints and paths by their display forms, encoding/json writes types,
// constraints and paths as the text of theirs and a value as its projection,
// failing where the projection fails. A secret shows in none of them.
func TestConformance_DI018_HostForms(t *testing.T) {
	conformance.Covers(t, "DI-018", "MK-011")
	secret := stamp{id: "secret", redact: true}
	v := tenon.Object(map[string]tenon.Value{
		"name":  tenon.String("web"),
		"token": tenon.WithMarks(tenon.String("hunter2"), secret),
	})
	typ := tenon.ListType(tenon.StringType())
	c := tenon.ListOf(tenon.Any())
	p := tenon.Path{}.Attribute("a").Index(tenon.NumberFromInt(0))

	// Each slog handler logs the display form, which withholds the secret.
	for _, handler := range []func(*bytes.Buffer) slog.Handler{
		func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
		func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
	} {
		var b bytes.Buffer
		slog.New(handler(&b)).Info("plan", "value", v, "type", typ, "constraint", c, "path", p)
		line := b.String()
		for _, want := range []string{v.String(), typ.String(), c.String(), p.String()} {
			if !strings.Contains(line, want) && !strings.Contains(line, strings.ReplaceAll(want, `"`, `\"`)) {
				t.Errorf("the log line %q does not hold the display form %s", line, want)
			}
		}
		if strings.Contains(line, "hunter2") {
			t.Errorf("the log line %q shows the secret", line)
		}
	}

	// encoding/json writes the text of types, constraints and paths, and a
	// value's projection.
	plain := tenon.Object(map[string]tenon.Value{"name": tenon.String("web"), "port": tenon.NumberFromInt(443)})
	got, err := json.Marshal(struct {
		T tenon.Type
		C tenon.Constraint
		P tenon.Path
		V tenon.Value
	}{typ, c, p, plain})
	want := `{"T":"list(string)","C":"list_of(any)","P":".a[0]","V":{"name":"web","port":443}}`
	if err != nil || string(got) != want {
		t.Errorf("json.Marshal gave %s, %v, want %s", got, err, want)
	}

	// A value that does not project fails the marshaling with the *Error the
	// projection gives, whatever wraps it on the way out.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		code tenon.Code
	}{
		{"a secret", v, tenon.CodeSerializeRedacted},
		{"an unknown", tenon.Unknown(tenon.NumberType()), tenon.CodeSerializeNotKnown},
	} {
		out, err := json.Marshal(tt.v)
		var failed *tenon.Error
		if !errors.As(err, &failed) || failed.Diagnostics()[0].Code != tt.code || strings.Contains(string(out)+err.Error(), "hunter2") {
			t.Errorf("json.Marshal of %s gave %s, %v, want a *tenon.Error of code %s", tt.name, out, err, tt.code)
		}
	}

	// The zero values fail rather than panic, and omitzero leaves the zero
	// Value out.
	if _, err := json.Marshal(tenon.Value{}); err == nil {
		t.Error("json.Marshal of the zero Value succeeded")
	}
	if _, err := (tenon.Type{}).MarshalText(); err == nil {
		t.Error("MarshalText of the zero Type succeeded")
	}
	if _, err := (tenon.Constraint{}).MarshalText(); err == nil {
		t.Error("MarshalText of the zero Constraint succeeded")
	}
	if text, err := (tenon.Path{}).MarshalText(); err != nil || string(text) != "." {
		t.Errorf("MarshalText of the empty path = %q, %v, want \".\"", text, err)
	}
	out, err := json.Marshal(struct {
		V tenon.Value `json:",omitzero"`
	}{})
	if err != nil || string(out) != "{}" {
		t.Errorf("an omitzero field holding the zero Value marshaled as %s, %v, want {}", out, err)
	}
	if got := (tenon.Value{}).LogValue().String(); got != "<zero Value>" {
		t.Errorf("the zero Value logs as %q", got)
	}
}
