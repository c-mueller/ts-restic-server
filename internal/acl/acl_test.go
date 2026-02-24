package acl

import (
	"fmt"
	"testing"
)

func TestDefaultFullAccess(t *testing.T) {
	e, err := New(FullAccess, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/repo"); got != FullAccess {
		t.Errorf("got %v, want full-access", got)
	}
}

func TestDefaultDeny(t *testing.T) {
	e, err := New(Deny, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/repo"); got != Deny {
		t.Errorf("got %v, want deny", got)
	}
}

func TestDefaultReadOnly(t *testing.T) {
	e, err := New(ReadOnly, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/repo"); got != ReadOnly {
		t.Errorf("got %v, want read-only", got)
	}
}

func TestDefaultAppendOnly(t *testing.T) {
	e, err := New(AppendOnly, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/repo"); got != AppendOnly {
		t.Errorf("got %v, want append-only", got)
	}
}

func TestExactPathMatch(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/server-a"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/server-a"); got != FullAccess {
		t.Errorf("exact match: got %v, want full-access", got)
	}
}

func TestSubPathMatch(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/server-a"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/server-a/sub"); got != FullAccess {
		t.Errorf("sub-path: got %v, want full-access", got)
	}
}

func TestNoMatchFallsToDefault(t *testing.T) {
	e, err := New(ReadOnly, []Rule{
		{Paths: []string{"/server-a"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/other"); got != ReadOnly {
		t.Errorf("no match: got %v, want read-only (default)", got)
	}
}

func TestSegmentBoundary(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/server-a"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	// /server-ab should NOT match /server-a (segment boundary)
	if got := e.Resolve("10.0.0.1", "/server-ab"); got != Deny {
		t.Errorf("segment boundary: got %v, want deny", got)
	}
}

func TestWildcardIdentity(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"*"}, Permission: ReadOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("any-host", "/repo"); got != ReadOnly {
		t.Errorf("wildcard identity: got %v, want read-only", got)
	}
}

func TestSpecificIdentity(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"10.0.0.5"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.5", "/repo"); got != FullAccess {
		t.Errorf("matching identity: got %v, want full-access", got)
	}
	if got := e.Resolve("10.0.0.6", "/repo"); got != Deny {
		t.Errorf("non-matching identity: got %v, want deny", got)
	}
}

func TestDeeperPathOverrides(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/"}, Identities: []string{"*"}, Permission: ReadOnly},
		{Paths: []string{"/server-a"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/server-a"); got != FullAccess {
		t.Errorf("deeper path: got %v, want full-access", got)
	}
	if got := e.Resolve("10.0.0.1", "/other"); got != ReadOnly {
		t.Errorf("shallow path: got %v, want read-only", got)
	}
}

func TestDenyIsAbsolute(t *testing.T) {
	e, err := New(ReadOnly, []Rule{
		{Paths: []string{"/secret"}, Identities: []string{"*"}, Permission: FullAccess},
		{Paths: []string{"/secret"}, Identities: []string{"*"}, Permission: Deny},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/secret"); got != Deny {
		t.Errorf("deny absolute: got %v, want deny", got)
	}
}

func TestHighestPermissionWins(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"*"}, Permission: ReadOnly},
		{Paths: []string{"/repo"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/repo"); got != FullAccess {
		t.Errorf("highest perm: got %v, want full-access", got)
	}
}

func TestAppendOnlyLockSemantics(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"*"}, Permission: AppendOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !e.Allowed("10.0.0.1", "/repo", OpRead) {
		t.Error("append-only should allow read")
	}
	if !e.Allowed("10.0.0.1", "/repo", OpWrite) {
		t.Error("append-only should allow write")
	}
	if e.Allowed("10.0.0.1", "/repo", OpDelete) {
		t.Error("append-only should deny delete")
	}
}

func TestReadOnlyBlocksWrite(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"*"}, Permission: ReadOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !e.Allowed("10.0.0.1", "/repo", OpRead) {
		t.Error("read-only should allow read")
	}
	if e.Allowed("10.0.0.1", "/repo", OpWrite) {
		t.Error("read-only should deny write")
	}
	if e.Allowed("10.0.0.1", "/repo", OpDelete) {
		t.Error("read-only should deny delete")
	}
}

func TestDenyBlocksAll(t *testing.T) {
	e, err := New(FullAccess, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"*"}, Permission: Deny},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Allowed("10.0.0.1", "/repo", OpRead) {
		t.Error("deny should block read")
	}
	if e.Allowed("10.0.0.1", "/repo", OpWrite) {
		t.Error("deny should block write")
	}
	if e.Allowed("10.0.0.1", "/repo", OpDelete) {
		t.Error("deny should block delete")
	}
}

func TestEmptyRepoPath(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/"}, Identities: []string{"*"}, Permission: ReadOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", ""); got != ReadOnly {
		t.Errorf("empty path: got %v, want read-only", got)
	}
}

func TestMultiplePathsPerRule(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo-a", "/repo-b"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/repo-a"); got != FullAccess {
		t.Errorf("repo-a: got %v, want full-access", got)
	}
	if got := e.Resolve("10.0.0.1", "/repo-b"); got != FullAccess {
		t.Errorf("repo-b: got %v, want full-access", got)
	}
}

func TestMultipleIdentitiesPerRule(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"10.0.0.1", "10.0.0.2"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/repo"); got != FullAccess {
		t.Errorf("identity 1: got %v, want full-access", got)
	}
	if got := e.Resolve("10.0.0.2", "/repo"); got != FullAccess {
		t.Errorf("identity 2: got %v, want full-access", got)
	}
	if got := e.Resolve("10.0.0.3", "/repo"); got != Deny {
		t.Errorf("identity 3: got %v, want deny", got)
	}
}

func TestParsePermissionValid(t *testing.T) {
	cases := []struct {
		input string
		want  Permission
	}{
		{"deny", Deny},
		{"read-only", ReadOnly},
		{"append-only", AppendOnly},
		{"full-access", FullAccess},
		{"DENY", Deny},
		{"Full-Access", FullAccess},
	}
	for _, tc := range cases {
		got, err := ParsePermission(tc.input)
		if err != nil {
			t.Errorf("ParsePermission(%q): unexpected error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("ParsePermission(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestParsePermissionInvalid(t *testing.T) {
	_, err := ParsePermission("invalid")
	if err == nil {
		t.Error("expected error for invalid permission string")
	}
}

func TestEngineValidationEmptyPaths(t *testing.T) {
	_, err := New(Deny, []Rule{
		{Paths: nil, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err == nil {
		t.Error("expected error for empty paths")
	}
}

func TestEngineValidationEmptyIdentities(t *testing.T) {
	_, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: nil, Permission: FullAccess},
	})
	if err == nil {
		t.Error("expected error for empty identities")
	}
}

func TestPathNormalization(t *testing.T) {
	// Paths without leading "/" should be normalized
	e, err := New(Deny, []Rule{
		{Paths: []string{"server-a"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	// RepoPrefix provides paths without leading /
	if got := e.Resolve("10.0.0.1", "server-a"); got != FullAccess {
		t.Errorf("normalized path: got %v, want full-access", got)
	}
}

func TestRootRuleMatchesAll(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/"}, Identities: []string{"*"}, Permission: ReadOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Resolve("10.0.0.1", "/anything/deep/path"); got != ReadOnly {
		t.Errorf("root rule: got %v, want read-only", got)
	}
}

func TestFullAccessAllowsAll(t *testing.T) {
	e, err := New(Deny, []Rule{
		{Paths: []string{"/repo"}, Identities: []string{"*"}, Permission: FullAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !e.Allowed("10.0.0.1", "/repo", OpRead) {
		t.Error("full-access should allow read")
	}
	if !e.Allowed("10.0.0.1", "/repo", OpWrite) {
		t.Error("full-access should allow write")
	}
	if !e.Allowed("10.0.0.1", "/repo", OpDelete) {
		t.Error("full-access should allow delete")
	}
}

func TestPermissionString(t *testing.T) {
	cases := []struct {
		p    Permission
		want string
	}{
		{Deny, "deny"},
		{ReadOnly, "read-only"},
		{AppendOnly, "append-only"},
		{FullAccess, "full-access"},
		{Permission(99), "unknown(99)"},
	}
	for _, tc := range cases {
		if got := tc.p.String(); got != tc.want {
			t.Errorf("Permission(%d).String() = %q, want %q", int(tc.p), got, tc.want)
		}
	}
}

func BenchmarkResolve(b *testing.B) {
	rules := make([]Rule, 30)
	for i := range rules {
		rules[i] = Rule{
			Paths:      []string{fmt.Sprintf("/host-%d", i)},
			Identities: []string{fmt.Sprintf("10.0.0.%d", i)},
			Permission: FullAccess,
		}
	}
	e, err := New(ReadOnly, rules)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Resolve("10.0.0.15", "/host-15/backup")
	}
}
