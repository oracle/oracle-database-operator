package commons

import (
	"testing"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
)

func TestResolveGsmServiceAnnotationsPrecedence(t *testing.T) {
	instance := &databasev4.ShardingDatabase{
		Spec: databasev4.ShardingDatabaseSpec{
			GsmServiceAnnotations: map[string]string{
				"mode":   "legacy",
				"legacy": "true",
			},
			GsmExternalServiceAnnotations: map[string]string{
				"scope":  "legacy",
				"legacy": "true",
			},
			GsmInfo: &databasev4.GsmInfo{
				ServiceAnnotations: map[string]string{
					"mode": "info",
					"info": "true",
				},
				ExternalServiceAnnotations: map[string]string{
					"scope": "info",
					"info":  "true",
				},
			},
		},
	}

	spec := databasev4.GsmSpec{
		ServiceAnnotations: map[string]string{
			"mode": "item",
			"item": "true",
		},
		ExternalServiceAnnotations: map[string]string{
			"scope": "item",
			"item":  "true",
		},
	}

	internal := resolveGsmServiceAnnotations(instance, spec, shardServiceTypeLocal)
	if internal["mode"] != "item" || internal["item"] != "true" || internal["info"] != "true" || internal["legacy"] != "true" {
		t.Fatalf("unexpected internal annotation merge result: %+v", internal)
	}

	external := resolveGsmServiceAnnotations(instance, spec, shardServiceTypeExternal)
	if external["scope"] != "item" || external["item"] != "true" || external["info"] != "true" || external["legacy"] != "true" {
		t.Fatalf("unexpected external annotation merge result: %+v", external)
	}
}
