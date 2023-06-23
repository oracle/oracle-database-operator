package observability

import "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"

const UnknownValue = "UNKNOWN"

// Observability Status
const (
	StatusObservabilityPending     v1alpha1.StatusEnum = "PENDING"
	StatusObservabilityError       v1alpha1.StatusEnum = "ERROR"
	StatusObservabilityReady       v1alpha1.StatusEnum = "READY"
	StatusObservabilityTerminating v1alpha1.StatusEnum = "TERMINATING"
)

// ConditionTypes
const (
	PhaseReconcile                 = "Reconcile"
	PhaseControllerValidation      = "Validation"
	PhaseExportersDeploy           = "CreateExportersDeployment"
	PhaseExportersDeployValidation = "ValidateExportersDeployment"
	PhaseExportersDeployUpdate     = "UpdateExportersDeployment"
	PhaseExportersSVC              = "CreateExporterService"
	PhaseExportersServiceMonitor   = "CreateExporterServiceMonitor"
	PhaseExportersGrafanaConfigMap = "CreateGrafanaConfigMap"
	PhaseExportersConfigMap        = "CreateExporterConfigurationConfigMap"
)

// Reason
const (
	ReasonInitStart         = "InitializationStarted"
	ReasonInitSuccess       = "InitializationSucceeded"
	ReasonValidationFailure = "ValidationFailed"
	ReasonSetFailure        = "SetResourceFailed"
	ReasonCreateFailure     = "CreateResourceFailed"
	ReasonCreateSuccess     = "CreateResourceSucceeded"
	ReasonUpdateSuccess     = "UpdateResourceSucceeded"
	ReasonUpdateFailure     = "UpdateResourceFailed"
	ReasonRetrieveFailure   = "RetrieveResourceFailed"
	ReasonRetrieveSuccess   = "RetrieveResourceSucceeded"
)

// Log Names
const (
	LogReconcile                 = "LogReconcile"
	LogControllerValidation      = "LogValidation"
	LogExportersDeploy           = "LogCreateExportersDeployment"
	LogExportersSVC              = "LogCreateExporterService"
	LogExportersServiceMonitor   = "LogCreateExporterServiceMonitor"
	LogExportersGrafanaConfigMap = "LogCreateGrafanaConfigMap"
	LogExportersConfigMap        = "LogCreateExporterConfigurationConfigMap"
)
const (
	DefaultDbUserKey             = "user"
	DefaultDBServiceNameKey      = "sn"
	DefaultDBPasswordKey         = "password"
	DefaultDBConnectionStringKey = "access"
	DefaultDBWalletMountPath     = "/creds"
)
const (
	DefaultExporterImage  = "container-registry.oracle.com/database/observability-exporter:0.1.0"
	DefaultServicePort    = 9161
	DefaultPrometheusPort = "metrics"
	DefaultReplicaCount   = 1
	DefaultConfig         = "[[metric]]\ncontext = \"sessions\"\nlabels = [\"inst_id\", \"status\", \"type\"]\nmetricsdesc = { value = \"Gauge metric with count of sessions by status and type.\" }\nrequest = '''\nSELECT\n    inst_id,\n    status,\n    type,\n    COUNT(*) AS value\nFROM\n    gv$session\nGROUP BY\n    status,\n    type,\n    inst_id\n'''\n\n"

	DefaultExporterConfigMountRootPath = "/observability"
	DefaultConfigurationConfigmapKey   = "config.toml"
	DefaultExporterConfigmapPath       = "config.toml"
	DefaultExporterConfigMountPathFull = "/observability/config.toml"
	DefaultGrafanaConfigmapKey         = "dashboard.json"
	DefaultExporterConfigmapPrefix     = "obs-cm-"
	DefaultServicemonitorPrefix        = "obs-servicemonitor-"
)

const (
	DefaultLabelKey                   = "app"
	DefaultLabelPrefix                = "obs-"
	DefaultGrafanaConfigMapNamePrefix = "obs-json-dash-"
	DefaultExporterDeploymentPrefix   = "obs-deploy-"
	DefaultExporterContainerName      = "observability-exporter"
)

// Known environment variables
const (
	EnvVarDataSourceServiceName        = "DATA_SOURCE_SERVICENAME"
	EnvVarDataSourceUser               = "DATA_SOURCE_USER"
	EnvVarDataSourcePassword           = "DATA_SOURCE_PASSWORD"
	EnvVarDataSourceName               = "DATA_SOURCE_NAME"
	EnvVarDataSourcePwdVaultSecretName = "vault_secret_name"
	EnvVarDataSourcePwdVaultId         = "vault_id"
	EnvVarCustomConfigmap              = "CUSTOM_METRICS"
)
