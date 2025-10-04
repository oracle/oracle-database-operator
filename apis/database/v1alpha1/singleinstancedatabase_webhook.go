/*
** Copyright (c) 2023 Oracle and/or its affiliates.
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

package v1alpha1

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var singleinstancedatabaselog = logf.Log.WithName("singleinstancedatabase-resource")

func (r *SingleInstanceDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-singleinstancedatabase,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,verbs=create;update,versions=v1alpha1,name=msingleinstancedatabase.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomDefaulter = &SingleInstanceDatabase{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *SingleInstanceDatabase) Default(ctx context.Context, obj runtime.Object) error {
	sidb, ok := obj.(*SingleInstanceDatabase)
	if !ok {
		return apierrors.NewInternalError(fmt.Errorf("failed to cast obj object to SingleInstanceDatabase"))
	}

	singleinstancedatabaselog.Info("default", "name", sidb.Name)

	if sidb.Spec.LoadBalancer {
		// Annotations required for a flexible load balancer on oci
		if sidb.Spec.ServiceAnnotations == nil {
			sidb.Spec.ServiceAnnotations = make(map[string]string)
		}
		_, ok := sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape"]
		if !ok {
			sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape"] = "flexible"
		}
		_, ok = sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-min"]
		if !ok {
			sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-min"] = "10"
		}
		_, ok = sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-max"]
		if !ok {
			sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-max"] = "100"
		}
	}

	if sidb.Spec.AdminPassword.KeepSecret == nil {
		keepSecret := true
		sidb.Spec.AdminPassword.KeepSecret = &keepSecret
	}

	if sidb.Spec.Edition == "" {
		if sidb.Spec.CreateAs == "clone" && !sidb.Spec.Image.PrebuiltDB {
			sidb.Spec.Edition = "enterprise"
		}
	}

	if sidb.Spec.CreateAs == "" {
		sidb.Spec.CreateAs = "primary"
	}

	if sidb.Spec.Sid == "" {
		if sidb.Spec.Edition == "express" {
			sidb.Spec.Sid = "XE"
		} else if sidb.Spec.Edition == "free" {
			sidb.Spec.Sid = "FREE"
		} else {
			sidb.Spec.Sid = "ORCLCDB"
		}
	}

	if sidb.Spec.Pdbname == "" {
		if sidb.Spec.Edition == "express" {
			sidb.Spec.Pdbname = "XEPDB1"
		} else if sidb.Spec.Edition == "free" {
			sidb.Spec.Pdbname = "FREEPDB1"
		} else {
			sidb.Spec.Pdbname = "ORCLPDB1"
		}
	}

	if sidb.Spec.Edition == "express" || sidb.Spec.Edition == "free" {
		// Allow zero replicas as a means to bounce the DB
		if sidb.Status.Replicas == 1 && sidb.Spec.Replicas > 1 {
			// If not zero, default the replicas to 1
			sidb.Spec.Replicas = 1
		}
	}

	if sidb.Spec.TrueCacheServices == nil {
		sidb.Spec.TrueCacheServices = make([]string, 0)
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v1alpha1-singleinstancedatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,versions=v1alpha1,name=vsingleinstancedatabase.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomValidator = &SingleInstanceDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	sidb, ok := obj.(*SingleInstanceDatabase)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast obj object to SingleInstanceDatabase"))
	}
	singleinstancedatabaselog.Info("validate create", "name", sidb.Name)
	var allErrs field.ErrorList

	namespaces := dbcommons.GetWatchNamespaces()
	_, containsNamespace := namespaces[sidb.Namespace]
	// Check if the allowed namespaces maps contains the required namespace
	if len(namespaces) != 0 && !containsNamespace {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("metadata").Child("namespace"), sidb.Namespace,
				"Oracle database operator doesn't watch over this namespace"))
	}

	// Persistence spec validation
	if sidb.Spec.Persistence.Size == "" && (sidb.Spec.Persistence.AccessMode != "" ||
		sidb.Spec.Persistence.StorageClass != "" || sidb.Spec.Persistence.DatafilesVolumeName != "") {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("persistence").Child("size"), sidb.Spec.Persistence,
				"invalid persistence specification, specify required size"))
	}

	if sidb.Spec.Persistence.Size != "" {
		if sidb.Spec.Persistence.AccessMode == "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("size"), sidb.Spec.Persistence,
					"invalid persistence specification, specify accessMode"))
		}
		if sidb.Spec.Persistence.AccessMode != "ReadWriteMany" && sidb.Spec.Persistence.AccessMode != "ReadWriteOnce" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("accessMode"),
					sidb.Spec.Persistence.AccessMode, "should be either \"ReadWriteOnce\" or \"ReadWriteMany\""))
		}
	}

	if sidb.Spec.CreateAs == "standby" {
		if sidb.Spec.ArchiveLog != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("archiveLog"),
					sidb.Spec.ArchiveLog, "archiveLog cannot be specified for standby databases"))
		}
		if sidb.Spec.FlashBack != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("flashBack"),
					sidb.Spec.FlashBack, "flashBack cannot be specified for standby databases"))
		}
		if sidb.Spec.ForceLogging != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("forceLog"),
					sidb.Spec.ForceLogging, "forceLog cannot be specified for standby databases"))
		}
		if sidb.Spec.InitParams != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("initParams"),
					sidb.Spec.InitParams, "initParams cannot be specified for standby databases"))
		}
		if sidb.Spec.Persistence.ScriptsVolumeName != "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("scriptsVolumeName"),
					sidb.Spec.Persistence.ScriptsVolumeName, "scriptsVolumeName cannot be specified for standby databases"))
		}
		if sidb.Spec.EnableTCPS {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("enableTCPS"),
					sidb.Spec.EnableTCPS, "enableTCPS cannot be specified for standby databases"))
		}

	}

	// Replica validation
	if sidb.Spec.Replicas > 1 {
		valMsg := ""
		if sidb.Spec.Edition == "express" || sidb.Spec.Edition == "free" {
			valMsg = "should be 1 for " + sidb.Spec.Edition + " edition"
		}
		if sidb.Spec.Persistence.Size == "" {
			valMsg = "should be 1 if no persistence is specified"
		}
		if valMsg != "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("replicas"), sidb.Spec.Replicas, valMsg))
		}
	}

	if (sidb.Spec.CreateAs == "clone" || sidb.Spec.CreateAs == "standby") && sidb.Spec.PrimaryDatabaseRef == "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("primaryDatabaseRef"), sidb.Spec.PrimaryDatabaseRef, "Primary Database reference cannot be null for a secondary database"))
	}

	if sidb.Spec.Edition == "express" || sidb.Spec.Edition == "free" {
		if sidb.Spec.CreateAs == "clone" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("createAs"), sidb.Spec.CreateAs,
					"Cloning not supported for "+sidb.Spec.Edition+" edition"))
		}
		if sidb.Spec.CreateAs == "standby" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("createAs"), sidb.Spec.CreateAs,
					"Physical Standby Database creation is not supported for "+sidb.Spec.Edition+" edition"))
		}
		if sidb.Spec.Edition == "express" && strings.ToUpper(sidb.Spec.Sid) != "XE" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("sid"), sidb.Spec.Sid,
					"Express edition SID must only be XE"))
		}
		if sidb.Spec.Edition == "free" && strings.ToUpper(sidb.Spec.Sid) != "FREE" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("sid"), sidb.Spec.Sid,
					"Free edition SID must only be FREE"))
		}
		if sidb.Spec.Edition == "express" && strings.ToUpper(sidb.Spec.Pdbname) != "XEPDB1" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("pdbName"), sidb.Spec.Pdbname,
					"Express edition PDB must be XEPDB1"))
		}
		if sidb.Spec.Edition == "free" && strings.ToUpper(sidb.Spec.Pdbname) != "FREEPDB1" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("pdbName"), sidb.Spec.Pdbname,
					"Free edition PDB must be FREEPDB1"))
		}
		if sidb.Spec.InitParams != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("initParams"), *sidb.Spec.InitParams,
					sidb.Spec.Edition+" edition does not support changing init parameters"))
		}
	} else {
		if sidb.Spec.Sid == "XE" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("sid"), sidb.Spec.Sid,
					"XE is reserved as the SID for Express edition of the database"))
		}
		if sidb.Spec.Sid == "FREE" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("sid"), sidb.Spec.Sid,
					"FREE is reserved as the SID for FREE edition of the database"))
		}
	}

	if sidb.Spec.CreateAs == "clone" {
		if sidb.Spec.Image.PrebuiltDB {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("createAs"), sidb.Spec.CreateAs,
					"cannot clone to create a prebuilt db"))
		} else if strings.Contains(sidb.Spec.PrimaryDatabaseRef, ":") && strings.Contains(sidb.Spec.PrimaryDatabaseRef, "/") && sidb.Spec.Edition == "" {
			//Edition must be passed when cloning from a source database other than same k8s cluster
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("edition"), sidb.Spec.CreateAs,
					"Edition must be passed when cloning from a source database other than same k8s cluster"))
		}
	}

	if sidb.Spec.CreateAs != "truecache" {
		if len(sidb.Spec.TrueCacheServices) > 0 {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("trueCacheServices"), sidb.Spec.TrueCacheServices,
					"Creation of trueCacheServices only supported with True Cache instances"))
		}
	}

	if sidb.Status.FlashBack == "true" && sidb.Spec.FlashBack != nil && *sidb.Spec.FlashBack {
		if sidb.Spec.ArchiveLog != nil && !*sidb.Spec.ArchiveLog {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("archiveLog"), sidb.Spec.ArchiveLog,
					"Cannot disable Archivelog. Please disable Flashback first."))
		}
	}

	if sidb.Status.ArchiveLog == "false" && sidb.Spec.ArchiveLog != nil && !*sidb.Spec.ArchiveLog {
		if sidb.Spec.FlashBack != nil && *sidb.Spec.FlashBack {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("flashBack"), sidb.Spec.FlashBack,
					"Cannot enable Flashback. Please enable Archivelog first."))
		}
	}

	if sidb.Spec.Persistence.VolumeClaimAnnotation != "" {
		strParts := strings.Split(sidb.Spec.Persistence.VolumeClaimAnnotation, ":")
		if len(strParts) != 2 {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("volumeClaimAnnotation"), sidb.Spec.Persistence.VolumeClaimAnnotation,
					"volumeClaimAnnotation should be in <key>:<value> format."))
		}
	}

	// servicePort and tcpServicePort validation
	if !sidb.Spec.LoadBalancer {
		// NodePort service is expected. In this case servicePort should be in range 30000-32767
		if sidb.Spec.ListenerPort != 0 && (sidb.Spec.ListenerPort < 30000 || sidb.Spec.ListenerPort > 32767) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("listenerPort"), sidb.Spec.ListenerPort,
					"listenerPort should be in 30000-32767 range."))
		}
		if sidb.Spec.EnableTCPS && sidb.Spec.TcpsListenerPort != 0 && (sidb.Spec.TcpsListenerPort < 30000 || sidb.Spec.TcpsListenerPort > 32767) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), sidb.Spec.TcpsListenerPort,
					"tcpsListenerPort should be in 30000-32767 range."))
		}
	} else {
		// LoadBalancer Service is expected.
		if sidb.Spec.EnableTCPS && sidb.Spec.TcpsListenerPort == 0 && sidb.Spec.ListenerPort == int(dbcommons.CONTAINER_TCPS_PORT) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("listenerPort"), sidb.Spec.ListenerPort,
					"listenerPort can not be 2484 as the default port for tcpsListenerPort is 2484."))
		}
	}

	if sidb.Spec.EnableTCPS && sidb.Spec.ListenerPort != 0 && sidb.Spec.TcpsListenerPort != 0 && sidb.Spec.ListenerPort == sidb.Spec.TcpsListenerPort {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), sidb.Spec.TcpsListenerPort,
				"listenerPort and tcpsListenerPort can not be equal."))
	}

	// Certificate Renew Duration Validation
	if sidb.Spec.EnableTCPS && sidb.Spec.TcpsCertRenewInterval != "" {
		duration, err := time.ParseDuration(sidb.Spec.TcpsCertRenewInterval)
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), sidb.Spec.TcpsCertRenewInterval,
					"Please provide valid string to parse the tcpsCertRenewInterval."))
		}
		maxLimit, _ := time.ParseDuration("8760h")
		minLimit, _ := time.ParseDuration("24h")
		if duration > maxLimit || duration < minLimit {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), sidb.Spec.TcpsCertRenewInterval,
					"Please specify tcpsCertRenewInterval in the range: 24h to 8760h"))
		}
	}

	// tcpsTlsSecret validations
	if !sidb.Spec.EnableTCPS && sidb.Spec.TcpsTlsSecret != "" {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("tcpsTlsSecret"),
				" is allowed only if enableTCPS is true"))
	}
	if sidb.Spec.TcpsTlsSecret != "" && sidb.Spec.TcpsCertRenewInterval != "" {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("tcpsCertRenewInterval"),
				" is applicable only for self signed certs"))
	}

	if sidb.Spec.InitParams != nil {
		if (sidb.Spec.InitParams.PgaAggregateTarget != 0 && sidb.Spec.InitParams.SgaTarget == 0) || (sidb.Spec.InitParams.PgaAggregateTarget == 0 && sidb.Spec.InitParams.SgaTarget != 0) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("initParams"),
					sidb.Spec.InitParams, "initParams value invalid : Provide values for both pgaAggregateTarget and SgaTarget"))
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		sidb.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateUpdate(ctx context.Context, oldRuntimeObject, newRuntimeObj runtime.Object) (admission.Warnings, error) {
	new, ok := newRuntimeObj.(*SingleInstanceDatabase)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast newRuntimeObj object to SingleInstanceDatabase"))
	}
	singleinstancedatabaselog.Info("validate update", "name", new.Name)
	var allErrs field.ErrorList

	// check creation validations first
	warnings, err := new.ValidateCreate(ctx, newRuntimeObj)
	if err != nil {
		return warnings, err
	}

	// Validate Deletion
	if new.GetDeletionTimestamp() != nil {
		warnings, err := new.ValidateDelete(ctx, newRuntimeObj)
		if err != nil {
			return warnings, err
		}
	}

	// Now check for updation errors
	old, okay := oldRuntimeObject.(*SingleInstanceDatabase)
	if !okay {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast oldRuntimeObject object to SingleInstanceDatabase"))
	}

	if old.Status.CreatedAs == "clone" {
		if new.Spec.Edition != "" && old.Status.Edition != "" && !strings.EqualFold(old.Status.Edition, new.Spec.Edition) {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("edition"), "Edition of a cloned singleinstancedatabase cannot be changed post creation"))
		}

		if !strings.EqualFold(old.Status.PrimaryDatabase, new.Spec.PrimaryDatabaseRef) {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("primaryDatabaseRef"), "Primary database of a cloned singleinstancedatabase cannot be changed post creation"))
		}
	}

	if old.Status.Role != dbcommons.ValueUnavailable && old.Status.Role != "PRIMARY" {
		// Restriciting Patching of secondary databases archiveLog, forceLog, flashBack
		statusArchiveLog, _ := strconv.ParseBool(old.Status.ArchiveLog)
		if new.Spec.ArchiveLog != nil && (statusArchiveLog != *new.Spec.ArchiveLog) {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("archiveLog"), "cannot be changed"))
		}
		statusFlashBack, _ := strconv.ParseBool(old.Status.FlashBack)
		if new.Spec.FlashBack != nil && (statusFlashBack != *new.Spec.FlashBack) {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("flashBack"), "cannot be changed"))
		}
		statusForceLogging, _ := strconv.ParseBool(old.Status.ForceLogging)
		if new.Spec.ForceLogging != nil && (statusForceLogging != *new.Spec.ForceLogging) {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("forceLog"), "cannot be changed"))
		}

		// Restriciting Patching of secondary databases InitParams
		if new.Spec.InitParams != nil {
			if old.Status.InitParams.SgaTarget != new.Spec.InitParams.SgaTarget {
				allErrs = append(allErrs,
					field.Forbidden(field.NewPath("spec").Child("initParams").Child("sgaTarget"), "cannot be changed"))
			}
			if old.Status.InitParams.PgaAggregateTarget != new.Spec.InitParams.PgaAggregateTarget {
				allErrs = append(allErrs,
					field.Forbidden(field.NewPath("spec").Child("initParams").Child("pgaAggregateTarget"), "cannot be changed"))
			}
			if old.Status.InitParams.CpuCount != new.Spec.InitParams.CpuCount {
				allErrs = append(allErrs,
					field.Forbidden(field.NewPath("spec").Child("initParams").Child("cpuCount"), "cannot be changed"))
			}
			if old.Status.InitParams.Processes != new.Spec.InitParams.Processes {
				allErrs = append(allErrs,
					field.Forbidden(field.NewPath("spec").Child("initParams").Child("processes"), "cannot be changed"))
			}
		}
	}

	// if Db is in a dataguard configuration or referred by Standby databases then Restrict enabling Tcps on the Primary DB
	if new.Spec.EnableTCPS {
		if old.Status.DgBroker != nil {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("enableTCPS"), "cannot enable tcps as database is in a dataguard configuration"))
		} else if len(old.Status.StandbyDatabases) != 0 {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("enableTCPS"), "cannot enable tcps as database is referred by one or more standby databases"))
		}
	}

	if old.Status.DatafilesCreated == "true" && (old.Status.PrebuiltDB != new.Spec.Image.PrebuiltDB) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("image").Child("prebuiltDB"), "cannot be changed"))
	}
	if new.Spec.Edition != "" && old.Status.Edition != "" && !strings.EqualFold(old.Status.Edition, new.Spec.Edition) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("edition"), "cannot be changed"))
	}
	if old.Status.Charset != "" && !strings.EqualFold(old.Status.Charset, new.Spec.Charset) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("charset"), "cannot be changed"))
	}
	if old.Status.Sid != "" && !strings.EqualFold(new.Spec.Sid, old.Status.Sid) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("sid"), "cannot be changed"))
	}
	if old.Status.Pdbname != "" && !strings.EqualFold(old.Status.Pdbname, new.Spec.Pdbname) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("pdbname"), "cannot be changed"))
	}
	if old.Status.CreatedAs == "clone" &&
		(old.Status.PrimaryDatabase == dbcommons.ValueUnavailable && new.Spec.PrimaryDatabaseRef != "" ||
			old.Status.PrimaryDatabase != dbcommons.ValueUnavailable && old.Status.PrimaryDatabase != new.Spec.PrimaryDatabaseRef) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("primaryDatabaseRef"), "cannot be changed"))
	}
	if old.Status.OrdsReference != "" && new.Status.Persistence != new.Spec.Persistence {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("persistence"), "uninstall ORDS to change Persistence"))
	}

	if old.Status.Replicas != new.Spec.Replicas && old.Status.DgBroker != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("replicas"), "cannot be updated for a database in a Data Guard configuration"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		new.Name, allErrs)

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	sidb, ok := obj.(*SingleInstanceDatabase)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast obj object to SingleInstanceDatabase"))
	}

	singleinstancedatabaselog.Info("validate delete", "name", sidb.Name)
	var allErrs field.ErrorList
	if sidb.Status.OrdsReference != "" {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("status").Child("ordsReference"), "delete "+sidb.Status.OrdsReference+" to cleanup this SIDB"))
	}
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		sidb.Name, allErrs)
}
