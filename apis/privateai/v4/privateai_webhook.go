package v4

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
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

	httpEnabled, _ := strconv.ParseBool(privateai.Spec.PaiHTTPEnabled)
	httpsEnabled, _ := strconv.ParseBool(privateai.Spec.PaiHTTPSEnabled)
	if httpEnabled && httpsEnabled {
		return fmt.Errorf("paiHTTPEnabled and paiHTTPSEnabled both cannot be true")
	}
	if !httpEnabled {
		privateai.Spec.PaiHTTPEnabled = "false"
		privateai.Spec.PaiHTTPPort = 0
		privateai.Spec.PaiHTTPSEnabled = "true"
		if privateai.Spec.PaiHTTPSPort == 0 {
			privateai.Spec.PaiHTTPSPort = 8443
		}
	} else {
		privateai.Spec.PaiHTTPEnabled = "true"
		if privateai.Spec.PaiHTTPPort == 0 {
			privateai.Spec.PaiHTTPPort = 8080
		}
		privateai.Spec.PaiHTTPSEnabled = "false"
		privateai.Spec.PaiHTTPSPort = 0
	}

	if len(privateai.Spec.PaiService.PortMappings) == 0 {
		targetPort := privateai.Spec.PaiHTTPSPort
		if httpEnabled {
			targetPort = privateai.Spec.PaiHTTPPort
		}
		privateai.Spec.PaiService.PortMappings = []PaiPortMapping{{
			Port:       443,
			TargetPort: targetPort,
			Protocol:   "TCP",
		}}
	}
	if privateai.Spec.PaiConfigFile != nil && privateai.Spec.PaiConfigFile.MountLocation == "" {
		privateai.Spec.PaiConfigFile.MountLocation = "/privateai/config"
	}
	if privateai.Spec.PaiSecret != nil && privateai.Spec.PaiSecret.MountLocation == "" {
		privateai.Spec.PaiSecret.MountLocation = "/privateai/ssl"
	}
	if privateai.Spec.PaiService.External != nil && privateai.Spec.PaiService.External.ServiceType == "" {
		privateai.Spec.PaiService.External.ServiceType = "LoadBalancer"
	}
	if privateai.Spec.PaiService.External != nil && privateai.Spec.PaiService.External.ExternalTrafficPolicy == "" {
		privateai.Spec.PaiService.External.ExternalTrafficPolicy = "Cluster"
	}
	if privateai.Spec.TrafficManager != nil &&
		strings.TrimSpace(privateai.Spec.TrafficManager.Ref) != "" &&
		strings.TrimSpace(privateai.Spec.TrafficManager.RoutePath) == "" {
		privateai.Spec.TrafficManager.RoutePath = fmt.Sprintf("/%s/v1/", strings.ToLower(strings.TrimSpace(privateai.Name)))
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

	authEnabled, _ := strconv.ParseBool(privateai.Spec.PaiEnableAuthentication)
	if authEnabled && (privateai.Spec.PaiSecret == nil || privateai.Spec.PaiSecret.Name == "") {
		return warnings, fmt.Errorf("paiEnableAuthentication=true requires paiSecret.name to be set")
	}
	if privateai.Spec.PaiSecret != nil && strings.TrimSpace(privateai.Spec.PaiSecret.MountLocation) != "" &&
		!filepath.IsAbs(strings.TrimSpace(privateai.Spec.PaiSecret.MountLocation)) {
		return warnings, fmt.Errorf("paiSecret.mountLocation must be an absolute path")
	}
	if privateai.Spec.PaiConfigFile != nil && strings.TrimSpace(privateai.Spec.PaiConfigFile.MountLocation) != "" &&
		!filepath.IsAbs(strings.TrimSpace(privateai.Spec.PaiConfigFile.MountLocation)) {
		return warnings, fmt.Errorf("paiConfigFile.mountLocation must be an absolute path")
	}
	if privateai.Spec.PaiService.External != nil {
		if policy := strings.TrimSpace(privateai.Spec.PaiService.External.ExternalTrafficPolicy); policy != "" &&
			!strings.EqualFold(policy, "Local") && !strings.EqualFold(policy, "Cluster") {
			return warnings, fmt.Errorf("spec.paiService.external.externalTrafficPolicy must be Local or Cluster")
		}
	}
	if privateai.Spec.TrafficManager != nil {
		if strings.TrimSpace(privateai.Spec.TrafficManager.Ref) == "" {
			return warnings, fmt.Errorf("spec.trafficManager.ref must be set when spec.trafficManager is provided")
		}
	}
	if privateai.Spec.TrafficManager != nil && strings.TrimSpace(privateai.Spec.TrafficManager.Ref) != "" {
		if v.Client == nil {
			return warnings, fmt.Errorf("traffic manager reference validation is not configured")
		}
		trafficManager := &networkv4.TrafficManager{}
		ref := strings.TrimSpace(privateai.Spec.TrafficManager.Ref)
		if err := v.Client.Get(ctx, types.NamespacedName{Name: ref, Namespace: privateai.Namespace}, trafficManager); err != nil {
			if apierrors.IsNotFound(err) {
				return warnings, fmt.Errorf("spec.trafficManager.ref %q not found", ref)
			}
			return warnings, err
		}
		if trafficManager.Spec.Type != networkv4.TrafficManagerTypeNginx {
			return warnings, fmt.Errorf("spec.trafficManager.ref %q points to unsupported TrafficManager type %q", ref, trafficManager.Spec.Type)
		}
		if path := strings.TrimSpace(privateai.Spec.TrafficManager.RoutePath); path != "" {
			if !strings.HasPrefix(path, "/") || !strings.HasSuffix(path, "/") {
				return warnings, fmt.Errorf("spec.trafficManager.routePath must start and end with '/'")
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
		appendWarn("spec.isExternalSvc is deprecated; use spec.paiService.external.enabled")
	}
	if spec.PaiLBPort != 0 {
		appendWarn("spec.paiLBPort is deprecated; use spec.paiService.external.port")
	}
	if strings.TrimSpace(spec.PaiLBIP) != "" {
		appendWarn("spec.paiLBIP is deprecated; use spec.paiService.external.loadBalancerIP")
	}
	if strings.TrimSpace(spec.PaiLBExternalTrafficPolicy) != "" {
		appendWarn("spec.paiLBExternalTrafficPolicy is deprecated; use spec.paiService.external.externalTrafficPolicy")
	}
	if strings.TrimSpace(spec.PaiInternalLB) != "" {
		appendWarn("spec.paiInternalLB is deprecated; use spec.paiService.external.internal")
	}
	if len(spec.PailbAnnotation) > 0 {
		appendWarn("spec.pailbAnnotation is deprecated; use spec.paiService.external.annotations")
	}

	return warnings
}
