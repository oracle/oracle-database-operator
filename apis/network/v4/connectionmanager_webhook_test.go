package v4

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConnectionManagerDefaultAndValidation(t *testing.T) {
	obj := &ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman-sample", Namespace: "net"},
		Spec: ConnectionManagerSpec{
			Runtime: ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0"},
			RestAPI: &ConnectionManagerRestAPISpec{
				Enabled:             true,
				PasswordSecretRef:   &ConnectionManagerSecretKeyRef{Name: "cman-rest", Key: "password"},
				PrivateKeySecretRef: &ConnectionManagerSecretKeyRef{Name: "cman-rest", Key: "private-key"},
			},
		},
	}

	if err := obj.Default(context.Background(), obj); err != nil {
		t.Fatalf("Default returned error: %v", err)
	}
	if got := obj.Spec.LogLevel; got != "user" {
		t.Fatalf("expected logLevel user, got %q", got)
	}
	if got := obj.Spec.Runtime.Port; got != 1521 {
		t.Fatalf("expected CMAN port 1521 by default, got %d", got)
	}
	if got := obj.Spec.RestAPI.Port; got != 1525 {
		t.Fatalf("expected REST port 1525 by default, got %d", got)
	}
	if obj.Spec.Service.Internal.Enabled == nil || !*obj.Spec.Service.Internal.Enabled {
		t.Fatalf("expected internal service enabled by default")
	}
	if got := obj.Spec.RestAPI.Host; got != "cman-sample.net.svc.cluster.local" {
		t.Fatalf("expected REST host from internal service DNS, got %q", got)
	}
	if err := validateConnectionManager(obj); err != nil {
		t.Fatalf("expected ConnectionManager to validate, got %v", err)
	}
}

func TestConnectionManagerCustomScriptAndMultiFileConfigValidation(t *testing.T) {
	obj := &ConnectionManager{
		Spec: ConnectionManagerSpec{
			Runtime: ConnectionManagerRuntimeSpec{
				Image: "oracle/cman:23.8.0",
				StartupScript: &ConnectionManagerStartupScriptSpec{
					ConfigMapRef: &ConnectionManagerConfigMapKeyRef{Name: "cman-startup", Key: "launch-prod.sh"},
				},
			},
			ConfigSource: &ConnectionManagerConfigSourceSpec{
				ConfigMapRef: &ConnectionManagerConfigMapKeyRef{Name: "cman-config", Key: "custom-config.conf"},
				MountPath:    "/opt/oracle/cman-config",
			},
		},
	}
	if err := validateConnectionManager(obj); err != nil {
		t.Fatalf("expected custom script and multi-file config to validate, got %v", err)
	}

	obj.Spec.ConfigSource.ConfigMapRef.Key = ""
	if err := validateConnectionManager(obj); err != nil {
		t.Fatalf("expected multi-file config without a key to validate with a startup script, got %v", err)
	}

	obj.Spec.Runtime.StartupScript = nil
	if err := validateConnectionManager(obj); err == nil {
		t.Fatal("expected multi-file config without a key and startup script to be rejected")
	}

	obj.Spec.Runtime.StartupScript = &ConnectionManagerStartupScriptSpec{
		ConfigMapRef: &ConnectionManagerConfigMapKeyRef{Name: "cman-startup", Key: "launch-prod.sh"},
	}
	obj.Spec.ConfigSource.ConfigMapRef = nil
	if err := validateConnectionManager(obj); err == nil {
		t.Fatal("expected missing configMapRef to be rejected")
	}

	obj.Spec.ConfigSource.ConfigMapRef = &ConnectionManagerConfigMapKeyRef{Key: "custom-config.conf"}
	if err := validateConnectionManager(obj); err == nil {
		t.Fatal("expected missing configMapRef.name to be rejected")
	}

	obj.Spec.ConfigSource.ConfigMapRef = &ConnectionManagerConfigMapKeyRef{Name: "cman-config", Key: "custom-config.conf"}
	obj.Spec.ConfigSource.MountPath = "/opt/oracle/cman-scripts"
	if err := validateConnectionManager(obj); err == nil {
		t.Fatal("expected overlapping config source mount path to be rejected")
	}
}

func TestConnectionManagerStorageValidation(t *testing.T) {
	base := func(storage *ConnectionManagerStorageSpec) *ConnectionManager {
		return &ConnectionManager{Spec: ConnectionManagerSpec{
			Runtime: ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0"},
			Storage: storage,
		}}
	}

	tests := []struct {
		name    string
		storage *ConnectionManagerStorageSpec
		wantErr bool
	}{
		{
			name:    "existing pvc",
			storage: &ConnectionManagerStorageSpec{MountPath: "/var/lib/cman", PvcName: "precreated-cman-pvc"},
		},
		{
			name:    "managed pvc",
			storage: &ConnectionManagerStorageSpec{MountPath: "/var/lib/cman", StorageSizeInGb: 10, StorageClass: "oci-bv"},
		},
		{
			name:    "missing mount path",
			storage: &ConnectionManagerStorageSpec{PvcName: "precreated-cman-pvc"},
			wantErr: true,
		},
		{
			name:    "reserved config path",
			storage: &ConnectionManagerStorageSpec{MountPath: "/opt/oracle/cman-config/data", PvcName: "precreated-cman-pvc"},
			wantErr: true,
		},
		{
			name:    "pvc and storage class",
			storage: &ConnectionManagerStorageSpec{MountPath: "/var/lib/cman", PvcName: "precreated-cman-pvc", StorageClass: "oci-bv"},
			wantErr: true,
		},
		{
			name:    "pvc and size",
			storage: &ConnectionManagerStorageSpec{MountPath: "/var/lib/cman", PvcName: "precreated-cman-pvc", StorageSizeInGb: 10},
			wantErr: true,
		},
		{
			name:    "managed pvc without class",
			storage: &ConnectionManagerStorageSpec{MountPath: "/var/lib/cman", StorageSizeInGb: 10},
			wantErr: true,
		},
		{
			name:    "neither existing nor managed",
			storage: &ConnectionManagerStorageSpec{MountPath: "/var/lib/cman"},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateConnectionManager(base(test.storage))
			if test.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("expected validation to pass, got %v", err)
			}
		})
	}
}

func TestConnectionManagerPortValidation(t *testing.T) {
	valid := &ConnectionManager{Spec: ConnectionManagerSpec{
		Runtime: ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0", Port: 1621},
		RestAPI: &ConnectionManagerRestAPISpec{Port: 1625},
	}}
	if err := validateConnectionManager(valid); err != nil {
		t.Fatalf("expected configured ports to validate, got %v", err)
	}

	invalidRuntime := valid.DeepCopy()
	invalidRuntime.Spec.Runtime.Port = 65536
	if err := validateConnectionManager(invalidRuntime); err == nil {
		t.Fatal("expected runtime port above 65535 to be rejected")
	}

	invalidREST := valid.DeepCopy()
	invalidREST.Spec.RestAPI.Port = -1
	if err := validateConnectionManager(invalidREST); err == nil {
		t.Fatal("expected REST port below zero to be rejected")
	}
}

func TestConnectionManagerMaintenanceValidation(t *testing.T) {
	obj := &ConnectionManager{Spec: ConnectionManagerSpec{
		Runtime:     ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0"},
		Maintenance: &ConnectionManagerMaintenanceSpec{Enabled: true},
	}}
	if err := validateConnectionManager(obj); err != nil {
		t.Fatalf("expected maintenance mode to validate, got %v", err)
	}
}
