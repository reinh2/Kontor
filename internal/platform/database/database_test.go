package database

import "testing"

func TestMigrationVersion(t *testing.T) {
	got, err := migrationVersion("001_initial.sql")
	if err != nil {
		t.Fatalf("migrationVersion: %v", err)
	}
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestMigrationVersionRejectsInvalidName(t *testing.T) {
	if _, err := migrationVersion("initial.sql"); err == nil {
		t.Fatal("expected invalid name error")
	}
}
