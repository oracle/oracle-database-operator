package dataguard

import "fmt"

// WorkflowStep is a named phase in standby + DG broker setup.
type WorkflowStep string

const (
	StepEnsureBrokerFilesAndStart     WorkflowStep = "ensure_broker_files_and_start"
	StepRunPrimaryPrerequisites       WorkflowStep = "run_primary_prerequisites"
	StepEnsureStandbyRedoLogs         WorkflowStep = "ensure_standby_redo_logs"
	StepConfigurePrimaryRedoTransport WorkflowStep = "configure_primary_redo_transport"
	StepEnsureStandbyApplyRunning     WorkflowStep = "ensure_standby_apply_running"
	StepForceArchiveAndCheckTransport WorkflowStep = "force_archive_and_check_transport"
	StepCreateDgBrokerConfig          WorkflowStep = "create_dg_broker_config"
	StepAddStandbyToDgBrokerConfig    WorkflowStep = "add_standby_to_dg_broker_config"
	StepSetDgConnectIdentifiers       WorkflowStep = "set_dg_connect_identifiers"
	StepEnableAndValidateDgBroker     WorkflowStep = "enable_and_validate_dg_broker"
)

// StandbyDGBrokerWorkflow abstracts execution details (SQL/scripts/tools) behind semantic steps.
// Controllers can keep topology-specific logic separate and reuse this ordered workflow.
type StandbyDGBrokerWorkflow interface {
	EnsureBrokerFilesAndStart() error
	RunPrimaryPrerequisites() error
	EnsureStandbyRedoLogs() error
	ConfigurePrimaryRedoTransport() error
	EnsureStandbyApplyRunning() error
	ForceArchiveAndCheckRedoTransport() error
	CreateDgBrokerConfig() error
	AddStandbyToDgBrokerConfig() error
	SetDgConnectIdentifiers() error
	EnableAndValidateDgBroker() error
}

// StepError returns the semantic step where the workflow failed.
type StepError struct {
	Step WorkflowStep
	Err  error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("standby dg workflow step %s failed: %v", e.Step, e.Err)
}

func (e *StepError) Unwrap() error { return e.Err }

// RunStandbyDGBrokerWorkflow executes common standby + DG broker setup steps in order.
func RunStandbyDGBrokerWorkflow(flow StandbyDGBrokerWorkflow) error {
	steps := []struct {
		step WorkflowStep
		run  func() error
	}{
		{StepEnsureBrokerFilesAndStart, flow.EnsureBrokerFilesAndStart},
		{StepRunPrimaryPrerequisites, flow.RunPrimaryPrerequisites},
		{StepEnsureStandbyRedoLogs, flow.EnsureStandbyRedoLogs},
		{StepConfigurePrimaryRedoTransport, flow.ConfigurePrimaryRedoTransport},
		{StepEnsureStandbyApplyRunning, flow.EnsureStandbyApplyRunning},
		{StepForceArchiveAndCheckTransport, flow.ForceArchiveAndCheckRedoTransport},
		{StepCreateDgBrokerConfig, flow.CreateDgBrokerConfig},
		{StepAddStandbyToDgBrokerConfig, flow.AddStandbyToDgBrokerConfig},
		{StepSetDgConnectIdentifiers, flow.SetDgConnectIdentifiers},
		{StepEnableAndValidateDgBroker, flow.EnableAndValidateDgBroker},
	}

	for _, s := range steps {
		if err := s.run(); err != nil {
			return &StepError{Step: s.step, Err: err}
		}
	}

	return nil
}
