package sharding

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	dataguardcommon "github.com/oracle/oracle-database-operator/commons/dataguard"
	shardingv1 "github.com/oracle/oracle-database-operator/commons/sharding"
	"k8s.io/client-go/rest"
)

type StandbyWorkflowOptions struct {
	Instance        *databasev4.ShardingDatabase
	Primary         databasev4.ShardSpec
	Standby         databasev4.ShardSpec
	CfgName         string
	PrimaryConnects []string
	StandbyConnects []string
	KubeConfig      *rest.Config
	Log             logr.Logger

	ConfigurePrimaryRedoTransport func(instance *databasev4.ShardingDatabase, primary, standby databasev4.ShardSpec) error
	EnsureStandbyApplyRunning     func(instance *databasev4.ShardingDatabase, standby databasev4.ShardSpec) error
	ForceArchiveAndCheckTransport func(instance *databasev4.ShardingDatabase, primary databasev4.ShardSpec) error
	SetDgConnectIdentifiers       func(instance *databasev4.ShardingDatabase, primary, standby databasev4.ShardSpec) error
}

type StandbyWorkflow struct {
	opts            StandbyWorkflowOptions
	primaryDbUnique string
	standbyDbUnique string
	primaryPod      string
	standbyPod      string
}

func NewStandbyWorkflow(opts StandbyWorkflowOptions) *StandbyWorkflow {
	return &StandbyWorkflow{
		opts:            opts,
		primaryDbUnique: strings.ToUpper(strings.TrimSpace(opts.Primary.Name)),
		standbyDbUnique: strings.ToUpper(strings.TrimSpace(opts.Standby.Name)),
		primaryPod:      opts.Primary.Name + "-0",
		standbyPod:      opts.Standby.Name + "-0",
	}
}

func (w *StandbyWorkflow) EnsureBrokerFilesAndStart() error {
	if err := shardingv1.EnsureDgBrokerFilesAndStart(w.primaryPod, w.primaryDbUnique, w.opts.Instance, w.opts.KubeConfig, w.opts.Log); err != nil {
		return err
	}
	return shardingv1.EnsureDgBrokerFilesAndStart(w.standbyPod, w.standbyDbUnique, w.opts.Instance, w.opts.KubeConfig, w.opts.Log)
}

func (w *StandbyWorkflow) RunPrimaryPrerequisites() error {
	if err := shardingv1.RunStandbyDatabasePrerequisitesSQL(w.primaryPod, w.opts.Instance, w.opts.KubeConfig, w.opts.Log); err != nil {
		return err
	}
	if err := shardingv1.EnableArchiveLogInPod(w.primaryPod, w.opts.Instance, w.opts.KubeConfig, w.opts.Log); err != nil {
		return err
	}
	if err := shardingv1.RunSQLPlusInPod(w.primaryPod, dbcommons.ForceLoggingTrueSQL, w.opts.Instance, w.opts.KubeConfig, w.opts.Log); err != nil {
		return err
	}
	return shardingv1.RunSQLPlusInPod(w.primaryPod, dbcommons.FlashBackTrueSQL, w.opts.Instance, w.opts.KubeConfig, w.opts.Log)
}

func (w *StandbyWorkflow) EnsureStandbyRedoLogs() error {
	return shardingv1.EnsureStandbyRedoLogsForShards(w.primaryPod, w.standbyPod, w.opts.Instance, w.opts.KubeConfig, w.opts.Log)
}

func (w *StandbyWorkflow) ConfigurePrimaryRedoTransport() error {
	if w.opts.ConfigurePrimaryRedoTransport == nil {
		return fmt.Errorf("ConfigurePrimaryRedoTransport callback is required")
	}
	return w.opts.ConfigurePrimaryRedoTransport(w.opts.Instance, w.opts.Primary, w.opts.Standby)
}

func (w *StandbyWorkflow) EnsureStandbyApplyRunning() error {
	if w.opts.EnsureStandbyApplyRunning == nil {
		return fmt.Errorf("EnsureStandbyApplyRunning callback is required")
	}
	return w.opts.EnsureStandbyApplyRunning(w.opts.Instance, w.opts.Standby)
}

func (w *StandbyWorkflow) ForceArchiveAndCheckRedoTransport() error {
	if w.opts.ForceArchiveAndCheckTransport == nil {
		return fmt.Errorf("ForceArchiveAndCheckTransport callback is required")
	}
	return w.opts.ForceArchiveAndCheckTransport(w.opts.Instance, w.opts.Primary)
}

func (w *StandbyWorkflow) CreateDgBrokerConfig() error {
	return shardingv1.CreateDgBrokerConfigTryConnects(
		w.primaryPod, w.opts.CfgName, w.primaryDbUnique, w.opts.PrimaryConnects, w.opts.Instance, w.opts.KubeConfig, w.opts.Log,
	)
}

func (w *StandbyWorkflow) AddStandbyToDgBrokerConfig() error {
	return shardingv1.AddStandbyToDgBrokerConfigTryConnects(
		w.primaryPod, w.standbyDbUnique, w.opts.StandbyConnects, w.opts.Instance, w.opts.KubeConfig, w.opts.Log,
	)
}

func (w *StandbyWorkflow) SetDgConnectIdentifiers() error {
	if w.opts.SetDgConnectIdentifiers == nil {
		return fmt.Errorf("SetDgConnectIdentifiers callback is required")
	}
	return w.opts.SetDgConnectIdentifiers(w.opts.Instance, w.opts.Primary, w.opts.Standby)
}

func (w *StandbyWorkflow) EnableAndValidateDgBroker() error {
	return shardingv1.EnableAndValidateDgBroker(w.primaryPod, w.opts.CfgName, w.opts.Instance, w.opts.KubeConfig, w.opts.Log)
}

func StatusForWorkflowStep(step dataguardcommon.WorkflowStep) string {
	switch step {
	case dataguardcommon.StepEnsureBrokerFilesAndStart:
		return "error:broker-files"
	case dataguardcommon.StepRunPrimaryPrerequisites:
		return "error:prereqs-primary"
	case dataguardcommon.StepEnsureStandbyRedoLogs:
		return "error:standby-redo-logs"
	case dataguardcommon.StepConfigurePrimaryRedoTransport:
		return "error:redo-transport"
	case dataguardcommon.StepEnsureStandbyApplyRunning:
		return "error:start-apply"
	case dataguardcommon.StepForceArchiveAndCheckTransport:
		return "error:force-archive-check"
	case dataguardcommon.StepCreateDgBrokerConfig:
		return "error:create-config"
	case dataguardcommon.StepAddStandbyToDgBrokerConfig:
		return "error:add-standby"
	case dataguardcommon.StepSetDgConnectIdentifiers:
		return "error:set-connect-identifiers"
	case dataguardcommon.StepEnableAndValidateDgBroker:
		return "error:enable-validate"
	default:
		return "error:workflow"
	}
}
