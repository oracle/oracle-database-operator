/*
Copyright (c) 2026, Oracle and/or its affiliates.
Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
*/

package controllers

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
)

func TestCreateOracleHostnameEnvVarObj_StandbyAndCloneUsePodIP(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	_ = dbapi.AddToScheme(scheme)

	localPrimary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-sample", Namespace: "default"},
		Status:     dbapi.SingleInstanceDatabaseStatus{ReleaseUpdate: "19.27.0.0.0"},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localPrimary).Build()

	cases := []struct {
		name     string
		sidb     *dbapi.SingleInstanceDatabase
		referred *dbapi.SingleInstanceDatabase
	}{
		{
			name: "external connectString IP + 128786 image uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby2", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "standby",
					PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
						ConnectString: "10.0.2.147:1521/ORCLPRD",
					},
					Image: dbapi.SingleInstanceDatabaseImage{
						PullFrom: "example.com/oracle/database:128786",
					},
				},
			},
		},
		{
			name: "external connectString IP with empty referred stub uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby2", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "standby",
					PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
						ConnectString: "10.0.2.147:1521/ORCLPRD",
					},
					Image: dbapi.SingleInstanceDatabaseImage{
						PullFrom: "example.com/oracle/database:128786",
					},
				},
			},
			referred: &dbapi.SingleInstanceDatabase{},
		},
		{
			name: "external details host + 19 version uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby3", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "standby",
					PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
						Details: &dbapi.SingleInstanceDatabasePrimaryDetails{
							Host: "10.0.2.147",
							Port: 1521,
							Sid:  "ORCLPRD",
						},
					},
					Image: dbapi.SingleInstanceDatabaseImage{
						Version:  "19.3.0",
						PullFrom: "example.com/oracle/database:19.3.0-ee",
					},
				},
			},
		},
		{
			name: "external connectString IP + 23ai/26ai image uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby2", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "standby",
					PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
						ConnectString: "10.0.2.147:1521/ORCLPRD",
					},
					Image: dbapi.SingleInstanceDatabaseImage{
						Version:  "23.26.0",
						PullFrom: "example.com/oracle/database-singleinstance:23.26.3-dgtcps",
					},
				},
			},
		},
		{
			name: "same-ns service DNS connectString + local 19c primary uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby1", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "standby",
					PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
						ConnectString: "sidb-sample.default.svc.cluster.local:1521/ORCLPRD",
					},
					Image: dbapi.SingleInstanceDatabaseImage{
						PullFrom: "example.com/oracle/database:128786",
					},
				},
			},
		},
		{
			name: "referred 19c primary uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby1", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "standby",
					Image: dbapi.SingleInstanceDatabaseImage{
						PullFrom: "example.com/oracle/database:128786",
					},
				},
			},
			referred: localPrimary,
		},
		{
			name: "referred 23ai/26ai primary uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby1", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "standby",
					Image: dbapi.SingleInstanceDatabaseImage{
						PullFrom: "container-registry.oracle.com/database/enterprise_ru:latest-26",
					},
				},
			},
			referred: &dbapi.SingleInstanceDatabase{
				Status: dbapi.SingleInstanceDatabaseStatus{ReleaseUpdate: "23.26.4.0.0"},
			},
		},
		{
			name: "clone uses podIP",
			sidb: &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-clone1", Namespace: "default"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: "clone",
					Image: dbapi.SingleInstanceDatabaseImage{
						PullFrom: "container-registry.oracle.com/database/enterprise_ru:latest-26",
					},
				},
			},
			referred: &dbapi.SingleInstanceDatabase{
				Status: dbapi.SingleInstanceDatabaseStatus{ReleaseUpdate: "23.26.4.0.0"},
			},
		},
		{
			name: "nil sidb returns empty ORACLE_HOSTNAME",
			sidb: nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CreateOracleHostnameEnvVarObj(tc.sidb, tc.referred, context.Background(), reader)
			if got.Name != "ORACLE_HOSTNAME" {
				t.Fatalf("unexpected env name %q", got.Name)
			}
			if tc.sidb == nil {
				if got.Value != "" || got.ValueFrom != nil {
					t.Fatalf("expected empty ORACLE_HOSTNAME for nil sidb, got %#v", got)
				}
				return
			}
			if got.ValueFrom == nil || got.ValueFrom.FieldRef == nil || got.ValueFrom.FieldRef.FieldPath != "status.podIP" {
				t.Fatalf("expected status.podIP ValueFrom, got %#v", got)
			}
		})
	}
}

func TestSidbControllerImageLooksLike19c(t *testing.T) {
	t.Parallel()
	cases := []struct {
		image dbapi.SingleInstanceDatabaseImage
		want  bool
	}{
		{image: dbapi.SingleInstanceDatabaseImage{PullFrom: "x:128786"}, want: true},
		{image: dbapi.SingleInstanceDatabaseImage{PullFrom: "x:128786-dgtde"}, want: true},
		{image: dbapi.SingleInstanceDatabaseImage{Version: "19.3.0", PullFrom: "x:latest"}, want: true},
		{image: dbapi.SingleInstanceDatabaseImage{Version: "23.26.0", PullFrom: "x:128786"}, want: false},
		{image: dbapi.SingleInstanceDatabaseImage{PullFrom: "x:23.26.3-dgtcps"}, want: false},
	}
	for _, tc := range cases {
		if got := sidbControllerImageLooksLike19c(tc.image); got != tc.want {
			t.Fatalf("image=%+v got=%v want=%v", tc.image, got, tc.want)
		}
	}
}
