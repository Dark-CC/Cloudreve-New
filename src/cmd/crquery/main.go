package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 {
		panic("usage: crquery <db> <sql>")
	}
	db, err := sql.Open("sqlite", os.Args[1])
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query(os.Args[2])
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			panic(err)
		}
		out := map[string]any{}
		for i, col := range cols {
			switch v := values[i].(type) {
			case []byte:
				out[col] = string(v)
			default:
				out[col] = v
			}
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
	}
}
