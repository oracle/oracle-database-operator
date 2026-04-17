//nolint:staticcheck // unit tests intentionally assert legacy requeue behavior.
package controllers

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSIDBUnit_GetPrimaryDatabaseConnectStringPrefersPrimarySource(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimaryDatabaseRef: "primary-db",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "custom-host:1521/CDB1",
			},
		},
	}
	got := GetPrimaryDatabaseConnectString(sidb, &dbapi.SingleInstanceDatabase{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}, Spec: dbapi.SingleInstanceDatabaseSpec{Sid: "IGN"}})
	if got != "custom-host:1521/CDB1" {
		t.Fatalf("expected primarySource connect string, got %q", got)
	}
}

func TestSIDBUnit_GetPrimaryDatabaseConnectStringFromPrimaryDetails(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				Details: &dbapi.SingleInstanceDatabasePrimaryDetails{
					Host: "external-primary",
					Port: 1522,
					Sid:  "PRIM",
				},
			},
		},
	}
	got := GetPrimaryDatabaseConnectString(sidb, nil)
	if got != "external-primary:1522/PRIM" {
		t.Fatalf("expected connect string from primaryDetails, got %q", got)
	}
}

func TestSIDBUnit_GetPrimaryDatabaseInfoFromConnectString(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "primary-host:1522/primdb",
			},
		},
	}

	if got := GetPrimaryDatabaseHost(sidb, nil); got != "primary-host" {
		t.Fatalf("expected primary host from connect string, got %q", got)
	}
	if got := GetPrimaryDatabasePort(sidb); got != 1522 {
		t.Fatalf("expected primary port from connect string, got %d", got)
	}
	if got := GetPrimaryDatabaseSid(sidb, nil); got != "PRIMDB" {
		t.Fatalf("expected primary sid from connect string, got %q", got)
	}
	if got := GetPrimaryDatabaseDisplayName(sidb, nil); got != "primary-host" {
		t.Fatalf("expected primary display name from connect string, got %q", got)
	}
}

func TestSIDBUnit_IsLocalPrimaryDatabaseSource(t *testing.T) {
	local := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			},
		},
	}
	if !isLocalPrimaryDatabaseSource(local) {
		t.Fatalf("expected databaseRef source to be treated as local")
	}

	connectString := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "primary-host:1521/PRIM",
			},
		},
	}
	if isLocalPrimaryDatabaseSource(connectString) {
		t.Fatalf("expected connect string source to be treated as external")
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusForStandby(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			Image: dbapi.SingleInstanceDatabaseImage{PullFrom: "oracle/db:19.3.0", PullSecrets: "pull-secret"},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			},
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIM",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "primary-admin",
			},
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, primary)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseReady {
		t.Fatalf("expected preview phase %q, got %q", dataguardPreviewPhaseReady, sidb.Status.Dataguard.Phase)
	}
	if !sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be true")
	}
	if sidb.Status.Dataguard.PrimaryMemberName != "primary-db" {
		t.Fatalf("expected primary member name primary-db, got %q", sidb.Status.Dataguard.PrimaryMemberName)
	}
	if sidb.Status.Dataguard.MemberName != "sidb-standby" {
		t.Fatalf("expected member name sidb-standby, got %q", sidb.Status.Dataguard.MemberName)
	}
	if sidb.Status.Dataguard.Role != "PHYSICAL_STANDBY" {
		t.Fatalf("expected standby member role, got %q", sidb.Status.Dataguard.Role)
	}
	if sidb.Status.Dataguard.PublishedTopologyHash == "" {
		t.Fatalf("expected published topology hash to be set")
	}
	if sidb.Status.Dataguard.TopologyHash == "" {
		t.Fatalf("expected topology hash to be set")
	}
	if sidb.Status.Dataguard.LastPublishedTime == nil {
		t.Fatalf("expected lastPublishedTime to be set")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec == nil {
		t.Fatalf("expected renderedBrokerSpec to be published")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Name != "sidb-standby-dg" {
		t.Fatalf("unexpected rendered broker name: %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Namespace != "ns1" {
		t.Fatalf("unexpected rendered broker namespace: %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Spec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology == nil {
		t.Fatalf("expected rendered broker spec topology, got %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Execution == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Execution.Image == "" {
		t.Fatalf("expected rendered broker execution image to be published")
	}
	if !sidb.Status.Dataguard.RenderedBrokerSpec.Ready {
		t.Fatalf("expected rendered broker spec to be marked ready")
	}
	gotMembers := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members
	if len(gotMembers) != 2 {
		t.Fatalf("expected two rendered broker members, got %#v", gotMembers)
	}
	defaults := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Defaults
	if defaults == nil || defaults.AdminSecretRef == nil {
		t.Fatalf("expected topology defaults adminSecretRef to be published")
	}
	if defaults.AdminSecretRef.SecretName != "standby-admin" || defaults.AdminSecretRef.SecretKey != "oracle_pwd" {
		t.Fatalf("unexpected topology defaults adminSecretRef %#v", defaults.AdminSecretRef)
	}
	for _, member := range gotMembers {
		switch member.Role {
		case "PRIMARY":
			if member.AdminSecretRef == nil || member.AdminSecretRef.SecretName != "primary-admin" || member.AdminSecretRef.SecretKey != "oracle_pwd" {
				t.Fatalf("expected primary override adminSecretRef to be published for member %#v", member)
			}
		case "PHYSICAL_STANDBY":
			if member.AdminSecretRef != nil {
				t.Fatalf("expected standby member to inherit adminSecretRef from topology defaults, got %#v", member)
			}
		}
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusExternalPrimaryPublishesReadyPreviewWithPlaceholder(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			Image: dbapi.SingleInstanceDatabaseImage{PullFrom: "oracle/db:19.3.0"},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "external-primary:1521/PRIM",
			},
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseReady {
		t.Fatalf("expected phase %q, got %q", dataguardPreviewPhaseReady, sidb.Status.Dataguard.Phase)
	}
	if !sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be true when placeholder values are published")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology == nil {
		t.Fatalf("expected rendered broker spec topology to be published")
	}
	if !sidb.Status.Dataguard.RenderedBrokerSpec.Ready {
		t.Fatalf("expected rendered broker spec to be marked ready")
	}
	condition := meta.FindStatusCondition(sidb.Status.Dataguard.Conditions, "TopologyPreviewReady")
	if condition == nil {
		t.Fatalf("expected TopologyPreviewReady condition to be set")
	}
	if condition.Reason != "PreviewReady" {
		t.Fatalf("expected PreviewReady condition reason, got %#v", condition)
	}
	if condition.Status != metav1.ConditionTrue {
		t.Fatalf("expected TopologyPreviewReady condition status true, got %#v", condition)
	}
	if !strings.Contains(condition.Message, "topology.defaults.adminSecretRef") {
		t.Fatalf("expected condition message to explain topology default admin secret usage, got %#v", condition)
	}
	members := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members
	if len(members) != 2 {
		t.Fatalf("expected two topology members, got %#v", members)
	}
	var externalPrimary *dbapi.DataguardTopologyMember
	for i := range members {
		if members[i].Role == "PRIMARY" {
			externalPrimary = &members[i]
			break
		}
	}
	if externalPrimary == nil {
		t.Fatalf("expected primary member in rendered topology, got %#v", members)
	}
	if externalPrimary.AdminSecretRef != nil {
		t.Fatalf("expected external primary member to inherit adminSecretRef from topology defaults, got %#v", externalPrimary.AdminSecretRef)
	}
	defaults := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Defaults
	if defaults == nil || defaults.AdminSecretRef == nil {
		t.Fatalf("expected topology defaults adminSecretRef to be published")
	}
	if defaults.AdminSecretRef.SecretName != "standby-admin" || defaults.AdminSecretRef.SecretKey != "oracle_pwd" {
		t.Fatalf("unexpected topology defaults adminSecretRef %#v", defaults.AdminSecretRef)
	}
	if externalPrimary.TCPS != nil {
		t.Fatalf("did not expect primary tcps block when standby tcps is disabled, got %#v", externalPrimary.TCPS)
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusExternalPrimaryInfersTCPSPlaceholders(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			Image: dbapi.SingleInstanceDatabaseImage{PullFrom: "oracle/db:19.3.0"},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled: true,
				},
			},
			TcpsListenerPort: 2484,
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "external-primary:1521/PRIM",
			},
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseReady {
		t.Fatalf("expected phase %q, got %q", dataguardPreviewPhaseReady, sidb.Status.Dataguard.Phase)
	}
	if !sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be true")
	}
	condition := meta.FindStatusCondition(sidb.Status.Dataguard.Conditions, "TopologyPreviewReady")
	if condition == nil {
		t.Fatalf("expected TopologyPreviewReady condition to be set")
	}
	if !strings.Contains(condition.Message, "topology.defaults.tcps.clientWalletSecret") {
		t.Fatalf("expected condition message to mention topology tcps defaults, got %#v", condition)
	}
	members := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members
	var externalPrimary *dbapi.DataguardTopologyMember
	for i := range members {
		if members[i].Role == "PRIMARY" {
			externalPrimary = &members[i]
			break
		}
	}
	if externalPrimary == nil {
		t.Fatalf("expected primary member in rendered topology, got %#v", members)
	}
	if externalPrimary.TCPS == nil || !externalPrimary.TCPS.Enabled {
		t.Fatalf("expected inferred primary tcps block, got %#v", externalPrimary.TCPS)
	}
	if externalPrimary.TCPS.ClientWalletSecret != "" {
		t.Fatalf("expected external primary to inherit tcps wallet secret from topology defaults, got %#v", externalPrimary.TCPS)
	}
	defaults := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Defaults
	if defaults == nil || defaults.TCPS == nil {
		t.Fatalf("expected topology tcps defaults to be published")
	}
	if defaults.TCPS.ClientWalletSecret != dataguardPreviewSharedWalletPlaceholder {
		t.Fatalf("unexpected topology tcps wallet default %#v", defaults.TCPS)
	}
	var tcpsEndpoint *dbapi.DataguardEndpointSpec
	for i := range externalPrimary.Endpoints {
		if externalPrimary.Endpoints[i].Protocol == "TCPS" {
			tcpsEndpoint = &externalPrimary.Endpoints[i]
			break
		}
	}
	if tcpsEndpoint == nil {
		t.Fatalf("expected primary tcps endpoint in rendered topology, got %#v", externalPrimary.Endpoints)
	}
	if tcpsEndpoint.Port != 2484 || tcpsEndpoint.ServiceName != "PRIM" {
		t.Fatalf("unexpected inferred primary tcps endpoint: %#v", tcpsEndpoint)
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusDefaultsRenderedExecutionAuthWalletWithoutImage(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "external-primary:1521/PRIM",
			},
			Dataguard: &dbapi.DataguardProducerSpec{},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil || sidb.Status.Dataguard.RenderedBrokerSpec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec == nil {
		t.Fatalf("expected rendered broker spec to be published, got %#v", sidb.Status.Dataguard)
	}
	execution := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Execution
	if execution == nil {
		t.Fatalf("expected rendered broker execution to be published even without image")
	}
	if execution.Image != "" {
		t.Fatalf("expected no execution image, got %q", execution.Image)
	}
	if execution.AuthWallet == nil || !execution.AuthWallet.Enabled {
		t.Fatalf("expected default auth wallet to be enabled, got %#v", execution.AuthWallet)
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusDisabledClearsStatus(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			Dataguard: &dbapi.DataguardProducerSpec{
				Mode: dbapi.DataguardProducerModeDisabled,
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			Dataguard: &dbapi.ProducerDataguardStatus{Phase: dataguardPreviewPhaseReady},
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard != nil {
		t.Fatalf("expected dataguard status to be cleared when mode is disabled")
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusTrueCacheIsNotApplicable(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-truecache", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:       "TC01",
			CreateAs:  "truecache",
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard status to be populated for not-applicable truecache")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseNotApplicable {
		t.Fatalf("expected phase %q, got %q", dataguardPreviewPhaseNotApplicable, sidb.Status.Dataguard.Phase)
	}
	if sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be false for truecache")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec != nil {
		t.Fatalf("expected no rendered broker spec for truecache, got %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
}

func TestSIDBUnit_BuildSIDBPreviewTCPSConfigUsesOverrideWalletSecret(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBY",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled:            true,
					TlsSecret:          "server-tls",
					ClientWalletSecret: "custom-client-wallet",
				},
			},
		},
	}

	tcps := buildSIDBPreviewTCPSConfig(sidb)
	if tcps == nil {
		t.Fatalf("expected TCPS config")
	}
	if tcps.ClientWalletSecret != "custom-client-wallet" {
		t.Fatalf("expected custom client wallet secret, got %q", tcps.ClientWalletSecret)
	}
}

func TestSIDBUnit_BuildSIDBPreviewTCPSConfigUsesGeneratedWalletSecretWhenEnabled(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBY",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled:   true,
					TlsSecret: "server-tls",
				},
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			IsTcpsEnabled:   true,
			ClientWalletLoc: "/opt/oracle/oradata/clientWallet/STBY",
		},
	}

	tcps := buildSIDBPreviewTCPSConfig(sidb)
	if tcps == nil {
		t.Fatalf("expected TCPS config")
	}
	if tcps.ClientWalletSecret != "sidb-standby-dg-client-wallet" {
		t.Fatalf("expected generated client wallet secret, got %q", tcps.ClientWalletSecret)
	}
}

func TestSIDBUnit_BuildAutomaticPrimaryTNSAliases(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			Sid:      "STBYDB",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled: true,
				},
			},
		},
	}
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "primary-db"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
		},
	}

	aliases, names := buildAutomaticStandbyPeerTNSAliases(sidb, primary)
	expectedNames := []string{"ORCLCDB", "ORCLCDBTCPS", "ORCLCDBTCPS_DGMGRL", "ORCLCDB_DGMGRL", "STBYDB", "STBYDBTCPS", "STBYDBTCPS_DGMGRL", "STBYDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected generated alias names: got %v want %v", names, expectedNames)
	}

	if got := aliases["STBYDB"]; got.Host != "sidb-standby" || got.Port != 1521 || got.ServiceName != "STBYDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected base standby self alias: %#v", got)
	}
	if got := aliases["STBYDB_DGMGRL"]; got.Host != "sidb-standby" || got.Port != 1521 || got.ServiceName != "STBYDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected standby self dgmgrl alias: %#v", got)
	}
	if got := aliases["STBYDBTCPS"]; got.Host != "sidb-standby" || got.Port != 2484 || got.ServiceName != "STBYDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected standby self tcps alias: %#v", got)
	}
	if got := aliases["STBYDBTCPS_DGMGRL"]; got.Host != "sidb-standby" || got.Port != 2484 || got.ServiceName != "STBYDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected standby self tcps dgmgrl alias: %#v", got)
	}
	if got := aliases["ORCLCDB"]; got.Host != "primary-db" || got.Port != 1521 || got.ServiceName != "ORCLCDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected base primary alias: %#v", got)
	}
	if got := aliases["ORCLCDB_DGMGRL"]; got.Host != "primary-db" || got.Port != 1521 || got.ServiceName != "ORCLCDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected dgmgrl alias: %#v", got)
	}
	if got := aliases["ORCLCDBTCPS"]; got.Host != "primary-db" || got.Port != 2484 || got.ServiceName != "ORCLCDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected tcps alias: %#v", got)
	}
	if got := aliases["ORCLCDBTCPS_DGMGRL"]; got.Host != "primary-db" || got.Port != 2484 || got.ServiceName != "ORCLCDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected tcps dgmgrl alias: %#v", got)
	}
}

func TestSIDBUnit_BuildManagedTNSAliasesAppliesOverridesAndAppendsExtras(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				Details: &dbapi.SingleInstanceDatabasePrimaryDetails{
					Host: "primary-host",
					Sid:  "PRIMDB",
				},
			},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled: true,
				},
			},
			TNSAliases: []dbapi.SingleInstanceDatabaseTNSAlias{
				{
					Name:        "PRIMDB",
					Host:        "override-host",
					Port:        1525,
					ServiceName: "override_svc",
				},
				{
					Name:        "PRIMDBTCPS",
					Host:        "secure-host",
					ServiceName: "primdb",
					SSLServerDN: "CN=primary",
				},
				{
					Name:        "DATAGUARD",
					Host:        "dg-host",
					ServiceName: "DATAGUARD",
				},
			},
		},
	}

	aliases, names := buildStandbyManagedTNSAliases(sidb, nil)
	expectedNames := []string{"DATAGUARD", "PRIMDB", "PRIMDBTCPS"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected managed alias names: got %v want %v", names, expectedNames)
	}

	if got := aliases["PRIMDB"]; got.Host != "primary-host" || got.Port != 1521 || got.ServiceName != "PRIMDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected protected PRIMDB alias: %#v", got)
	}
	if got := aliases["PRIMDBTCPS"]; got.Host != "primary-host" || got.Port != 2484 || got.ServiceName != "PRIMDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS || got.SSLServerDN != "" {
		t.Fatalf("unexpected protected PRIMDBTCPS alias: %#v", got)
	}
	if _, exists := aliases["PRIMDB_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDB_DGMGRL alias for truecache")
	}
	if _, exists := aliases["PRIMDBTCPS_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDBTCPS_DGMGRL alias for truecache")
	}
	if got := aliases["DATAGUARD"]; got.Host != "dg-host" || got.ServiceName != "DATAGUARD" {
		t.Fatalf("unexpected appended DATAGUARD alias: %#v", got)
	}
}

func TestSIDBUnit_BuildLegacySingleStandbyPrimaryPeerTNSAliasesAppliesOverridesWithoutAppendingExtras(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{
					Enabled: true,
				},
			},
			TNSAliases: []dbapi.SingleInstanceDatabaseTNSAlias{
				{
					Name:        "STBYDB",
					Host:        "override-standby",
					Port:        1525,
					ServiceName: "stby_service",
					Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS,
					SSLServerDN: "CN=standby",
				},
				{
					Name:        "EXTRA_ALIAS",
					Host:        "extra-host",
					ServiceName: "EXTRA_SERVICE",
				},
			},
		},
	}
	standby := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBYDB",
		},
	}

	aliases, names := buildLegacySingleStandbyPrimaryPeerTNSAliases(primary, standby)
	expectedNames := []string{"PRIMDB", "PRIMDB_DGMGRL", "STBYDB", "STBYDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected primary peer alias names: got %v want %v", names, expectedNames)
	}
	if got := aliases["PRIMDB"]; got.Host != "sidb-primary" ||
		got.Port != 1521 ||
		got.ServiceName != "PRIMDB" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected primary self alias: %#v", got)
	}
	got := aliases["STBYDB"]
	if got.Host != "sidb-standby" ||
		got.Port != 1521 ||
		got.ServiceName != "STBYDB" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected standby peer alias: %#v", got)
	}
	if got := aliases["STBYDB_DGMGRL"]; got.Host != "sidb-standby" ||
		got.Port != 1521 ||
		got.ServiceName != "STBYDB_DGMGRL" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected standby peer dgmgrl alias: %#v", got)
	}
	if got := aliases["PRIMDB_DGMGRL"]; got.Host != "sidb-primary" ||
		got.Port != 1521 ||
		got.ServiceName != "PRIMDB_DGMGRL" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected primary self dgmgrl alias: %#v", got)
	}
	if _, exists := aliases["EXTRA_ALIAS"]; exists {
		t.Fatalf("did not expect extra alias to be appended on primary peer path")
	}
}

func TestSIDBUnit_BuildLegacySingleStandbyPrimaryPeerTNSAliasesSkipsDGMGRLWhenPrimaryPrereqsDisabled(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
		},
	}
	standby := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBYDB",
		},
	}

	aliases, names := buildLegacySingleStandbyPrimaryPeerTNSAliases(primary, standby)
	expectedNames := []string{"PRIMDB", "STBYDB"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected primary peer alias names when prereqs disabled: got %v want %v", names, expectedNames)
	}
	if _, exists := aliases["PRIMDB_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDB_DGMGRL alias when primary dataguard prereqs are disabled")
	}
	if _, exists := aliases["STBYDB_DGMGRL"]; exists {
		t.Fatalf("did not expect STBYDB_DGMGRL alias when primary dataguard prereqs are disabled")
	}
}

func TestSIDBUnit_BuildPrimaryPeerTNSAliasesForConfiguredStandbySources(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{Enabled: true},
				StandbySources: []dbapi.DataguardStandbySourceSpec{
					{
						DBUniqueName: "STBYA",
						Host:         "stbya.example",
						TCPSEnabled:  true,
						TCPPort:      1621,
					},
					{
						DBUniqueName: "STBYB",
						Host:         "stbyb.example",
					},
				},
			},
			TNSAliases: []dbapi.SingleInstanceDatabaseTNSAlias{
				{
					Name:        "STBYA",
					Host:        "ignored-host",
					Port:        9999,
					ServiceName: "IGNORED",
				},
			},
		},
	}

	aliases, names := buildPrimaryPeerTNSAliasesForTargets(primary, configuredSIDBPrimaryPeerAliasTargets(primary), true)
	expectedNames := []string{
		"PRIMDB",
		"PRIMDBTCPS",
		"PRIMDBTCPS_DGMGRL",
		"PRIMDB_DGMGRL",
		"STBYA",
		"STBYATCPS",
		"STBYATCPS_DGMGRL",
		"STBYA_DGMGRL",
		"STBYB",
		"STBYB_DGMGRL",
	}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected configured primary peer alias names: got %v want %v", names, expectedNames)
	}
	if got := aliases["STBYA"]; got.Host != "stbya.example" || got.Port != 1621 || got.ServiceName != "STBYA" {
		t.Fatalf("unexpected STBYA alias: %#v", got)
	}
	if got := aliases["STBYATCPS"]; got.Host != "stbya.example" || got.Port != 2484 || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected STBYATCPS alias: %#v", got)
	}
	if got := aliases["STBYB"]; got.Host != "stbyb.example" || got.Port != 1521 || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected STBYB alias: %#v", got)
	}
	if _, exists := aliases["STBYBTCPS"]; exists {
		t.Fatalf("did not expect STBYBTCPS alias when configured standby does not enable TCPS")
	}
}

func TestSIDBUnit_SIDBPrimaryUsesExplicitStandbySources(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Dataguard: &dbapi.DataguardProducerSpec{
				StandbySources: []dbapi.DataguardStandbySourceSpec{{DBUniqueName: "STBYDB", Host: "sidb-standby"}},
			},
		},
	}
	if !sidbPrimaryUsesExplicitStandbySources(primary) {
		t.Fatalf("expected primary with standbySources to use explicit standby source management")
	}

	standby := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			Dataguard: &dbapi.DataguardProducerSpec{
				StandbySources: []dbapi.DataguardStandbySourceSpec{{DBUniqueName: "STBYDB", Host: "sidb-standby"}},
			},
		},
	}
	if sidbPrimaryUsesExplicitStandbySources(standby) {
		t.Fatalf("did not expect non-primary SIDB to use explicit standby source management")
	}
}

func TestSIDBUnit_ResolvePrimaryPeerTNSAliasesUsesConfiguredStandbySources(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Sid:      "PRIMDB",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{Enabled: true},
				StandbySources: []dbapi.DataguardStandbySourceSpec{
					{DBUniqueName: "STBYDB", Host: "sidb-standby.shns.svc.cluster.local", TCPSEnabled: true},
				},
			},
		},
	}

	aliases, names, source, err := resolvePrimaryPeerTNSAliases(&SingleInstanceDatabaseReconciler{}, primary, nil, context.Background())
	if err != nil {
		t.Fatalf("resolvePrimaryPeerTNSAliases() error = %v", err)
	}
	if source != "spec.dataguard.standbySources" {
		t.Fatalf("unexpected alias source: got %q", source)
	}
	expectedNames := []string{"PRIMDB", "PRIMDBTCPS", "PRIMDBTCPS_DGMGRL", "PRIMDB_DGMGRL", "STBYDB", "STBYDBTCPS", "STBYDBTCPS_DGMGRL", "STBYDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected alias names: got %v want %v", names, expectedNames)
	}
	if got := aliases["STBYDBTCPS"]; got.Host != "sidb-standby.shns.svc.cluster.local" || got.Port != 2484 {
		t.Fatalf("unexpected STBYDBTCPS alias: %#v", got)
	}
}

func TestSIDBUnit_DiscoverLegacyPrimaryPeerAliasTargetsIncludesAllStandbysForPrimary(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
		},
	}
	standbyOne := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby-a", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBYA",
			CreateAs: "standby",
			Security: &dbapi.SingleInstanceDatabaseSecurity{TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true}},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "sidb-primary",
			},
		},
	}
	standbyTwo := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby-b", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:                "STBYB",
			CreateAs:           "standby",
			PrimaryDatabaseRef: "sidb-primary",
		},
	}
	unrelated := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-other", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "OTHER",
			CreateAs: "primary",
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(primary, standbyOne, standbyTwo, unrelated).Build(),
	}

	targets, err := discoverLegacyPrimaryPeerAliasTargets(reconciler, primary, nil, context.Background())
	if err != nil {
		t.Fatalf("discoverLegacyPrimaryPeerAliasTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 standby targets, got %d: %#v", len(targets), targets)
	}
	if targets[0].DBUniqueName != "STBYA" || targets[0].Host != "sidb-standby-a" || !targets[0].TCPSEnabled {
		t.Fatalf("unexpected first discovered standby target: %#v", targets[0])
	}
	if targets[1].DBUniqueName != "STBYB" || targets[1].Host != "sidb-standby-b" || targets[1].TCPSEnabled {
		t.Fatalf("unexpected second discovered standby target: %#v", targets[1])
	}
}

func TestSIDBUnit_ComputeTNSAliasesHashChangesWhenAliasSetChanges(t *testing.T) {
	baseAliases := map[string]dbapi.SingleInstanceDatabaseTNSAlias{
		"PRIMDB": {
			Name:        "PRIMDB",
			Host:        "sidb-primary",
			Port:        1521,
			ServiceName: "PRIMDB",
			Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP,
		},
		"STBYDB": {
			Name:        "STBYDB",
			Host:        "sidb-standby",
			Port:        1521,
			ServiceName: "STBYDB",
			Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP,
		},
	}
	baseNames := []string{"PRIMDB", "STBYDB"}

	hash1 := computeTNSAliasesHash(baseAliases, baseNames)
	hash2 := computeTNSAliasesHash(baseAliases, baseNames)
	if hash1 == "" || hash1 != hash2 {
		t.Fatalf("expected stable non-empty alias hash, got %q and %q", hash1, hash2)
	}

	changedAliases := map[string]dbapi.SingleInstanceDatabaseTNSAlias{
		"PRIMDB": baseAliases["PRIMDB"],
		"STBYDB": {
			Name:        "STBYDB",
			Host:        "sidb-standby.shns.svc.cluster.local",
			Port:        2484,
			ServiceName: "STBYDB",
			Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS,
		},
	}
	hash3 := computeTNSAliasesHash(changedAliases, baseNames)
	if hash1 == hash3 {
		t.Fatalf("expected alias hash to change when desired primary peer aliases change")
	}
}

func TestSIDBUnit_ResolveExternalServiceConfigUsesNewLoadBalancerDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				External: &dbapi.SingleInstanceDatabaseExternalService{
					Type: dbapi.SingleInstanceDatabaseExternalServiceTypeLoadBalancer,
					TCP:  &dbapi.SingleInstanceDatabaseExternalServicePort{Enabled: true},
					TCPS: &dbapi.SingleInstanceDatabaseExternalServicePort{Enabled: true},
				},
			},
		},
	}

	cfg := resolveExternalServiceConfig(sidb)
	if cfg.Disabled {
		t.Fatalf("expected external service to be enabled")
	}
	if cfg.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected load balancer type, got %q", cfg.Type)
	}
	if !cfg.TCPEnabled || cfg.TCPServicePort != dbcommons.CONTAINER_LISTENER_PORT {
		t.Fatalf("expected default tcp load balancer port %d, got %#v", dbcommons.CONTAINER_LISTENER_PORT, cfg)
	}
	if !cfg.TCPSEnabled || cfg.TCPSServicePort != dbcommons.CONTAINER_TCPS_PORT {
		t.Fatalf("expected default tcps load balancer port %d, got %#v", dbcommons.CONTAINER_TCPS_PORT, cfg)
	}
}

func TestSIDBUnit_DesiredSIDBServiceAnnotationsUsesExternalOverrides(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ServiceAnnotations: map[string]string{
				"legacy": "value",
				"shared": "legacy",
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				External: &dbapi.SingleInstanceDatabaseExternalService{
					Annotations: map[string]string{
						"shared": "external",
						"new":    "value",
					},
				},
			},
		},
	}

	got := desiredSIDBServiceAnnotations(sidb, true)
	want := map[string]string{
		"legacy": "value",
		"shared": "external",
		"new":    "value",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected external annotations: got %#v want %#v", got, want)
	}
}

func TestSIDBUnit_DesiredSIDBServiceAnnotationsKeepsLegacyFallbackForExternalService(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ServiceAnnotations: map[string]string{
				"legacy": "value",
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				External: &dbapi.SingleInstanceDatabaseExternalService{},
			},
		},
	}

	got := desiredSIDBServiceAnnotations(sidb, true)
	want := map[string]string{"legacy": "value"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected legacy fallback annotations: got %#v want %#v", got, want)
	}
}

func TestSIDBUnit_ResolveExternalServiceConfigRetainsLegacyCompatibility(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ListenerPort:     32001,
			TcpsListenerPort: 32002,
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
		},
	}

	cfg := resolveExternalServiceConfig(sidb)
	if cfg.Type != corev1.ServiceTypeNodePort {
		t.Fatalf("expected nodeport type, got %q", cfg.Type)
	}
	if !cfg.TCPEnabled || cfg.TCPNodePort != 32001 {
		t.Fatalf("expected legacy tcp nodeport 32001, got %#v", cfg)
	}
	if !cfg.TCPSEnabled || cfg.TCPSNodePort != 32002 {
		t.Fatalf("expected legacy tcps nodeport 32002, got %#v", cfg)
	}
}

func TestSIDBUnit_ResolveExternalServiceConfigTreatsLegacyNonNodePortAsServicePort(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ListenerPort:     1522,
			TcpsListenerPort: 2484,
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
		},
	}

	cfg := resolveExternalServiceConfig(sidb)
	if cfg.Type != corev1.ServiceTypeNodePort {
		t.Fatalf("expected nodeport type, got %q", cfg.Type)
	}
	if !cfg.TCPEnabled || cfg.TCPServicePort != 1522 || cfg.TCPNodePort != 0 {
		t.Fatalf("expected legacy tcp port 1522 with auto nodePort, got %#v", cfg)
	}
	if !cfg.TCPSEnabled || cfg.TCPSServicePort != 2484 || cfg.TCPSNodePort != 0 {
		t.Fatalf("expected legacy tcps port 2484 with auto nodePort, got %#v", cfg)
	}
}

func TestSIDBUnit_HasExpectedTCPSListenerEndpointRequiresContainerTCPSPort(t *testing.T) {
	listenerStatus := `
Listening Endpoints Summary...
  (DESCRIPTION=(ADDRESS=(PROTOCOL=ipc)(KEY=EXTPROC1)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=0.0.0.0)(PORT=1521)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=0.0.0.0)(PORT=2484)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=sidb-standby)(PORT=5500))(Security=(my_wallet_directory=/opt/oracle/admin/STBYDB/xdb_wallet))(Presentation=HTTP)(Session=RAW))
`
	if !hasExpectedTCPSListenerEndpoint(listenerStatus, dbcommons.CONTAINER_TCPS_PORT) {
		t.Fatalf("expected tcps listener endpoint on port %d to be detected", dbcommons.CONTAINER_TCPS_PORT)
	}
}

func TestSIDBUnit_HasExpectedTCPSListenerEndpointIgnoresXDBTCPSOnly(t *testing.T) {
	listenerStatus := `
Listening Endpoints Summary...
  (DESCRIPTION=(ADDRESS=(PROTOCOL=ipc)(KEY=EXTPROC1)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=0.0.0.0)(PORT=1521)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=sidb-standby)(PORT=5500))(Security=(my_wallet_directory=/opt/oracle/admin/STBYDB/xdb_wallet))(Presentation=HTTP)(Session=RAW))
`
	if hasExpectedTCPSListenerEndpoint(listenerStatus, dbcommons.CONTAINER_TCPS_PORT) {
		t.Fatalf("did not expect xdb-only tcps endpoint to satisfy listener validation")
	}
}

func TestSIDBUnit_GetTcpsEnabledTreatsLegacyListenerPortAsEnabled(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			TcpsListenerPort: 2484,
		},
	}

	if !getTcpsEnabled(sidb) {
		t.Fatalf("expected deprecated spec.tcpsListenerPort to imply tcps enabled")
	}

	sidb = &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			TcpsListenerPort: 32002,
		},
	}

	if !getTcpsEnabled(sidb) {
		t.Fatalf("expected deprecated spec.tcpsListenerPort to imply tcps enabled")
	}
}

func TestSIDBUnit_GetStandbyWalletDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{}

	if got := GetStandbyWalletMountPath(sidb); got != "/mnt/standby-wallet" {
		t.Fatalf("unexpected default standby wallet mount path: %q", got)
	}
	if got := GetStandbyTDEWalletRoot(sidb); got != "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet" {
		t.Fatalf("unexpected default standby wallet root: %q", got)
	}
}

func TestSIDBUnit_ValidateStandbyWalletSecretRef(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	ctx := context.Background()
	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "wallet-secret", Namespace: "ns1"},
				Data:       map[string][]byte{"wallet.zip": []byte("zip-bytes")},
			},
		).Build(),
		Log: logr.Discard(),
	}

	t.Run("missing secret", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				Security: &dbapi.SingleInstanceDatabaseSecurity{
					Secrets: &dbapi.SingleInstanceDatabaseSecrets{
						TDE: &dbapi.SingleInstanceDatabasePasswordSecret{
							SecretName: "does-not-exist",
						},
					},
				},
			},
		}
		if err := ValidateStandbyWalletSecretRef(reconciler, sidb, ctx); err == nil {
			t.Fatalf("expected error for missing wallet secret")
		}
	})

	t.Run("zip key missing", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				Security: &dbapi.SingleInstanceDatabaseSecurity{
					Secrets: &dbapi.SingleInstanceDatabaseSecrets{
						TDE: &dbapi.SingleInstanceDatabasePasswordSecret{
							SecretName:       "wallet-secret",
							WalletZipFileKey: "missing.zip",
						},
					},
				},
			},
		}
		if err := ValidateStandbyWalletSecretRef(reconciler, sidb, ctx); err == nil {
			t.Fatalf("expected error for missing wallet zip key")
		}
	})

	t.Run("valid secret and zip key", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				Security: &dbapi.SingleInstanceDatabaseSecurity{
					Secrets: &dbapi.SingleInstanceDatabaseSecrets{
						TDE: &dbapi.SingleInstanceDatabasePasswordSecret{
							SecretName:       "wallet-secret",
							WalletZipFileKey: "wallet.zip",
						},
					},
				},
			},
		}
		if err := ValidateStandbyWalletSecretRef(reconciler, sidb, ctx); err != nil {
			t.Fatalf("expected valid wallet secret ref, got err: %v", err)
		}
	})
}

func TestSIDBUnit_GetRestoreCatalogStartWithDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Restore: &dbapi.SingleInstanceDatabaseRestoreSpec{
				FileSystem: &dbapi.SingleInstanceDatabaseRestoreFileSystemSpec{
					BackupPath: "/mnt/backup",
				},
			},
		},
	}
	if got := getRestoreCatalogStartWith(sidb); got != "/mnt/backup" {
		t.Fatalf("expected catalogStartWith default to backupPath, got %q", got)
	}
}

func TestSIDBUnit_IsRestoreFSPathVolumeBacked(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Size: "10Gi",
				AdditionalPVCs: []dbapi.AdditionalPVCSpec{
					{
						MountPath: "/mnt/backup",
						PvcName:   "backup-pvc",
					},
				},
			},
		},
	}
	if !isRestoreFSPathVolumeBacked(sidb, "/opt/oracle/oradata/rman") {
		t.Fatalf("expected /opt/oracle/oradata path to be treated as volume-backed when persistence is enabled")
	}
	if !isRestoreFSPathVolumeBacked(sidb, "/mnt/backup/full") {
		t.Fatalf("expected additionalPVC mount path to be treated as volume-backed")
	}
	if isRestoreFSPathVolumeBacked(sidb, "/tmp/random") {
		t.Fatalf("expected unrelated path to be treated as non volume-backed")
	}
}

func TestSIDBUnit_ValidateRestoreSpecRefsObjectStore(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	ctx := context.Background()
	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "ociconfig", Namespace: "ns1"},
				Data:       map[string]string{"oci.env": "DBID=123"},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "sshkeysecret", Namespace: "ns1"},
				Data:       map[string][]byte{"oci_api_key.pem": []byte("key")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "sourcedbtde", Namespace: "ns1"},
				Data:       map[string][]byte{"source-wallet.tar.gz": []byte("wallet")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "sourcedbwalletpwd", Namespace: "ns1"},
				Data:       map[string][]byte{"wallet_pwd": []byte("pwd")},
			},
		).Build(),
		Log: logr.Discard(),
	}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Restore: &dbapi.SingleInstanceDatabaseRestoreSpec{
				ObjectStore: &dbapi.SingleInstanceDatabaseRestoreObjectStoreSpec{
					OCIConfig:        &dbapi.SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociconfig", Key: "oci.env"},
					PrivateKey:       &dbapi.SingleInstanceDatabaseSecretKeyRef{SecretName: "sshkeysecret", Key: "oci_api_key.pem"},
					SourceDBWallet:   &dbapi.SingleInstanceDatabaseSecretKeyRef{SecretName: "sourcedbtde", Key: "source-wallet.tar.gz"},
					SourceDBWalletPw: &dbapi.SingleInstanceDatabaseSecretKeyRef{SecretName: "sourcedbwalletpwd", Key: "wallet_pwd"},
				},
			},
		},
	}
	if err := ValidateRestoreSpecRefs(reconciler, sidb, ctx); err != nil {
		t.Fatalf("expected restore refs to validate, got err: %v", err)
	}
}

func TestSIDBUnit_FraMountPathAndRecoverySizeDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					Size: "120Gi",
				},
			},
		},
	}
	if got := getFraMountPath(sidb); got != "/opt/oracle/oradata/fast_recovery_area" {
		t.Fatalf("unexpected FRA mount path default: %q", got)
	}
	if got := getFraRecoveryAreaSize(sidb); got != "120Gi" {
		t.Fatalf("expected FRA recovery area size to default from fra.size, got %q", got)
	}
}

func TestSIDBUnit_InstantiatePVCSpecMalformedVolumeClaimAnnotationDoesNotPanic(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Size:                  "10Gi",
				AccessMode:            "ReadWriteOnce",
				StorageClass:          "standard",
				VolumeClaimAnnotation: "malformed",
			},
		},
	}
	pvc := reconciler.instantiatePVCSpec(sidb)
	if pvc == nil {
		t.Fatalf("expected pvc to be created")
	}
	if len(pvc.Annotations) != 0 {
		t.Fatalf("expected malformed annotation to be ignored, got annotations: %v", pvc.Annotations)
	}
}

func TestSIDBUnit_InstantiatePodSpecCopiesHostAliases(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			HostAliases: []corev1.HostAlias{
				{
					IP:        "10.10.10.10",
					Hostnames: []string{"database.example.com", "db-alias.example.com"},
				},
				{
					IP:        "10.10.10.11",
					Hostnames: []string{"analytics.example.com"},
				},
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}
	if len(pod.Spec.HostAliases) != len(sidb.Spec.HostAliases) {
		t.Fatalf("expected %d host aliases, got %d", len(sidb.Spec.HostAliases), len(pod.Spec.HostAliases))
	}
	if pod.Spec.HostAliases[0].IP != "10.10.10.10" || len(pod.Spec.HostAliases[0].Hostnames) != 2 {
		t.Fatalf("unexpected first host alias: %#v", pod.Spec.HostAliases[0])
	}
	if pod.Spec.HostAliases[1].IP != "10.10.10.11" || len(pod.Spec.HostAliases[1].Hostnames) != 1 || pod.Spec.HostAliases[1].Hostnames[0] != "analytics.example.com" {
		t.Fatalf("unexpected second host alias: %#v", pod.Spec.HostAliases[1])
	}
}

func TestSIDBUnit_InstantiatePodSpecDefaultsCapabilitiesToSysNice(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-default-caps", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	got := pod.Spec.Containers[0].SecurityContext.Capabilities
	want := &corev1.Capabilities{Add: []corev1.Capability{"SYS_NICE"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected default capabilities %#v, got %#v", want, got)
	}
}

func TestSIDBUnit_InstantiatePodSpecAllowsEmptyCapabilitiesOverride(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-empty-caps", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:          "ORCLCDB",
			Capabilities: &corev1.Capabilities{},
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	got := pod.Spec.Containers[0].SecurityContext.Capabilities
	want := &corev1.Capabilities{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected empty capability override %#v, got %#v", want, got)
	}
}

func TestSIDBUnit_InstantiatePodSpecUsesExplicitCapabilitiesOverride(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-custom-caps", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
			Capabilities: &corev1.Capabilities{
				Add:  []corev1.Capability{"NET_RAW"},
				Drop: []corev1.Capability{"ALL"},
			},
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	got := pod.Spec.Containers[0].SecurityContext.Capabilities
	want := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_RAW"},
		Drop: []corev1.Capability{"ALL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected explicit capability override %#v, got %#v", want, got)
	}
}

func TestSIDBUnit_InstantiateTrueCachePodSpecCopiesSIDBHostAliases(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tc1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Sid:      "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			HostAliases: []corev1.HostAlias{
				{
					IP:        "10.10.10.20",
					Hostnames: []string{"primary.example.com", "primary-vip.example.com"},
				},
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}
	if len(pod.Spec.HostAliases) != 1 {
		t.Fatalf("expected 1 host alias, got %d", len(pod.Spec.HostAliases))
	}
	if pod.Spec.HostAliases[0].IP != "10.10.10.20" {
		t.Fatalf("expected sidb host alias to be copied to truecache pod, got %#v", pod.Spec.HostAliases[0])
	}
	if len(pod.Spec.HostAliases[0].Hostnames) != 2 || pod.Spec.HostAliases[0].Hostnames[0] != "primary.example.com" || pod.Spec.HostAliases[0].Hostnames[1] != "primary-vip.example.com" {
		t.Fatalf("unexpected truecache pod host alias hostnames: %#v", pod.Spec.HostAliases[0])
	}
}

func TestSIDBUnit_InstantiatePodSpecPrefersSeparateNodeFromLocalPrimary(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}

	t.Run("standby local primary adds anti-affinity and preserves nodeSelector", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				CreateAs: "standby",
				Sid:      "STBY",
				Image: dbapi.SingleInstanceDatabaseImage{
					PullFrom: "container-registry.oracle.com/database/free:latest",
				},
				PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
					DatabaseRef: "primary-db",
				},
				NodeSelector: map[string]string{"db-role": "ha"},
			},
		}
		primary := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		}

		pod, err := reconciler.instantiatePodSpec(sidb, nil, primary, false)
		if err != nil {
			t.Fatalf("instantiatePodSpec returned err: %v", err)
		}
		if got := pod.Spec.NodeSelector["db-role"]; got != "ha" {
			t.Fatalf("expected nodeSelector to be preserved, got %q", got)
		}
		if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
			t.Fatalf("expected pod anti-affinity to be configured")
		}
		terms := pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		if len(terms) == 0 {
			t.Fatalf("expected preferred anti-affinity terms")
		}
		last := terms[len(terms)-1]
		if last.Weight != 100 {
			t.Fatalf("unexpected anti-affinity weight: %d", last.Weight)
		}
		values := last.PodAffinityTerm.LabelSelector.MatchExpressions[0].Values
		if len(values) != 1 || values[0] != "primary-db" {
			t.Fatalf("unexpected anti-affinity target values: %#v", values)
		}
		if last.PodAffinityTerm.TopologyKey != "kubernetes.io/hostname" {
			t.Fatalf("unexpected topology key: %q", last.PodAffinityTerm.TopologyKey)
		}
	})

	t.Run("truecache local primary adds anti-affinity", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "sidb-tc", Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				CreateAs: "truecache",
				Sid:      "TCDB",
				Image: dbapi.SingleInstanceDatabaseImage{
					PullFrom: "container-registry.oracle.com/database/free:latest",
				},
				PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
					DatabaseRef: "primary-db",
				},
			},
		}
		primary := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		}

		pod, err := reconciler.instantiatePodSpec(sidb, nil, primary, false)
		if err != nil {
			t.Fatalf("instantiatePodSpec returned err: %v", err)
		}
		if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
			t.Fatalf("expected truecache pod anti-affinity to be configured")
		}
		terms := pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		if len(terms) == 0 {
			t.Fatalf("expected preferred anti-affinity terms for truecache")
		}
		values := terms[len(terms)-1].PodAffinityTerm.LabelSelector.MatchExpressions[0].Values
		if len(values) != 1 || values[0] != "primary-db" {
			t.Fatalf("unexpected truecache anti-affinity target values: %#v", values)
		}
	})

	t.Run("external primary source does not add anti-affinity", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "sidb-ext", Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				CreateAs: "standby",
				Sid:      "STBY",
				Image: dbapi.SingleInstanceDatabaseImage{
					PullFrom: "container-registry.oracle.com/database/free:latest",
				},
				PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
					ConnectString: "primary-host:1521/PRIM",
				},
			},
		}
		primary := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		}

		pod, err := reconciler.instantiatePodSpec(sidb, nil, primary, false)
		if err != nil {
			t.Fatalf("instantiatePodSpec returned err: %v", err)
		}
		if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
			t.Fatalf("expected base pod anti-affinity to exist")
		}
		for _, term := range pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			if term.PodAffinityTerm.LabelSelector == nil || len(term.PodAffinityTerm.LabelSelector.MatchExpressions) == 0 {
				continue
			}
			values := term.PodAffinityTerm.LabelSelector.MatchExpressions[0].Values
			if len(values) == 1 && values[0] == "primary-db" {
				t.Fatalf("did not expect external primary anti-affinity term to target primary-db")
			}
		}
	})
}

func TestSIDBUnit_PhaseScheduleFutureRequeueIsPerContext(t *testing.T) {
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard()}
	phaseCtx := &sidbPhaseContext{
		futureRequeue: ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Minute},
	}

	got, err := reconciler.phaseScheduleFutureRequeue(phaseCtx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue || got.RequeueAfter != 30*time.Minute {
		t.Fatalf("unexpected scheduled result: %#v", got)
	}
	if phaseCtx.futureRequeue != requeueN {
		t.Fatalf("expected future requeue to be reset on context")
	}

	got2, err := reconciler.phaseScheduleFutureRequeue(phaseCtx)
	if err != nil {
		t.Fatalf("unexpected err on second call: %v", err)
	}
	if got2 != requeueN {
		t.Fatalf("expected no requeue after reset, got %#v", got2)
	}
}

func TestSIDBUnit_PhaseConnectStringGate(t *testing.T) {
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard()}
	pending := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1"},
		Status: dbapi.SingleInstanceDatabaseStatus{
			ConnectString:     dbcommons.ValueUnavailable,
			TcpsConnectString: dbcommons.ValueUnavailable,
		},
	}
	res, err := reconciler.phaseConnectStringGate(pending)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Requeue {
		t.Fatalf("expected requeue when both connect strings are unavailable")
	}

	ready := &dbapi.SingleInstanceDatabase{Status: dbapi.SingleInstanceDatabaseStatus{ConnectString: "host:1521/ORCLCDB"}}
	res, err = reconciler.phaseConnectStringGate(ready)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res != requeueN {
		t.Fatalf("expected no requeue for available connect string, got %#v", res)
	}

	tcpsOnly := &dbapi.SingleInstanceDatabase{
		Status: dbapi.SingleInstanceDatabaseStatus{
			ConnectString:     dbcommons.ValueUnavailable,
			TcpsConnectString: "host:2484/ORCLCDB",
		},
	}
	res, err = reconciler.phaseConnectStringGate(tcpsOnly)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res != requeueN {
		t.Fatalf("expected no requeue for available tcps connect string, got %#v", res)
	}
}

func TestSIDBUnit_ReconcileBlockedByUpdateLockRequeues(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 5},
		Status: dbapi.SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 4,
				Message:            "controller lock active",
			}},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&dbapi.SingleInstanceDatabase{}).
			WithObjects(sidb).
			Build(),
		Log: logr.Discard(),
	}

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns1", Name: "sidb1"},
	})
	if err != nil {
		t.Fatalf("expected no error while lock-gated, got: %v", err)
	}
	if !res.Requeue || res.RequeueAfter != 30*time.Second {
		t.Fatalf("expected lock-gated requeue after 30s, got: %#v", res)
	}
}
