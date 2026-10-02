package maintenance

import (
	"context"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func TestResultsRecheckHiddenStateAfterScan(t *testing.T) {
	db := cleanerDB(t)
	if _, err := db.Exec("INSERT INTO notes VALUES(1,'',0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO note_attachments VALUES(1,1,'secret',3,'hash',NULL)"); err != nil {
		t.Fatal(err)
	}
	m := &Module{d: &module.Deps{DB: db, Registry: module.NewRegistry()}, snapshot: map[string][]contracts.CleanupItem{"notes": {{ID: "1", Kind: "note_attachments", Module: "notes", Name: "secret", RecordTable: "note_attachments", RecordID: 1}}}}
	if result := m.job(context.Background(), false); result.Total != 1 {
		t.Fatalf("visible %+v", result)
	}
	if _, err := db.Exec("UPDATE notes SET hidden=1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if result := m.job(context.Background(), false); result.Total != 0 || len(result.Groups) != 0 {
		t.Fatalf("hidden leaked %+v", result)
	}
}
