package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("INITDATA_TTL_HOURS", "")
	t.Setenv("WORKER_POLL_SECONDS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.InitDataTTL.Hours() != 24 {
		t.Errorf("InitDataTTL = %v, want 24h", cfg.InitDataTTL)
	}
	if cfg.WorkerPoll.Seconds() != 30 {
		t.Errorf("WorkerPoll = %v, want 30s", cfg.WorkerPoll)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}

func TestParseModeratorIDs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("MODERATOR_TG_IDS", "111, 222,,bad")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ModeratorTGIDs) != 2 || cfg.ModeratorTGIDs[0] != 111 || cfg.ModeratorTGIDs[1] != 222 {
		t.Errorf("ModeratorTGIDs = %v", cfg.ModeratorTGIDs)
	}
}
