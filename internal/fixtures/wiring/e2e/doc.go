// Package e2e is the behavioural half of the "wiring" codegen fixture (#28).
//
// It is a separate Go module for exactly one reason: it needs a real SQL driver
// and the library must not. internal/fixtures is otherwise part of
// github.com/githonllc/entapi, so a driver imported from there would enter
// every consumer's dependency graph. The same split is why internal/fixture
// (singular) is its own module.
//
// It contains no ent code of its own. It imports the code
// TestCodegenFixtures generated into ../ent and drives it against SQLite,
// because compiling generated wiring proves it type-checks, not that it returns
// the right page.
//
// Run it with:
//
//	(cd internal/fixtures/wiring/e2e && go test ./...)
package e2e
