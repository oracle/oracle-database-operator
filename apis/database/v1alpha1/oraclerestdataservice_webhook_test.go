/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package v1alpha1

import (
	"context"
	"strings"
	"testing"
)

func TestOracleRestDataServiceValidateCreateWarnsResourceDeprecated(t *testing.T) {
	t.Setenv("WATCH_NAMESPACE", "")
	ords := &OracleRestDataService{}
	ords.Name = "ords-sample"
	ords.Namespace = "default"
	ords.Spec.DatabaseRef = "sidb-sample"

	warnings, err := (&OracleRestDataService{}).ValidateCreate(context.Background(), ords)
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "OracleRestDataService is deprecated; use the OrdsSrvs controller instead") {
			return
		}
	}
	t.Fatalf("expected deprecation warning in %v", warnings)
}
