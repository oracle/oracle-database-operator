package observability

import (
	v4 "github.com/oracle/oracle-database-operator/apis/observability/v4"
)

const (
	UnknownValue = "UNKNOWN"
	DefaultValue = "DEFAULT"
)

// Signals
const (
	VaultUsernameInUse = "VAULT_USERNAME"
	VaultPasswordInUse = "VAULT_PASSWORD"
	VaultIDProvided    = "VAULT_ID_PROVIDED"
)

// Observability Status
const (
	StatusObservabilityPending v4.StatusEnum = "PENDING"
	StatusObservabilityError   v4.StatusEnum = "ERROR"
	StatusObservabilityReady   v4.StatusEnum = "READY"
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
	DefaultConfigVolumeString        = "metrics-volume"
	DefaultLogFilename               = "alert.log"
	DefaultLogVolumeString           = "log-volume"
	DefaultLogDestination            = "/log"
	DefaultConfigMountPath           = "/config"
	DefaultConfigVolumeName          = "config-volume"
	DefaultWalletVolumeString        = "creds"
	DefaultOCIConfigVolumeName       = "oci-config-volume"
	DefaultOCIConfigFingerprintKey   = "fingerprint"
	DefaultOCIConfigRegionKey        = "region"
	DefaultOCIConfigTenancyKey       = "tenancy"
	DefaultOCIConfigUserKey          = "user"
	DefaultAzureConfigTenantId       = "tenantId"
	DefaultAzureConfigClientId       = "clientId"
	DefaultAzureConfigClientSecret   = "clientSecret"
	DefaultEnvPasswordSuffix         = "_PASSWORD"
	DefaultEnvUserSuffix             = "_USERNAME"
	DefaultEnvConnectionStringSuffix = "_CONNECT_STRING"

	DefaultExporterImage               = "container-registry.oracle.com/database/observability-exporter:2.0.2"
	DefaultServicePort                 = 9161
	DefaultServiceTargetPort           = 9161
	DefaultAppPort                     = 8080
	DefaultPrometheusPort              = "metrics"
	DefaultServiceType                 = "ClusterIP"
	DefaultReplicaCount                = 1
	DefaultExporterConfigMountRootPath = "/oracle/observability"
	DefaultOracleHome                  = "/lib/oracle/21/client64/lib"
	DefaultOracleTNSAdmin              = DefaultOracleHome + "/network/admin"
	DefaultOCIConfigPath               = "/.oci"
	DefaultPrivateKeyFileName          = "private.pem"
	DefaultVaultPrivateKeyAbsolutePath = DefaultOCIConfigPath + "/" + DefaultPrivateKeyFileName
	DefaultExporterContainerName       = "exporter"
	DefaultSelectorLabelKey            = "app"
)

// Known environment variables
const (
	EnvVarOracleHome                   = "ORACLE_HOME"
	EnvVarDataSourceUsername           = "DB_USERNAME"
	EnvVarDataSourcePassword           = "DB_PASSWORD"
	EnvVarDataSourceConnectString      = "DB_CONNECT_STRING"
	EnvVarDataSourceLogDestination     = "LOG_DESTINATION"
	EnvVarDataSourcePwdVaultSecretName = "OCI_VAULT_SECRET_NAME"
	EnvVarDataSourcePwdVaultId         = "OCI_VAULT_ID"
	EnvVarCustomConfigmap              = "CUSTOM_METRICS"
	EnvVarTNSAdmin                     = "TNS_ADMIN"
	EnvVarOCIVaultTenancyOCID          = "OCI_CLI_TENANCY"
	EnvVarOCIVaultUserOCID             = "OCI_CLI_USER"
	EnvVarOCIVaultFingerprint          = "OCI_CLI_FINGERPRINT"
	EnvVarOCIVaultPrivateKeyPath       = "OCI_CLI_KEY_FILE"
	EnvVarOCIVaultRegion               = "OCI_CLI_REGION"
	EnvVarAzureVaultPasswordSecret     = "AZ_VAULT_PASSWORD_SECRET"
	EnvVarAzureVaultUsernameSecret     = "AZ_VAULT_USERNAME_SECRET"
	EnvVarAzureVaultID                 = "AZ_VAULT_ID"
	EnvVarAzureTenantID                = "AZURE_TENANT_ID"
	EnvVarAzureClientID                = "AZURE_CLIENT_ID"
	EnvVarAzureClientSecret            = "AZURE_CLIENT_SECRET"
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

	ReasonDeploymentSuccessful = "ResourceDeployed"
	ReasonResourceUpdated      = "ResourceUpdated"
	ReasonResourceUpdateFailed = "ResourceUpdateFailed"
	ReasonDeploymentFailed     = "ResourceDeploymentFailed"
	ReasonDeploymentPending    = "ResourceDeploymentInProgress"

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
	ErrorSpecValidationFailedDueToAnError     = "an error occurred with validating the exporter deployment spec"
	ErrorDeploymentPodsFailure                = "an error occurred with deploying exporter deployment pods"
	ErrorResourceCreationFailure              = "an error occurred with creating databaseobserver resource"
	ErrorResourceRetrievalFailureDueToAnError = "an error occurred with retrieving databaseobserver resource"
	LogErrorWithResourceUpdate                = "an error occurred with updating resource"
)

// Log Infos
const (
	LogCRStart                   = "Started DatabaseObserver instance reconciliation"
	LogCREnd                     = "Ended DatabaseObserver instance reconciliation, resource must have been deleted."
	LogResourceCreated           = "Created DatabaseObserver resource successfully"
	LogResourceUpdated           = "Updated DatabaseObserver resource successfully"
	LogResourceFound             = "Validated DatabaseObserver resource readiness"
	LogSuccessWithResourceUpdate = "Updated DatabaseObserver resource successfully"
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
	MessageExporterResourceUpdateFailed           = "Failed to update exporter resource due to an error"
	MessageExporterResourceUpdated                = "Updated exporter resource successfully"
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
	EventMessageSpecErrorConfigMapSpecifiedMissing       = "Spec validation failed due to referenced configMap not found"
	EventMessageSpecErrorDBConnectionStringSecretMissing = "Spec validation failed due to required dbConnectionString secret not found"
	EventMessageSpecErrorDBPUserSecretMissing            = "Spec validation failed due to dbUser secret not found"
	EventMessageSpecErrorDBPwdSecretMissing              = "Spec validation failed due to dbPassword secret not found"
	EventMessageSpecErrorDBWalletSecretMissing           = "Spec validation failed due to provided dbWallet secret not found"

	EventReasonUpdateSucceeded = "ExporterDeploymentUpdated"
)
