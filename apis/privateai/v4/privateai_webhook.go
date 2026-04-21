package v4

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var privateailog = logf.Log.WithName("privateai-resource")

func (r *PrivateAi) SetupPrivateAiWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy[*PrivateAi](mgr, r).
		WithValidator(&privateAiValidator{Client: mgr.GetClient()}).
		WithDefaulter(&PrivateAi{}).
		Complete()
}

var _ admission.Defaulter[*PrivateAi] = &PrivateAi{}
var _ admission.Validator[*PrivateAi] = &privateAiValidator{}

type privateAiValidator struct {
	Client client.Client
}

// +kubebuilder:webhook:path=/mutate-privateai-oracle-com-v4-privateai,mutating=true,failurePolicy=fail,sideEffects=None,groups=privateai.oracle.com,resources=privateais,verbs=create;update,versions=v4,name=mprivateai-v4.kb.io,admissionReviewVersions=v1
func (r *PrivateAi) Default(_ context.Context, obj *PrivateAi) error {
	privateai := obj
	privateailog.Info("Defaulting for PrivateAi", "name", privateai.GetName())

	listeners := EffectiveListeners(&privateai.Spec)
	httpEnabled := listeners.HTTP.Enabled
	httpsEnabled := listeners.HTTPS.Enabled
	if httpEnabled && httpsEnabled {
		return fmt.Errorf("spec.networking.listeners.http.enabled and spec.networking.listeners.https.enabled cannot both be true (deprecated paiHTTPEnabled/paiHTTPSEnabled are also supported)")
	}
	if !httpEnabled {
		httpsPort := listeners.HTTPS.Port
		if httpsPort == 0 {
			httpsPort = 8443
		}
		privateai.Spec.PaiHTTPEnabled = "false"
		privateai.Spec.PaiHTTPPort = 0
		privateai.Spec.PaiHTTPSEnabled = "true"
		privateai.Spec.PaiHTTPSPort = httpsPort
		setDefaultPrivateAIHTTPListener(privateai, false, 0)
		setDefaultPrivateAIHTTPSListener(privateai, true, httpsPort)
	} else {
		httpPort := listeners.HTTP.Port
		if httpPort == 0 {
			httpPort = 8080
		}
		privateai.Spec.PaiHTTPEnabled = "true"
		privateai.Spec.PaiHTTPPort = httpPort
		privateai.Spec.PaiHTTPSEnabled = "false"
		privateai.Spec.PaiHTTPSPort = 0
		setDefaultPrivateAIHTTPListener(privateai, true, httpPort)
		setDefaultPrivateAIHTTPSListener(privateai, false, 0)
	}

	service := EffectiveService(&privateai.Spec)
	if service == nil || len(service.PortMappings) == 0 {
		targetPort := privateai.Spec.PaiHTTPSPort
		if httpEnabled {
			targetPort = privateai.Spec.PaiHTTPPort
		}
		defaultMappings := []PaiPortMapping{{
			Port:       443,
			TargetPort: targetPort,
			Protocol:   "TCP",
		}}
		if privateai.Spec.Networking != nil && privateai.Spec.Networking.Service != nil {
			privateai.Spec.Networking.Service.PortMappings = defaultMappings
		} else {
			privateai.Spec.PaiService.PortMappings = defaultMappings
		}
	}
	if cfg := EffectiveConfigFile(&privateai.Spec); cfg != nil && cfg.MountLocation == "" {
		cfg.MountLocation = "/privateai/config"
		if privateai.Spec.Configuration != nil && privateai.Spec.Configuration.ConfigFile != nil {
			privateai.Spec.Configuration.ConfigFile.MountLocation = cfg.MountLocation
		}
		if privateai.Spec.PaiConfigFile != nil {
			privateai.Spec.PaiConfigFile.MountLocation = cfg.MountLocation
		}
	}
	if privateai.Spec.PaiConfigFile != nil && privateai.Spec.PaiConfigFile.MountLocation == "" {
		privateai.Spec.PaiConfigFile.MountLocation = "/privateai/config"
	}
	if privateai.Spec.Security != nil && privateai.Spec.Security.Secret != nil && privateai.Spec.Security.Secret.MountLocation == "" {
		privateai.Spec.Security.Secret.MountLocation = "/privateai/auth"
	}
	if privateai.Spec.Security != nil && privateai.Spec.Security.TLS != nil && privateai.Spec.Security.TLS.MountLocation == "" {
		privateai.Spec.Security.TLS.MountLocation = "/privateai/tls"
	}
	if privateai.Spec.PaiSecret != nil && privateai.Spec.PaiSecret.MountLocation == "" {
		privateai.Spec.PaiSecret.MountLocation = "/privateai/ssl"
	}
	if privateai.Spec.Networking != nil && privateai.Spec.Networking.Service != nil &&
		privateai.Spec.Networking.Service.External != nil && privateai.Spec.Networking.Service.External.ServiceType == "" {
		privateai.Spec.Networking.Service.External.ServiceType = "LoadBalancer"
	}
	if privateai.Spec.Networking != nil && privateai.Spec.Networking.Service != nil &&
		privateai.Spec.Networking.Service.External != nil && privateai.Spec.Networking.Service.External.ExternalTrafficPolicy == "" {
		privateai.Spec.Networking.Service.External.ExternalTrafficPolicy = "Cluster"
	}
	if privateai.Spec.PaiService.External != nil && privateai.Spec.PaiService.External.ServiceType == "" {
		privateai.Spec.PaiService.External.ServiceType = "LoadBalancer"
	}
	if privateai.Spec.PaiService.External != nil && privateai.Spec.PaiService.External.ExternalTrafficPolicy == "" {
		privateai.Spec.PaiService.External.ExternalTrafficPolicy = "Cluster"
	}
	if tm := EffectiveTrafficManager(&privateai.Spec); tm != nil &&
		strings.TrimSpace(tm.Ref) != "" &&
		strings.TrimSpace(tm.RoutePath) == "" {
		defaultRoute := fmt.Sprintf("/%s/v1/", strings.ToLower(strings.TrimSpace(privateai.Name)))
		if privateai.Spec.Networking != nil && privateai.Spec.Networking.TrafficManager != nil {
			privateai.Spec.Networking.TrafficManager.RoutePath = defaultRoute
		} else if privateai.Spec.TrafficManager != nil {
			privateai.Spec.TrafficManager.RoutePath = defaultRoute
		}
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-privateai-oracle-com-v4-privateai,mutating=false,failurePolicy=fail,sideEffects=None,groups=privateai.oracle.com,resources=privateais,verbs=create;update,versions=v4,name=vprivateai-v4.kb.io,admissionReviewVersions=v1
func (v *privateAiValidator) ValidateCreate(ctx context.Context, obj *PrivateAi) (admission.Warnings, error) {
	return v.validate(ctx, obj)
}

func (v *privateAiValidator) ValidateUpdate(ctx context.Context, _ *PrivateAi, newObj *PrivateAi) (admission.Warnings, error) {
	return v.validate(ctx, newObj)
}

func (v *privateAiValidator) ValidateDelete(_ context.Context, obj *PrivateAi) (admission.Warnings, error) {
	privateailog.Info("Validation for PrivateAi upon deletion", "name", obj.GetName())
	return nil, nil
}

func (v *privateAiValidator) validate(ctx context.Context, privateai *PrivateAi) (admission.Warnings, error) {
	privateailog.Info("Validation for PrivateAi", "name", privateai.GetName())
	warnings := deprecatedPrivateAIFieldWarnings(&privateai.Spec)
	authSecret := EffectiveAuthSecret(&privateai.Spec)
	tls := EffectiveTLS(&privateai.Spec)
	configFile := EffectiveConfigFile(&privateai.Spec)
	service := EffectiveService(&privateai.Spec)
	trafficManager := EffectiveTrafficManager(&privateai.Spec)
	authSecretItemsField := effectiveAuthSecretItemsField(&privateai.Spec)

	authEnabled := EffectiveAuthEnabled(&privateai.Spec)
	if authEnabled && (authSecret == nil || strings.TrimSpace(authSecret.Name) == "") {
		return warnings, fmt.Errorf("spec.security.authEnabled=true requires spec.security.secret.name or deprecated paiSecret.name to be set")
	}
	if privateai.Spec.Security != nil && privateai.Spec.Security.Secret != nil &&
		strings.TrimSpace(privateai.Spec.Security.Secret.MountLocation) != "" &&
		!filepath.IsAbs(strings.TrimSpace(privateai.Spec.Security.Secret.MountLocation)) {
		return warnings, fmt.Errorf("spec.security.secret.mountLocation must be an absolute path")
	}
	if authSecret != nil {
		if err := validateSecretMountItems(authSecret.Items, authSecretItemsField); err != nil {
			return warnings, err
		}
	}
	if privateai.Spec.Security == nil || privateai.Spec.Security.Secret == nil {
		if privateai.Spec.PaiSecret != nil && strings.TrimSpace(privateai.Spec.PaiSecret.MountLocation) != "" &&
			!filepath.IsAbs(strings.TrimSpace(privateai.Spec.PaiSecret.MountLocation)) {
			return warnings, fmt.Errorf("paiSecret.mountLocation must be an absolute path")
		}
		if privateai.Spec.PaiSecret != nil {
			if err := validateSecretMountItems(privateai.Spec.PaiSecret.Items, "paiSecret.items"); err != nil {
				return warnings, err
			}
		}
	}
	if tls != nil {
		if strings.TrimSpace(tls.SecretName) == "" {
			return warnings, fmt.Errorf("spec.security.tls.secretName must be set when spec.security.tls is provided")
		}
		if strings.TrimSpace(tls.MountLocation) != "" && !filepath.IsAbs(strings.TrimSpace(tls.MountLocation)) {
			return warnings, fmt.Errorf("spec.security.tls.mountLocation must be an absolute path")
		}
		if err := validateSecretMountItems(tls.Items, "spec.security.tls.items"); err != nil {
			return warnings, err
		}
	}
	if authSecret != nil && tls != nil &&
		strings.TrimSpace(authSecret.MountLocation) != "" &&
		strings.TrimSpace(authSecret.MountLocation) == strings.TrimSpace(tls.MountLocation) {
		if err := validateSharedSecretMountItems(authSecret.Items, tls.Items); err != nil {
			return warnings, err
		}
	}
	if configFile != nil && strings.TrimSpace(configFile.MountLocation) != "" &&
		!filepath.IsAbs(strings.TrimSpace(configFile.MountLocation)) {
		return warnings, fmt.Errorf("spec.configuration.configFile.mountLocation or paiConfigFile.mountLocation must be an absolute path")
	}
	if service != nil && service.External != nil {
		if policy := strings.TrimSpace(service.External.ExternalTrafficPolicy); policy != "" &&
			!strings.EqualFold(policy, "Local") && !strings.EqualFold(policy, "Cluster") {
			return warnings, fmt.Errorf("spec.networking.service.external.externalTrafficPolicy or spec.paiService.external.externalTrafficPolicy must be Local or Cluster")
		}
	}
	if trafficManager != nil {
		if strings.TrimSpace(trafficManager.Ref) == "" {
			return warnings, fmt.Errorf("spec.networking.trafficManager.ref or spec.trafficManager.ref must be set when traffic manager is provided")
		}
	}
	if trafficManager != nil && strings.TrimSpace(trafficManager.Ref) != "" {
		if v.Client == nil {
			return warnings, fmt.Errorf("traffic manager reference validation is not configured")
		}
		trafficManagerObj := &networkv4.TrafficManager{}
		ref := strings.TrimSpace(trafficManager.Ref)
		if err := v.Client.Get(ctx, types.NamespacedName{Name: ref, Namespace: privateai.Namespace}, trafficManagerObj); err != nil {
			if apierrors.IsNotFound(err) {
				return warnings, fmt.Errorf("spec.networking.trafficManager.ref or deprecated spec.trafficManager.ref %q not found", ref)
			}
			return warnings, err
		}
		if trafficManagerObj.Spec.Type != networkv4.TrafficManagerTypeNginx {
			return warnings, fmt.Errorf("spec.networking.trafficManager.ref or deprecated spec.trafficManager.ref %q points to unsupported TrafficManager type %q", ref, trafficManagerObj.Spec.Type)
		}
		if path := strings.TrimSpace(trafficManager.RoutePath); path != "" {
			if !strings.HasPrefix(path, "/") || !strings.HasSuffix(path, "/") {
				return warnings, fmt.Errorf("spec.networking.trafficManager.routePath or deprecated spec.trafficManager.routePath must start and end with '/'")
			}
		}
	}
	return warnings, nil
}

func deprecatedPrivateAIFieldWarnings(spec *PrivateAiSpec) admission.Warnings {
	warnings := make(admission.Warnings, 0)
	appendWarn := func(msg string) {
		warnings = append(warnings, msg)
	}

	if strings.TrimSpace(spec.IsExternalSvc) != "" {
		appendWarn("spec.isExternalSvc is deprecated; use spec.networking.service.external.enabled")
	}
	if spec.PaiLBPort != 0 {
		appendWarn("spec.paiLBPort is deprecated; use spec.networking.service.external.port")
	}
	if strings.TrimSpace(spec.PaiLBIP) != "" {
		appendWarn("spec.paiLBIP is deprecated; use spec.networking.service.external.loadBalancerIP")
	}
	if strings.TrimSpace(spec.PaiLBExternalTrafficPolicy) != "" {
		appendWarn("spec.paiLBExternalTrafficPolicy is deprecated; use spec.networking.service.external.externalTrafficPolicy")
	}
	if strings.TrimSpace(spec.PaiInternalLB) != "" {
		appendWarn("spec.paiInternalLB is deprecated; use spec.networking.service.external.internal")
	}
	if len(spec.PailbAnnotation) > 0 {
		appendWarn("spec.pailbAnnotation is deprecated; use spec.networking.service.external.annotations")
	}
	if spec.PaiSecret != nil {
		appendWarn("spec.paiSecret is deprecated; use spec.security.secret")
	}
	if strings.TrimSpace(spec.PaiEnableAuthentication) != "" {
		appendWarn("spec.paiEnableAuthentication is deprecated; use spec.security.authEnabled")
	}
	if spec.PaiConfigFile != nil {
		appendWarn("spec.paiConfigFile is deprecated; use spec.configuration.configFile")
	}
	if strings.TrimSpace(spec.PaiImage) != "" {
		appendWarn("spec.paiImage is deprecated; use spec.runtime.image.name")
	}
	if strings.TrimSpace(spec.PaiImagePullSecret) != "" {
		appendWarn("spec.paiImagePullSecret is deprecated; use spec.runtime.image.pullSecret")
	}
	if spec.IsDebug {
		appendWarn("spec.isDebug is deprecated; use spec.runtime.debug")
	}
	if spec.ReadinessCheckPeriod != 0 {
		appendWarn("spec.readinessCheckPeriod is deprecated; use spec.runtime.readinessCheckPeriod")
	}
	if spec.LivenessCheckPeriod != 0 {
		appendWarn("spec.livenessCheckPeriod is deprecated; use spec.runtime.livenessCheckPeriod")
	}
	if spec.IsDownloadScripts {
		appendWarn("spec.isDownloadScripts is deprecated; use spec.runtime.downloadScripts")
	}
	if spec.StorageClass != "" {
		appendWarn("spec.storageClass is deprecated; use spec.storage.storageClass")
	}
	if len(spec.PvcList) > 0 {
		appendWarn("spec.pvcList is deprecated; use spec.storage.pvcList")
	}
	if spec.StorageSizeInGb != 0 {
		appendWarn("spec.storageSizeInGb is deprecated; use spec.storage.sizeInGb")
	}
	if spec.IsDeleteOraPvc {
		appendWarn("spec.isDeleteOraPvc is deprecated; use spec.storage.deletePvcOnDelete")
	}
	if strings.TrimSpace(spec.PaiLogLocation) != "" {
		appendWarn("spec.paiLogLocation is deprecated; use spec.storage.logLocation")
	}
	if len(spec.EnvVars) > 0 {
		appendWarn("spec.envVars is deprecated; use spec.runtime.env")
	}
	if spec.Replicas != 0 {
		appendWarn("spec.replicas is deprecated; use spec.runtime.replicas")
	}
	if spec.Resources != nil {
		appendWarn("spec.resources is deprecated; use spec.runtime.resources")
	}
	if len(spec.NodePortSvc) > 0 {
		appendWarn("spec.nodePortSvc is deprecated; use spec.networking.nodePortServices")
	}
	if len(spec.PortMappings) > 0 {
		appendWarn("spec.portMappings is deprecated; use spec.networking.service.portMappings")
	}
	if len(spec.WorkerNodes) > 0 {
		appendWarn("spec.workerNodes is deprecated; use spec.runtime.workerNodes")
	}
	if spec.TrafficManager != nil {
		appendWarn("spec.trafficManager is deprecated; use spec.networking.trafficManager")
	}

	return warnings
}

func setDefaultPrivateAIHTTPListener(privateai *PrivateAi, enabled bool, port int32) {
	if privateai.Spec.Networking == nil || privateai.Spec.Networking.Listeners == nil {
		return
	}
	if privateai.Spec.Networking.Listeners.HTTP == nil {
		privateai.Spec.Networking.Listeners.HTTP = &PrivateAiListenerSpec{}
	}
	privateai.Spec.Networking.Listeners.HTTP.Enabled = boolPtr(enabled)
	privateai.Spec.Networking.Listeners.HTTP.Port = int32Ptr(port)
}

func setDefaultPrivateAIHTTPSListener(privateai *PrivateAi, enabled bool, port int32) {
	if privateai.Spec.Networking == nil || privateai.Spec.Networking.Listeners == nil {
		return
	}
	if privateai.Spec.Networking.Listeners.HTTPS == nil {
		privateai.Spec.Networking.Listeners.HTTPS = &PrivateAiListenerSpec{}
	}
	privateai.Spec.Networking.Listeners.HTTPS.Enabled = boolPtr(enabled)
	privateai.Spec.Networking.Listeners.HTTPS.Port = int32Ptr(port)
}

func boolPtr(v bool) *bool {
	return &v
}

func int32Ptr(v int32) *int32 {
	return &v
}

func effectiveAuthSecretItemsField(spec *PrivateAiSpec) string {
	if spec != nil && spec.Security != nil && spec.Security.Secret != nil {
		return "spec.security.secret.items"
	}
	return "paiSecret.items"
}

func validateSecretMountItems(items []SecretMountItem, field string) error {
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			return fmt.Errorf("%s[%d].key must be set", field, i)
		}
		resolvedPath := strings.TrimSpace(item.Path)
		if resolvedPath == "" {
			resolvedPath = key
		}
		if filepath.IsAbs(resolvedPath) {
			return fmt.Errorf("%s[%d].path must be a relative path", field, i)
		}
		if resolvedPath == "." || resolvedPath == ".." || strings.HasPrefix(resolvedPath, "../") || strings.Contains(resolvedPath, "/../") {
			return fmt.Errorf("%s[%d].path must not contain parent directory segments", field, i)
		}
		if _, exists := seen[resolvedPath]; exists {
			return fmt.Errorf("%s contains duplicate mounted path %q", field, resolvedPath)
		}
		seen[resolvedPath] = struct{}{}
	}
	return nil
}

func validateSharedSecretMountItems(authItems, tlsItems []SecretMountItem) error {
	if len(authItems) == 0 || len(tlsItems) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(authItems))
	for _, item := range authItems {
		seen[resolvedSecretMountItemPath(item)] = struct{}{}
	}
	for i, item := range tlsItems {
		resolvedPath := resolvedSecretMountItemPath(item)
		if _, exists := seen[resolvedPath]; exists {
			return fmt.Errorf("spec.security.secret.items and spec.security.tls.items cannot resolve to the same mounted path %q when mountLocation is shared (conflict at spec.security.tls.items[%d])", resolvedPath, i)
		}
	}
	return nil
}

func resolvedSecretMountItemPath(item SecretMountItem) string {
	if path := strings.TrimSpace(item.Path); path != "" {
		return path
	}
	return strings.TrimSpace(item.Key)
}
