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

package v1alpha1

import (
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
)

// log is for logging in this package.
var singleinstancedatabaselog = logf.Log.WithName("singleinstancedatabase-resource")

func (r *SingleInstanceDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-singleinstancedatabase,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,verbs=create;update,versions=v1alpha1,name=msingleinstancedatabase.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &SingleInstanceDatabase{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *SingleInstanceDatabase) Default() {
	singleinstancedatabaselog.Info("default", "name", r.Name)

	if r.Spec.AdminPassword.KeepSecret == nil {
		keepSecret := true
		r.Spec.AdminPassword.KeepSecret = &keepSecret
	}

	if r.Spec.Edition == "" {
		if r.Spec.CloneFrom == "" && !r.Spec.Image.PrebuiltDB {
			r.Spec.Edition = "enterprise"
		}
	}

	if r.Spec.Sid == "" {
		if r.Spec.Edition == "express" {
			r.Spec.Sid = "XE"
		} else {
			r.Spec.Sid = "ORCLCDB"
		}
	}

	if r.Spec.Pdbname == "" {
		if r.Spec.Edition == "express" {
			r.Spec.Pdbname = "XEPDB1"
		} else {
			r.Spec.Pdbname = "ORCLPDB1"
		}
	}

	if r.Spec.Edition == "express" {
		if r.Status.Replicas == 1 {
			// default the replicas for XE
			r.Spec.Replicas = 1
		}
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v1alpha1-singleinstancedatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,versions=v1alpha1,name=vsingleinstancedatabase.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &SingleInstanceDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateCreate() error {
	singleinstancedatabaselog.Info("validate create", "name", r.Name)
	var allErrs field.ErrorList

	// Persistence spec validation
	if r.Spec.Persistence.Size == "" && (r.Spec.Persistence.AccessMode != "" ||
		r.Spec.Persistence.StorageClass != "" || r.Spec.Persistence.VolumeName != "") {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("persistence").Child("size"), r.Spec.Persistence,
				"invalid persistence specification, specify required size"))
	}

	if r.Spec.Persistence.Size != "" {
		if r.Spec.Persistence.AccessMode == "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("size"), r.Spec.Persistence,
					"invalid persistence specification, specify accessMode"))
		}
		if r.Spec.Persistence.AccessMode != "ReadWriteMany" && r.Spec.Persistence.AccessMode != "ReadWriteOnce" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("accessMode"),
					r.Spec.Persistence.AccessMode, "should be either \"ReadWriteOnce\" or \"ReadWriteMany\""))
		}
	}

	// Replica validation
	if r.Spec.Replicas > 1 {
		valMsg := ""
		if r.Spec.Edition == "express" {
			valMsg = "should be 1 for express edition"
		}
		if r.Spec.Persistence.Size == "" {
			valMsg = "should be 1 if no persistence is specified"
		}
		if valMsg != "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("replicas"), r.Spec.Replicas, valMsg))
		}
	}

	if r.Spec.Edition == "express" {
		if r.Spec.CloneFrom != "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("cloneFrom"), r.Spec.CloneFrom,
					"Cloning not supported for Express edition"))
		}
		if strings.ToUpper(r.Spec.Sid) != "XE" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("sid"), r.Spec.Sid,
					"Express edition SID must only be XE"))
		}
		if strings.ToUpper(r.Spec.Pdbname) != "XEPDB1" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("pdbName"), r.Spec.Pdbname,
					"Express edition PDB must be XEPDB1"))
		}
		if r.Spec.InitParams.CpuCount != 0 {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("initParams").Child("cpuCount"), r.Spec.InitParams.CpuCount,
					"Express edition does not support changing init parameter cpuCount."))
		}
		if r.Spec.InitParams.Processes != 0 {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("initParams").Child("processes"), r.Spec.InitParams.Processes,
					"Express edition does not support changing init parameter process."))
		}
		if r.Spec.InitParams.SgaTarget != 0 {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("initParams").Child("sgaTarget"), r.Spec.InitParams.SgaTarget,
					"Express edition does not support changing init parameter sgaTarget."))
		}
		if r.Spec.InitParams.PgaAggregateTarget != 0 {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("initParams").Child("pgaAggregateTarget"), r.Spec.InitParams.PgaAggregateTarget,
					"Express edition does not support changing init parameter pgaAggregateTarget."))
		}
	} else {
		if r.Spec.Sid == "XE" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("sid"), r.Spec.Sid,
					"XE is reserved as the SID for Express edition of the database"))
		}
	}

	if r.Spec.CloneFrom != "" {
		if r.Spec.Image.PrebuiltDB {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("cloneFrom"), r.Spec.CloneFrom,
					"cannot clone to create a prebuilt db"))
		} else if strings.Contains(r.Spec.CloneFrom, ":") && strings.Contains(r.Spec.CloneFrom, "/") && r.Spec.Edition == "" {
			//Edition must be passed when cloning from a source database other than same k8s cluster
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("edition"), r.Spec.CloneFrom,
					"Edition must be passed when cloning from a source database other than same k8s cluster"))
		}
	}

	if r.Status.FlashBack == "true" && r.Spec.FlashBack {
		if !r.Spec.ArchiveLog {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("archiveLog"), r.Spec.ArchiveLog,
					"Cannot disable Archivelog. Please disable Flashback first."))
		}
	}

	if r.Status.ArchiveLog == "false" && !r.Spec.ArchiveLog {
		if r.Spec.FlashBack {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("flashBack"), r.Spec.FlashBack,
					"Cannot enable Flashback. Please enable Archivelog first."))
		}
	}

	if r.Spec.Persistence.VolumeClaimAnnotation != "" {
		strParts := strings.Split(r.Spec.Persistence.VolumeClaimAnnotation, ":")
		if len(strParts) != 2 {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("volumeClaimAnnotation"), r.Spec.Persistence.VolumeClaimAnnotation,
					"volumeClaimAnnotation should be in <key>:<value> format."))
		}
	}

	// servicePort and tcpServicePort validation
	if !r.Spec.LoadBalancer {
		// NodePort service is expected. In this case servicePort should be in range 30000-32767
		if r.Spec.ListenerPort != 0 && (r.Spec.ListenerPort < 30000 || r.Spec.ListenerPort > 32767) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("listenerPort"), r.Spec.ListenerPort,
					"listenerPort should be in 30000-32767 range."))
		}
		if r.Spec.TcpsListenerPort != 0 && (r.Spec.TcpsListenerPort < 30000 || r.Spec.TcpsListenerPort > 32767) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), r.Spec.TcpsListenerPort,
					"tcpsListenerPort should be in 30000-32767 range."))
		}
	}
	if r.Spec.ListenerPort != 0 && r.Spec.TcpsListenerPort != 0 && r.Spec.ListenerPort == r.Spec.TcpsListenerPort {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), r.Spec.TcpsListenerPort,
				"listenerPort and tcpsListenerPort can not be equal."))
	}
	if r.Spec.EnableTCPS && r.Spec.TcpsListenerPort == 0 && r.Spec.ListenerPort == 1522 {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("listenerPort"), r.Spec.ListenerPort,
				"listenerPort can not be 1522 as the default port for tcpsListenerPort is 1522."))
	}

	// Certificate Renew Duration Validation
	if r.Spec.TcpsCertRenewInterval != "" {
		duration, err := time.ParseDuration(r.Spec.TcpsCertRenewInterval)
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), r.Spec.TcpsCertRenewInterval,
					"Please provide valid string to parse the tcpsCertRenewInterval."))
		}
		maxLimit, _ := time.ParseDuration("26280h")
		minLimit, _ := time.ParseDuration("1m")
		if duration > maxLimit || duration < minLimit {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), r.Spec.TcpsCertRenewInterval,
					"Please specify tcpsCertRenewInterval in the range: 1m to 26280h"))
		}
	}
	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateUpdate(oldRuntimeObject runtime.Object) error {
	singleinstancedatabaselog.Info("validate update", "name", r.Name)
	var allErrs field.ErrorList

	// check creation validations first
	err := r.ValidateCreate()
	if err != nil {
		return err
	}

	// Validate Deletion
	if r.GetDeletionTimestamp() != nil {
		err := r.ValidateDelete()
		if err != nil {
			return err
		}
	}

	// Now check for updation errors
	old, ok := oldRuntimeObject.(*SingleInstanceDatabase)
	if !ok {
		return nil
	}
	if old.Status.DatafilesCreated == "true" && (old.Status.PrebuiltDB != r.Spec.Image.PrebuiltDB) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("image").Child("prebuiltDB"), "cannot be changed"))
	}
	if r.Spec.CloneFrom == "" && old.Status.Edition != "" && !strings.EqualFold(old.Status.Edition, r.Spec.Edition) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("edition"), "cannot be changed"))
	}
	if old.Status.Charset != "" && !strings.EqualFold(old.Status.Charset, r.Spec.Charset) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("charset"), "cannot be changed"))
	}
	if old.Status.Sid != "" && !strings.EqualFold(r.Spec.Sid, old.Status.Sid) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("sid"), "cannot be changed"))
	}
	if old.Status.Pdbname != "" && !strings.EqualFold(old.Status.Pdbname, r.Spec.Pdbname) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("pdbname"), "cannot be changed"))
	}
	if old.Status.CloneFrom != "" &&
		(old.Status.CloneFrom == dbcommons.NoCloneRef && r.Spec.CloneFrom != "" ||
			old.Status.CloneFrom != dbcommons.NoCloneRef && old.Status.CloneFrom != r.Spec.CloneFrom) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("cloneFrom"), "cannot be changed"))
	}
	if old.Status.OrdsReference != "" && r.Status.Persistence != r.Spec.Persistence {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("persistence"), "uninstall ORDS to change Persistence"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		r.Name, allErrs)

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateDelete() error {
	singleinstancedatabaselog.Info("validate delete", "name", r.Name)
	var allErrs field.ErrorList
	if r.Status.OrdsReference != "" {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("status").Child("ordsReference"), "delete "+r.Status.OrdsReference+" to cleanup this SIDB"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		r.Name, allErrs)
}
