// Package catalog aggregates the blank-import side effects that register
// every built-in connector with the connectors registry. Add a new
// connector by creating a subpackage under this one and adding a blank
// import here.
package catalog

import (
	_ "github.com/openrow/openrow/internal/connectors/catalog/fakturoid"
	_ "github.com/openrow/openrow/internal/connectors/catalog/revolut"
	_ "github.com/openrow/openrow/internal/connectors/catalog/stripe"
)
