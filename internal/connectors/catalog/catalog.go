// Package catalog aggregates the blank-import side effects that register
// every built-in connector with the connectors registry. Add a new
// connector by creating a subpackage under this one and adding a blank
// import here.
package catalog

import (
	_ "github.com/openrow/openrow/internal/connectors/catalog/ares"
	_ "github.com/openrow/openrow/internal/connectors/catalog/cnb"
	_ "github.com/openrow/openrow/internal/connectors/catalog/discord"
	_ "github.com/openrow/openrow/internal/connectors/catalog/fakturoid"
	_ "github.com/openrow/openrow/internal/connectors/catalog/fio"
	_ "github.com/openrow/openrow/internal/connectors/catalog/github"
	_ "github.com/openrow/openrow/internal/connectors/catalog/linear"
	_ "github.com/openrow/openrow/internal/connectors/catalog/notion"
	_ "github.com/openrow/openrow/internal/connectors/catalog/resend"
	_ "github.com/openrow/openrow/internal/connectors/catalog/revolut"
	_ "github.com/openrow/openrow/internal/connectors/catalog/slack"
	_ "github.com/openrow/openrow/internal/connectors/catalog/stripe"
	_ "github.com/openrow/openrow/internal/connectors/catalog/vies"
)
