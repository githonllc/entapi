// Command petstore is a runnable EntAPI example: four ent schemas in
// petstoreent/schema, one `go generate`, and a full CRUD + query + OpenAPI HTTP
// surface.
//
// Everything below the ent client is generated. Nothing in this file knows the
// entities' filters or routes.
package main

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	// modernc.org/sqlite is a cgo-free SQLite driver. It registers itself under
	// the database/sql driver name "sqlite", which is why the client is built
	// from an explicit *sql.DB below rather than from ent.Open — ent's
	// dialect.SQLite constant is the *dialect* name "sqlite3", not a driver name.
	_ "modernc.org/sqlite"

	"github.com/githonllc/entapi/examples/petstore/petstoreent"
)

func main() {
	// In-memory database: the example leaves nothing behind on disk.
	// For a real file, use "file:petstore.db?_fk=1" instead.
	db, err := stdsql.Open("sqlite", "file:petstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	client := petstoreent.NewClient(petstoreent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := seed(ctx, client); err != nil {
		log.Fatalf("seed: %v", err)
	}

	// API(client) returns the generated *APIHandler. It is an http.Handler in
	// its own right; Mount registers the same endpoints on a mux you own.
	api := petstoreent.API(client)

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

func seed(ctx context.Context, client *petstoreent.Client) error {
	dogs, err := client.Category.Create().SetName("Dogs").Save(ctx)
	if err != nil {
		return fmt.Errorf("create dogs category: %w", err)
	}
	cats, err := client.Category.Create().SetName("Cats").Save(ctx)
	if err != nil {
		return fmt.Errorf("create cats category: %w", err)
	}

	friendly, err := client.Tag.Create().SetName("friendly").Save(ctx)
	if err != nil {
		return fmt.Errorf("create friendly tag: %w", err)
	}
	young, err := client.Tag.Create().SetName("young").Save(ctx)
	if err != nil {
		return fmt.Errorf("create young tag: %w", err)
	}
	trained, err := client.Tag.Create().SetName("trained").Save(ctx)
	if err != nil {
		return fmt.Errorf("create trained tag: %w", err)
	}

	buddy, err := client.Pet.Create().
		SetName("Buddy").
		SetStatus("available").
		SetPrice(499).
		SetCategory(dogs).
		AddTags(friendly, trained).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create Buddy: %w", err)
	}
	luna, err := client.Pet.Create().
		SetName("Luna").
		SetStatus("pending").
		SetPrice(350).
		SetCategory(cats).
		AddTags(friendly, young).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create Luna: %w", err)
	}
	_, err = client.Pet.Create().
		SetName("Max").
		SetStatus("sold").
		SetPrice(275).
		SetCategory(dogs).
		AddTags(trained).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create Max: %w", err)
	}

	if _, err := client.Order.Create().
		SetQuantity(1).
		SetStatus("placed").
		SetShipDate(time.Now().Add(24 * time.Hour)).
		SetPet(buddy).
		Save(ctx); err != nil {
		return fmt.Errorf("create Buddy order: %w", err)
	}
	if _, err := client.Order.Create().
		SetQuantity(2).
		SetStatus("approved").
		SetShipDate(time.Now().Add(48 * time.Hour)).
		SetPet(luna).
		Save(ctx); err != nil {
		return fmt.Errorf("create Luna order: %w", err)
	}

	return nil
}
