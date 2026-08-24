package repository

import (
	"os"
	"strings"
	"testing"
)

func TestPointWriteRepositoryDoesNotOwnTransactions(t *testing.T) {
	raw, err := os.ReadFile("postgres_writes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "BeginTx") || strings.Contains(source, ".Commit()") {
		t.Fatal("point write repository must use caller-owned transactions")
	}
}
