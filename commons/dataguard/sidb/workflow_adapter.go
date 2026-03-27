package sidb

import "fmt"

// StandbyWorkflowOptions wires SIDB primary setup actions into the shared
// standby workflow contract. SIDB uses SQL-based setup for these stages.
type StandbyWorkflowOptions struct {
	EnsureBrokerFilesAndStart func() error
	RunPrimaryPrerequisites   func() error
	EnsureStandbyRedoLogs     func() error
}

type StandbyWorkflow struct {
	opts StandbyWorkflowOptions
}

func NewStandbyWorkflow(opts StandbyWorkflowOptions) *StandbyWorkflow {
	return &StandbyWorkflow{opts: opts}
}

func (w *StandbyWorkflow) EnsureBrokerFilesAndStart() error {
	if w.opts.EnsureBrokerFilesAndStart == nil {
		return fmt.Errorf("EnsureBrokerFilesAndStart callback is required")
	}
	return w.opts.EnsureBrokerFilesAndStart()
}

func (w *StandbyWorkflow) RunPrimaryPrerequisites() error {
	if w.opts.RunPrimaryPrerequisites == nil {
		return fmt.Errorf("RunPrimaryPrerequisites callback is required")
	}
	return w.opts.RunPrimaryPrerequisites()
}

func (w *StandbyWorkflow) EnsureStandbyRedoLogs() error {
	if w.opts.EnsureStandbyRedoLogs == nil {
		return fmt.Errorf("EnsureStandbyRedoLogs callback is required")
	}
	return w.opts.EnsureStandbyRedoLogs()
}

// SIDB local standby bootstrap does not perform these broker lifecycle
// operations here; they are handled in SIDB-specific flows/controllers.
func (w *StandbyWorkflow) ConfigurePrimaryRedoTransport() error { return nil }
func (w *StandbyWorkflow) EnsureStandbyApplyRunning() error     { return nil }
func (w *StandbyWorkflow) ForceArchiveAndCheckRedoTransport() error {
	return nil
}
func (w *StandbyWorkflow) CreateDgBrokerConfig() error       { return nil }
func (w *StandbyWorkflow) AddStandbyToDgBrokerConfig() error { return nil }
func (w *StandbyWorkflow) SetDgConnectIdentifiers() error    { return nil }
func (w *StandbyWorkflow) EnableAndValidateDgBroker() error  { return nil }
