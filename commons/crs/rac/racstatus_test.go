package commons

import (
	"testing"

	"github.com/go-logr/logr"
	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestShouldPreserveAvailableClusterState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		next    string
		want    bool
	}{
		{
			name:    "available to provisioning is preserved",
			current: string(racdb.RACAvailableState),
			next:    string(racdb.RACProvisionState),
			want:    true,
		},
		{
			name:    "available to pending is preserved",
			current: string(racdb.RACAvailableState),
			next:    string(racdb.RACPendingState),
			want:    true,
		},
		{
			name:    "available to failed is not preserved",
			current: string(racdb.RACAvailableState),
			next:    string(racdb.RACFailedState),
			want:    false,
		},
		{
			name:    "pending to provisioning is not preserved",
			current: string(racdb.RACPendingState),
			next:    string(racdb.RACProvisionState),
			want:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldPreserveAvailableClusterState(tc.current, tc.next); got != tc.want {
				t.Fatalf("shouldPreserveAvailableClusterState(%q, %q) = %t, want %t", tc.current, tc.next, got, tc.want)
			}
		})
	}
}

func TestUpdateRacNodeStatusDataForClusterPreservesHealthyAvailableState(t *testing.T) {
	t.Parallel()

	racDatabase := &racdb.RacDatabase{}
	racDatabase.Status.State = string(racdb.RACAvailableState)
	racDatabase.Status.ReleaseUpdate = "19.30.0.0.0"

	clusterSpec := &racdb.RacClusterDetailSpec{RacNodeName: "racnode"}

	UpdateRacNodeStatusDataForCluster(
		racDatabase,
		nil,
		ctrl.Request{},
		clusterSpec,
		0,
		string(racdb.RACProvisionState),
		nil,
		nil,
		logr.Discard(),
		nil,
	)

	if got := racDatabase.Status.State; got != string(racdb.RACAvailableState) {
		t.Fatalf("expected healthy cluster state to remain AVAILABLE, got %q", got)
	}
}
