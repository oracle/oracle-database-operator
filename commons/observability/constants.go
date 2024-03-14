package observability

import "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"

const (
	UnknownValue = "UNKNOWN"
	DefaultValue = "DEFAULT"
)

// Observability Status
const (
	StatusObservabilityPending v1alpha1.StatusEnum = "PENDING"
	StatusObservabilityError   v1alpha1.StatusEnum = "ERROR"
	StatusObservabilityReady   v1alpha1.StatusEnum = "READY"
)

// Log Names
const (
	LogReconcile               = "ObservabilityExporterLogger"
	LogExportersDeploy         = "ObservabilityExporterDeploymentLogger"
	LogExportersSVC            = "ObservabilityExporterServiceLogger"
	LogExportersServiceMonitor = "ObservabilityExporterServiceMonitorLogger"
)

// Defaults
const (
	DefaultDbUserKey                 = "username"
	DefaultDBPasswordKey             = "password"
	DefaultDBConnectionStringKey     = "connection"
	DefaultLabelKey                  = "app"
	DefaultConfigVolumeString        = "config-volume"
	DefaultWalletVolumeString        = "creds"
	DefaultOCIPrivateKeyVolumeString = "ocikey"
	DefaultOCIConfigFingerprintKey   = "fingerprint"
	DefaultOCIConfigRegionKey        = "region"
	DefaultOCIConfigTenancyKey       = "tenancy"
	DefaultOCIConfigUserKey          = "user"

	DefaultExporterImage                 = "container-registry.oracle.com/database/observability-exporter:1.1.0"
	DefaultServicePort                   = 9161
	DefaultPrometheusPort                = "metrics"
	DefaultReplicaCount                  = 1
	DefaultExporterConfigMountRootPath   = "/oracle/observability"
	DefaultOracleHome                    = "/lib/oracle/21/client64/lib"
	DefaultOracleTNSAdmin                = DefaultOracleHome + "/network/admin"
	DefaultExporterConfigmapFilename     = "config.toml"
	DefaultVaultPrivateKeyRootPath       = "/oracle/config"
	DefaultPrivateKeyFileKey             = "privatekey"
	DefaultPrivateKeyFileName            = "private.pem"
	DefaultVaultPrivateKeyAbsolutePath   = DefaultVaultPrivateKeyRootPath + "/" + DefaultPrivateKeyFileName
	DefaultExporterConfigmapAbsolutePath = DefaultExporterConfigMountRootPath + "/" + DefaultExporterConfigmapFilename
)

// default resource prefixes
const (
	DefaultServiceMonitorPrefix     = "obs-servicemonitor-"
	DefaultLabelPrefix              = "obs-"
	DefaultExporterDeploymentPrefix = "obs-deploy-"
	DefaultExporterContainerName    = "observability-exporter"
)

// Known environment variables
const (
	EnvVarOracleHome                   = "ORACLE_HOME"
	EnvVarDataSourceUser               = "DB_USERNAME"
	EnvVarDataSourcePassword           = "DB_PASSWORD"
	EnvVarDataSourceConnectString      = "DB_CONNECT_STRING"
	EnvVarDataSourcePwdVaultSecretName = "VAULT_SECRET_NAME"
	EnvVarDataSourcePwdVaultId         = "VAULT_ID"
	EnvVarCustomConfigmap              = "CUSTOM_METRICS"
	EnvVarTNSAdmin                     = "TNS_ADMIN"
	EnvVarVaultTenancyOCID             = "vault_tenancy_ocid"
	EnvVarVaultUserOCID                = "vault_user_ocid"
	EnvVarVaultFingerprint             = "vault_fingerprint"
	EnvVarVaultPrivateKeyPath          = "vault_private_key_path"
	EnvVarVaultRegion                  = "vault_region"
)

// Positive ConditionTypes
const (
	IsCRAvailable                 = "ExporterReady"
	IsExporterDeploymentReady     = "DeploymentReady"
	IsExporterServiceReady        = "ServiceReady"
	IsExporterServiceMonitorReady = "ServiceMonitorReady"
)

// Reason
const (
	ReasonInitStart                      = "InitializationStarted"
	ReasonReadyValidated                 = "ReadinessValidated"
	ReasonValidationInProgress           = "ReadinessValidationInProgress"
	ReasonReadyFailed                    = "ReadinessValidationFailed"
	ReasonDeploymentSpecValidationFailed = "SpecValidationFailed"

	ReasonDeploymentSuccessful   = "ResourceDeployed"
	ReasonDeploymentUpdated      = "ResourceDeploymentUpdated"
	ReasonDeploymentUpdateFailed = "ResourceDeploymentUpdateFailed"
	ReasonDeploymentFailed       = "ResourceDeploymentFailed"
	ReasonDeploymentPending      = "ResourceDeploymentInProgress"

	ReasonGeneralResourceGenerationFailed            = "ResourceGenerationFailed"
	ReasonGeneralResourceCreated                     = "ResourceCreated"
	ReasonGeneralResourceCreationFailed              = "ResourceCreationFailed"
	ReasonGeneralResourceValidationCompleted         = "ResourceDeployed"
	ReasonGeneralResourceValidationFailureDueToError = "ResourceCouldNotBeValidated"
)

// Log Errors
const (
	ErrorCRRetrieve                           = "an error occurred with retrieving the cr"
	ErrorStatusUpdate                         = "an error occurred with updating the cr status"
	ErrorSpecValidationMissingDBPassword      = "an error occurred with validating the spec due to missing dbpassword values"
	ErrorSpecValidationFailedDueToAnError     = "an error occurred with validating the exporter deployment spec"
	ErrorDeploymentPodsFailure                = "an error occurred with deploying exporter deployment pods"
	ErrorDeploymentUpdate                     = "an error occurred with updating exporter deployment"
	ErrorResourceCreationFailure              = "an error occurred with creating databaseobserver resource"
	ErrorResourceRetrievalFailureDueToAnError = "an error occurred with retrieving databaseobserver resource"
)

// Log Infos
const (
	LogCRStart         = "Started DatabaseObserver instance reconciliation"
	LogCREnd           = "Ended DatabaseObserver instance reconciliation, resource must have been deleted."
	LogResourceCreated = "Created DatabaseObserver resource successfully"
	LogResourceUpdated = "Updated DatabaseObserver resource successfully"
	LogResourceFound   = "Validated DatabaseObserver resource readiness"
)

// Messages
const (
	MessageCRInitializationStarted = "Started initialization of custom resource"
	MessageCRValidated             = "Completed validation of custom resource readiness successfully"
	MessageCRValidationFailed      = "Failed to validate readiness of custom resource due to an error"
	MessageCRValidationWaiting     = "Waiting for other resources to be ready to fully validate readiness"

	MessageResourceCreated                   = "Completed creation of resource successfully"
	MessageResourceCreationFailed            = "Failed to create resource due to an error"
	MessageResourceReadinessValidated        = "Completed validation of resource readiness"
	MessageResourceReadinessValidationFailed = "Failed to validate resource due to an error retrieving resource"
	MessageResourceGenerationFailed          = "Failed to generate resource due to an error"

	MessageExporterDeploymentSpecValidationFailed = "Failed to validate export deployment spec due to an error with the spec"
	MessageExporterDeploymentImageUpdated         = "Completed updating exporter deployment image successfully"
	MessageExporterDeploymentEnvironmentUpdated   = "Completed updating exporter deployment environment values successfully"
	MessageExporterDeploymentReplicaUpdated       = "Completed updating exporter deployment replicaCount successfully"
	MessageExporterDeploymentVolumesUpdated       = "Completed updating exporter deployment volumes successfully"
	MessageExporterDeploymentUpdateFailed         = "Failed to update exporter deployment due to an error"
	MessageExporterDeploymentValidationFailed     = "Failed to validate exporter deployment due to an error retrieving resource"
	MessageExporterDeploymentSuccessful           = "Completed validation of exporter deployment readiness"
	MessageExporterDeploymentFailed               = "Failed to deploy exporter deployment due to PodFailure"
	MessageExporterDeploymentListingFailed        = "Failed to list exporter deployment pods"
	MessageExporterDeploymentPending              = "Waiting for exporter deployment pods to be ready"
)

// Event Recorder Outputs
const (
	EventReasonFailedCRRetrieval  = "ExporterRetrievalFailed"
	EventMessageFailedCRRetrieval = "Encountered error retrieving databaseObserver instance"

	EventReasonSpecError                                 = "DeploymentSpecValidationFailed"
	EventMessageSpecErrorDBPasswordMissing               = "Spec validation failed due to missing dbPassword field values"
	EventMessageSpecErrorDBPasswordSecretMissing         = "Spec validation failed due to required dbPassword secret not found"
	EventMessageSpecErrorDBConnectionStringSecretMissing = "Spec validation failed due to required dbConnectionString secret not found"
	EventMessageSpecErrorDBPUserSecretMissing            = "Spec validation failed due to dbUser secret not found"
	EventMessageSpecErrorConfigmapMissing                = "Spec validation failed due to custom config configmap not found"
	EventMessageSpecErrorDBWalletSecretMissing           = "Spec validation failed due to provided dbWallet secret not found"

	EventReasonUpdateSucceeded              = "ExporterDeploymentUpdated"
	EventMessageUpdatedImageSucceeded       = "Exporter deployment image updated successfully"
	EventMessageUpdatedEnvironmentSucceeded = "Exporter deployment environment values updated successfully"
	EventMessageUpdatedVolumesSucceeded     = "Exporter deployment volumes updated successfully"
	EventMessageUpdatedReplicaSucceeded     = "Exporter deployment replicaCount updated successfully"
)
