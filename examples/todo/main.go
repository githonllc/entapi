// Command todo is a runnable EntAPI example: one ent schema in todoent/schema,
// one `go generate`, and a full CRUD + query + OpenAPI HTTP surface.
//
// Everything below the ent client is generated. Nothing in this file knows the
// entity's fields, its filters or its routes.
package main

import (
	"context"
	stdsql "database/sql"
	"log"
	"net/http"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	// modernc.org/sqlite is a cgo-free SQLite driver. It registers itself under
	// the database/sql driver name "sqlite", which is why the client is built
	// from an explicit *sql.DB below rather than from ent.Open — ent's
	// dialect.SQLite constant is the *dialect* name "sqlite3", not a driver name.
	_ "modernc.org/sqlite"

	"github.com/githonllc/entapi/examples/todo/todoent"
)

func main() {
	// In-memory database: the example leaves nothing behind on disk.
	// For a real file, use "file:todo.db?_fk=1" instead.
	db, err := stdsql.Open("sqlite", "file:todo?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	client := todoent.NewClient(todoent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	defer func() { _ = client.Close() }()

	if err := client.Schema.Create(context.Background()); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// API(client) returns the generated *APIHandler. It is an http.Handler in
	// its own right; Mount registers the same endpoints on a mux you own.
	api := todoent.API(client)

	mux := http.NewServeMux()
	api.Mount(mux)

	for _, ep := range api.Endpoints() {
		log.Printf("%-6s %s", ep.Method, ep.Path)
	}
	log.Println("listening on http://localhost:8080")
	if err := http.ListenAndServe("localhost:8080", mux); err != nil {
		log.Fatal(err)
	}
}
