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

	rows, err := db.Query(`
		select column_name, data_type
		from information_schema.columns
		where table_schema = 'public' and table_name = 'conversation_messages'
		order by ordinal_position`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		found = true
		var name string
		var dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			panic(err)
		}
		fmt.Printf("%s|%s\n", name, dataType)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	if !found {
		fmt.Println("__NO_ROWS__")
	}
}
