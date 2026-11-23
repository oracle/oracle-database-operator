/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package commons

import _ "embed"

// AgentFactoryDatabaseSetupScript is temporarily mounted into the bootstrap
// Job. Keep credentials out of this file; they are supplied through mounted
// Secret files at runtime.
//
//go:embed paf-db-setup
var AgentFactoryDatabaseSetupScript string
