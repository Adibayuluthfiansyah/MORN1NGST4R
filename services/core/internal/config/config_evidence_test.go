package config_test

import (
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
)

func TestEvidenceDirectoryDefaultsFollowXDGData(t *testing.T) {
	for name, tc := range map[string]struct {
		environ []string
		want    string
	}{
		"home only":               {[]string{"HOME=/srv/ana"}, "/srv/ana/.local/share/morningstar/evidence"},
		"xdg data":                {[]string{"HOME=/srv/ana", "XDG_DATA_HOME=/data"}, "/data/morningstar/evidence"},
		"relative xdg is ignored": {[]string{"HOME=/srv/ana", "XDG_DATA_HOME=rel"}, "/srv/ana/.local/share/morningstar/evidence"},
		"override":                {[]string{"HOME=/srv/ana", "MORNINGSTAR_EVIDENCE_DIR=/var/lib/ms/evidence"}, "/var/lib/ms/evidence"},
		"override without a home": {[]string{"MORNINGSTAR_EVIDENCE_DIR=/var/lib/ms/evidence", "MORNINGSTAR_DB_SOCKET_DIR=/run/ms", "MORNINGSTAR_STATE_DIR=/var/lib/ms"}, "/var/lib/ms/evidence"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := config.Load(tc.environ)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Evidence.Dir != tc.want {
				t.Errorf("Evidence.Dir = %q, want %q", cfg.Evidence.Dir, tc.want)
			}
		})
	}
}

func TestEvidenceDirectoryIsNeverRepaired(t *testing.T) {
	for name, raw := range map[string]string{
		"relative":     "evidence",
		"empty":        "",
		"trailing":     "/var/ms/evidence/",
		"dot elements": "/var/ms/../evidence",
		"control":      "/var/ms/e\tv",
		"too long":     "/" + strings.Repeat("a", 300),
	} {
		_, err := config.Load([]string{"HOME=/h", "MORNINGSTAR_EVIDENCE_DIR=" + raw})
		if err == nil || !strings.Contains(err.Error(), "MORNINGSTAR_EVIDENCE_DIR") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestEvidenceSizeLimit(t *testing.T) {
	cfg, err := config.Load([]string{"HOME=/h"})
	if err != nil || cfg.Evidence.MaxBytes != 8<<20 {
		t.Fatalf("default = %d, %v", cfg.Evidence.MaxBytes, err)
	}
	cfg, err = config.Load([]string{"HOME=/h", "MORNINGSTAR_EVIDENCE_MAX_BYTES=1048576"})
	if err != nil || cfg.Evidence.MaxBytes != 1<<20 {
		t.Fatalf("set = %d, %v", cfg.Evidence.MaxBytes, err)
	}
	for _, raw := range []string{"", "0", "-1", "1k", "1.5", "0x10", "99999999999", "67108865", " 5"} {
		_, err := config.Load([]string{"HOME=/h", "MORNINGSTAR_EVIDENCE_MAX_BYTES=" + raw})
		if err == nil || !strings.Contains(err.Error(), "MORNINGSTAR_EVIDENCE_MAX_BYTES") {
			t.Errorf("%q: err = %v", raw, err)
		}
	}
	if cfg, err = config.Load([]string{"HOME=/h", "MORNINGSTAR_EVIDENCE_MAX_BYTES=67108864"}); err != nil || cfg.Evidence.MaxBytes != 64<<20 {
		t.Errorf("the largest accepted value = %d, %v", cfg.Evidence.MaxBytes, err)
	}
}
