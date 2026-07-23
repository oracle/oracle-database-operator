package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestDbcsSystemCloneForRequestKeepsRequestScopedClientsIsolated(t *testing.T) {
	t.Parallel()

	base := &DbcsSystemReconciler{
		Logger: logr.Discard(),
	}
	base.dbClient.Host = "shared-db"
	base.nwClient.Host = "shared-nw"
	base.wrClient.Host = "shared-wr"

	ctxA := log.IntoContext(context.Background(), logr.Discard().WithValues("request", "a"))
	ctxB := log.IntoContext(context.Background(), logr.Discard().WithValues("request", "b"))

	reqA := base.cloneForRequest(ctxA)
	reqB := base.cloneForRequest(ctxB)

	reqA.dbClient.Host = "tenant-a-db"
	reqA.nwClient.Host = "tenant-a-nw"
	reqA.wrClient.Host = "tenant-a-wr"

	reqB.dbClient.Host = "tenant-b-db"
	reqB.nwClient.Host = "tenant-b-nw"
	reqB.wrClient.Host = "tenant-b-wr"

	if reqA == base || reqB == base {
		t.Fatalf("cloneForRequest must return a distinct reconciler instance")
	}
	if reqA == reqB {
		t.Fatalf("cloneForRequest must isolate concurrent requests")
	}

	if got := base.dbClient.Host; got != "shared-db" {
		t.Fatalf("base dbClient.Host = %q, want %q", got, "shared-db")
	}
	if got := base.nwClient.Host; got != "shared-nw" {
		t.Fatalf("base nwClient.Host = %q, want %q", got, "shared-nw")
	}
	if got := base.wrClient.Host; got != "shared-wr" {
		t.Fatalf("base wrClient.Host = %q, want %q", got, "shared-wr")
	}

	if got := reqA.dbClient.Host; got != "tenant-a-db" {
		t.Fatalf("reqA dbClient.Host = %q, want %q", got, "tenant-a-db")
	}
	if got := reqA.nwClient.Host; got != "tenant-a-nw" {
		t.Fatalf("reqA nwClient.Host = %q, want %q", got, "tenant-a-nw")
	}
	if got := reqA.wrClient.Host; got != "tenant-a-wr" {
		t.Fatalf("reqA wrClient.Host = %q, want %q", got, "tenant-a-wr")
	}

	if got := reqB.dbClient.Host; got != "tenant-b-db" {
		t.Fatalf("reqB dbClient.Host = %q, want %q", got, "tenant-b-db")
	}
	if got := reqB.nwClient.Host; got != "tenant-b-nw" {
		t.Fatalf("reqB nwClient.Host = %q, want %q", got, "tenant-b-nw")
	}
	if got := reqB.wrClient.Host; got != "tenant-b-wr" {
		t.Fatalf("reqB wrClient.Host = %q, want %q", got, "tenant-b-wr")
	}
}
