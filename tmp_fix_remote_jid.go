//go:build ignore
// +build ignore

package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/fmx?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	_, err = db.Exec(`
DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'conversation_messages'
		  AND column_name = 'remote_j_id'
	) AND NOT EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'conversation_messages'
		  AND column_name = 'remote_jid'
	) THEN
		ALTER TABLE conversation_messages RENAME COLUMN remote_j_id TO remote_jid;
	END IF;
END $$;`)
	if err != nil {
		panic(err)
	}

	fmt.Println("schema_fix_ok")
}
