package metrics

import (
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/buildinfo"
)

func TestBuildInfo_ExposesResolvedBuild(t *testing.T) {
	families, err := Registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	info := buildinfo.Get()
	want := map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"build_date": info.BuildDate,
		"channel":    info.Channel,
	}

	for _, mf := range families {
		if mf.GetName() != "restic_server_build_info" {
			continue
		}
		if n := len(mf.GetMetric()); n != 1 {
			t.Fatalf("restic_server_build_info has %d series, want 1", n)
		}
		m := mf.GetMetric()[0]
		if v := m.GetGauge().GetValue(); v != 1 {
			t.Errorf("value = %v, want 1", v)
		}
		got := map[string]string{}
		for _, lp := range m.GetLabel() {
			got[lp.GetName()] = lp.GetValue()
		}
		if len(got) != len(want) {
			t.Errorf("labels = %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("label %s = %q, want %q", k, got[k], v)
			}
		}
		return
	}
	t.Fatal("restic_server_build_info not registered")
}
