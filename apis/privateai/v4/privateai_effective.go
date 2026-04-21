package v4

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// EffectivePrivateAiSpec is the normalized compatibility view used by the
// controller and builders while both the old flat fields and the new grouped
// fields are supported.
type EffectivePrivateAiSpec struct {
	Security      EffectivePrivateAiSecurity
	Runtime       EffectivePrivateAiRuntime
	Configuration EffectivePrivateAiConfiguration
	Storage       EffectivePrivateAiStorage
	Networking    EffectivePrivateAiNetworking
	Logging       *LoggingSpec
}

type EffectivePrivateAiSecurity struct {
	AuthEnabled bool
	Secret      *PaiSecretSpec
	TLS         *PaiTLSSpec
}

type EffectivePrivateAiRuntime struct {
	Image                *PrivateAiImageSpec
	Replicas             int32
	Env                  []EnvironmentVariable
	Resources            *corev1.ResourceRequirements
	Debug                bool
	ReadinessCheckPeriod int
	LivenessCheckPeriod  int
	DownloadScripts      bool
	WorkerNodes          []string
}

type EffectivePrivateAiConfiguration struct {
	ConfigFile *PaiConfigMap
}

type EffectivePrivateAiStorage struct {
	StorageClass      string
	PvcList           map[string]string
	SizeInGb          int32
	DeletePvcOnDelete bool
	LogLocation       string
}

type EffectivePrivateAiNetworking struct {
	Listeners        EffectivePrivateAiListeners
	Service          *PaiServiceSpec
	NodePortServices []PaiNodePortSvc
	TrafficManager   *TrafficManagerRefSpec
}

type EffectivePrivateAiListeners struct {
	HTTP  EffectivePrivateAiListener
	HTTPS EffectivePrivateAiListener
}

type EffectivePrivateAiListener struct {
	Enabled bool
	Port    int32
}

// EffectivePrivateAiSpecFromSpec returns the normalized compatibility view for
// the provided PrivateAI spec. New hierarchical fields take precedence and
// deprecated flat fields are used as fallback only.
func EffectivePrivateAiSpecFromSpec(spec *PrivateAiSpec) *EffectivePrivateAiSpec {
	if spec == nil {
		return &EffectivePrivateAiSpec{}
	}

	effective := &EffectivePrivateAiSpec{
		Security: EffectivePrivateAiSecurity{
			AuthEnabled: effectiveAuthEnabled(spec),
			Secret:      effectiveAuthSecret(spec),
			TLS:         effectiveTLS(spec),
		},
		Runtime: EffectivePrivateAiRuntime{
			Image:                effectiveImage(spec),
			Replicas:             effectiveReplicas(spec),
			Env:                  copyEnvironmentVariables(effectiveEnv(spec)),
			Resources:            copyResourceRequirements(effectiveResources(spec)),
			Debug:                effectiveDebug(spec),
			ReadinessCheckPeriod: effectiveReadinessCheckPeriod(spec),
			LivenessCheckPeriod:  effectiveLivenessCheckPeriod(spec),
			DownloadScripts:      effectiveDownloadScripts(spec),
			WorkerNodes:          copyStringSlice(effectiveWorkerNodes(spec)),
		},
		Configuration: EffectivePrivateAiConfiguration{
			ConfigFile: effectiveConfigFile(spec),
		},
		Storage: EffectivePrivateAiStorage{
			StorageClass:      effectiveStorageClass(spec),
			PvcList:           copyStringMap(effectivePVCList(spec)),
			SizeInGb:          effectiveStorageSizeInGb(spec),
			DeletePvcOnDelete: effectiveDeletePvcOnDelete(spec),
			LogLocation:       effectiveLogLocation(spec),
		},
		Networking: EffectivePrivateAiNetworking{
			Listeners:        effectiveListeners(spec),
			Service:          effectiveService(spec),
			NodePortServices: copyNodePortServices(effectiveNodePortServices(spec)),
			TrafficManager:   effectiveTrafficManager(spec),
		},
		Logging: copyLoggingSpec(spec.Logging),
	}

	return effective
}

// EffectiveAuthSecret returns the preferred auth secret configuration, favoring
// spec.security.secret over the deprecated spec.paiSecret field.
func EffectiveAuthSecret(spec *PrivateAiSpec) *PaiSecretSpec {
	return EffectivePrivateAiSpecFromSpec(spec).Security.Secret
}

// EffectiveTLS returns the configured TLS secret settings for PrivateAI HTTPS.
func EffectiveTLS(spec *PrivateAiSpec) *PaiTLSSpec {
	return EffectivePrivateAiSpecFromSpec(spec).Security.TLS
}

// EffectiveAuthEnabled returns the preferred authentication enabled state.
func EffectiveAuthEnabled(spec *PrivateAiSpec) bool {
	return EffectivePrivateAiSpecFromSpec(spec).Security.AuthEnabled
}

// EffectiveConfigFile returns the preferred config file mount definition.
func EffectiveConfigFile(spec *PrivateAiSpec) *PaiConfigMap {
	return EffectivePrivateAiSpecFromSpec(spec).Configuration.ConfigFile
}

// EffectiveImage returns the preferred PrivateAI image configuration.
func EffectiveImage(spec *PrivateAiSpec) *PrivateAiImageSpec {
	return EffectivePrivateAiSpecFromSpec(spec).Runtime.Image
}

// EffectiveReplicas returns the preferred replica count.
func EffectiveReplicas(spec *PrivateAiSpec) int32 {
	return EffectivePrivateAiSpecFromSpec(spec).Runtime.Replicas
}

// EffectiveEnvVars returns the preferred environment variables.
func EffectiveEnvVars(spec *PrivateAiSpec) []EnvironmentVariable {
	return EffectivePrivateAiSpecFromSpec(spec).Runtime.Env
}

// EffectiveResources returns the preferred container resource requirements.
func EffectiveResources(spec *PrivateAiSpec) *corev1.ResourceRequirements {
	return EffectivePrivateAiSpecFromSpec(spec).Runtime.Resources
}

// EffectiveDebug returns the preferred debug flag.
func EffectiveDebug(spec *PrivateAiSpec) bool {
	return EffectivePrivateAiSpecFromSpec(spec).Runtime.Debug
}

// EffectiveWorkerNodes returns the preferred worker node list.
func EffectiveWorkerNodes(spec *PrivateAiSpec) []string {
	return EffectivePrivateAiSpecFromSpec(spec).Runtime.WorkerNodes
}

// EffectiveStorageClass returns the preferred storage class.
func EffectiveStorageClass(spec *PrivateAiSpec) string {
	return EffectivePrivateAiSpecFromSpec(spec).Storage.StorageClass
}

// EffectivePVCList returns the preferred PVC mount mapping.
func EffectivePVCList(spec *PrivateAiSpec) map[string]string {
	return EffectivePrivateAiSpecFromSpec(spec).Storage.PvcList
}

// EffectiveStorageSizeInGb returns the preferred storage size.
func EffectiveStorageSizeInGb(spec *PrivateAiSpec) int32 {
	return EffectivePrivateAiSpecFromSpec(spec).Storage.SizeInGb
}

// EffectiveDeletePvcOnDelete returns the preferred PVC deletion flag.
func EffectiveDeletePvcOnDelete(spec *PrivateAiSpec) bool {
	return EffectivePrivateAiSpecFromSpec(spec).Storage.DeletePvcOnDelete
}

// EffectiveLogLocation returns the preferred log mount location.
func EffectiveLogLocation(spec *PrivateAiSpec) string {
	return EffectivePrivateAiSpecFromSpec(spec).Storage.LogLocation
}

// EffectiveService returns the preferred PrivateAI service configuration.
func EffectiveService(spec *PrivateAiSpec) *PaiServiceSpec {
	return EffectivePrivateAiSpecFromSpec(spec).Networking.Service
}

// EffectiveNodePortServices returns the preferred nodeport service definitions.
func EffectiveNodePortServices(spec *PrivateAiSpec) []PaiNodePortSvc {
	return EffectivePrivateAiSpecFromSpec(spec).Networking.NodePortServices
}

// EffectiveTrafficManager returns the preferred TrafficManager reference.
func EffectiveTrafficManager(spec *PrivateAiSpec) *TrafficManagerRefSpec {
	return EffectivePrivateAiSpecFromSpec(spec).Networking.TrafficManager
}

// EffectiveListeners returns the preferred listener settings.
func EffectiveListeners(spec *PrivateAiSpec) EffectivePrivateAiListeners {
	return EffectivePrivateAiSpecFromSpec(spec).Networking.Listeners
}

func effectiveAuthEnabled(spec *PrivateAiSpec) bool {
	if spec.Security != nil && spec.Security.AuthEnabled != nil {
		return *spec.Security.AuthEnabled
	}
	return boolFromFlag(spec.PaiEnableAuthentication)
}

func effectiveAuthSecret(spec *PrivateAiSpec) *PaiSecretSpec {
	if spec.Security != nil && spec.Security.Secret != nil {
		return copyPaiSecretSpec(spec.Security.Secret)
	}
	return copyPaiSecretSpec(spec.PaiSecret)
}

func effectiveTLS(spec *PrivateAiSpec) *PaiTLSSpec {
	if spec.Security == nil || spec.Security.TLS == nil {
		return nil
	}
	return copyPaiTLSSpec(spec.Security.TLS)
}

func effectiveImage(spec *PrivateAiSpec) *PrivateAiImageSpec {
	if spec.Runtime != nil && spec.Runtime.Image != nil {
		image := copyPrivateAiImageSpec(spec.Runtime.Image)
		if image.PullPolicy == "" {
			image.PullPolicy = corev1.PullIfNotPresent
		}
		return image
	}
	if strings.TrimSpace(spec.PaiImage) == "" && strings.TrimSpace(spec.PaiImagePullSecret) == "" {
		return nil
	}
	return &PrivateAiImageSpec{
		Name:       spec.PaiImage,
		PullPolicy: corev1.PullIfNotPresent,
		PullSecret: spec.PaiImagePullSecret,
	}
}

func effectiveReplicas(spec *PrivateAiSpec) int32 {
	if spec.Runtime != nil && spec.Runtime.Replicas != nil {
		return *spec.Runtime.Replicas
	}
	return spec.Replicas
}

func effectiveEnv(spec *PrivateAiSpec) []EnvironmentVariable {
	if spec.Runtime != nil && spec.Runtime.Env != nil {
		return spec.Runtime.Env
	}
	return spec.EnvVars
}

func effectiveResources(spec *PrivateAiSpec) *corev1.ResourceRequirements {
	if spec.Runtime != nil && spec.Runtime.Resources != nil {
		return spec.Runtime.Resources.DeepCopy()
	}
	if spec.Resources == nil {
		return nil
	}
	return spec.Resources.DeepCopy()
}

func effectiveDebug(spec *PrivateAiSpec) bool {
	if spec.Runtime != nil && spec.Runtime.Debug != nil {
		return *spec.Runtime.Debug
	}
	return spec.IsDebug
}

func effectiveReadinessCheckPeriod(spec *PrivateAiSpec) int {
	if spec.Runtime != nil && spec.Runtime.ReadinessCheckPeriod != nil {
		return *spec.Runtime.ReadinessCheckPeriod
	}
	return spec.ReadinessCheckPeriod
}

func effectiveLivenessCheckPeriod(spec *PrivateAiSpec) int {
	if spec.Runtime != nil && spec.Runtime.LivenessCheckPeriod != nil {
		return *spec.Runtime.LivenessCheckPeriod
	}
	return spec.LivenessCheckPeriod
}

func effectiveDownloadScripts(spec *PrivateAiSpec) bool {
	if spec.Runtime != nil && spec.Runtime.DownloadScripts != nil {
		return *spec.Runtime.DownloadScripts
	}
	return spec.IsDownloadScripts
}

func effectiveWorkerNodes(spec *PrivateAiSpec) []string {
	if spec.Runtime != nil && spec.Runtime.WorkerNodes != nil {
		return spec.Runtime.WorkerNodes
	}
	return spec.WorkerNodes
}

func effectiveConfigFile(spec *PrivateAiSpec) *PaiConfigMap {
	if spec.Configuration != nil && spec.Configuration.ConfigFile != nil {
		return copyPaiConfigMap(spec.Configuration.ConfigFile)
	}
	return copyPaiConfigMap(spec.PaiConfigFile)
}

func effectiveStorageClass(spec *PrivateAiSpec) string {
	if spec.Storage != nil && strings.TrimSpace(spec.Storage.StorageClass) != "" {
		return spec.Storage.StorageClass
	}
	return spec.StorageClass
}

func effectivePVCList(spec *PrivateAiSpec) map[string]string {
	if spec.Storage != nil && spec.Storage.PvcList != nil {
		return spec.Storage.PvcList
	}
	return spec.PvcList
}

func effectiveStorageSizeInGb(spec *PrivateAiSpec) int32 {
	if spec.Storage != nil && spec.Storage.SizeInGb != nil {
		return *spec.Storage.SizeInGb
	}
	return spec.StorageSizeInGb
}

func effectiveDeletePvcOnDelete(spec *PrivateAiSpec) bool {
	if spec.Storage != nil && spec.Storage.DeletePvcOnDelete != nil {
		return *spec.Storage.DeletePvcOnDelete
	}
	return spec.IsDeleteOraPvc
}

func effectiveLogLocation(spec *PrivateAiSpec) string {
	if spec.Storage != nil && strings.TrimSpace(spec.Storage.LogLocation) != "" {
		return spec.Storage.LogLocation
	}
	return spec.PaiLogLocation
}

func effectiveService(spec *PrivateAiSpec) *PaiServiceSpec {
	if spec.Networking != nil && spec.Networking.Service != nil {
		return spec.Networking.Service.DeepCopy()
	}

	var service *PaiServiceSpec
	if !isPaiServiceSpecZero(spec.PaiService) {
		service = spec.PaiService.DeepCopy()
	}
	if len(spec.PortMappings) > 0 {
		if service == nil {
			service = &PaiServiceSpec{}
		}
		if len(service.PortMappings) == 0 {
			service.PortMappings = append([]PaiPortMapping(nil), spec.PortMappings...)
		}
	}
	if service == nil || isPaiServiceSpecZero(*service) {
		return nil
	}
	return service
}

func effectiveNodePortServices(spec *PrivateAiSpec) []PaiNodePortSvc {
	if spec.Networking != nil && spec.Networking.NodePortServices != nil {
		return spec.Networking.NodePortServices
	}
	return spec.NodePortSvc
}

func effectiveTrafficManager(spec *PrivateAiSpec) *TrafficManagerRefSpec {
	if spec.Networking != nil && spec.Networking.TrafficManager != nil {
		return copyTrafficManagerRefSpec(spec.Networking.TrafficManager)
	}
	return copyTrafficManagerRefSpec(spec.TrafficManager)
}

func effectiveListeners(spec *PrivateAiSpec) EffectivePrivateAiListeners {
	listeners := EffectivePrivateAiListeners{
		HTTP: EffectivePrivateAiListener{
			Enabled: boolFromFlag(spec.PaiHTTPEnabled),
			Port:    spec.PaiHTTPPort,
		},
		HTTPS: EffectivePrivateAiListener{
			Enabled: boolFromFlag(spec.PaiHTTPSEnabled),
			Port:    spec.PaiHTTPSPort,
		},
	}

	if spec.Networking == nil || spec.Networking.Listeners == nil {
		return listeners
	}
	if spec.Networking.Listeners.HTTP != nil {
		if spec.Networking.Listeners.HTTP.Enabled != nil {
			listeners.HTTP.Enabled = *spec.Networking.Listeners.HTTP.Enabled
		}
		if spec.Networking.Listeners.HTTP.Port != nil {
			listeners.HTTP.Port = *spec.Networking.Listeners.HTTP.Port
		}
	}
	if spec.Networking.Listeners.HTTPS != nil {
		if spec.Networking.Listeners.HTTPS.Enabled != nil {
			listeners.HTTPS.Enabled = *spec.Networking.Listeners.HTTPS.Enabled
		}
		if spec.Networking.Listeners.HTTPS.Port != nil {
			listeners.HTTPS.Port = *spec.Networking.Listeners.HTTPS.Port
		}
	}
	return listeners
}

func boolFromFlag(flag string) bool {
	val := strings.TrimSpace(flag)
	return strings.EqualFold(val, "true")
}

func copyPaiSecretSpec(in *PaiSecretSpec) *PaiSecretSpec {
	if in == nil {
		return nil
	}
	out := *in
	if in.Items != nil {
		out.Items = append([]SecretMountItem(nil), in.Items...)
	}
	return &out
}

func copyPaiTLSSpec(in *PaiTLSSpec) *PaiTLSSpec {
	if in == nil {
		return nil
	}
	out := *in
	if in.Items != nil {
		out.Items = append([]SecretMountItem(nil), in.Items...)
	}
	return &out
}

func copyPrivateAiImageSpec(in *PrivateAiImageSpec) *PrivateAiImageSpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func copyPaiConfigMap(in *PaiConfigMap) *PaiConfigMap {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func copyTrafficManagerRefSpec(in *TrafficManagerRefSpec) *TrafficManagerRefSpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func copyLoggingSpec(in *LoggingSpec) *LoggingSpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func copyEnvironmentVariables(in []EnvironmentVariable) []EnvironmentVariable {
	if in == nil {
		return nil
	}
	out := make([]EnvironmentVariable, len(in))
	copy(out, in)
	return out
}

func copyStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyNodePortServices(in []PaiNodePortSvc) []PaiNodePortSvc {
	if in == nil {
		return nil
	}
	out := make([]PaiNodePortSvc, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].PortMappings != nil {
			out[i].PortMappings = append([]PaiPortMapping(nil), in[i].PortMappings...)
		}
	}
	return out
}

func copyResourceRequirements(in *corev1.ResourceRequirements) *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func isPaiServiceSpecZero(in PaiServiceSpec) bool {
	return len(in.PortMappings) == 0 && in.SvcName == "" && in.SvcType == "" && in.External == nil
}
