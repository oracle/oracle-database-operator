// Package sidb adapts SIDB operations to the shared Data Guard workflow.
package sidb

import "fmt"

// StandbyWorkflowOptions wires SIDB primary setup actions into the shared
// standby workflow contract. SIDB uses SQL-based setup for these stages.
type StandbyWorkflowOptions struct {
	EnsureBrokerFilesAndStart func() error
	RunPrimaryPrerequisites   func() error
	EnsureStandbyRedoLogs     func() error
}

// StandbyWorkflow implements the shared standby workflow using SIDB callbacks.
type StandbyWorkflow struct {
	opts StandbyWorkflowOptions
}

// NewStandbyWorkflow constructs a SIDB standby workflow adapter.
func NewStandbyWorkflow(opts StandbyWorkflowOptions) *StandbyWorkflow {
	return &StandbyWorkflow{opts: opts}
}

// EnsureBrokerFilesAndStart executes the callback that prepares broker files and startup.
func (w *StandbyWorkflow) EnsureBrokerFilesAndStart() error {
	if w.opts.EnsureBrokerFilesAndStart == nil {
		return fmt.Errorf("EnsureBrokerFilesAndStart callback is required")
	}
	return w.opts.EnsureBrokerFilesAndStart()
}

// RunPrimaryPrerequisites executes primary-side SQL prerequisites callback.
func (w *StandbyWorkflow) RunPrimaryPrerequisites() error {
	if w.opts.RunPrimaryPrerequisites == nil {
		return fmt.Errorf("RunPrimaryPrerequisites callback is required")
	}
	return w.opts.RunPrimaryPrerequisites()
}

// EnsureStandbyRedoLogs executes the callback that ensures standby redo logs.
func (w *StandbyWorkflow) EnsureStandbyRedoLogs() error {
	if w.opts.EnsureStandbyRedoLogs == nil {
		return fmt.Errorf("EnsureStandbyRedoLogs callback is required")
	}
	return w.opts.EnsureStandbyRedoLogs()
}

// SIDB local standby bootstrap does not perform broker lifecycle operations
// in this adapter; those are handled in SIDB-specific controller flows.

// ConfigurePrimaryRedoTransport is a no-op for this SIDB adapter.
func (w *StandbyWorkflow) ConfigurePrimaryRedoTransport() error { return nil }

// EnsureStandbyApplyRunning is a no-op for this SIDB adapter.
func (w *StandbyWorkflow) EnsureStandbyApplyRunning() error { return nil }

// ForceArchiveAndCheckRedoTransport is a no-op for this SIDB adapter.
func (w *StandbyWorkflow) ForceArchiveAndCheckRedoTransport() error {
	return nil
}

// CreateDgBrokerConfig is a no-op for this SIDB adapter.
func (w *StandbyWorkflow) CreateDgBrokerConfig() error { return nil }

// AddStandbyToDgBrokerConfig is a no-op for this SIDB adapter.
func (w *StandbyWorkflow) AddStandbyToDgBrokerConfig() error { return nil }

// SetDgConnectIdentifiers is a no-op for this SIDB adapter.
func (w *StandbyWorkflow) SetDgConnectIdentifiers() error { return nil }

// EnableAndValidateDgBroker is a no-op for this SIDB adapter.
func (w *StandbyWorkflow) EnableAndValidateDgBroker() error { return nil }
