/*
** Copyright (c) 2022 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package v4

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager registers the DataguardBroker webhook with the manager.
func (r *DataguardBroker) SetupWebhookWithManager(mgr ctrl.Manager) error {

	return ctrl.NewWebhookManagedBy[*DataguardBroker](mgr, r).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v4-dataguardbroker,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dataguardbrokers,versions=v4,name=vdataguardbrokerv4.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*DataguardBroker] = &DataguardBroker{}

var dataguardHostNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
var dataguardDBUniqueNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_$#]{0,29}$`)
var dataguardServiceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_$#.-]{0,255}$`)

// ValidateDataguardHost enforces the hostname shape used in generated TNS
// descriptors. The value is later emitted as a HOST token, so descriptor
// punctuation and multiline values are not accepted.
func ValidateDataguardHost(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("host is required")
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("must not contain control characters")
		}
	}
	ipCandidate := strings.TrimPrefix(strings.TrimSuffix(trimmed, "]"), "[")
	if net.ParseIP(ipCandidate) != nil {
		return nil
	}
	if len(trimmed) > 253 {
		return fmt.Errorf("must be no more than 253 characters")
	}
	if !dataguardHostNamePattern.MatchString(trimmed) {
		return fmt.Errorf("must be a DNS hostname, IPv4 address, or IPv6 address without TNS descriptor punctuation")
	}
	return nil
}

// ValidateDataguardDBUniqueName enforces the identifier shape used by generated
// SQL and DGMGRL scripts for Oracle DB_UNIQUE_NAME values.
func ValidateDataguardDBUniqueName(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("DB_UNIQUE_NAME is required")
	}
	if !dataguardDBUniqueNamePattern.MatchString(trimmed) {
		return fmt.Errorf("must be a valid Oracle DB_UNIQUE_NAME using 1-30 characters: start with a letter and use only letters, numbers, '_', '$', or '#'")
	}
	return nil
}

// ValidateDataguardServiceName enforces the service-name shape used by
// generated TNS entries.
func ValidateDataguardServiceName(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("serviceName is required")
	}
	if !dataguardServiceNamePattern.MatchString(trimmed) {
		return fmt.Errorf("must use 1-256 characters: start with a letter or number and use only letters, numbers, '_', '$', '#', '.', or '-'")
	}
	return nil
}

// ValidateDataguardSingleLineText rejects control characters in topology text
// values that are emitted into generated scripts or Oracle Net files.
func ValidateDataguardSingleLineText(fieldName, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s must not contain control characters", fieldName)
		}
	}
	return nil
}

// ValidateCreate validates DataguardBroker create requests.
func (r *DataguardBroker) ValidateCreate(ctx context.Context, obj *DataguardBroker) (admission.Warnings, error) {
	_ = ctx
	allErrs := validateDataguardBroker(nil, obj)
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "DataguardBroker"},
		obj.Name, allErrs)
}

// ValidateUpdate validates DataguardBroker update requests.
func (r *DataguardBroker) ValidateUpdate(ctx context.Context, oldObj, newObj *DataguardBroker) (admission.Warnings, error) {
	_ = ctx
	allErrs := validateDataguardBroker(oldObj, newObj)
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "DataguardBroker"},
		newObj.Name, allErrs)
}

// ValidateDelete validates DataguardBroker delete requests.
func (r *DataguardBroker) ValidateDelete(ctx context.Context, obj *DataguardBroker) (admission.Warnings, error) {
	_ = ctx
	_ = obj
	return nil, nil
}

func validateDataguardBroker(oldObj, newObj *DataguardBroker) field.ErrorList {
	var allErrs field.ErrorList
	if newObj == nil {
		return allErrs
	}

	if newObj.Spec.Topology != nil {
		allErrs = append(allErrs, validateNoMixedTopologyAndLegacy(newObj)...)
		allErrs = append(allErrs, validateDataguardExecution(newObj)...)
		allErrs = append(allErrs, validateDataguardTopology(newObj.Namespace, newObj.Spec.Topology)...)
		allErrs = append(allErrs, validateDataguardBrokerOperations(newObj)...)
		if oldObj != nil {
			allErrs = append(allErrs, validateDataguardTopologyUpdate(oldObj, newObj)...)
		}
		return allErrs
	}

	allErrs = append(allErrs, validateLegacyDataguardBroker(newObj)...)
	return allErrs
}

func validateDataguardBrokerOperations(obj *DataguardBroker) field.ErrorList {
	var allErrs field.ErrorList
	if obj == nil || obj.Spec.Operations == nil || obj.Spec.Operations.RoleConversion == nil {
		return allErrs
	}
	path := field.NewPath("spec").Child("operations").Child("roleConversion")
	op := obj.Spec.Operations.RoleConversion
	if strings.TrimSpace(op.RequestID) == "" {
		allErrs = append(allErrs, field.Required(path.Child("requestId"), "requestId is required"))
	}
	if strings.TrimSpace(op.Target) == "" {
		allErrs = append(allErrs, field.Required(path.Child("target"), "target is required"))
	}
	role := normalizeDataguardMemberRole(op.Role)
	if role != "PHYSICAL_STANDBY" && role != "SNAPSHOT_STANDBY" {
		allErrs = append(allErrs, field.Invalid(path.Child("role"), op.Role, "role conversion supports PHYSICAL_STANDBY or SNAPSHOT_STANDBY"))
	}
	return allErrs
}

func validateLegacyDataguardBroker(obj *DataguardBroker) field.ErrorList {
	var allErrs field.ErrorList
	if obj == nil {
		return allErrs
	}

	specPath := field.NewPath("spec")
	primaryRef := strings.TrimSpace(obj.Spec.PrimaryDatabaseRef)
	if primaryRef == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("primaryDatabaseRef"), "primaryDatabaseRef is required when spec.topology is not set"))
	}
	if len(obj.Spec.StandbyDatabaseRefs) == 0 {
		allErrs = append(allErrs, field.Required(specPath.Child("standbyDatabaseRefs"), "at least one standbyDatabaseRef is required when spec.topology is not set"))
	}

	seenStandbys := map[string]struct{}{}
	for i, standbyRef := range obj.Spec.StandbyDatabaseRefs {
		trimmed := strings.TrimSpace(standbyRef)
		if trimmed == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("standbyDatabaseRefs").Index(i), "standbyDatabaseRef cannot be empty"))
			continue
		}
		if trimmed == primaryRef && primaryRef != "" {
			allErrs = append(allErrs, field.Invalid(specPath.Child("standbyDatabaseRefs").Index(i), standbyRef, "standbyDatabaseRef cannot match primaryDatabaseRef"))
		}
		key := strings.ToLower(trimmed)
		if _, ok := seenStandbys[key]; ok {
			allErrs = append(allErrs, field.Duplicate(specPath.Child("standbyDatabaseRefs").Index(i), standbyRef))
			continue
		}
		seenStandbys[key] = struct{}{}
	}

	if !isValidDataguardProtectionMode(obj.Spec.ProtectionMode) {
		allErrs = append(allErrs, field.Invalid(specPath.Child("protectionMode"), obj.Spec.ProtectionMode, "must be MaxPerformance or MaxAvailability"))
	}
	if obj.Spec.Execution != nil {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("execution"), "spec.execution is only valid when spec.topology is set"))
	}

	return allErrs
}

func validateDataguardExecution(obj *DataguardBroker) field.ErrorList {
	var allErrs field.ErrorList
	if obj == nil {
		return allErrs
	}

	execution := obj.Spec.Execution
	specPath := field.NewPath("spec")
	if execution == nil {
		if dataguardTopologyRequiresExecutionRuntime(obj.Spec.Topology) {
			allErrs = append(allErrs, field.Required(specPath.Child("execution"), "spec.execution is required when spec.topology includes external, TCPS, or non-SIDB members"))
		}
		return allErrs
	}

	execPath := specPath.Child("execution")
	if strings.TrimSpace(execution.Image) == "" && (len(execution.ImagePullSecrets) > 0 ||
		strings.TrimSpace(execution.WalletMountPath) != "" ||
		strings.TrimSpace(execution.TNSAdminPath) != "") {
		allErrs = append(allErrs, field.Required(execPath.Child("image"), "image is required when execution settings are provided"))
	}
	if dataguardTopologyRequiresExecutionRuntime(obj.Spec.Topology) && strings.TrimSpace(execution.Image) == "" {
		allErrs = append(allErrs, field.Required(execPath.Child("image"), "image is required when spec.topology needs topology-native broker execution"))
	}
	for i, secretName := range execution.ImagePullSecrets {
		if strings.TrimSpace(secretName) == "" {
			allErrs = append(allErrs, field.Required(execPath.Child("imagePullSecrets").Index(i), "image pull secret name cannot be empty"))
		}
	}
	if execution.AuthWallet != nil {
		authWalletPath := execPath.Child("authWallet")
		if execution.AuthWallet.PasswordSecretRef != nil && strings.TrimSpace(execution.AuthWallet.PasswordSecretRef.SecretName) == "" {
			allErrs = append(allErrs, field.Required(authWalletPath.Child("passwordSecretRef").Child("secretName"), "secretName is required when passwordSecretRef is set"))
		}
	}
	return allErrs
}

func validateNoMixedTopologyAndLegacy(obj *DataguardBroker) field.ErrorList {
	var allErrs field.ErrorList
	if obj == nil || obj.Spec.Topology == nil {
		return allErrs
	}

	specPath := field.NewPath("spec")
	if strings.TrimSpace(obj.Spec.PrimaryDatabaseRef) != "" {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("primaryDatabaseRef"), "spec.primaryDatabaseRef cannot be used with spec.topology"))
	}
	if len(obj.Spec.StandbyDatabaseRefs) > 0 {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("standbyDatabaseRefs"), "spec.standbyDatabaseRefs cannot be used with spec.topology"))
	}
	if strings.TrimSpace(obj.Spec.ProtectionMode) != "" {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("protectionMode"), "spec.protectionMode cannot be used with spec.topology; use spec.topology.policy.protectionMode"))
	}
	if obj.Spec.FastStartFailover {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("fastStartFailover"), "spec.fastStartFailover cannot be used with spec.topology; use spec.topology.policy.fastStartFailover"))
	}
	return allErrs
}

func validateDataguardTopology(brokerNamespace string, topology *DataguardTopologySpec) field.ErrorList {
	var allErrs field.ErrorList
	if topology == nil {
		return allErrs
	}

	topologyPath := field.NewPath("spec").Child("topology")
	if len(topology.Members) == 0 {
		allErrs = append(allErrs, field.Required(topologyPath.Child("members"), "at least one topology member is required"))
	}

	memberRoles := map[string]string{}
	dbUniqueNames := map[string]int{}
	primaryCount := 0
	for i := range topology.Members {
		member := &topology.Members[i]
		memberPath := topologyPath.Child("members").Index(i)
		allErrs = append(allErrs, validateDataguardTopologyTextField(memberPath.Child("name"), member.Name)...)
		allErrs = append(allErrs, validateDataguardTopologyTextField(memberPath.Child("dbUniqueName"), member.DBUniqueName)...)
		allErrs = append(allErrs, validateDataguardTopologyDBUniqueName(memberPath, member)...)
		name := strings.TrimSpace(member.Name)
		if name == "" {
			allErrs = append(allErrs, field.Required(memberPath.Child("name"), "member name is required"))
		} else {
			key := strings.ToLower(name)
			if _, ok := memberRoles[key]; ok {
				allErrs = append(allErrs, field.Duplicate(memberPath.Child("name"), member.Name))
			} else {
				memberRoles[key] = normalizeDataguardMemberRole(member.Role)
			}
		}

		role := normalizeDataguardMemberRole(member.Role)
		if !isValidDataguardMemberRole(role) {
			allErrs = append(allErrs, field.Invalid(memberPath.Child("role"), member.Role, "must be PRIMARY, PHYSICAL_STANDBY, or SNAPSHOT_STANDBY"))
		}
		if role == "PRIMARY" {
			primaryCount++
		}

		resolvedDBUniqueName := strings.TrimSpace(member.DBUniqueName)
		if resolvedDBUniqueName == "" {
			resolvedDBUniqueName = strings.TrimSpace(member.Name)
		}
		if resolvedDBUniqueName != "" {
			key := strings.ToLower(resolvedDBUniqueName)
			if _, ok := dbUniqueNames[key]; ok {
				allErrs = append(allErrs, field.Duplicate(memberPath.Child("dbUniqueName"), resolvedDBUniqueName))
			} else {
				dbUniqueNames[key] = i
			}
		}

		if member.LocalRef == nil && len(member.Endpoints) == 0 {
			allErrs = append(allErrs, field.Required(memberPath, "set at least one of localRef or endpoints"))
		}
		adminSecretName, _, hasExplicitOrDefaultAdminSecret := ResolveDataguardTopologyMemberExplicitAdminSecretRef(topology, member)
		if hasExplicitOrDefaultAdminSecret && adminSecretName == "" {
			allErrs = append(allErrs, field.Required(memberPath.Child("adminSecretRef").Child("secretName"), "secretName is required"))
		}
		if member.LocalRef == nil && !hasExplicitOrDefaultAdminSecret {
			allErrs = append(allErrs, field.Required(memberPath.Child("adminSecretRef"), "adminSecretRef is required for external topology members"))
		}
		if member.LocalRef != nil && strings.TrimSpace(member.LocalRef.Name) == "" {
			allErrs = append(allErrs, field.Required(memberPath.Child("localRef").Child("name"), "name is required"))
		}
		if member.LocalRef != nil {
			localNamespace := strings.TrimSpace(member.LocalRef.Namespace)
			if localNamespace != "" && localNamespace != strings.TrimSpace(brokerNamespace) && !hasExplicitOrDefaultAdminSecret {
				allErrs = append(allErrs, field.Invalid(memberPath.Child("localRef").Child("namespace"), member.LocalRef.Namespace, "must match the DataguardBroker namespace when deriving admin secrets from localRef"))
			}
			kind := strings.TrimSpace(member.LocalRef.Kind)
			if kind != "" && !strings.EqualFold(kind, "SingleInstanceDatabase") && !hasExplicitOrDefaultAdminSecret {
				allErrs = append(allErrs, field.Required(memberPath.Child("adminSecretRef"), "adminSecretRef is required when localRef.kind is not SingleInstanceDatabase"))
			}
		}
		if member.AdminSecretRef != nil && strings.TrimSpace(member.AdminSecretRef.SecretName) == "" {
			allErrs = append(allErrs, field.Required(memberPath.Child("adminSecretRef").Child("secretName"), "secretName is required"))
		}
		if member.TCPS != nil {
			allErrs = append(allErrs, validateDataguardTopologyTextField(memberPath.Child("tcps").Child("sslServerDN"), member.TCPS.SSLServerDN)...)
		}

		tcpsEndpointCount := 0
		for j := range member.Endpoints {
			endpoint := &member.Endpoints[j]
			endpointPath := memberPath.Child("endpoints").Index(j)
			allErrs = append(allErrs, validateDataguardTopologyTextField(endpointPath.Child("host"), endpoint.Host)...)
			allErrs = append(allErrs, validateDataguardTopologyTextField(endpointPath.Child("serviceName"), endpoint.ServiceName)...)
			allErrs = append(allErrs, validateDataguardTopologyTextField(endpointPath.Child("sslServerDN"), endpoint.SSLServerDN)...)
			protocol := strings.ToUpper(strings.TrimSpace(endpoint.Protocol))
			if endpoint.Port <= 0 {
				allErrs = append(allErrs, field.Invalid(endpointPath.Child("port"), endpoint.Port, "must be greater than zero"))
			}
			if strings.TrimSpace(endpoint.Host) == "" {
				allErrs = append(allErrs, field.Required(endpointPath.Child("host"), "host is required"))
			} else if err := ValidateDataguardHost(endpoint.Host); err != nil {
				allErrs = append(allErrs, field.Invalid(endpointPath.Child("host"), endpoint.Host, err.Error()))
			}
			if strings.TrimSpace(endpoint.ServiceName) == "" {
				allErrs = append(allErrs, field.Required(endpointPath.Child("serviceName"), "serviceName is required"))
			} else if err := ValidateDataguardServiceName(endpoint.ServiceName); err != nil {
				allErrs = append(allErrs, field.Invalid(endpointPath.Child("serviceName"), endpoint.ServiceName, err.Error()))
			}
			switch protocol {
			case "TCP":
			case "TCPS":
				tcpsEndpointCount++
			default:
				allErrs = append(allErrs, field.Invalid(endpointPath.Child("protocol"), endpoint.Protocol, "must be TCP or TCPS"))
			}
		}
		if member.TCPS != nil && member.TCPS.Enabled && tcpsEndpointCount == 0 {
			allErrs = append(allErrs, field.Required(memberPath.Child("endpoints"), "at least one TCPS endpoint is required when tcps.enabled=true"))
		}
		if member.TCPS != nil && member.TCPS.Enabled && ResolveDataguardTopologyMemberClientWalletSecret(topology, member) == "" {
			allErrs = append(allErrs, field.Required(memberPath.Child("tcps").Child("clientWalletSecret"), "clientWalletSecret is required unless spec.topology.defaults.tcps.clientWalletSecret is set"))
		}
	}

	if primaryCount != 1 {
		allErrs = append(allErrs, field.Invalid(topologyPath.Child("members"), primaryCount, "exactly one PRIMARY member is required"))
	}

	if topology.Policy != nil {
		policyPath := topologyPath.Child("policy")
		if mode := strings.TrimSpace(topology.Policy.ProtectionMode); mode != "" && !isValidDataguardProtectionMode(mode) {
			allErrs = append(allErrs, field.Invalid(policyPath.Child("protectionMode"), topology.Policy.ProtectionMode, "must be MaxPerformance or MaxAvailability"))
		}
		if mode := strings.ToUpper(strings.TrimSpace(topology.Policy.TransportMode)); mode != "" && mode != "SYNC" && mode != "ASYNC" {
			allErrs = append(allErrs, field.Invalid(policyPath.Child("transportMode"), topology.Policy.TransportMode, "must be SYNC or ASYNC"))
		}
	}

	if topology.Observer != nil && topology.Observer.Enabled {
		observerPath := topologyPath.Child("observer")
		if strings.TrimSpace(topology.Observer.Name) == "" {
			allErrs = append(allErrs, field.Required(observerPath.Child("name"), "name is required when observer.enabled=true"))
		}
		if strings.TrimSpace(topology.Observer.Image) == "" {
			allErrs = append(allErrs, field.Required(observerPath.Child("image"), "image is required when observer.enabled=true"))
		}
	}
	if topology.Defaults != nil {
		defaultsPath := topologyPath.Child("defaults")
		if topology.Defaults.AdminSecretRef != nil && strings.TrimSpace(topology.Defaults.AdminSecretRef.SecretName) == "" {
			allErrs = append(allErrs, field.Required(defaultsPath.Child("adminSecretRef").Child("secretName"), "secretName is required"))
		}
	}

	return allErrs
}

func validateDataguardTopologyDBUniqueName(memberPath *field.Path, member *DataguardTopologyMember) field.ErrorList {
	var allErrs field.ErrorList
	if member == nil {
		return allErrs
	}
	if dbUniqueName := strings.TrimSpace(member.DBUniqueName); dbUniqueName != "" {
		if err := ValidateDataguardDBUniqueName(dbUniqueName); err != nil {
			allErrs = append(allErrs, field.Invalid(memberPath.Child("dbUniqueName"), member.DBUniqueName, err.Error()))
		}
		return allErrs
	}
	name := strings.TrimSpace(member.Name)
	if name == "" {
		return allErrs
	}
	if err := ValidateDataguardDBUniqueName(name); err != nil {
		allErrs = append(allErrs, field.Invalid(memberPath.Child("dbUniqueName"), member.Name, "dbUniqueName is required when member.name is not a valid Oracle DB_UNIQUE_NAME"))
	}
	return allErrs
}

func validateDataguardTopologyTextField(path *field.Path, value string) field.ErrorList {
	var allErrs field.ErrorList
	if strings.TrimSpace(value) == "" {
		return allErrs
	}
	if err := ValidateDataguardSingleLineText(path.String(), value); err != nil {
		allErrs = append(allErrs, field.Invalid(path, value, "must not contain control characters"))
	}
	return allErrs
}

func validateDataguardTopologyUpdate(oldObj, newObj *DataguardBroker) field.ErrorList {
	var allErrs field.ErrorList
	if oldObj == nil || newObj == nil {
		return allErrs
	}
	if !isDataguardBrokerTopologyLocked(oldObj) {
		return allErrs
	}
	if !reflect.DeepEqual(oldObj.Spec.Topology, newObj.Spec.Topology) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("topology"), "spec.topology cannot be changed after DataguardBroker reconciliation has started"))
	}
	if !reflect.DeepEqual(normalizeMutableDataguardExecutionSpec(oldObj.Spec.Execution), normalizeMutableDataguardExecutionSpec(newObj.Spec.Execution)) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("execution"), "spec.execution cannot be changed after DataguardBroker reconciliation has started"))
	}
	return allErrs
}

func normalizeMutableDataguardExecutionSpec(spec *DataguardExecutionSpec) *DataguardExecutionSpec {
	if spec == nil {
		return nil
	}
	clone := spec.DeepCopy()
	if clone.AuthWallet != nil {
		clone.AuthWallet.RebuildToken = ""
	}
	return clone
}

func isDataguardBrokerTopologyLocked(broker *DataguardBroker) bool {
	if broker == nil {
		return false
	}
	return strings.TrimSpace(broker.Status.Status) != "" ||
		strings.TrimSpace(broker.Status.ObservedTopologyHash) != "" ||
		len(broker.Status.ResolvedMembers) > 0 ||
		len(broker.Status.Conditions) > 0
}

func normalizeDataguardMemberRole(role string) string {
	return strings.ToUpper(strings.TrimSpace(role))
}

func isValidDataguardMemberRole(role string) bool {
	switch normalizeDataguardMemberRole(role) {
	case "PRIMARY", "PHYSICAL_STANDBY", "SNAPSHOT_STANDBY":
		return true
	default:
		return false
	}
}

func isStandbyCompatibleDataguardRole(role string) bool {
	switch normalizeDataguardMemberRole(role) {
	case "PHYSICAL_STANDBY", "SNAPSHOT_STANDBY":
		return true
	default:
		return false
	}
}

func isValidDataguardProtectionMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "MaxPerformance", "MaxAvailability":
		return true
	default:
		return false
	}
}

func dataguardTopologyRequiresExecutionRuntime(topology *DataguardTopologySpec) bool {
	if topology == nil {
		return false
	}
	for i := range topology.Members {
		member := topology.Members[i]
		if member.LocalRef == nil {
			return true
		}
		kind := strings.TrimSpace(member.LocalRef.Kind)
		if kind != "" && !strings.EqualFold(kind, "SingleInstanceDatabase") {
			return true
		}
		if member.TCPS != nil && member.TCPS.Enabled {
			return true
		}
		for j := range member.Endpoints {
			if strings.EqualFold(strings.TrimSpace(member.Endpoints[j].Protocol), "TCPS") {
				return true
			}
		}
	}
	return false
}
