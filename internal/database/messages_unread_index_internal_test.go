package database

import (
	"strings"
	"testing"

	"github.com/nikyjain/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The unread partial index is built CONCURRENTLY; it must run outside a
// transaction, be idempotent, and end up valid.
func TestMessagesUnreadIndexBuiltConcurrently(t *testing.T) {
	db := testutil.SetupTestDB(t)
	var stmts []string
	for _, stmt := range getIndexes() {
		if strings.Contains(stmt, "idx_messages_contact_unread") {
			stmts = append(stmts, stmt)
		}
	}
	require.Len(t, stmts, 2)
	assert.Contains(t, stmts[1], "CREATE INDEX CONCURRENTLY IF NOT EXISTS")

	for i := 0; i < 2; i++ {
		for _, stmt := range stmts {
			require.NoError(t, db.Exec(stmt).Error, stmt)
		}
	}

	var valid bool
	require.NoError(t, db.Raw(`SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = 'idx_messages_contact_unread'`).Scan(&valid).Error)
	assert.True(t, valid)
}
