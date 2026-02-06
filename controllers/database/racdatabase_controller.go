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

package controllers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	v4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	raccommon "github.com/oracle/oracle-database-operator/commons/rac"
	utils "github.com/oracle/oracle-database-operator/commons/rac/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RacDatabaseReconciler reconciles a RacDatabase object
type RacDatabaseReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Config     *rest.Config
	kubeClient kubernetes.Interface
	kubeConfig clientcmd.ClientConfig
	Recorder   record.EventRecorder
}

const racDatabaseFinalizer = "database.oracle.com/racdatabasefinalizer"

//+kubebuilder:rbac:groups="database.oracle.com",resources=racdatabases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="database.oracle.com",resources=racdatabases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="database.oracle.com",resources=racdatabases/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;secrets;endpoints;services;events;configmaps;persistentvolumes;persistentvolumeclaims;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups='',resources=statefulsets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the reconciliation loop for RacDatabase resources.
// It orchestrates the complete lifecycle management of Oracle RAC databases in Kubernetes, including:
//
// 1. Resource Retrieval and Validation
//   - Fetches the RacDatabase resource from the cluster
//   - Determines configuration style (old-style InstDetails vs new-style ClusterDetails)
//   - Validates that only one configuration style is specified
//
// 2. Status Initialization
//   - Initializes ConfigParams and status fields with default values
//   - Sets up Kubernetes client configuration for RAC operations
//
// 3. Deletion Management
//   - Handles RacDatabase resource deletion
//   - Performs cleanup of RAC instances based on old specifications
//   - Validates RAC object state via webhooks (when enabled)
//
// 4. Service Creation
//   - Creates or replaces Kubernetes Services for RAC components:
//   - VIP services for virtual IPs
//   - Local services for node-local connectivity
//   - SCAN services for Oracle SCAN listeners
//   - External services for ONS and listeners (based on port configuration)
//   - NodePort services for external access
//
// 5. ASM Storage Management
//   - Detects new setups, upgrades, and disk changes
//   - Runs disk-check DaemonSet for ASM disk discovery
//   - Creates and updates PersistentVolumes and PersistentVolumeClaims
//   - Handles ASM disk addition and removal (exclusive operations)
//
// 6. Configuration and StatefulSet Creation
//   - Generates ConfigMaps with RAC configuration parameters
//   - Creates or updates StatefulSets for RAC database instances
//   - Handles both old-style (per-instance) and new-style (cluster-level) configurations
//   - Supports ASM disk changes with automatic configuration updates
//
// 7. Post-Reconciliation Steps
//   - Updates current specification annotations
//   - Defers status update via updateReconcileStatus
//
// Returns a ctrl.Result indicating whether requeuing is needed and any errors encountered.
// Uses a 60-second requeue interval for most operations and 10-second intervals during disk discovery.
func (r *RacDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	//ctx := context.Background()
	_ = r.Log.WithValues("racdatabase", req.NamespacedName)

	r.Log.Info("Reconcile requested")
	var result ctrl.Result
	var err error
	completed := false
	blocked := false
	var i int32
	var svcType string
	var nilErr error = nil
	var oraRacInst racdb.RacInstDetailSpec
	resultNq := ctrl.Result{Requeue: false}
	resultQ := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}
	// time.Sleep(50000 * time.Second)

	racDatabase := &racdb.RacDatabase{}
	configMapData := make(map[string]string)

	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, racDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource not found")
			return requeueN, nil
		}
		r.Log.Error(err, err.Error())
		racDatabase.Spec.IsFailed = true
		return resultQ, err
	}
	// Kube Client Config Setup
	if r.kubeConfig == nil && r.kubeClient == nil {
		r.kubeConfig, r.kubeClient, err = raccommon.GetRacK8sClientConfig(r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	// Determine which config style is in use
	var isOldStyle bool
	if len(racDatabase.Spec.InstDetails) > 0 && racDatabase.Spec.ClusterDetails == nil {
		isOldStyle = true
	} else if racDatabase.Spec.ClusterDetails != nil && len(racDatabase.Spec.InstDetails) == 0 {
		isOldStyle = false
	} else if racDatabase.Spec.ClusterDetails != nil && len(racDatabase.Spec.InstDetails) > 0 {
		// Both styles provided -- warn/error as needed
		r.Log.Info("Both instDetails and instanceDetails are set. Please specify only one style. Defaulting to old style (instDetails).")
		isOldStyle = true
		return resultQ, fmt.Errorf("invalid specification: must provide either instDetails or instanceDetails and not both")
	} else {
		// Neither style provided -- error
		r.Log.Error(nil, "Neither instDetails nor instanceDetails is provided. One must be set.")
		return resultQ, fmt.Errorf("invalid specification: must provide either instDetails or instanceDetails")
	}
	// Execute for every reconcile except deletion where it give error in logs
	if racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {
		defer r.updateReconcileStatus(racDatabase, ctx, req, &result, &err, &blocked, &completed, isOldStyle)
	}

	// Retrieve the old spec from annotations
	oldSpec, err := r.GetOldSpec(racDatabase)
	if err != nil {
		r.Log.Error(err, "Failed to update old spec annotation")
		racDatabase.Spec.IsFailed = true
		return resultQ, nil
	}

	// Initialize racDatabase.Status if it's not already initialized
	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = &racdb.RacInitParams{}
	}

	// Initialize ConfigParams fields if they are not already initialized
	if racDatabase.Status.ConfigParams.DbHome == "" {
		racDatabase.Status.ConfigParams.DbHome = string(racdb.RACFieldNotDefined)
	}

	if racDatabase.Status.DbState == "" {
		racDatabase.Status.State = string(racdb.RACPendingState)
		racDatabase.Status.DbState = string(racdb.RACPendingState)
		racDatabase.Status.Role = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ConnectString = string(racdb.RACFieldNotDefined)
		racDatabase.Status.PdbConnectString = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ReleaseUpdate = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ConfigParams.DbHome = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ConfigParams.GridHome = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ClientEtcHost = []string{string(racdb.RACFieldNotDefined)}
		r.Status().Update(ctx, racDatabase)
	}

	// Manage RACDatabase Deletion , if delete topology is called
	err = r.manageRacDatabaseDeletion(req, ctx, racDatabase, isOldStyle)
	if err != nil {
		result = resultNq
		return result, err
	}

	// cleanup RAC Instance
	if oldSpec != nil {
		_, err = r.cleanupRacInstance(req, ctx, racDatabase, isOldStyle, oldSpec)
		if err != nil {
			result = resultQ
			r.Log.Info(err.Error())
			return result, nilErr
		}
	} else {
		_, err = r.cleanupRacInstance(req, ctx, racDatabase, isOldStyle, &racdb.RacDatabaseSpec{})
		if err != nil {
			result = resultQ
			r.Log.Info(err.Error())
			return result, nilErr
		}
	}

	podList := &corev1.PodList{}

	err = r.List(ctx, podList,
		client.InNamespace(req.Namespace),
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	ownedPods := podsOwnedByRacDatabase(podList.Items, racDatabase)

	handled, err := updatePendingStateIfAny(ctx, r, racDatabase, ownedPods)
	if err != nil {
		return ctrl.Result{}, err
	}
	if handled {
		r.Log.Info("Some RAC pods are Pending; requeueing")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if racDatabase.Status.State == string(racdb.RACFailedState) &&
		racDatabase.Generation != racDatabase.Status.ObservedGeneration {

		r.Log.Info("Spec updated after FAILED state — allowing recovery")

		racDatabase.Status.State = string(racdb.RACUpdateState)
		racDatabase.Spec.IsFailed = false

		err := r.updateStatusWithRetry(ctx, req, func(latest *racdb.RacDatabase) {
			latest.Status = racDatabase.Status
		})
		if err != nil {
			return resultQ, err
		}
	}

	webhooksEnabled := os.Getenv("ENABLE_WEBHOOKS") != "false"

	if webhooksEnabled {
		err = checkRACStateAndReturn(racDatabase)
		if err != nil {
			result = resultQ
			r.Log.Info("RAC object is in restricted state, returning back")
			return result, nilErr
		}
	} else {
		r.Log.Info("Webhooks disabled — skipping RAC state validation")
	}

	// If the object is being deleted, stop reconcile here
	if racDatabase.GetDeletionTimestamp() != nil {
		r.Log.Info("RacDatabase is being deleted, skipping normal reconcile")
		return ctrl.Result{}, nil
	}
	// set defaults
	var cName, fName string

	cp := racDatabase.Spec.ConfigParams
	if cp != nil {

		// Prefer Grid response file
		if cp.GridResponseFile != nil {
			if cp.GridResponseFile.ConfigMapName != "" {
				cName = cp.GridResponseFile.ConfigMapName
			}
			if cp.GridResponseFile.Name != "" {
				fName = cp.GridResponseFile.Name
			}
		}

		// Fallback to DB response file (only if grid not set)
		if cName == "" && fName == "" && cp.DbResponseFile != nil {
			if cp.DbResponseFile.ConfigMapName != "" {
				cName = cp.DbResponseFile.ConfigMapName
			}
			if cp.DbResponseFile.Name != "" {
				fName = cp.DbResponseFile.Name
			}
		}
	}

	err = setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(racDatabase, r.Client, cName, fName)
	if err != nil {
		r.Log.Info("Failed to set disk group defaults")
		return ctrl.Result{}, err
	}

	// First Validate
	err = r.validateSpex(racDatabase, oldSpec, ctx)
	if err != nil {
		r.Log.Info("Spec validation failed")
		result = resultQ
		r.Log.Info(err.Error())
		return result, nilErr
	}

	// Update RAC ConfigParams
	err = r.updateGiConfigParamStatus(racDatabase)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		result = resultQ
		r.Log.Info(err.Error())
		return result, nilErr
	}

	err = r.updateDbConfigParamStatus(racDatabase)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		result = resultQ
		r.Log.Info(err.Error())
		err = nilErr
		return result, err
	}

	// Service creation
	// Following check and loop will make sure  to create the service
	if racDatabase.Spec.ExternalSvcType != nil {
		svcType = *racDatabase.Spec.ExternalSvcType
	} else {
		svcType = "nodeport"
	}
	if isOldStyle {
		for i = 0; i < int32(len(racDatabase.Spec.InstDetails)); i++ {
			if !utils.CheckStatusFlag(racDatabase.Spec.InstDetails[i].IsDelete) {
				result, err = r.createOrReplaceService(ctx, racDatabase, raccommon.BuildServiceDefForRac(racDatabase, 0, racDatabase.Spec.InstDetails[i], "vip"))
				if err != nil {
					result = resultNq
					return result, err
				}

				result, err = r.createOrReplaceService(ctx, racDatabase, raccommon.BuildServiceDefForRac(racDatabase, 0, racDatabase.Spec.InstDetails[i], "local"))
				if err != nil {
					result = resultNq
					return result, err
				}

				result, err = r.createOrReplaceService(ctx, racDatabase, raccommon.BuildServiceDefForRac(racDatabase, 0, racDatabase.Spec.InstDetails[i], "scan"))
				if err != nil {
					result = resultNq
					return result, err
				}

				if racDatabase.Spec.InstDetails[i].OnsTargetPort != nil {
					result, err = r.createOrReplaceService(ctx, racDatabase, raccommon.BuildExternalServiceDefForRac(racDatabase, 0, racDatabase.Spec.InstDetails[i], svcType, "onssvc"))
					if err != nil {
						result = resultNq
						return result, err
					}
				}

				if racDatabase.Spec.InstDetails[i].LsnrTargetPort != nil {
					result, err = r.createOrReplaceService(ctx, racDatabase, raccommon.BuildExternalServiceDefForRac(racDatabase, int32(i), racDatabase.Spec.InstDetails[i], svcType, "lsnrsvc"))
					if err != nil {
						result = resultNq
						return result, err
					}
				}

				if len(oraRacInst.NodePortSvc) != 0 {
					for index, _ := range oraRacInst.NodePortSvc {
						result, err = r.createOrReplaceService(ctx, racDatabase, raccommon.BuildExternalServiceDefForRac(racDatabase, int32(index), racDatabase.Spec.InstDetails[i], "nodeport", "nodeport"))
						if err != nil {
							result = resultNq
							return result, err
						}

					}
				}
			}
		}

		// Creating RAC Service

		if racDatabase.Spec.ScanSvcTargetPort != nil {
			result, err = r.createOrReplaceService(ctx, racDatabase, raccommon.BuildExternalServiceDefForRac(racDatabase, int32(0), racDatabase.Spec.InstDetails[int32(0)], svcType, "scansvc"))
			if err != nil {
				result = resultNq
				return result, err
			}
		}
	} else {
		cd := racDatabase.Spec.ClusterDetails
		for i := 0; i < cd.NodeCount; i++ {
			// nodeName := fmt.Sprintf("%s-%d", cd.RacNodeName, i)

			// VIP Service
			result, err = r.createOrReplaceService(ctx, racDatabase,
				raccommon.BuildClusterServiceDefForRac(racDatabase, cd, i, "vip"))
			if err != nil {
				result = resultNq
				return result, err
			}

			// Local Service
			result, err = r.createOrReplaceService(ctx, racDatabase,
				raccommon.BuildClusterServiceDefForRac(racDatabase, cd, i, "local"))
			if err != nil {
				result = resultNq
				return result, err
			}

			// ONS Service, use per-node port
			if cd.BaseOnsTargetPort > 0 {
				result, err = r.createOrReplaceService(ctx, racDatabase,
					raccommon.BuildClusterExternalServiceDefForRac(racDatabase, cd, i, svcType, "onssvc"))
				if err != nil {
					result = resultNq
					return result, err
				}
			}

			// Listener Service, use per-node port
			if cd.BaseLsnrTargetPort > 0 {
				result, err = r.createOrReplaceService(ctx, racDatabase,
					raccommon.BuildClusterExternalServiceDefForRac(racDatabase, cd, i, svcType, "lsnrsvc"))
				if err != nil {
					result = resultNq
					return result, err
				}
			}
			// Scan Service -- likely same for all nodes, so could create only once for i == 0
			if i == 0 {
				result, err = r.createOrReplaceService(ctx, racDatabase,
					raccommon.BuildClusterExternalServiceDefForRac(racDatabase, cd, i, svcType, "scansvc"))
				if err != nil {
					result = resultNq
					return result, err
				}
			}

		}
		// run only once
		result, err = r.createOrReplaceService(ctx, racDatabase,
			raccommon.BuildClusterServiceDefForRac(racDatabase, cd, 0, "scan"))
		if err != nil {
			result = resultNq
			return result, err
		}
	}

	r.ensureAsmStorageStatus(racDatabase)

	isNewSetup := true
	upgradeSetup := false

	// Detect upgrade scenario — if old spec has no ASM storage details
	if oldSpec != nil && oldSpec.OldAsmStorageDetails != nil {
		upgradeSetup = true
		isNewSetup = false // explicitly not a new install
		r.Log.Info("Detected upgrade scenario — marking upgradeSetup = true")
	} else {
		// Normal check for new setups
		for _, diskgroup := range racDatabase.Status.AsmDiskGroups {
			if len(diskgroup.Disks) > 0 && diskgroup.Name != "Pending" {
				isNewSetup = false
				break
			}
		}
	}

	isDiskChanged := false
	addedAsmDisks := []string{}
	removedAsmDisks := []string{}

	if !isNewSetup && oldSpec != nil {
		addedAsmDisks, removedAsmDisks, err = r.computeDiskChanges(racDatabase, oldSpec)
		if err != nil {
			return ctrl.Result{}, err
		}
		//debugger
		// addedAsmDisks := []string{"/dev/disk/by-partlabel/ocne_asm_disk_03"}

		// Cannot process add & remove together
		if len(addedAsmDisks) > 0 && len(removedAsmDisks) > 0 {
			r.Log.Info("Detected addition as well as deletion; cannot process both together",
				"addedAsmDisks", addedAsmDisks, "removedAsmDisks", removedAsmDisks)
			return resultQ, fmt.Errorf("cannot add and remove ASM disks in the same step")
		}

		// Set change flags and log
		if len(addedAsmDisks) > 0 {
			r.Log.Info("Detected addition of ASM disks", "addedAsmDisks", addedAsmDisks)
			isDiskChanged = true
		}
		if len(removedAsmDisks) > 0 {
			r.Log.Info("Detected removal of ASM disks", "removedAsmDisks", removedAsmDisks)
			isDiskChanged = true
		}
	}
	// Check if any ASM disk has missing/zero size
	missingSize := false
	for _, dg := range racDatabase.Status.AsmDiskGroups {
		for _, disk := range dg.Disks {
			if disk.SizeInGb == 0 {
				missingSize = true
				break
			}
		}
		if missingSize {
			break
		}
	}

	shouldRunDiscovery :=
		(len(removedAsmDisks) == 0) && (isNewSetup ||
			upgradeSetup ||
			missingSize ||
			len(addedAsmDisks) > 0 ||
			len(racDatabase.Status.AsmDiskGroups) == 0)

	var disks []racdb.AsmDiskStatus

	if shouldRunDiscovery {
		r.Log.Info("ASM discovery decision",
			"removed", len(removedAsmDisks),
			"added", len(addedAsmDisks),
			"missingSize", missingSize,
			"isNewSetup", isNewSetup,
			"upgradeSetup", upgradeSetup,
			"dgCount", len(racDatabase.Status.AsmDiskGroups),
		)

		if err := r.createDaemonSet(racDatabase, ctx, isOldStyle); err != nil {
			r.Log.Error(err, "failed to create disk-check daemonset")
			return ctrl.Result{}, err // Return error to requeue on failure
		}

		ready, err := checkRacDaemonSetStatusforRAC(ctx, r, racDatabase)
		if err != nil {
			r.Log.Error(err, "ASM disk-check daemonset status error, cleaning up")

			_ = r.cleanupDaemonSet(racDatabase, ctx, isOldStyle)

			racDatabase.Status.State = string(racdb.RACFailedState)

			meta.SetStatusCondition(&racDatabase.Status.Conditions, metav1.Condition{
				Type:               string(racdb.CrdReconcileErrorState),
				Status:             metav1.ConditionTrue,
				Reason:             string(racdb.CrdReconcileErrorReason),
				Message:            err.Error(),
				ObservedGeneration: racDatabase.Generation,
				LastTransitionTime: metav1.Now(),
			})

			err := r.updateStatusWithRetry(ctx, req, func(latest *racdb.RacDatabase) {
				latest.Status = racDatabase.Status
			})
			if err != nil {
				return resultNq, err
			}

			return resultNq, nil
		}

		if !ready {
			// Not ready is NOT an error → no cleanup
			r.Log.Info("ASM disks not ready yet. Waiting for disk-check daemonset to complete discovery.")

			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		// Update disk sizes into Status AND get discovered disks
		disks, err = r.updateDiskSizes(ctx, racDatabase)
		if err != nil {
			r.Log.Error(err, "failed updating disk sizes")
		}

	}

	// PV/PVC creation using discovered sizes
	if len(racDatabase.Status.AsmDiskGroups) == 0 {
		return resultNq, fmt.Errorf("no ASM disk group status available")
	}
	err = setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(racDatabase, r.Client, cName, fName)
	if err != nil {
		r.Log.Info("Failed to set disk group defaults")
		return ctrl.Result{}, err
	}

	diskStatusMap := make(map[string]racdb.AsmDiskStatus)
	for _, d := range disks {
		diskStatusMap[d.Name] = d
	}

	for dgIndex, dgSpec := range racDatabase.Spec.AsmStorageDetails {
		groupName := dgSpec.Name
		dgType := dgSpec.Type

		// --- CASE 1: OTHERS (mount-only, no ASM group) ---
		if dgType == racdb.OthersDiskDg {
			for diskIdx, diskName := range dgSpec.Disks {
				diskStatus, ok := diskStatusMap[diskName]
				if !ok || !diskStatus.Valid || diskStatus.SizeInGb == 0 {
					// r.Log.Info("Invalid or missing disk status for OTHERS disk, skipping",
					// 	"disk", diskName)
					continue
				}

				sizeStr := fmt.Sprintf("%dGi", diskStatus.SizeInGb)

				pv := raccommon.VolumePVForASM(
					racDatabase, dgIndex, diskIdx,
					diskName, groupName, sizeStr, isOldStyle,
				)
				if _, result, err = r.createOrReplaceAsmPv(ctx, racDatabase, pv, string(dgType)); err != nil {
					return resultNq, err
				}

				pvc := raccommon.VolumePVCForASM(
					racDatabase, dgIndex, diskIdx,
					diskName, groupName, sizeStr,
				)
				if _, result, err = r.createOrReplaceAsmPvC(ctx, racDatabase, pvc, string(dgType)); err != nil {
					return resultNq, err
				}
			}
			continue
		}

		// --- CASE 2: Real ASM disk groups ---
		var dgStatus *racdb.AsmDiskGroupStatus
		for i, dgSt := range racDatabase.Status.AsmDiskGroups {
			if dgSt.Name == groupName {
				dgStatus = &racDatabase.Status.AsmDiskGroups[i]
				break
			}
		}
		if dgStatus == nil {
			r.Log.Info("ASM disk group not present in status, skipping", "diskGroup", groupName)
			continue
		}

		for diskIdx, diskName := range dgSpec.Disks {
			var diskStatus *racdb.AsmDiskStatus
			for i, d := range dgStatus.Disks {
				if d.Name == diskName {
					diskStatus = &dgStatus.Disks[i]
					break
				}
			}
			if diskStatus == nil || !diskStatus.Valid || diskStatus.SizeInGb == 0 {
				continue
			}

			sizeStr := fmt.Sprintf("%dGi", diskStatus.SizeInGb)

			pv := raccommon.VolumePVForASM(
				racDatabase, dgIndex, diskIdx,
				diskName, groupName, sizeStr, isOldStyle,
			)
			if _, result, err = r.createOrReplaceAsmPv(ctx, racDatabase, pv, string(dgType)); err != nil {
				return resultNq, err
			}

			pvc := raccommon.VolumePVCForASM(
				racDatabase, dgIndex, diskIdx,
				diskName, groupName, sizeStr,
			)
			if _, result, err = r.createOrReplaceAsmPvC(ctx, racDatabase, pvc, string(dgType)); err != nil {
				return resultNq, err
			}
		}
	}

	err = r.cleanupDaemonSet(racDatabase, ctx, isOldStyle)
	if err != nil {
		result = resultQ
		// r.Log.Info(err.Error())
		err = nilErr
		return result, err
	}
	// }
	// Continue with ConfigMap and StatefulSet creation...

	if racDatabase.Spec.ConfigParams != nil {
		configMapData, err = r.generateConfigMap(racDatabase, isOldStyle)
		if err != nil {
			result = resultNq
			return result, err
		}
	}
	if isOldStyle && len(racDatabase.Spec.InstDetails) > 0 {

		if len(racDatabase.Spec.InstDetails) > 0 {
			for index := range racDatabase.Spec.InstDetails {
				// Determine if this is the last iteration for statefulset
				isLast := index == len(racDatabase.Spec.InstDetails)-1
				oldState := racDatabase.Status.State
				// check if its delete statefulset execution
				if !utils.CheckStatusFlag(racDatabase.Spec.InstDetails[index].IsDelete) {
					switch {
					case isNewSetup || !isDiskChanged:
						cmName := racDatabase.Spec.InstDetails[index].Name + racDatabase.Name + "-cmap"
						cm := raccommon.ConfigMapSpecs(racDatabase, configMapData, cmName)
						result, err = r.createConfigMap(ctx, racDatabase, cm)
						if err != nil {
							result = resultNq
							return result, err
						}
						racDatabase.Spec.InstDetails[index].EnvFile = cmName
						// Call createOrReplaceSfs first time and without change
						// dep := raccommon.BuildStatefulSetForRac(racDatabase, racDatabase.Spec.InstDetails[index], r.Client)
						dep, err := raccommon.BuildStatefulSetForRac(racDatabase, racDatabase.Spec.InstDetails[index], r.Client)
						if err != nil {
							result = resultNq
							return result, err
						}

						result, err = r.createOrReplaceSfs(ctx, req, racDatabase, dep, index, isLast, oldState, isOldStyle)
						if err != nil {
							result = resultNq
							return result, err
						}

					case isDiskChanged && !isNewSetup:

						cmName := racDatabase.Spec.InstDetails[index].Name + racDatabase.Name + "-cmap"
						configMapDataAutoUpdate, err := r.generateConfigMapAutoUpdate(ctx, racDatabase, cmName)
						if err != nil {
							result = resultNq
							return result, err
						}
						result, err = r.updateConfigMap(ctx, racDatabase, configMapDataAutoUpdate, cmName)
						if err != nil {
							result = resultNq
							return result, err
						}
						r.Log.Info("Config Map updated successfully with new asm details")
						racDatabase.Spec.InstDetails[index].EnvFile = cmName
						// Call createOrReplaceSfs with new ASM Devices and Auto update
						// dep := raccommon.BuildStatefulSetForRac(racDatabase, racDatabase.Spec.InstDetails[index], r.Client)
						dep, err := raccommon.BuildStatefulSetForRac(racDatabase, racDatabase.Spec.InstDetails[index], r.Client)
						if err != nil {
							result = resultNq
							return result, err
						}
						result, err = r.createOrReplaceSfsAsm(ctx, req, racDatabase, dep, index, isLast, oldSpec, isOldStyle)
						if err != nil {
							result = resultNq
							return result, err
						}

					}

				}
			}

		}
	} else if !isOldStyle && racDatabase.Spec.ClusterDetails != nil {
		// --- New-style, cluster-level creation ---
		cd := racDatabase.Spec.ClusterDetails
		// Flag similar to old-style condition
		isDiskChangedNew := isDiskChanged && !isNewSetup
		err = raccommon.CreateServiceAccountIfNotExists(racDatabase, r.Client)
		if err != nil {
			result = resultNq
			return result, err
		}

		for i := 0; i < cd.NodeCount; i++ {

			isLast := i == int(cd.NodeCount)-1
			nodeName := fmt.Sprintf("%s%d", cd.RacNodeName, i+1)
			cmName := nodeName + racDatabase.Name + "-cmap"

			// Mirror old-style switch block
			switch {
			//
			// ─────────────────────────────────────────────
			// CASE 1: New setup OR disk not changed
			// ─────────────────────────────────────────────
			//
			case isNewSetup || !isDiskChangedNew:

				cm := raccommon.ConfigMapSpecs(racDatabase, configMapData, cmName)
				result, err = r.createConfigMap(ctx, racDatabase, cm)
				if err != nil {
					result = resultNq
					return result, err
				}

				spec := raccommon.BuildStatefulSpecForRacCluster(racDatabase, cd, i, r.Client)
				dep := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nodeName,
						Namespace: racDatabase.Namespace,
						Labels:    raccommon.BuildLabelsForRac(racDatabase, "RAC"),
					},
					Spec: *spec,
				}

				result, err = r.createOrReplaceSfs(
					ctx, req, racDatabase, dep, i, isLast, racDatabase.Status.State, isOldStyle,
				)
				if err != nil {
					result = resultNq
					return result, err
				}

			//
			// ─────────────────────────────────────────────
			// CASE 2: Disk changed AND NOT new setup → ASM update
			// ─────────────────────────────────────────────
			//
			case isDiskChangedNew && !isNewSetup:

				configMapDataAutoUpdate, err :=
					r.generateConfigMapAutoUpdateCluster(ctx, racDatabase, cmName)
				if err != nil {
					result = resultNq
					return result, err
				}

				result, err = r.updateConfigMap(ctx, racDatabase, configMapDataAutoUpdate, cmName)
				if err != nil {
					result = resultNq
					return result, err
				}

				r.Log.Info("ConfigMap updated successfully with new ASM disk details (new-style cluster mode)")

				spec := raccommon.BuildStatefulSpecForRacCluster(racDatabase, cd, i, r.Client)
				dep := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nodeName,
						Namespace: racDatabase.Namespace,
						Labels:    raccommon.BuildLabelsForRac(racDatabase, "RAC"),
					},
					Spec: *spec,
				}

				// ---- Same pattern as old-style createOrReplaceSfsAsm ----
				result, err = r.createOrReplaceSfsAsmCluster(
					ctx, req, racDatabase, dep, i, isLast, oldSpec, isOldStyle,
				)
				if err != nil {
					result = resultNq
					return result, err
				}
			}
		}

	}
	completed = true
	// Update the current spec + observedGeneration after successful reconciliation
	if err := r.SetCurrentSpecAndObservedGeneration(ctx, racDatabase, req); err != nil {
		r.Log.Error(err, "Failed to persist current spec / observed generation")
		racDatabase.Spec.IsFailed = true
		return resultQ, err
	}
	// r.updateReconcileStatus(racDatabase, ctx, req, &result, &err, &blocked, &completed)
	r.Log.Info("Reconcile completed. Requeuing....")
	return resultQ, nil
}
func podsOwnedByRacDatabase(pods []corev1.Pod, racdb *racdb.RacDatabase) []corev1.Pod {
	var owned []corev1.Pod

	// Determine RAC node name prefix
	var nodePrefix string
	if racdb.Spec.ClusterDetails != nil {
		nodePrefix = racdb.Spec.ClusterDetails.RacNodeName
	} else if len(racdb.Spec.InstDetails) > 0 {
		nodePrefix = racdb.Spec.InstDetails[0].Name
	}

	for _, pod := range pods {

		// Namespace must match
		if pod.Namespace != racdb.Namespace {
			continue
		}

		// Pod name must match RAC naming convention: <racNodeName>-<ordinal>
		// Examples: racnode1-0, racnode2-0
		if nodePrefix != "" && strings.HasPrefix(pod.Name, nodePrefix) {
			owned = append(owned, pod)
			continue
		}

		// Fallback: match via StatefulSet owner reference name
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "StatefulSet" &&
				nodePrefix != "" &&
				strings.HasPrefix(ref.Name, nodePrefix) {
				owned = append(owned, pod)
				break
			}
		}
	}

	return owned
}

func updatePendingStateIfAny(
	ctx context.Context,
	r *RacDatabaseReconciler,
	racDatabase *racdb.RacDatabase,
	pods []corev1.Pod,
) (bool, error) {

	var pendingPods []string
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodPending {
			pendingPods = append(pendingPods, pod.Name)
		}
	}

	if len(pendingPods) == 0 {
		return false, nil
	}

	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
	)

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {

		// Always fetch latest version
		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(racDatabase), latest); err != nil {
			return true, err
		}

		// ----- Update only fields owned by this logic -----
		latest.Status.State = string(racdb.RACPendingState)
		latest.Status.DbState = string(racdb.RACPendingState)

		// Merge or upsert condition (do NOT overwrite entire slice)
		cond := metav1.Condition{
			Type:               string(racdb.RacCrdReconcileWaitingState),
			Status:             metav1.ConditionTrue,
			Reason:             string(racdb.RacCrdReconcileWaitingReason),
			Message:            fmt.Sprintf("RAC pods pending: %v", pendingPods),
			ObservedGeneration: latest.Generation,
			LastTransitionTime: metav1.Now(),
		}
		meta.SetStatusCondition(&latest.Status.Conditions, cond)

		// Attempt update
		err := r.Status().Update(ctx, latest)
		if err == nil {
			// Success → copy back so caller continues with fresh object
			racDatabase.Status = latest.Status
			racDatabase.ResourceVersion = latest.ResourceVersion
			return true, nil
		}

		if apierrors.IsConflict(err) {
			r.Log.Info(
				"Conflict updating pending state, retrying",
				"attempt", attempt,
				"racDatabase", racDatabase.Name,
			)
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}

		// Non-conflict error → return immediately
		return true, err
	}

	return true, fmt.Errorf(
		"failed to update pending state after %d retries: %w",
		maxRetries, lastErr,
	)
}

// computeDiskChanges inspects spec and recorded status to find ASM disks
// that were added or removed since the last reconcile cycle. It blends spec
// deltas with runtime observations to decide follow-up operations.
func (r *RacDatabaseReconciler) computeDiskChanges(
	racDatabase *racdb.RacDatabase,
	oldSpec *racdb.RacDatabaseSpec,
) (addedAsmDisks []string, removedAsmDisks []string, err error) {

	if oldSpec == nil {
		return nil, nil, nil
	}
	podList := &corev1.PodList{}
	_ = r.Client.List(context.TODO(), podList,
		client.InNamespace(racDatabase.Namespace),
	)

	ownedPods := podsOwnedByRacDatabase(podList.Items, racDatabase)

	if len(ownedPods) > 0 && !isRacSetupStable(racDatabase, ownedPods) {
		r.Log.Info(
			"RAC setup not stable yet — skipping ASM runtime disk diff",
			"state", racDatabase.Status.State,
			"pods", len(ownedPods),
		)
		// IMPORTANT: return only spec-based changes
		addedAsmDisks, removedAsmDisks = getRACDisksChangedSpecforRAC(*racDatabase, *oldSpec)
		return addedAsmDisks, removedAsmDisks, nil

	}

	// 1. Compare spec changes
	addedAsmDisks, removedAsmDisks = getRACDisksChangedSpecforRAC(*racDatabase, *oldSpec)

	// --- NEW: If any diskgroup's AutoUpdate toggled to true, perform ASM state diff ---
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		// Defensive: bounds/slice check for oldSpec
		if i >= len(oldSpec.AsmStorageDetails) {
			continue
		}
		if dg.Type == racdb.OthersDiskDg {
			continue
		}
		// if strings.ToLower(oldSpec.AsmStorageDetails[i].AutoUpdate) == "false" &&
		// 	strings.ToLower(dg.AutoUpdate) == "true" {

		// Get pod name of at least one active node (use 1st node; change as needed)
		sfsName := racDatabase.Spec.ClusterDetails.RacNodeName + "1"

		// Safely get statefulset and pod list
		racSfSet, err := raccommon.CheckSfset(sfsName, racDatabase, r.Client)
		if err != nil {
			return nil, nil, fmt.Errorf("failed retrieving StatefulSet '%s': %w", sfsName, err)
		}
		inst := racdb.RacInstDetailSpec{Name: sfsName}
		podList, err := raccommon.GetPodList(racSfSet.Name, racDatabase, r.Client, inst)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot get pod list for ASM inspection: %w", err)
		}
		if len(podList.Items) == 0 {
			return nil, nil, fmt.Errorf("no pods available for ASM disk validation")
		}

		// Use the last pod (or any pod)
		podName := podList.Items[len(podList.Items)-1].Name

		// Use helper to query true ASM disk state for this diskgroup
		asmGroups := raccommon.GetAsmInstState(podName, racDatabase, i, r.kubeClient, r.kubeConfig, r.Log)

		// Find group for this dg
		var foundState racdb.AsmDiskGroupStatus
		found := false
		for _, group := range asmGroups {
			if group.Name == dg.Name {
				foundState = group
				found = true
				break
			}
		}

		// Build set of actually present disks
		asmPresentDisks := map[string]struct{}{}
		if found {
			for _, diskStatus := range foundState.Disks {
				asmPresentDisks[diskStatus.Name] = struct{}{}
			}
		}

		// For every disk in spec, if not present in ASM, add to "addedAsmDisks"
		for _, specDisk := range dg.Disks {
			if _, inASM := asmPresentDisks[specDisk]; !inASM {
				addedAsmDisks = append(addedAsmDisks, specDisk)
			}
		}
		// }
	}

	// 2. Include disks to add from status (unchanged)
	if disksToAdd, addErr := getDisksToAddStatusforRAC(racDatabase); addErr != nil {
		racDatabase.Spec.IsFailed = true
		return nil, nil, fmt.Errorf("cannot get ASM disks to add: %w", addErr)
	} else if len(disksToAdd) > 0 && len(addedAsmDisks) == 0 {
		addedAsmDisks = disksToAdd
	}

	// 3. Include disks to remove from status (unchanged)
	if disksToRemove, removeErr := getDisksToRemoveStatusforRAC(racDatabase); removeErr != nil {
		racDatabase.Spec.IsFailed = true
		return nil, nil, fmt.Errorf("cannot get ASM disks to remove: %w", removeErr)
	} else if len(disksToRemove) > 0 && len(removedAsmDisks) == 0 {
		removedAsmDisks = disksToRemove
	}

	// 4. Final check for disks pending to be mounted on cluster nodes (unchanged)
	pendingDisks := getPendingDisksToMount(racDatabase)
	if len(pendingDisks) > 0 {
		existing := make(map[string]bool)
		for _, disk := range addedAsmDisks {
			existing[disk] = true
		}
		for _, pdisk := range pendingDisks {
			if !existing[pdisk] {
				addedAsmDisks = append(addedAsmDisks, pdisk)
			}
		}
	}

	// 5. Validate removed disks using ASM runtime state (unchanged)
	if len(removedAsmDisks) > 0 {
		sfsName := racDatabase.Spec.ClusterDetails.RacNodeName + "1"
		racSfSet, err := raccommon.CheckSfset(sfsName, racDatabase, r.Client)
		if err != nil {
			return nil, nil, fmt.Errorf("failed retrieving StatefulSet '%s': %w", sfsName, err)
		}
		inst := racdb.RacInstDetailSpec{Name: sfsName}
		podList, err := raccommon.GetPodList(racSfSet.Name, racDatabase, r.Client, inst)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot get pod list for ASM inspection: %w", err)
		}
		if len(podList.Items) == 0 {
			return nil, nil, fmt.Errorf("no pods available for ASM disk validation")
		}
		podName := podList.Items[len(podList.Items)-1].Name
		asmGroups := raccommon.GetAsmInstState(podName, racDatabase, 0, r.kubeClient, r.kubeConfig, r.Log)
		for _, removed := range removedAsmDisks {
			for _, group := range asmGroups {
				for _, disk := range group.Disks {
					if disk.Name == removed {
						return nil, nil, fmt.Errorf(
							"disk '%s' is still part of diskgroup '%s' and must be removed manually",
							removed, group.Name,
						)
					}
				}
			}
		}
	}

	return addedAsmDisks, removedAsmDisks, nil
}
func isRacSetupStable(racDatabase *racdb.RacDatabase, pods []corev1.Pod) bool {
	// State-based guard
	switch racdb.RacLifecycleState(racDatabase.Status.State) {
	case racdb.RACAvailableState:
		// only AVAILABLE is safe
	default:
		return false
	}

	// Pod-based guard
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status != corev1.ConditionTrue {
				return false
			}
		}
	}

	return true
}

// getPendingDisksToMount walks desired ASM disk entries and cross-checks
// mounted device information from status. It returns disks not yet mounted
// so the controller can trigger discovery or retry orchestration steps.
func getPendingDisksToMount(racDatabase *racdb.RacDatabase) []string {
	pending := make(map[string]bool)
	specDisks := map[string]bool{}
	// Gather all desired disks from spec
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		for _, disk := range dg.Disks {
			if disk != "" {
				specDisks[disk] = true
			}
		}
	}
	// For each node's mounted devices, mark disks as mounted
	mounted := map[string]bool{}
	for _, node := range racDatabase.Status.RacNodes { // assuming .RacNodes slice
		for _, disk := range node.NodeDetails.MountedDevices {
			if disk != "" {
				mounted[disk] = true
			}
		}
	}
	// If a disk exists in spec but is missing from mounted on any node, mark as pending
	for disk := range specDisks {
		if !mounted[disk] {
			pending[disk] = true
		}
	}
	// Flatten set to slice
	pendingList := []string{}
	for disk := range pending {
		pendingList = append(pendingList, disk)
	}
	return pendingList
}

// checkRACStateAndReturn guards reconcile operations against restricted
// lifecycle phases on the RAC object. It returns an error when the state or
// spec flags indicate work should not proceed.
func checkRACStateAndReturn(racDatabase *racdb.RacDatabase) error {

	// Block only transient in-progress states
	switch racDatabase.Status.State {
	case string(racdb.RACProvisionState),
		string(racdb.RACUpdateState),
		string(racdb.RACPodAvailableState),
		string(racdb.RACAddInstState),
		string(racdb.RACDeletingState):
		return fmt.Errorf("oracle RAC database is busy: %s", racDatabase.Status.State)

	case string(racdb.RACManualState):
		return fmt.Errorf("oracle RAC database is in manual state")
	}
	return nil
}

// generateConfigMapAutoUpdate reloads a RAC ConfigMap and refreshes its
// environment payload with current ASM device details. It returns the updated
// data map for reuse when persisting the ConfigMap.
func (r *RacDatabaseReconciler) generateConfigMapAutoUpdate(ctx context.Context, instance *racdb.RacDatabase, cmName string) (map[string]string, error) {
	// Fetch the existing ConfigMap
	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: instance.Namespace}, cm)
	if err != nil {
		return nil, err
	}
	r.Log.Info("Updating existing configmap")
	// Get the existing config map data
	configMapData := cm.Data
	envFileData := configMapData["envfile"]
	envVars := make(map[string]string)

	// Parse the envfile into a map
	lines := strings.Split(envFileData, "\r\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// CRS_ASM_DEVICE_LIST
	if crsList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.CrsAsmDiskDg); crsList != "" {
		envVars["CRS_ASM_DEVICE_LIST"] = crsList
	}
	if recoList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbRecoveryDiskDg); recoList != "" {
		envVars["RECO_ASM_DEVICE_LIST"] = recoList
	}
	if redoList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.RedoDiskDg); redoList != "" {
		envVars["REDO_ASM_DEVICE_LIST"] = redoList
	}
	if dataList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbDataDiskDg); dataList != "" {
		envVars["DATA_ASM_DEVICE_LIST"] = dataList
	}
	// Convert the envVars map back to a single string
	var updatedData []string
	for key, value := range envVars {
		updatedData = append(updatedData, fmt.Sprintf("%s=%s", key, value))
	}
	configMapData["envfile"] = strings.Join(updatedData, "\r\n")

	return configMapData, nil
}

// generateConfigMapAutoUpdateCluster synthesizes cluster-scoped environment
// configuration for RAC nodes. It pulls latest disk information and returns
// an updated data map suited for ConfigMap persistence.
func (r *RacDatabaseReconciler) generateConfigMapAutoUpdateCluster(
	ctx context.Context,
	instance *racdb.RacDatabase,
	cmName string,
) (map[string]string, error) {

	// ---------------------------------------------------------
	// 1. Read existing ConfigMap
	// ---------------------------------------------------------
	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: instance.Namespace}, cm)
	if err != nil {
		return nil, err
	}

	r.Log.Info("Updating existing configmap (cluster-style ASM update)", "ConfigMap", cmName)

	configMapData := cm.Data
	envFile := configMapData["envfile"]

	// Parse into key=value map
	envVars := map[string]string{}
	for _, line := range strings.Split(envFile, "\r\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// ---------------------------------------------------------
	// 2. Extract ASMDG names + redundancy from Spec (NEW MODEL)
	// ---------------------------------------------------------
	crsDiskGroup := ""
	crsRedundancy := ""

	dataDgName := ""
	dataRedundancy := ""

	recoDgName := ""
	recoRedundancy := ""

	redoDgName := ""
	redoRedundancy := ""

	for _, dg := range instance.Spec.AsmStorageDetails {
		switch dg.Type {
		case racdb.CrsAsmDiskDg:
			if dg.Name != "" {
				crsDiskGroup = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				crsRedundancy = dg.Redundancy
			}

		case racdb.DbDataDiskDg:
			if dg.Name != "" {
				dataDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				dataRedundancy = dg.Redundancy
			}

		case racdb.DbRecoveryDiskDg:
			if dg.Name != "" {
				recoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				recoRedundancy = dg.Redundancy
			}

		case racdb.RedoDiskDg:
			if dg.Name != "" {
				redoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				redoRedundancy = dg.Redundancy
			}
		}
	}

	// Fallback default for CRS if not present
	if crsDiskGroup == "" {
		crsDiskGroup = "+DATA"
	}

	// ---------------------------------------------------------
	// 3. Build ASM DEVICE LISTS using runtime Status
	// ---------------------------------------------------------

	crsDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.CrsAsmDiskDg)
	dataDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.DbDataDiskDg)
	recoDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.DbRecoveryDiskDg)
	redoDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.RedoDiskDg)

	// ---------------------------------------------------------
	// 4. Populate envVars (overwrite or add)
	// ---------------------------------------------------------
	// CRS
	envVars["CRS_ASM_DISKGROUP"] = crsDiskGroup
	if crsDeviceList != "" {
		envVars["CRS_ASM_DEVICE_LIST"] = crsDeviceList
	}
	if crsRedundancy != "" {
		envVars["CRS_ASMDG_REDUNDANCY"] = crsRedundancy
	}

	// DATA
	if dataDgName != "" {
		envVars["DB_DATA_FILE_DEST"] = dataDgName
	} else {
		envVars["DB_DATA_FILE_DEST"] = crsDiskGroup
	}

	if dataDeviceList != "" {
		envVars["DB_ASM_DEVICE_LIST"] = dataDeviceList
	}
	if dataRedundancy != "" {
		envVars["DB_ASMDG_PROPERTIES"] = "redundancy:" + dataRedundancy
	}

	// RECO
	if recoDgName != "" {
		envVars["DB_RECOVERY_FILE_DEST"] = recoDgName
	} else {
		envVars["DB_RECOVERY_FILE_DEST"] = crsDiskGroup
	}

	if recoDeviceList != "" {
		envVars["RECO_ASM_DEVICE_LIST"] = recoDeviceList
	}
	if recoRedundancy != "" {
		envVars["RECO_ASMDG_PROPERTIES"] = "redundancy:" + recoRedundancy
	}

	// REDO
	if redoDgName != "" {
		envVars["LOG_FILE_DEST"] = redoDgName
	}
	if redoDeviceList != "" {
		envVars["REDO_ASM_DEVICE_LIST"] = redoDeviceList
	}
	if redoRedundancy != "" {
		envVars["REDO_ASMDG_PROPERTIES"] = "redundancy:" + redoRedundancy
	}

	// Charset default
	if instance.Spec.ConfigParams.DbCharSet == "" {
		instance.Spec.ConfigParams.DbCharSet = "AL32UTF8"
	}
	envVars["DB_CHARACTERSET"] = instance.Spec.ConfigParams.DbCharSet

	// ---------------------------------------------------------
	// 5. Convert back to envfile format
	// ---------------------------------------------------------
	updated := []string{}
	for k, v := range envVars {
		updated = append(updated, fmt.Sprintf("%s=%s", k, v))
	}

	configMapData["envfile"] = strings.Join(updated, "\r\n")

	return configMapData, nil
}

// updateConfigMap writes revised RAC configuration into Kubernetes by
// updating the ConfigMap resource. It ensures the cluster sees the latest
// environment file contents derived during reconcile.
func (r *RacDatabaseReconciler) updateConfigMap(ctx context.Context, instance *racdb.RacDatabase, configMapData map[string]string, cmName string) (ctrl.Result, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: instance.Namespace,
		},
		Data: configMapData,
	}

	err := r.Client.Update(ctx, cm)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateReconcileStatus maintains reconcile conditions and topology details
// after each controller pass. It refreshes status fields based on reconcile
// outcome flags and observed cluster data.
func (r *RacDatabaseReconciler) updateReconcileStatus(racDatabase *racdb.RacDatabase, ctx context.Context, req ctrl.Request, result *ctrl.Result, err *error, blocked *bool, completed *bool, isOldStyle bool) {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	if racDatabase == nil || !racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {
		// object is deleted or being deleted; skip status update
		return
	}

	// First update RAC topology
	if racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {

		podNames, nodeDetails, err1 :=
			r.updateRacInstTopologyStatus(racDatabase, ctx, req, isOldStyle)

		// ---- CASE 1: Pending pods → topology intentionally skipped ----
		if err1 == nil && len(podNames) == 0 {
			r.Log.Info(
				"RAC topology update skipped (pods pending)",
				"racDatabase", racDatabase.Name,
			)
			// Do NOT update DB topology
		}

		// ---- CASE 2: Hard error ----
		if err1 != nil {
			r.Log.Info(
				"RAC topology update encountered a non-fatal issue",
				"racDatabase", racDatabase.Name,
				"error", err1,
			)
		}

		// ---- CASE 3: Topology valid → update DB topology ----
		if len(podNames) > 0 {
			if err := r.updateRacDbTopologyStatus(
				racDatabase, ctx, req, podNames, nodeDetails,
			); err != nil {
				r.Log.Error(
					err,
					"Failed to update RAC DB topology",
					"racDatabase", racDatabase.Name,
				)
			}
		}
	}

	// ---------------------------------------------
	// CLEAN OLD RECONCILE CONDITIONS
	// ---------------------------------------------
	for _, t := range []string{
		string(racdb.CrdReconcileCompeleteState),
		string(racdb.CrdReconcileQueuedState),
		string(racdb.CrdReconcileWaitingState),
	} {
		meta.RemoveStatusCondition(&racDatabase.Status.Conditions, t)
	}
	// ---------------------------------------------
	// BUILD NEW CONDITION
	// ---------------------------------------------
	errMsg := func() string {
		if *err != nil {
			return (*err).Error()
		}
		return "no reconcile errors"
	}()

	var condition metav1.Condition

	switch {
	case *completed:
		condition = metav1.Condition{
			Type:               string(racdb.CrdReconcileCompeleteState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.RacCrdReconcileCompleteReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	case *blocked:
		condition = metav1.Condition{
			Type:               string(racdb.CrdReconcileWaitingState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.RacCrdReconcileWaitingReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	case result.Requeue:
		condition = metav1.Condition{
			Type:               string(racdb.CrdReconcileQueuedState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.CrdReconcileQueuedReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	case err != nil && *err != nil:
		condition = metav1.Condition{
			Type:               string(racdb.CrdReconcileErrorState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.CrdReconcileErrorReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	default:
		return
	}

	// ---------------------------------------------
	// SET ONLY THE NEW CONDITION
	// ---------------------------------------------
	meta.SetStatusCondition(&racDatabase.Status.Conditions, condition)

	if racDatabase.Status.State == string(racdb.RACPodAvailableState) && condition.Type == string(racdb.CrdReconcileCompeleteState) {
		r.Log.Info("All validations and updation are completed. Changing State to AVAILABLE")
		racDatabase.Status.State = string(racdb.RACAvailableState)
	}
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch the latest version of the object
		latestInstance := &racdb.RacDatabase{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if apierrors.IsNotFound(err) {
			// Object already deleted, no update needed
			return
		}
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of RAC instance, retrying...")
			time.Sleep(retryDelay)
			continue // Retry fetching the latest instance
		}

		// Merge the instance fields into latestInstance
		err = mergeRacInstancesFromLatest(racDatabase, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to merge instances, retrying...")
			time.Sleep(retryDelay)
			continue // Retry merging
		}

		// Update the ResourceVersion of instance from latestInstance to avoid conflict
		racDatabase.ResourceVersion = latestInstance.ResourceVersion
		err = r.Client.Status().Patch(ctx, racDatabase, client.MergeFrom(latestInstance))

		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected, retrying update...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				continue // Retry on conflict
			}
			r.Log.Error(err, "Failed to update the RAC DB instance, retrying...")
			time.Sleep(retryDelay)
			continue // Retry on other errors
		}

		// If update was successful, exit the loop
		r.Log.Info("Updated RAC instance status successfully", "Instance", racDatabase.Name)
		break
	}

	r.Log.Info("Returning from updateReconcileStatus")
	if racDatabase == nil || !racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {
		r.Log.Info("Completed Rac Database Deletion, no more reconcile required")
	}
}

// validateSpex performs comprehensive validation of RAC database specs.
// It checks secrets, response files, network capacity, and ASM settings to
// ensure reconcile work proceeds with well-formed configuration.
func (r *RacDatabaseReconciler) validateSpex(racDatabase *racdb.RacDatabase, oldSpec *racdb.RacDatabaseSpec, ctx context.Context) error {
	var err error
	eventReason := "Spec Error"
	//var eventMsgs []string

	r.Log.Info("Entering reconcile validation")

	//First check image pull secrets
	if racDatabase.Spec.ImagePullSecret != "" {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: racDatabase.Spec.ImagePullSecret, Namespace: racDatabase.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Secret not found
				r.Recorder.Eventf(racDatabase, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				return err
			}
			r.Log.Error(err, err.Error())
			return err
		}
	}

	// ========  Config Params Checks
	// Checking Secret for ssh key
	privKeyFlag, pubKeyFlag := raccommon.GetSSHkey(racDatabase, racDatabase.Spec.SshKeySecret.Name, r.Client)
	if !privKeyFlag {
		return errors.New("private key name is not set to " + racDatabase.Spec.SshKeySecret.PrivKeySecretName + " in SshKeySecret")
	}
	if !pubKeyFlag {
		return errors.New("public key name is not set to " + racDatabase.Spec.SshKeySecret.PubKeySecretName + " in SshKeySecret")
	}

	// Checking Gi Responsefile
	cfg := racDatabase.Spec.ConfigParams

	// ---------------------------
	// GRID RESPONSE FILE CHECK
	// ---------------------------
	if cfg != nil && cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {

		giRspFlg, _ := raccommon.GetGiResponseFile(racDatabase, r.Client)

		if !giRspFlg {
			return errors.New("GridResponseFile name must be " + cfg.GridResponseFile.Name)
		}
	}

	// ---------------------------
	// DB RESPONSE FILE CHECK
	// ---------------------------
	if cfg != nil && cfg.DbResponseFile != nil && cfg.DbResponseFile.ConfigMapName != "" {

		DbRspFlg, _ := raccommon.GetDbResponseFile(racDatabase, r.Client)

		if !DbRspFlg {
			return errors.New("DbResponseFile name must be " + cfg.DbResponseFile.Name)
		}
	}

	r.ensureAsmStorageStatus(racDatabase)
	_, diskRemoveErr := getDisksToRemoveStatusforRAC(racDatabase)
	if diskRemoveErr != nil {
		racDatabase.Spec.IsFailed = true
		return diskRemoveErr
	}
	for _, statusDG := range racDatabase.Status.AsmDiskGroups {
		// Find matching group in spec
		var specDisks []string
		for _, specDG := range racDatabase.Spec.AsmStorageDetails {
			if specDG.Name == statusDG.Name {
				specDisks = specDG.Disks
				break
			}
		}
		if specDisks == nil {
			// Optionally skip or handle missing group
			continue
		}
		// Collect status side disk names for this group
		var statusDiskNames []string
		for _, disk := range statusDG.Disks {
			statusDiskNames = append(statusDiskNames, disk.Name)
		}
		// If number of disks in spec is greater than in status, we're trying to add new disks
		if len(specDisks) > len(statusDiskNames) {
			if _, err := findRacDisksToAddforRAC(specDisks, statusDiskNames, racDatabase, oldSpec); err != nil {
				return err
			}
		}
	}
	// Checking the network cards in response files

	cfg = racDatabase.Spec.ConfigParams

	// -----------------------------------------------------------------------------
	// 1. GRID RESPONSE FILE NETWORK INTERFACE VALIDATION (nil-safe)
	// -----------------------------------------------------------------------------
	if cfg != nil && cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {

		netRspData, err := raccommon.CheckRspData(
			racDatabase,
			r.Client,
			"networkInterfaceList",
			cfg.GridResponseFile.ConfigMapName,
			cfg.GridResponseFile.Name,
		)
		if err != nil {
			racDatabase.Spec.IsFailed = true
			return err
		}

		// Validate InstDetails (if provided)
		if len(racDatabase.Spec.InstDetails) > 0 {
			for idx := range racDatabase.Spec.InstDetails {

				isDeleteStr := racDatabase.Spec.InstDetails[idx].IsDelete
				switch isDeleteStr {
				case "true":
					r.Log.Info("Performing operation for IsDelete true")
				default:
					if isDeleteStr != "" {
						r.Log.Info("Unexpected value for IsDelete: " + isDeleteStr)
					}

					// PrivateIPDetails can be nil → must check
					if racDatabase.Spec.InstDetails[idx].PrivateIPDetails != nil {
						for _, iface := range racDatabase.Spec.InstDetails[idx].PrivateIPDetails {
							interfaceName := iface.Interface

							err = raccommon.ValidateNetInterface(interfaceName, racDatabase, netRspData)
							if err != nil {
								racDatabase.Spec.IsFailed = true
								return fmt.Errorf(
									"The network card name '%s' does not match the interface list in the Grid Response File",
									interfaceName,
								)
							}
						}
					}
				}
			}
		}
	}

	// -----------------------------------------------------------------------------
	// 2. GRID RESPONSE FILE EXISTENCE / DISK VALIDATION (nil-safe)
	// -----------------------------------------------------------------------------
	if cfg != nil && cfg.GridResponseFile != nil &&
		cfg.GridResponseFile.ConfigMapName != "" &&
		cfg.GridResponseFile.Name != "" {

		cm := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      cfg.GridResponseFile.ConfigMapName,
			Namespace: racDatabase.Namespace,
		}, cm)
		if err != nil {
			return fmt.Errorf("error fetching Grid ResponseFile ConfigMap: %v", err)
		}

		rspContent, exists := cm.Data[cfg.GridResponseFile.Name]
		if !exists {
			return fmt.Errorf(
				"response file %s not found in ConfigMap %s",
				cfg.GridResponseFile.Name, cfg.GridResponseFile.ConfigMapName,
			)
		}

		// --- Parse ASM disks from RSP file ---
		rspDisks := parseAsmDisksFromRsp(rspContent)

		// --- Collect disks from CRD spec ---
		specDisks := make(map[string]bool)
		for _, dg := range racDatabase.Spec.AsmStorageDetails {
			for _, disk := range dg.Disks {
				specDisks[disk] = true
			}
		}

		// --- Compare RSP disks with CRD spec ---
		for _, disk := range rspDisks {
			if !specDisks[disk] {
				return fmt.Errorf(
					"ASM disk %s appears in Grid Response File but is not listed in the CRD spec",
					disk,
				)
			}
		}
	}
	clusterSpec := racDatabase.Spec.ClusterDetails

	if clusterSpec != nil && len(clusterSpec.PrivateIPDetails) > 0 {
		for _, net := range clusterSpec.PrivateIPDetails {

			// Validate NAD capacity
			err := validateNadIPCapacity(
				clusterSpec.NodeCount,
				racDatabase.Namespace,
				net.Name,
				r.Client,
			)
			if err != nil {
				return field.Invalid(
					field.NewPath("spec").Child("clusterDetails").Child("privateIPDetails"),
					net.Name,
					err.Error(),
				)
			}
		}
	}

	r.Log.Info("Completed Validation of Spex")

	return nil

}

// validateNadIPCapacity confirms the target NAD provides enough IP addresses
// for the requested RAC node count. It parses the NAD IP range and raises
// errors when capacity is insufficient or malformed.
func validateNadIPCapacity(
	nodeCount int,
	namespace, nadName string,
	kClient client.Client,
) error {

	ipam, err := getNadIPAM(namespace, nadName, kClient)
	if err != nil {
		return fmt.Errorf("failed to read IPAM from NAD %s: %v", nadName, err)
	}

	startIP := net.ParseIP(ipam.RangeStart).To4()
	endIP := net.ParseIP(ipam.RangeEnd).To4()

	if startIP == nil || endIP == nil {
		return fmt.Errorf("invalid IPAM rangeStart or rangeEnd in NAD %s", nadName)
	}

	startInt := uint32(startIP[0])<<24 | uint32(startIP[1])<<16 | uint32(startIP[2])<<8 | uint32(startIP[3])
	endInt := uint32(endIP[0])<<24 | uint32(endIP[1])<<16 | uint32(endIP[2])<<8 | uint32(endIP[3])

	if endInt < startInt {
		return fmt.Errorf("NAD %s has an invalid IP range: start > end", nadName)
	}

	totalIPs := int(endInt-startInt) + 1

	if nodeCount > totalIPs {
		return fmt.Errorf(
			"NAD %s has insufficient IP range: only %d usable IPs, but RAC nodeCount=%d",
			nadName, totalIPs, nodeCount,
		)
	}

	return nil
}

// getNadIPAM retrieves the macvlan IPAM configuration from a NAD resource
// so callers can inspect available address ranges during validation logic.
func getNadIPAM(namespace, name string, kClient client.Client) (*racdb.MacvlanIPAM, error) {
	nad := &unstructured.Unstructured{}
	nad.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "k8s.cni.cncf.io",
		Version: "v1",
		Kind:    "NetworkAttachmentDefinition",
	})

	err := kClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, nad)
	if err != nil {
		return nil, err
	}

	configStr, found, err := unstructured.NestedString(nad.Object, "spec", "config")
	if err != nil || !found {
		return nil, fmt.Errorf("NAD spec.config not found")
	}

	var mv *racdb.MacvlanConfig
	if err := json.Unmarshal([]byte(configStr), &mv); err != nil {
		return nil, fmt.Errorf("failed to decode NAD config JSON: %v", err)
	}

	return &mv.IPAM, nil
}

// parseAsmDisksFromRsp scans a Grid response file blob to extract ASM disk
// entries. It normalizes disks listed under supported keys into a string slice.
func parseAsmDisksFromRsp(rspContent string) []string {
	var disks []string
	lines := strings.Split(rspContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var value string
		if strings.HasPrefix(line, "oracle.install.asm.diskGroup.disks=") {
			value = strings.TrimPrefix(line, "oracle.install.asm.diskGroup.disks=")
		} else if strings.HasPrefix(line, "diskList=") {
			value = strings.TrimPrefix(line, "diskList=")
		} else {
			continue
		}
		// Split and collect disks
		for _, d := range strings.Split(value, ",") {
			disk := strings.TrimSpace(d)
			if disk != "" {
				disks = append(disks, disk)
			}
		}
	}
	return disks
}

// getDisksToRemoveStatusforRAC compares spec vs. status to determine which
// ASM disks should be removed. It filters duplicates and validates against
// disk group membership before returning the removal list.
func getDisksToRemoveStatusforRAC(racDatabase *racdb.RacDatabase) ([]string, error) {
	disksToRemove := []string{}
	disksToRemoveSet := make(map[string]struct{})

	for _, statusDG := range racDatabase.Status.AsmDiskGroups {
		// Find matching group in spec
		var specDisks []string
		for _, specDG := range racDatabase.Spec.AsmStorageDetails {
			if specDG.Name == statusDG.Name {
				specDisks = specDG.Disks
				break
			}
		}
		if specDisks == nil {
			continue
		}

		if len(specDisks) < len(statusDG.Disks) {
			// Unique disk names for status group
			statusDiskSet := make(map[string]struct{})
			statusDiskNames := make([]string, 0, len(statusDG.Disks))
			for _, disk := range statusDG.Disks {
				if disk.Name == "" {
					continue
				}
				if _, exists := statusDiskSet[disk.Name]; !exists {
					statusDiskSet[disk.Name] = struct{}{}
					statusDiskNames = append(statusDiskNames, disk.Name)
				}
			}

			groupDisksToRemove, err := findRacDisksToRemoveforRAC(specDisks, statusDiskNames, racDatabase)
			if err != nil {
				return groupDisksToRemove, fmt.Errorf("required disk is part of the disk group %s and cannot be removed. Review it manually", statusDG.Name)
			}
			for _, disk := range groupDisksToRemove {
				if _, exists := disksToRemoveSet[disk]; !exists {
					disksToRemoveSet[disk] = struct{}{}
					disksToRemove = append(disksToRemove, disk)
				}
			}
		}
	}

	return disksToRemove, nil
}

// getDisksToAddStatusforRAC inspects desired ASM disk groups against recorded
// status entries. It returns new disks that should be provisioned so the
// controller can expand storage appropriately.
func getDisksToAddStatusforRAC(racDatabase *racdb.RacDatabase) ([]string, error) {
	disksToAdd := []string{}
	disksToAddSet := make(map[string]struct{})

	for _, statusDG := range racDatabase.Status.AsmDiskGroups {
		// // Find matching group in spec
		// if len(statusDG.Disks) == 0 {
		// 	continue
		// }
		var specDisks []string
		for _, specDG := range racDatabase.Spec.AsmStorageDetails {
			if specDG.Name == statusDG.Name {
				specDisks = specDG.Disks
				break
			}
		}
		if specDisks == nil {
			continue
		}

		if len(specDisks) > len(statusDG.Disks) {
			// Unique disk names for status group
			statusDiskSet := make(map[string]struct{})
			for _, disk := range statusDG.Disks {
				if disk.Name != "" {
					statusDiskSet[disk.Name] = struct{}{}
				}
			}

			// Find disks in spec that are not in status
			for _, disk := range specDisks {
				if disk == "" {
					continue
				}
				if _, exists := statusDiskSet[disk]; !exists {
					if _, alreadyAdded := disksToAddSet[disk]; !alreadyAdded {
						disksToAddSet[disk] = struct{}{}
						disksToAdd = append(disksToAdd, disk)
					}
				}
			}
		}
	}

	return disksToAdd, nil
}

// flattenAsmDisksForRAC flattens all ASM disk references from the RAC spec
// into a single slice. Utilities reuse this to cross-check disk lists.
func flattenAsmDisksForRAC(racDbSpec *racdb.RacDatabaseSpec) []string {
	var allDisks []string

	if racDbSpec == nil {
		return allDisks
	}

	if racDbSpec.AsmStorageDetails == nil {
		return allDisks
	}

	for _, dg := range racDbSpec.AsmStorageDetails {
		if dg.Disks == nil {
			continue
		}
		allDisks = append(allDisks, dg.Disks...)
	}

	return allDisks
}

// createDaemonSet ensures the disk discovery DaemonSet exists with the
// expected spec. It creates or updates the workload so ASM disk metadata
// stays current across reconcile iterations.
func (r *RacDatabaseReconciler) createDaemonSet(racDatabase *racdb.RacDatabase, ctx context.Context, oldStyle bool) error {
	r.Log.Info("Validate New ASM Disks")

	// Build the desired DaemonSet (disk-check)
	desiredDaemonSet := raccommon.BuildDiskCheckDaemonSet(racDatabase, oldStyle)

	// Try to get the existing DaemonSet
	existingDaemonSet := &appsv1.DaemonSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      desiredDaemonSet.Name,
		Namespace: desiredDaemonSet.Namespace,
	}, existingDaemonSet)

	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Creating DaemonSet", "name", desiredDaemonSet.Name)
			if err := r.Client.Create(ctx, desiredDaemonSet); err != nil {
				racDatabase.Spec.IsFailed = true
				return err
			}
			r.Log.Info("DaemonSet created successfully", "DaemonSet.Name", desiredDaemonSet.Name)

		} else {
			racDatabase.Spec.IsFailed = true
			return err
		}
	} else {
		// Check if volumes need update
		if !reflect.DeepEqual(existingDaemonSet.Spec.Template.Spec.Volumes, desiredDaemonSet.Spec.Template.Spec.Volumes) {
			r.Log.Info("Updating DaemonSet volumes", "name", desiredDaemonSet.Name)
			existingDaemonSet.Spec.Template.Spec.Volumes = desiredDaemonSet.Spec.Template.Spec.Volumes
			existingDaemonSet.Spec.Template.Spec.Containers = desiredDaemonSet.Spec.Template.Spec.Containers
			if err := r.Client.Update(ctx, existingDaemonSet); err != nil {
				return err
			}
			r.Log.Info("DaemonSet updated, waiting for pods to restart...")
		} else {
			r.Log.Info("DaemonSet already up-to-date", "name", desiredDaemonSet.Name)
		}
	}

	// r.Log.Info("Disk sizes updated successfully")
	return nil
}

// updateDiskSizes refreshes ASM disk size information within status using
// latest metrics from disk discovery outputs. It aligns spec sizing with
// what was actually discovered on cluster nodes.
func (r *RacDatabaseReconciler) updateDiskSizes(
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
) ([]racdb.AsmDiskStatus, error) {

	// 1. Collect discovered disks (ASM + OTHERS)
	var disks []racdb.AsmDiskStatus

	podList := &corev1.PodList{}
	labels := raccommon.BuildLabelsForDaemonSet(racDatabase, "disk-check")
	if err := r.Client.List(
		ctx,
		podList,
		client.InNamespace(racDatabase.Namespace),
		client.MatchingLabels(labels),
	); err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		req := r.kubeClient.CoreV1().
			Pods(pod.Namespace).
			GetLogs(pod.Name, &corev1.PodLogOptions{Container: "disk-check"})

		logs, err := req.Stream(ctx)
		if err != nil {
			r.Log.Error(err, "Failed to stream logs", "pod", pod.Name)
			continue
		}

		func() {
			defer logs.Close()
			scanner := bufio.NewScanner(logs)
			for scanner.Scan() {
				var entry struct {
					Disk   string `json:"disk"`
					Valid  bool   `json:"valid"`
					SizeGb int    `json:"sizeGb"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
					r.Log.Error(err, "Failed to unmarshal disk info", "pod", pod.Name)
					continue
				}
				disks = append(disks, racdb.AsmDiskStatus{
					Name:     entry.Disk,
					SizeInGb: entry.SizeGb,
					Valid:    entry.Valid,
				})
			}
		}()
	}

	// 2. Build ASM disk group status (exclude OTHERS)
	var diskGroups []racdb.AsmDiskGroupStatus

	for _, dgSpec := range racDatabase.Spec.AsmStorageDetails {

		// ---- FIX: Skip OTHERS in ASM status ----
		if dgSpec.Type == racdb.OthersDiskDg {
			continue
		}

		groupName := strings.TrimSpace(dgSpec.Name)
		if groupName == "" {
			groupName = "+DATA"
		}

		var groupDisks []racdb.AsmDiskStatus
		for _, diskName := range dgSpec.Disks {
			for _, d := range disks {
				if d.Name == diskName {
					groupDisks = append(groupDisks, d)
					break
				}
			}
		}

		if len(groupDisks) == 0 {
			continue
		}

		// Size consistency check
		if len(groupDisks) > 1 {
			expected := groupDisks[0].SizeInGb
			for _, gd := range groupDisks[1:] {
				if gd.SizeInGb != expected {
					return nil, fmt.Errorf(
						"disk group %q has mismatched disk sizes: disk %q = %dGB, expected %dGB",
						groupName, gd.Name, gd.SizeInGb, expected,
					)
				}
			}
		}

		diskGroups = append(diskGroups, racdb.AsmDiskGroupStatus{
			Name:         groupName,
			Redundancy:   dgSpec.Redundancy,
			Type:         dgSpec.Type,
			AutoUpdate:   dgSpec.AutoUpdate,
			StorageClass: dgSpec.StorageClass,
			Disks:        groupDisks,
		})
	}

	// 3. Persist ASM disk group status
	racDatabase.Status.AsmDiskGroups = diskGroups

	// 4. Patch status with retry
	const maxRetries = 3
	const retryDelay = 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(
			ctx,
			client.ObjectKey{
				Namespace: racDatabase.Namespace,
				Name:      racDatabase.Name,
			},
			latest,
		); err != nil {
			return nil, err
		}

		latest.Status.AsmDiskGroups = racDatabase.Status.AsmDiskGroups

		if err := mergeRacInstancesFromLatest(racDatabase, latest); err != nil {
			return nil, err
		}

		racDatabase.ResourceVersion = latest.ResourceVersion
		if err := r.Client.Status().Update(ctx, racDatabase); err != nil {
			if apierrors.IsConflict(err) {
				time.Sleep(retryDelay)
				continue
			}
			return nil, err
		}

		// Success
		return disks, nil
	}

	return nil, fmt.Errorf("failed to update disk sizes after %d retries", maxRetries)
}

// cleanupDaemonSet removes the temporary disk discovery DaemonSet when it is
// no longer required. This keeps the cluster clean once ASM updates finish.
func (r *RacDatabaseReconciler) cleanupDaemonSet(racDatabase *racdb.RacDatabase, ctx context.Context, oldStyle bool) error {
	// r.Log.Info("CleanupDaemonSet")
	desiredDaemonSet := raccommon.BuildDiskCheckDaemonSet(racDatabase, oldStyle)

	// Try to get the existing DaemonSet
	existingDaemonSet := &appsv1.DaemonSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      desiredDaemonSet.Name,
		Namespace: desiredDaemonSet.Namespace,
	}, existingDaemonSet)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// DaemonSet does not exist, so nothing to delete
			// r.Log.Info("No DaemonSet Found To Delete")
			return nil
		}
		// Some other error occurred in fetching the DaemonSet
		// r.Log.Error(err, "Some other error occurred in fetching the DaemonSet")
		return err
	}

	// DaemonSet exists, attempt to delete it
	if err := r.Client.Delete(ctx, existingDaemonSet); err != nil {
		r.Log.Error(err, "Failed to delete DaemonSet", "DaemonSet.Name", existingDaemonSet.Name)
		return err
	}

	// Poll for the DaemonSet to be deleted
	timeout := 30 * time.Second
	pollInterval := 5 * time.Second
	startTime := time.Now()

	for {
		// Check if we have exceeded the timeout
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for DaemonSet %s to be deleted", existingDaemonSet.Name)
		}

		// Check if the DaemonSet still exists
		err = r.Client.Get(ctx, types.NamespacedName{
			Name:      existingDaemonSet.Name,
			Namespace: existingDaemonSet.Namespace,
		}, existingDaemonSet)

		if err != nil {
			if apierrors.IsNotFound(err) {
				// DaemonSet no longer exists
				// r.Log.Info("DaemonSet deleted successfully", "DaemonSet.Name", existingDaemonSet.Name)
				return nil
			}
			// Some other error occurred in fetching the DaemonSet
			r.Log.Error(err, "Error checking for DaemonSet deletion", "DaemonSet.Name", existingDaemonSet.Name)
			return err
		}

		// Wait before checking again
		time.Sleep(pollInterval)
	}
}

// findRacDisksToRemoveforRAC identifies disks present in status but missing
// from spec, ensuring they are safe to remove and not referenced elsewhere
// in the RAC configuration.
func findRacDisksToRemoveforRAC(
	specDisks []string,
	statusDisks []string,
	instance *racdb.RacDatabase,
) ([]string, error) {

	// ---------------------------------------
	// 1. Spec disks for THIS disk group
	// ---------------------------------------
	localSpecSet := make(map[string]struct{})
	for _, d := range specDisks {
		localSpecSet[d] = struct{}{}
	}

	// ---------------------------------------
	// 2. ALL spec disks across ALL disk groups
	// ---------------------------------------
	globalSpecSet := make(map[string]struct{})
	for _, dg := range instance.Spec.AsmStorageDetails {
		for _, d := range dg.Disks {
			globalSpecSet[d] = struct{}{}
		}
	}

	// ---------------------------------------
	// 3. Decide removals safely
	// ---------------------------------------
	var disksToRemove []string

	for _, disk := range statusDisks {

		// 🔒 HARD SAFETY:
		// If disk exists in ANY diskgroup spec → NEVER remove
		if _, exists := globalSpecSet[disk]; exists {
			continue
		}

		// Disk does not belong to this DG spec → removable
		if _, exists := localSpecSet[disk]; !exists {
			disksToRemove = append(disksToRemove, disk)
		}
	}

	return disksToRemove, nil
}

// findRacDisksToAddforRAC determines new ASM disks that should be provisioned
// by comparing updated specs against both prior spec and current status,
// rejecting duplicates or conflicting allocations.
func findRacDisksToAddforRAC(newSpecDisks, statusDisks []string, instance *racdb.RacDatabase, oldSpec *racdb.RacDatabaseSpec) ([]string, error) {
	// Create a set for statusDisks to allow valid reuse of existing disks
	// Step 1: Check for duplicates within newSpecDisks itself
	oldAsmDisks := flattenAsmDisksForRAC(oldSpec)

	if len(oldAsmDisks) == len(newSpecDisks) {
		oldDiskSet := make(map[string]struct{})
		for _, disk := range oldAsmDisks {
			oldDiskSet[strings.TrimSpace(disk)] = struct{}{}
		}

		allDisksMatch := true
		for _, newDisk := range newSpecDisks {
			if _, found := oldDiskSet[strings.TrimSpace(newDisk)]; !found {
				allDisksMatch = false
				break
			}
		}

		if allDisksMatch {
			return nil, nil // No new disks to add
		}
	}

	seenDisks := make(map[string]struct{})
	for _, newDisk := range newSpecDisks {
		trimmedDisk := strings.TrimSpace(newDisk)

		// Check if the disk is already in the seenDisks set, indicating a duplicate within newSpecDisks
		if _, found := seenDisks[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is defined more than once in the new spec and cannot be added multiple times", trimmedDisk)
		}
		seenDisks[trimmedDisk] = struct{}{}
	}

	// Step 2: Create a set for the actual statusDisks by splitting each entry
	statusDiskSet := make(map[string]struct{})
	for _, diskEntry := range statusDisks {
		// Split the disk entry by commas to handle multiple disks in a single string
		for _, disk := range strings.Split(diskEntry, ",") {
			statusDiskSet[strings.TrimSpace(disk)] = struct{}{}
		}
	}

	// Create sets for each of the individual ASM device lists
	crsAsmDeviceSet := make(map[string]struct{})
	recoAsmDeviceSet := make(map[string]struct{})
	redoAsmDeviceSet := make(map[string]struct{})
	dbAsmDeviceSet := make(map[string]struct{})

	// Step 4: Create a set to track newly added disks that are valid for addition
	var validDisksToAdd []string
	newDiskSet := make(map[string]struct{})

	for _, newDisk := range newSpecDisks {
		trimmedDisk := strings.TrimSpace(newDisk)

		// If the disk is already part of the statusDisks (existing disks), allow it to stay
		if _, found := statusDiskSet[trimmedDisk]; found {
			continue
		}

		// Check if the disk is already part of any individual ASM device list
		if _, found := crsAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of CRS ASM device list and cannot be added again", trimmedDisk)
		}
		if _, found := recoAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of RECO ASM device list and cannot be added again", trimmedDisk)
		}
		if _, found := redoAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of REDO ASM device list and cannot be added again", trimmedDisk)
		}
		if _, found := dbAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of DB ASM device list and cannot be added again", trimmedDisk)
		}

		// Add the disk to newDiskSet and consider it valid for addition
		newDiskSet[trimmedDisk] = struct{}{}
		validDisksToAdd = append(validDisksToAdd, trimmedDisk)
	}

	return validDisksToAdd, nil
}

// updateGiConfigParamStatus ensures Grid Infrastructure parameters in status
// are populated by reading from response files or spec defaults when needed.
func (r *RacDatabaseReconciler) updateGiConfigParamStatus(racDatabase *racdb.RacDatabase) error {
	var cName, fName string

	cfg := racDatabase.Spec.ConfigParams

	if cfg != nil && cfg.GridResponseFile != nil {
		if cfg.GridResponseFile.ConfigMapName != "" {
			cName = cfg.GridResponseFile.ConfigMapName
		}
		if cfg.GridResponseFile.Name != "" {
			fName = cfg.GridResponseFile.Name
		}
	}

	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = new(racdb.RacInitParams)
	}

	// --- GI Parameters (Inventory, GridBase, GridHome, ScanSvcName) ---

	if racDatabase.Status.ConfigParams.Inventory == "" {
		if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.Inventory != "" {
			racDatabase.Status.ConfigParams.Inventory = racDatabase.Spec.ConfigParams.Inventory
		} else {
			invlocation, err := raccommon.CheckRspData(racDatabase, r.Client, "INVENTORY_LOCATION", cName, fName)
			if err != nil {
				racDatabase.Spec.IsFailed = true
				return errors.New("error in responsefile, unable to read INVENTORY_LOCATION")
			} else {
				racDatabase.Status.ConfigParams.Inventory = invlocation
			}
		}
	}

	if racDatabase.Status.ConfigParams.GridBase == "" {
		if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.GridBase != "" {
			racDatabase.Status.ConfigParams.GridBase = racDatabase.Spec.ConfigParams.GridBase
		} else {
			gibase, err := raccommon.CheckRspData(racDatabase, r.Client, "ORACLE_BASE", cName, fName)
			if err != nil {
				racDatabase.Spec.IsFailed = true
				return errors.New("error in responsefile, unable to read ORACLE_BASE")
			} else {
				racDatabase.Status.ConfigParams.GridBase = gibase
			}
		}
	}

	if racDatabase.Status.ConfigParams.GridHome == "" || racDatabase.Status.ConfigParams.GridHome == "NOT_DEFINED" {
		if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.GridHome != "" {
			racDatabase.Status.ConfigParams.GridHome = racDatabase.Spec.ConfigParams.GridHome
		} else {
			gihome, err := raccommon.CheckRspData(racDatabase, r.Client, "GRID_HOME", cName, fName)
			if err != nil {
				racDatabase.Spec.IsFailed = true
				return errors.New("error in responsefile, unable to read GRID_HOME")
			} else {
				racDatabase.Status.ConfigParams.GridHome = gihome
			}
		}
	}

	if racDatabase.Status.ScanSvcName == "" {
		if racDatabase.Spec.ScanSvcName != "" {
			racDatabase.Status.ScanSvcName = racDatabase.Spec.ScanSvcName
		} else {
			scanname, err := raccommon.CheckRspData(racDatabase, r.Client, "scanName", cName, fName)
			if err != nil {
				racDatabase.Spec.IsFailed = true
				return errors.New("error in responsefile, unable to read scanName")
			} else {
				racDatabase.Status.ScanSvcName = scanname
			}
		}
	}

	return nil
}

// setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC harmonizes ASM disk
// group configuration by ensuring mandatory groups exist and default values
// populate when they are missing from spec or status.
func setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(
	racDatabase *racdb.RacDatabase,
	client client.Client,
	cName, fName string,
) error {
	ensureCrsDiskGroupforRAC(racDatabase, client, cName, fName)
	ensureDbDataDiskGroupforRAC(racDatabase)
	ensureDbRecoveryDiskGroupforRAC(racDatabase)
	ensureDefaultCharsetforRAC(racDatabase)

	return nil
}

// ensureCrsDiskGroupforRAC injects or enriches the CRS disk group entry
// derived from response files, guaranteeing type and redundancy defaults.
func ensureCrsDiskGroupforRAC(racDatabase *racdb.RacDatabase, client client.Client, cName, fName string) {
	crsDgFound := false
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		// If name, redundancy, and type are missing but disks are provided,
		// treat it as a candidate for CRSDG populated from the response file.
		if dg.Name == "" && dg.Redundancy == "" && dg.Type == "" && len(dg.Disks) > 0 {
			name := lookupCrsDgResponseValueforRAC(racDatabase, client, cName, fName)
			redundancy := lookupRedundancyResponseValueforRAC(racDatabase, client, cName, fName)
			racDatabase.Spec.AsmStorageDetails[i].Name = name
			racDatabase.Spec.AsmStorageDetails[i].Redundancy = redundancy
			racDatabase.Spec.AsmStorageDetails[i].Type = racdb.CrsAsmDiskDg
			crsDgFound = true
		} else if dg.Type == racdb.CrsAsmDiskDg {
			// If type is already set to CRSDG, still fill defaults if missing
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = lookupCrsDgResponseValueforRAC(racDatabase, client, cName, fName)
			}
			if dg.Redundancy == "" {
				racDatabase.Spec.AsmStorageDetails[i].Redundancy = lookupRedundancyResponseValueforRAC(racDatabase, client, cName, fName)
			}
			crsDgFound = true
		}
	}

	if !crsDgFound {
		// Add default if no CRSDG found
		racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, racdb.AsmDiskGroupDetails{
			Name:       "+DATA",
			Type:       racdb.CrsAsmDiskDg,
			Redundancy: "EXTERNAL",
			Disks:      []string{},
		})
	}
}

// lookupCrsDgResponseValueforRAC fetches disk group names from response
// files, falling back to defaults when explicit values are unavailable.
func lookupCrsDgResponseValueforRAC(
	racDatabase *racdb.RacDatabase,
	client client.Client,
	cName, fName string,
) string {

	normalize := func(val string) string {
		val = strings.TrimSpace(val)
		if val == "" {
			return ""
		}

		// DB rsp format: +DG/...
		if strings.HasPrefix(val, "+") {
			if idx := strings.Index(val, "/"); idx > 0 {
				return val[:idx]
			}
			return val
		}

		// GI rsp format: DG (no '+')
		if strings.Contains(val, "/") {
			// Defensive: CRSDATA/something → CRSDATA
			return strings.SplitN(val, "/", 2)[0]
		}

		return val
	}

	// --- GI response file keys ---
	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"oracle.install.asm.diskGroup.name",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"diskGroupName",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	// --- DB response file keys (ASM destinations) ---
	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"datafileDestination",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"db_create_file_dest",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	return "+DATA"
}

// lookupRedundancyResponseValueforRAC reads redundancy settings from response
// files so disk groups receive consistent redundancy defaults.
func lookupRedundancyResponseValueforRAC(racDatabase *racdb.RacDatabase, client client.Client, cName, fName string) string {
	redundancy, err := raccommon.CheckRspData(racDatabase, client, "redundancy", cName, fName)
	if err == nil && redundancy != "" {
		return redundancy
	}
	return "EXTERNAL"
}

// ensureDbDataDiskGroupforRAC guarantees the DB data disk group exists and
// inherits sensible defaults derived from CRS group metadata.
func ensureDbDataDiskGroupforRAC(racDatabase *racdb.RacDatabase) {
	var crsName string
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.CrsAsmDiskDg {
			crsName = dg.Name
			break
		}
	}

	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.DbDataDiskDg {
			// Set to CRS disk group name if blank
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = crsName
			}
			return
		}
	}
	// Not found, add default, use CRS name
	racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, racdb.AsmDiskGroupDetails{
		Name: crsName, Type: racdb.DbDataDiskDg,
	})
}

// ensureDbRecoveryDiskGroupforRAC enforces the presence of a recovery disk
// group, aligning its naming with the primary data group when unspecified.
func ensureDbRecoveryDiskGroupforRAC(racDatabase *racdb.RacDatabase) {
	var dataName string
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.DbDataDiskDg {
			dataName = dg.Name
			break
		}
	}
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.DbRecoveryDiskDg {
			// Set to DATA disk group if blank
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = dataName
			}
			return
		}
	}
	// Not found, add default, use DATA name
	racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, racdb.AsmDiskGroupDetails{
		Name: dataName, Type: racdb.DbRecoveryDiskDg,
	})
}

// ensureDefaultCharsetforRAC applies the default database character set when
// the spec omits it, avoiding host-side deployment issues.
func ensureDefaultCharsetforRAC(racDatabase *racdb.RacDatabase) {
	if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.DbCharSet == "" {
		racDatabase.Spec.ConfigParams.DbCharSet = "AL32UTF8"
	}
}

// updateDbConfigParamStatus backs fills database-specific configuration
// values into status, reading from env files or response data when the spec
// doesn't provide them directly.
func (r *RacDatabaseReconciler) updateDbConfigParamStatus(
	racDatabase *racdb.RacDatabase,
) error {

	var cName, fName string
	var rspData string

	cfg := racDatabase.Spec.ConfigParams

	if cfg != nil && cfg.DbResponseFile != nil {
		cName = cfg.DbResponseFile.ConfigMapName
		fName = cfg.DbResponseFile.Name
	}

	if racDatabase.Spec.ConfigParams == nil {
		return nil
	}

	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = new(racdb.RacInitParams)
	}

	// Load response file ONCE (only if needed)
	loadRspData := func() (string, error) {
		if rspData != "" {
			return rspData, nil
		}
		data, err := raccommon.CheckRspData(
			racDatabase,
			r.Client,
			"variables=",
			cName,
			fName,
		)
		if err != nil {
			racDatabase.Spec.IsFailed = true
			return "", fmt.Errorf("error in responsefile, unable to read variables")
		}
		rspData = data
		return rspData, nil
	}

	// DbName
	if racDatabase.Status.ConfigParams.DbName == "" {
		if cfg.DbName != "" {
			racDatabase.Status.ConfigParams.DbName = cfg.DbName
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			dbName := utils.GetValue(data, "DB_NAME")
			if dbName == "" {
				racDatabase.Spec.IsFailed = true
				return fmt.Errorf("error in responsefile, unable to read DB_NAME")
			}
			racDatabase.Status.ConfigParams.DbName = dbName
		}
	}

	// DbBase (ORACLE_BASE)
	if racDatabase.Status.ConfigParams.DbBase == "" {
		if cfg.DbBase != "" {
			racDatabase.Status.ConfigParams.DbBase = cfg.DbBase
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			obase := utils.GetValue(data, "ORACLE_BASE")
			if obase == "" {
				return fmt.Errorf("error in responsefile, unable to read ORACLE_BASE")
			}
			racDatabase.Status.ConfigParams.DbBase = obase
		}
	}

	// DbHome (ORACLE_HOME)
	if racDatabase.Status.ConfigParams.DbHome == "" ||
		racDatabase.Status.ConfigParams.DbHome == "NOT_DEFINED" {

		if cfg.DbHome != "" {
			racDatabase.Status.ConfigParams.DbHome = cfg.DbHome
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			ohome := utils.GetValue(data, "ORACLE_HOME")
			if ohome == "" {
				return fmt.Errorf("error in responsefile, unable to read ORACLE_HOME")
			}
			racDatabase.Status.ConfigParams.DbHome = ohome
		}
	}

	// GridHome
	if racDatabase.Status.ConfigParams.GridHome == "" {
		if cfg.GridHome != "" {
			racDatabase.Status.ConfigParams.GridHome = cfg.GridHome
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			ghome := utils.GetValue(data, "ORACLE_HOME")
			if ghome == "" {
				return fmt.Errorf("error in responsefile, unable to read ORACLE_HOME")
			}
			racDatabase.Status.ConfigParams.GridHome = ghome
		}
	}

	return nil
}

// updateRacInstTopologyStatus collects pod and node details for RAC
// instances and updates topology fields in status. It returns state needed
// by downstream topology validation logic.
func (r *RacDatabaseReconciler) updateRacInstTopologyStatus(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	isOldStyle bool,
) ([]string, map[string]*corev1.Node, error) {

	var (
		err         error
		pod         *corev1.Pod
		podNames    []string
		nodeDetails = make(map[string]*corev1.Node)
	)

	// -------------------------------------------------------------
	// STEP 0: HARD STOP if ANY RAC pod is Pending
	// -------------------------------------------------------------
	pending, err := hasPendingRacPods(ctx, r, racDatabase)
	if err != nil {
		return podNames, nodeDetails, err
	}

	if pending {
		r.Log.Info(
			"RAC topology validation skipped: pods are Pending",
			"racDatabase", racDatabase.Name,
		)

		// Always update using latest object
		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(racDatabase), latest); err != nil {
			return podNames, nodeDetails, err
		}

		latest.Status.State = string(racdb.RACPendingState)
		latest.Status.DbState = string(racdb.RACPendingState)

		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               string(racdb.RacCrdReconcileWaitingState),
			Status:             metav1.ConditionTrue,
			Reason:             string(racdb.RacCrdReconcileWaitingReason),
			Message:            "Waiting for all RAC pods to become Running",
			ObservedGeneration: latest.Generation,
			LastTransitionTime: metav1.Now(),
		})

		err := r.updateStatusWithRetry(ctx, req, func(latest *racdb.RacDatabase) {
			latest.Status = racDatabase.Status
		})
		if err != nil {
			return nil, nil, err
		}

		// DO NOT validate, DO NOT fail
		return podNames, nodeDetails, nil
	}

	// -------------------------------------------------------------
	// STEP 1: Validate ONLY when cluster is converged
	// -------------------------------------------------------------
	if isOldStyle && len(racDatabase.Spec.InstDetails) > 0 {

		for index, oraRacSpec := range racDatabase.Spec.InstDetails {
			if strings.EqualFold(oraRacSpec.IsDelete, "true") {
				continue
			}

			_, pod, err = r.validateRacInst(
				racDatabase, ctx, req, oraRacSpec, index, isOldStyle,
			)
			if err != nil {
				return podNames, nodeDetails, err
			}
			if pod == nil {
				continue
			}

			podNames = append(podNames, pod.Name)

			node, err := r.getNodeDetails(pod.Spec.NodeName)
			if err != nil {
				return podNames, nodeDetails,
					fmt.Errorf("failed to get node details for pod %s: %w", pod.Name, err)
			}
			nodeDetails[pod.Name] = node
		}

	} else if racDatabase.Spec.ClusterDetails != nil {

		clusterSpec := racDatabase.Spec.ClusterDetails

		for index := 0; index < clusterSpec.NodeCount; index++ {

			_, pod, err = r.validateRacNodeCluster(
				racDatabase, ctx, req, clusterSpec, index,
			)
			if err != nil {
				return podNames, nodeDetails, err
			}
			if pod == nil {
				continue
			}

			podNames = append(podNames, pod.Name)

			node, err := r.getNodeDetails(pod.Spec.NodeName)
			if err != nil {
				return podNames, nodeDetails,
					fmt.Errorf("failed to get node details for pod %s: %w", pod.Name, err)
			}
			nodeDetails[pod.Name] = node
		}
		desiredNodes := map[string]struct{}{}

		for i := 0; i < clusterSpec.NodeCount; i++ {
			stsName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, i+1)
			podName := fmt.Sprintf("%s-0", stsName) // StatefulSet pod 0
			desiredNodes[podName] = struct{}{}
		}

		filteredStatus := []*v4.RacNodeStatus{}

		for _, nodeStatus := range racDatabase.Status.RacNodes {
			if nodeStatus == nil {
				continue
			}

			if _, ok := desiredNodes[nodeStatus.Name]; ok {
				filteredStatus = append(filteredStatus, nodeStatus)
			} else {
				r.Log.Info(
					"Pruning stale RAC node from status",
					"node", nodeStatus.Name,
				)
			}
		}

		racDatabase.Status.RacNodes = filteredStatus
		err := r.updateStatusNoGetRetry(ctx, racDatabase)
		if err != nil {
			return nil, nil, err
		}

	}

	// -------------------------------------------------------------
	// STEP 2: Final sanity (ONLY after convergence)
	// -------------------------------------------------------------
	if len(podNames) == 0 || len(nodeDetails) == 0 {
		// Not Pending → real failure
		racDatabase.Spec.IsFailed = true
		return podNames, nodeDetails,
			errors.New("failed to collect RAC pod or node details")
	}

	racDatabase.Spec.IsFailed = false
	return podNames, nodeDetails, nil
}

func hasPendingRacPods(
	ctx context.Context,
	r *RacDatabaseReconciler,
	racDatabase *racdb.RacDatabase,
) (bool, error) {

	podList := &corev1.PodList{}

	err := r.List(ctx, podList,
		client.InNamespace(racDatabase.Namespace),
	)
	if err != nil {
		return false, err
	}

	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodPending {
			return true, nil
		}
	}
	return false, nil
}

// validateRacNodeCluster inspects cluster-level spec fields, nodes, and
// network resources to verify topology prerequisites are satisfied before
// provisioning RAC structures.
func (r *RacDatabaseReconciler) validateRacNodeCluster(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	racSfSet, err := raccommon.CheckSfset(nodeName, racDatabase, r.Client)
	if err != nil {
		r.updateRacNodeStatusForCluster(racDatabase, ctx, req, clusterSpec, nodeIndex, string(racdb.RACProvisionState))
		return racSfSet, nil, err
	}
	if racSfSet == nil {
		return nil, nil, fmt.Errorf("StatefulSet for %s not found", nodeName)
	}
	podList, err := raccommon.GetPodList(racSfSet.Name, racDatabase, r.Client, racdb.RacInstDetailSpec{Name: nodeName})
	if err != nil {
		msg := "Unable to find any pod in statefulset " + raccommon.GetFmtStr(racSfSet.Name) + "."
		raccommon.LogMessages("INFO", msg, nil, racDatabase, r.Log)
		r.updateRacNodeStatusForCluster(racDatabase, ctx, req, clusterSpec, nodeIndex, string(racdb.RACProvisionState))
		return racSfSet, nil, err
	}

	isPodExist, racPod, notReadyPod := raccommon.PodListValidation(podList, racSfSet.Name, racDatabase, r.Client)
	if !isPodExist {
		msg := ""
		if notReadyPod != nil {
			msg = "unable to validate RAC pod. The  pod not ready  is: " + notReadyPod.Name
		} else {
			msg = "unable to validate RAC pod. No pods matching the criteria were found"
		}
		raccommon.LogMessages("INFO", msg, nil, racDatabase, r.Log)
		return racSfSet, racPod, fmt.Errorf(msg)
	}

	// Update status when PODs are ready
	state := racDatabase.Status.State
	if racDatabase.Spec.IsManual {
		state = string(racdb.RACManualState)
	}
	if racDatabase.Spec.IsFailed {
		state = string(racdb.RACFailedState)
	}

	switch {
	case isPodExist && (state == string(racdb.RACProvisionState) ||
		state == string(racdb.RACUpdateState) ||
		state == string(racdb.RACPendingState)):
		state = string(racdb.RACPodAvailableState)
	case state == string(racdb.RACFailedState):
		state = string(racdb.RACFailedState)
	case state == string(racdb.RACManualState):
		state = string(racdb.RACManualState)
	default:
		state = racDatabase.Status.State
	}
	r.updateRacNodeStatusForCluster(racDatabase, ctx, req, clusterSpec, nodeIndex, state)
	r.Log.Info("Completed Update of RAC cluster node status", "NodeName", nodeName)
	return racSfSet, racPod, nil
}

// getNodeDetails retrieves Kubernetes node metadata for RAC validation
// steps, allowing the controller to inspect node labels and status fields.
func (r *RacDatabaseReconciler) getNodeDetails(nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := r.Client.Get(context.TODO(), client.ObjectKey{
		Namespace: "",
		Name:      nodeName,
	}, node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// updateRacDbTopologyStatus synchronizes database-level topology status using
// pod placement and node details gathered earlier in the reconcile loop.
func (r *RacDatabaseReconciler) updateRacDbTopologyStatus(racDatabase *racdb.RacDatabase, ctx context.Context, req ctrl.Request, podNames []string, nodeDetails map[string]*corev1.Node) error {

	//racPod := &corev1.Pod{}
	var err error
	_, _, err = r.validateRacDb(racDatabase, ctx, req, podNames, nodeDetails)
	if err != nil {
		return err
	}
	return nil
}

// validateRacDb checks database pods, placement, and configuration to ensure
// runtime topology aligns with expectations before status updates proceed.
func (r *RacDatabaseReconciler) validateRacDb(racDatabase *racdb.RacDatabase, ctx context.Context, req ctrl.Request, podNames []string, nodeDetails map[string]*corev1.Node,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	racSfSet := &appsv1.StatefulSet{}
	racPod := &corev1.Pod{}
	const maxRetries = 5
	const retryDelay = 2 * time.Second
	raccommon.UpdateRacDbStatusData(racDatabase, ctx, req, podNames, r.kubeClient, r.kubeConfig, r.Log, nodeDetails)
	// Log the start of the status update process
	r.Log.Info(
		"Updating RAC instance status with validateRacDb",
		"Instance", racDatabase.Name,
	)

	err := r.updateStatusNoGetRetry(ctx, racDatabase)
	if err != nil {
		r.Log.Error(err, "Failed to update RAC instance status with validateRacDb")
		return racSfSet, racPod, err
	}

	r.Log.Info(
		"Updated RAC instance status with validateRacDb",
		"Instance", racDatabase.Name,
	)

	return racSfSet, racPod, nil

}

// validateRacInst inspects per-instance configuration, pods, and nodes to
// verify RAC instance state matches desired spec before orchestration moves on.
func (r *RacDatabaseReconciler) validateRacInst(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	OraRacSpex racdb.RacInstDetailSpec,
	index int,
	isOldStyle bool,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	var err error
	racSfSet := &appsv1.StatefulSet{}
	racPod := &corev1.Pod{}

	racSfSet, err = raccommon.CheckSfset(OraRacSpex.Name, racDatabase, r.Client)
	if err != nil {
		r.updateRacInstStatus(racDatabase, ctx, req, racDatabase.Spec.InstDetails[index], index, string(racdb.RACProvisionState), r.Client, true)
		return racSfSet, racPod, err
	}
	if racSfSet == nil {
		return nil, nil, fmt.Errorf("StatefulSet for %s not found", OraRacSpex.Name)
	}
	podList, err := raccommon.GetPodList(racSfSet.Name, racDatabase, r.Client, OraRacSpex)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + raccommon.GetFmtStr(racSfSet.Name) + "."
		raccommon.LogMessages("INFO", msg, nil, racDatabase, r.Log)
		r.updateRacInstStatus(racDatabase, ctx, req, racDatabase.Spec.InstDetails[index], index, string(racdb.RACProvisionState), r.Client, true)
		return racSfSet, racPod, err
	}

	isPodExist, racPod, notReadyPod := raccommon.PodListValidation(podList, racSfSet.Name, racDatabase, r.Client)
	if !isPodExist {
		msg := ""
		if notReadyPod != nil {
			msg = "unable to validate RAC pod. The  pod not ready  is: " + notReadyPod.Name
			raccommon.LogMessages("INFO", msg, nil, racDatabase, r.Log)
			return racSfSet, racPod, fmt.Errorf(msg)
		} else {
			msg = "unable to validate RAC pod. No pods matching the criteria were found"
			raccommon.LogMessages("INFO", msg, nil, racDatabase, r.Log)
			return racSfSet, racPod, fmt.Errorf(msg)
		}
	}

	// Update status when PODs are ready
	state := racDatabase.Status.State
	if racDatabase.Spec.IsManual {
		state = string(racdb.RACManualState)
	}
	if racDatabase.Spec.IsFailed {
		state = string(racdb.RACFailedState)
	}

	switch {
	case isPodExist && (state == string(racdb.RACProvisionState) ||
		state == string(racdb.RACUpdateState) ||
		state == string(racdb.RACPendingState)):
		state = string(racdb.RACPodAvailableState)
	case state == string(racdb.RACFailedState):
		state = string(racdb.RACFailedState)
	case state == string(racdb.RACManualState):
		state = string(racdb.RACManualState)
	default:
		state = racDatabase.Status.State
	}

	r.updateRacInstStatus(racDatabase, ctx, req, racDatabase.Spec.InstDetails[index], index, state, r.Client, true)

	r.Log.Info("Completed Update of RAC instance status", "Name", OraRacSpex.Name)
	return racSfSet, racPod, nil
}

// updateRacInstStatus refreshes instance-level status fields using the
// current reconcile observations, capturing pod readiness and node info.
func (r *RacDatabaseReconciler) updateRacInstStatus(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	OraRacSpex racdb.RacInstDetailSpec,
	specIdx int,
	state string,
	kClient client.Client,
	mergingRequired bool,
) {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	var lastErr error
	var failedUpdate bool
	// Get/Update RAC instance status data
	raccommon.UpdateRacInstStatusData(racDatabase, ctx, req, OraRacSpex, specIdx, state, r.kubeClient, r.kubeConfig, r.Log, r.Client)

	for attempt := 0; attempt < maxRetries; attempt++ {

		// Fetch the latest version of the object
		latestInstance := &racdb.RacDatabase{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of RAC instance")
			lastErr = err
			continue // Continue to retry
		}
		latestInstance.Status.RacNodes = racDatabase.Status.RacNodes
		if mergingRequired {

			// Ensure latestInstance has the most recent version
			r.ensureAsmStorageStatus(latestInstance)

			// Merge the instance fields into latestInstance
			err = mergeRacInstancesFromLatest(racDatabase, latestInstance)
			if err != nil {
				r.Log.Error(err, "Failed to merge instances")
			}
		}

		// Attempt to update the combined instance back to the Kubernetes API
		// err = r.Status().Update(ctx, instance)
		racDatabase.ResourceVersion = latestInstance.ResourceVersion

		err = r.Status().Update(ctx, racDatabase)
		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected in updateRacInstStatus, retrying...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				failedUpdate = true
				continue // Retry
			}
			// For other errors, log and return
			r.Log.Error(err, "Failed to update the RAC instance")
			lastErr = err
			failedUpdate = true
			continue // Continue to retry
		}
		r.Log.Info("RAC Object updated with updateRacInstStatus")
		failedUpdate = false
		break //break if its updated successfully
	}

	// If we exhaust all retries, print the last error encountered
	if failedUpdate {
		r.Log.Info("failed to update RAC instance after 5 attempts", "lastErr", lastErr)
	}
}

// RacGetRestrictedFields returns a set of field names that are restricted from being updated.
// RacGetRestrictedFields returns the set of fields that are protected from
// modification via manual updates, allowing validation logic to block edits.
func RacGetRestrictedFields() map[string]struct{} {
	return map[string]struct{}{
		"ConfigParams.DbName":                         {},
		"ConfigParams.GridBase":                       {},
		"ConfigParams.GridHome":                       {},
		"ConfigParams.DbBase":                         {},
		"ConfigParams.DbHome":                         {},
		"ConfigParams.CrsAsmDiskDg":                   {},
		"ConfigParams.CrsAsmDiskDgRedundancy":         {},
		"ConfigParams.DBAsmDiskDgRedundancy":          {},
		"ConfigParams.DbCharSet":                      {},
		"ConfigParams.DbConfigType":                   {},
		"ConfigParams.DbDataFileDestDg":               {},
		"ConfigParams.DbUniqueName":                   {},
		"ConfigParams.DbRecoveryFileDest":             {},
		"ConfigParams.DbRedoFileSize":                 {},
		"ConfigParams.DbStorageType":                  {},
		"ConfigParams.DbSwZipFile":                    {},
		"ConfigParams.GridSwZipFile":                  {},
		"ConfigParams.GridResponseFile.ConfigMapName": {},
		"ConfigParams.GridResponseFile.Name":          {},
		"ConfigParams.DbResponseFile.ConfigMapName":   {},
		"ConfigParams.DbResponseFile.Name":            {},
	}
}

// mergeInstancesFromUpdated updates latestInstance with fields from updatedInstance
// except those that are restricted by the RacGetRestrictedFields function in align with webhooks.
// Assuming mergeInstancesFromUpdated merges instance details from updatedInstance to latestInstance.

// mergeRacInstancesFromLatest copies mutable fields from the latest object
// into the reconcile instance, ensuring status updates patch cleanly.
func mergeRacInstancesFromLatest(instance, latestInstance *racdb.RacDatabase) error {
	instanceVal := reflect.ValueOf(instance).Elem()
	latestVal := reflect.ValueOf(latestInstance).Elem()

	// Assuming `Status` is a field in `RacDatabase`
	instanceStatus := instanceVal.FieldByName("Status")
	latestStatus := latestVal.FieldByName("Status")

	if !instanceStatus.IsValid() || !latestStatus.IsValid() {
		return fmt.Errorf("status field is not valid in one of the instances")
	}

	// Merge the Status field
	return mergeRacStructFields(instanceStatus, latestStatus)
}

// mergeRacStructFields recursively merges exported struct fields when the
// destination field is unset, preserving existing values from latest status.
func mergeRacStructFields(instanceField, latestField reflect.Value) error {
	if instanceField.Kind() != reflect.Struct || latestField.Kind() != reflect.Struct {
		return fmt.Errorf("fields to be merged must be of struct type")
	}

	for i := 0; i < instanceField.NumField(); i++ {
		subField := instanceField.Type().Field(i)
		instanceSubField := instanceField.Field(i)
		latestSubField := latestField.Field(i)

		if !isRacExported(subField) || !instanceSubField.CanSet() {
			continue
		}

		switch latestSubField.Kind() {
		case reflect.Ptr:
			if !latestSubField.IsNil() {
				if instanceSubField.IsNil() {
					// Allocate new pointer struct
					instanceSubField.Set(reflect.New(latestSubField.Type().Elem()))
				}
				// Merge inside pointer struct
				if err := mergeRacStructFields(instanceSubField.Elem(), latestSubField.Elem()); err != nil {
					return err
				}
			}

		case reflect.String:
			if latestSubField.String() != "" && latestSubField.String() != "NOT_DEFINED" && instanceSubField.String() == "" {
				instanceSubField.Set(latestSubField)
			}
		case reflect.Struct:
			if err := mergeRacStructFields(instanceSubField, latestSubField); err != nil {
				return err
			}
		case reflect.Slice:
			if latestSubField.Len() > 0 {
				if instanceSubField.IsNil() {
					// Initialize empty slice first
					instanceSubField.Set(reflect.MakeSlice(instanceSubField.Type(), 0, latestSubField.Len()))
				}
				if instanceSubField.Len() == 0 {
					instanceSubField.Set(latestSubField)
				} else {
					// Merge slice items by index
					for j := 0; j < latestSubField.Len(); j++ {
						if j < instanceSubField.Len() {
							if latestSubField.Index(j).Kind() == reflect.Struct {
								if err := mergeRacStructFields(instanceSubField.Index(j), latestSubField.Index(j)); err != nil {
									return err
								}
							} else if instanceSubField.Index(j).IsZero() {
								instanceSubField.Index(j).Set(latestSubField.Index(j))
							}
						} else {
							instanceSubField.Set(reflect.Append(instanceSubField, latestSubField.Index(j)))
						}
					}
				}
			}

		default:
			if reflect.DeepEqual(instanceSubField.Interface(), reflect.Zero(instanceSubField.Type()).Interface()) {
				instanceSubField.Set(latestSubField)
			}
		}
	}
	return nil
}

// isRacExported reports whether a struct field is exported, used to control
// reflection-based merges without touching private data.
func isRacExported(field reflect.StructField) bool {
	return field.PkgPath == ""
}

// generateConfigMap builds ConfigMap data for RAC setup, producing the envfile
// content tailored to either legacy or cluster-style configurations.
func (r *RacDatabaseReconciler) generateConfigMap(instance *racdb.RacDatabase, isOldStyle bool) (map[string]string, error) {
	configMapData := make(map[string]string, 0)
	var new_crs_nodes, existing_crs_nodes_healthy, existing_crs_nodes_not_healthy, install_node string
	if isOldStyle {
		new_crs_nodes, existing_crs_nodes_healthy, existing_crs_nodes_not_healthy, install_node, _ =
			raccommon.GetCrsNodes(instance, r.kubeClient, r.kubeConfig, r.Log, r.Client)
	} else {
		new_crs_nodes, existing_crs_nodes_healthy, existing_crs_nodes_not_healthy, install_node, _ =
			raccommon.GetCrsNodesForCluster(instance, r.kubeClient, r.kubeConfig, r.Log, r.Client)
	} // asm_devices := raccommon.GetAsmDevices(instance)
	var data []string
	var addnodeFlag bool
	scan_name := raccommon.GetScanname(instance)

	//Defaults from webhook
	if instance.Spec.ImagePullPolicy == nil || *instance.Spec.ImagePullPolicy == corev1.PullPolicy("") {
		policy := corev1.PullPolicy("Always")
		instance.Spec.ImagePullPolicy = &policy
	}

	if instance.Spec.SshKeySecret != nil {
		if instance.Spec.SshKeySecret.KeyMountLocation == "" {
			instance.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSshSecretMount
		}
	}

	if instance.Spec.DbSecret != nil && instance.Spec.DbSecret.Name != "" {
		if instance.Spec.DbSecret.PwdFileMountLocation == "" {
			instance.Spec.DbSecret.PwdFileMountLocation = utils.OraRacDbPwdFileSecretMount
		}
		if instance.Spec.DbSecret.KeyFileMountLocation == "" {
			instance.Spec.DbSecret.KeyFileMountLocation = utils.OraRacDbKeyFileSecretMount
		}
	}

	if instance.Spec.TdeWalletSecret != nil && instance.Spec.TdeWalletSecret.Name != "" {
		if instance.Spec.TdeWalletSecret.PwdFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.PwdFileMountLocation = utils.OraRacTdePwdFileSecretMount
		}
		if instance.Spec.TdeWalletSecret.KeyFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.KeyFileMountLocation = utils.OraRacTdeKeyFileSecretMount
		}
	}
	//debug
	if len(new_crs_nodes) == 0 {
		return configMapData, nil
	}

	if len(new_crs_nodes) > 0 && len(existing_crs_nodes_not_healthy) > 0 {
		return configMapData, errors.New("cannot perform node addition as there are unhealthy CRS nodes")
	}

	if len(new_crs_nodes) > 0 {
		data = append(data, "CRS_NODES="+new_crs_nodes)
	}

	if len(existing_crs_nodes_healthy) > 0 {
		data = append(data, "EXISTING_CLS_NODE="+existing_crs_nodes_healthy)
		if len(instance.Spec.ConfigParams.OpType) == 0 {
			data = append(data, "OP_TYPE=racaddnode")
			data = append(data, "ADD_CDP=true")
			addnodeFlag = true

		}
	} else {
		if len(instance.Spec.ConfigParams.OpType) == 0 {
			data = append(data, "OP_TYPE=setuprac")
		}
	}

	// --- Pick ALL envVars directly from CR spec ---
	for _, e := range instance.Spec.EnvVars {
		data = append(data, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}

	// Service Parameters
	if instance.Spec.ServiceDetails.Name != "" {
		sparams := raccommon.GetServiceParams(instance)
		data = append(data, "DB_SERVICE="+sparams)
	}

	if instance.Spec.ConfigParams.PdbName != "" {
		data = append(data, "ORACLE_PDB="+instance.Spec.ConfigParams.PdbName)
	}

	// Settings for DB_LISTENER_PORT
	var endp string
	var locallsnr string
	var err error

	if isOldStyle {
		locallsnr, endp, err = raccommon.GetDBLsnrEndPoints(instance)
	} else {
		locallsnr, endp, err = raccommon.GetDBLsnrEndPointsForCluster(instance)
	}
	if err == nil {
		// Only add if non-empty, and don't duplicate DB_LISTENER_ENDPOINTS
		if endp != "" {
			data = append(data, "DB_LISTENER_ENDPOINTS="+endp)
		}
		if locallsnr != "" {
			data = append(data, "LOCAL_LISTENER="+locallsnr)
		}
	}

	// Setting for DB Listener Ends here

	if instance.Spec.ConfigParams.DbHome != "" {
		data = append(data, "DB_HOME="+instance.Spec.ConfigParams.DbHome)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbHome != "" {
				data = append(data, "DB_HOME="+instance.Status.ConfigParams.DbHome)
			}
		}
	}

	if instance.Spec.ConfigParams.DbBase != "" {
		data = append(data, "DB_BASE="+instance.Spec.ConfigParams.DbBase)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbBase != "" {
				data = append(data, "DB_BASE="+instance.Status.ConfigParams.DbBase)
			}
		}
	}

	if instance.Spec.ConfigParams.GridBase != "" {
		data = append(data, "GRID_BASE="+instance.Spec.ConfigParams.GridBase)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.GridBase != "" {
				data = append(data, "GRID_BASE="+instance.Status.ConfigParams.GridBase)
			}
		}
	}

	if instance.Spec.ConfigParams.GridHome != "" {
		data = append(data, "GRID_HOME="+instance.Spec.ConfigParams.GridHome)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.GridHome != "" {
				data = append(data, "GRID_HOME="+instance.Status.ConfigParams.GridHome)
			}
		}
	}

	if instance.Spec.ConfigParams.Inventory != "" {
		data = append(data, "INVENTORY="+instance.Spec.ConfigParams.Inventory)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.Inventory != "" {
				data = append(data, "INVENTORY="+instance.Status.ConfigParams.Inventory)
			}
		}
	}

	if instance.Spec.SshKeySecret.Name != " " {
		//SecretMap check is done in ValidateSpex
		data = append(data, "SSH_PRIVATE_KEY="+instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PrivKeySecretName)
		data = append(data, "SSH_PUBLIC_KEY="+instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PubKeySecretName)
	}

	if instance.Spec.DbSecret != nil {
		if instance.Spec.DbSecret.Name != "" {
			data = append(data, "SECRET_VOLUME="+instance.Spec.DbSecret.PwdFileMountLocation)
			commonpassflag, pwdkeyflag, _ := raccommon.GetDbSecret(instance, instance.Spec.DbSecret.Name, r.Client)
			if commonpassflag && pwdkeyflag {
				data = append(data, "DB_PWD_FILE="+instance.Spec.DbSecret.PwdFileName)
				data = append(data, "PWD_KEY="+instance.Spec.DbSecret.KeyFileName)
			} else {
				data = append(data, "PASSWORD_FILE=pwdfile")
			}
		}
	}

	if instance.Spec.TdeWalletSecret != nil {
		if instance.Spec.TdeWalletSecret.Name != "" {
			data = append(data, "TDE_SECRET_VOLUME="+instance.Spec.TdeWalletSecret.PwdFileMountLocation)
			data = append(data, "SETUP_TDE_WALLET=true")
			tdepassflag, tdepwdkeyflag, _ := raccommon.GetTdeWalletSecret(instance, instance.Spec.TdeWalletSecret.Name, r.Client)
			if tdepassflag && tdepwdkeyflag {
				data = append(data, "TDE_PWD_FILE="+instance.Spec.TdeWalletSecret.PwdFileName)
				data = append(data, "TDE_PWD_KEY="+instance.Spec.TdeWalletSecret.KeyFileName)
			} else {
				data = append(data, "PASSWORD_FILE=tdepwdfile")
			}
		}
	}

	data = append(data, "PROFILE_FLAG=true")
	data = append(data, "SCAN_NAME="+scan_name)

	data = append(data, "INSTALL_NODE="+install_node)

	if instance.Spec.ConfigParams.DbName != "" {
		data = append(data, "DB_NAME="+instance.Spec.ConfigParams.DbName)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbName != "" {
				data = append(data, "DB_NAME="+instance.Status.ConfigParams.DbName)
			}
		}
	}

	if instance.Spec.ConfigParams.DbUniqueName != "" {
		// Configmap check is done in ValidateSpex
		data = append(data, "DB_UNIQUE_NAME="+instance.Spec.ConfigParams.DbUniqueName)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbUniqueName != "" {
				data = append(data, "DB_UNIQUE_NAME="+instance.Status.ConfigParams.DbUniqueName)
			}
		}
	}

	if instance.Spec.ConfigParams.GridSwZipFile != "" {
		data = append(data, "GRID_SW_ZIP_FILE="+instance.Spec.ConfigParams.GridSwZipFile)
		//data = append(data, "COPY_GRID_SOFTWARE=true")
	}
	if instance.Spec.ConfigParams.HostSwStageLocation != "" {
		data = append(data, "STAGING_SOFTWARE_LOC="+utils.OraSwStageLocation)

	}

	if instance.Spec.ConfigParams.RuPatchLocation != "" {
		data = append(data, "APPLY_RU_LOCATION="+utils.OraRuPatchStageLocation)
	}

	if instance.Spec.ConfigParams.RuFolderName != "" {
		data = append(data, "RU_FOLDER_NAME="+instance.Spec.ConfigParams.RuFolderName)
	}
	if instance.Spec.ConfigParams.OPatchLocation != "" {
		data = append(data, "OPATCH_ZIP_FILE="+utils.OraOPatchStageLocation+"/"+instance.Spec.ConfigParams.OPatchSwZipFile)
	}
	if instance.Spec.ConfigParams.OneOffLocation != "" {
		data = append(data, "ONEOFF_FOLDER_NAME="+instance.Spec.ConfigParams.OneOffLocation)
	}
	if instance.Spec.ConfigParams.DbOneOffIds != "" {
		data = append(data, "DB_ONEOFF_IDS="+instance.Spec.ConfigParams.DbOneOffIds)
	}

	if instance.Spec.ConfigParams.GridOneOffIds != "" {
		data = append(data, "GRID_ONEOFF_IDS="+instance.Spec.ConfigParams.GridOneOffIds)
	}

	if instance.Spec.ConfigParams.DbSwZipFile != "" {
		data = append(data, "DB_SW_ZIP_FILE="+instance.Spec.ConfigParams.DbSwZipFile)
		//data = append(data, "COPY_DB_SOFTWARE=true")
	}

	// ---- ASM DISK GROUP FIELDS: now using new model ----
	crsDiskGroup := ""
	crsDeviceList := ""
	crsRedundancy := ""
	dataDeviceList := ""
	recoDeviceList := ""
	redoDeviceList := ""
	dataDgName := ""
	recoDgName := ""
	redoDgName := ""
	dataRedundancy := ""
	recoRedundancy := ""
	redoRedundancy := ""

	for _, dg := range instance.Spec.AsmStorageDetails {
		switch dg.Type {
		case racdb.CrsAsmDiskDg:
			if dg.Name != "" {
				crsDiskGroup = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				crsRedundancy = dg.Redundancy
			}
		case racdb.DbDataDiskDg:
			if dg.Name != "" {
				dataDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				dataRedundancy = dg.Redundancy
			}
		case racdb.DbRecoveryDiskDg:
			if dg.Name != "" {
				recoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				recoRedundancy = dg.Redundancy
			}
		case racdb.RedoDiskDg:
			if dg.Name != "" {
				redoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				redoRedundancy = dg.Redundancy
			}
		}
	}

	crsDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.CrsAsmDiskDg)
	dataDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbDataDiskDg)
	recoDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbRecoveryDiskDg)
	redoDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.RedoDiskDg)

	// Environment variables ("KEY=VAL" entries), set only if non-empty
	if crsDiskGroup != "" {
		data = append(data, "CRS_ASM_DISKGROUP="+crsDiskGroup)
	} else {
		crsDiskGroup = "+DATA"
		data = append(data, "CRS_ASM_DISKGROUP="+crsDiskGroup)
	}
	if crsDeviceList != "" {
		data = append(data, "CRS_ASM_DEVICE_LIST="+crsDeviceList)
	}
	if crsRedundancy != "" {
		data = append(data, "CRS_ASMDG_REDUNDANCY="+crsRedundancy)
	}
	if dataDgName == "" {
		data = append(data, "DB_DATA_FILE_DEST="+crsDiskGroup)
	} else {
		data = append(data, "DB_DATA_FILE_DEST="+dataDgName)
	}
	if dataDeviceList != "" {
		data = append(data, "DB_ASM_DEVICE_LIST="+dataDeviceList)
	}
	if dataRedundancy != "" {
		data = append(data, "DB_ASMDG_PROPERTIES=redundancy:"+dataRedundancy)
	}
	if recoDeviceList != "" {
		data = append(data, "RECO_ASM_DEVICE_LIST="+recoDeviceList)
	}
	if recoDgName == "" {
		data = append(data, "DB_RECOVERY_FILE_DEST="+crsDiskGroup)
	} else {
		data = append(data, "DB_RECOVERY_FILE_DEST="+recoDgName)
	}
	if recoRedundancy != "" {
		data = append(data, "RECO_ASMDG_PROPERTIES=redundancy:"+recoRedundancy)
	}
	if redoDgName != "" {
		data = append(data, "LOG_FILE_DEST="+redoDgName)
	}
	if redoDeviceList != "" {
		data = append(data, "REDO_ASM_DEVICE_LIST="+redoDeviceList)
	}
	if redoRedundancy != "" {
		data = append(data, "REDO_ASMDG_PROPERTIES=redundancy:"+redoRedundancy)
	}
	if instance.Spec.ConfigParams.DbCharSet == "" {
		instance.Spec.ConfigParams.DbCharSet = "AL32UTF8"
		data = append(data, "DB_CHARACTERSET="+instance.Spec.ConfigParams.DbCharSet)
	}

	// ---- ALL OTHER CONFIG PARAMS - use as before ----

	if !addnodeFlag {
		cfg := instance.Spec.ConfigParams
		if cfg != nil {
			if cfg.DbStorageType != "" {
				data = append(data, "DB_STORAGE_TYPE="+cfg.DbStorageType)
			}
			if cfg.DbCharSet != "" {
				data = append(data, "DB_CHARACTERSET="+cfg.DbCharSet)
			}

			if cfg.DbType != "" {
				data = append(data, "DB_TYPE="+cfg.DbType)
			}
			if cfg.DbConfigType != "" {
				data = append(data, "DB_CONFIG_TYPE="+cfg.DbConfigType)
			}
			if cfg.EnableArchiveLog != "" {
				data = append(data, "ENABLE_ARCHIVELOG="+cfg.EnableArchiveLog)
			}
			// GRID RESPONSE FILE
			if cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {
				data = append(data,
					"GRID_RESPONSE_FILE="+utils.OraGiRsp+"/"+cfg.GridResponseFile.Name)
			}

			// DB RESPONSE FILE
			if cfg.DbResponseFile != nil && cfg.DbResponseFile.ConfigMapName != "" {
				data = append(data,
					"DBCA_RESPONSE_FILE="+utils.OraDbRsp+"/"+cfg.DbResponseFile.Name)
			}
			if cfg.SgaSize != "" {
				data = append(data, "INIT_SGA_SIZE="+cfg.SgaSize)
			}
			if cfg.PgaSize != "" {
				data = append(data, "INIT_PGA_SIZE="+cfg.PgaSize)
			}
			if cfg.Processes > 0 {
				data = append(data, "INIT_PROCESSES="+strconv.Itoa(cfg.Processes))
			}
			if cfg.CpuCount > 0 {
				data = append(data, "CPU_COUNT="+strconv.Itoa(cfg.CpuCount))
			}
			if instance.Spec.ConfigParams.SgaSize != "" {
				normalizedSGA := normalizeOracleMemoryUnitforRAC(instance.Spec.ConfigParams.SgaSize)
				data = append(data, "INIT_SGA_SIZE="+normalizedSGA)
			}

			if instance.Spec.ConfigParams.PgaSize != "" {
				normalizedPGA := normalizeOracleMemoryUnitforRAC(instance.Spec.ConfigParams.PgaSize)
				data = append(data, "INIT_PGA_SIZE="+normalizedPGA)
			}
			if instance.Spec.ConfigParams.Processes > 0 {
				// Configmap check is done in ValidateSpex
				data = append(data, "INIT_PROCESSES="+strconv.Itoa(instance.Spec.ConfigParams.Processes))
			}

			if instance.Spec.ConfigParams.CpuCount > 0 {
				// Configmap check is done in ValidateSpex
				data = append(data, "CPU_COUNT="+strconv.Itoa(instance.Spec.ConfigParams.CpuCount))
			}

		}
	}

	configMapData["envfile"] = strings.Join(data, "\r\n")
	return configMapData, nil
}

// normalizeOracleMemoryUnitforRAC standardizes memory strings onto Oracle's
// expected units, simplifying downstream comparisons and validation work.
func normalizeOracleMemoryUnitforRAC(s string) string {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.ReplaceAll(s, "GI", "G")
	s = strings.ReplaceAll(s, "MI", "M")
	return s
}

// ensurePlusPrefixforRAC guarantees ASM disk group names start with '+' to
// meet Oracle conventions when the spec omits the prefix.
func ensurePlusPrefixforRAC(name string) string {
	if name == "" {
		return ""
	}
	if !strings.HasPrefix(name, "+") {
		return "+" + name
	}
	return name
}

// createConfigMap ensures the target ConfigMap exists with the desired
// contents, creating it when first provisioning RAC configuration.
func (r *RacDatabaseReconciler) createConfigMap(ctx context.Context, instance *racdb.RacDatabase, cm *corev1.ConfigMap) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	found := &corev1.ConfigMap{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      cm.Name,
		Namespace: instance.Namespace,
	}, found)

	if err != nil && apierrors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating Configmap Normally")
		err = r.Create(ctx, cm)
		if err != nil {
			// Service creation failed
			reqLogger.Error(err, "failed to create configmap", " namespace", instance.Namespace)
			return ctrl.Result{}, nil
		} else {
			// Service creation was successful
			return ctrl.Result{Requeue: true}, nil
		}
	} else if err != nil {
		// Error that isn't due to the Service not existing
		reqLogger.Error(err, "failed to find the  configmap details")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil

}

// createOrReplaceService deploys or updates the Kubernetes Service backing
// RAC networking endpoints, reconciling metadata with the desired template.
func (r *RacDatabaseReconciler) createOrReplaceService(ctx context.Context, instance *racdb.RacDatabase,
	dep *corev1.Service,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name, "dep.Name", dep.Name)

	found := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: instance.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	raccommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	if err != nil && apierrors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating a service")
		err = r.Create(ctx, dep)
		if err != nil {
			// Service creation failed
			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create Service", "Service.Namespace", dep.Namespace, "Service.Name", dep.Name)
			return ctrl.Result{}, nil
		} else {
			// Service creation was successful
			return ctrl.Result{Requeue: true}, nil
		}
	} else if err != nil {
		// Error that isn't due to the Service not existing
		reqLogger.Error(err, "Failed to find the  Service details")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createOrReplaceAsmPv reconciles ASM persistent volumes for RAC disk
// groups, creating or patching PV resources to match the desired spec.
func (r *RacDatabaseReconciler) createOrReplaceAsmPv(
	ctx context.Context,
	instance *racdb.RacDatabase,
	dep *corev1.PersistentVolume,
	dgType string,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	found := &corev1.PersistentVolume{}

	// Fetch the existing PV
	err := r.Get(context.TODO(), types.NamespacedName{
		Name: dep.Name,
	}, found)

	jsn, _ := json.Marshal(dep)
	raccommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)

	if err != nil && apierrors.IsNotFound(err) {
		// PV does not exist, create it
		reqLogger.Info("Creating a new PV", "dep.Name", dep.Name)
		err = r.Create(context.TODO(), dep)
		if err != nil {
			// PV creation failed
			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create Persistent Volume", "PV.Name", dep.Name)
			return "", ctrl.Result{}, err
		}
		return dep.Name, ctrl.Result{}, nil
	} else if err != nil {
		// Other errors fetching the PV
		reqLogger.Error(err, "Failed to get Persistent Volume details")
		return "", ctrl.Result{}, err
	}

	// Check if the disk path or configuration differs from the existing PV
	if !reflect.DeepEqual(dep.Spec.PersistentVolumeSource.Local, found.Spec.PersistentVolumeSource.Local) {
		// Disk configuration has changed, delete the old PV and create a new one
		reqLogger.Info("Detected existing PV with different disk details and as the configuration has changed, setup cannot continue", "dep.Name", dep.Name)
		return "", ctrl.Result{}, fmt.Errorf("persistent volume %s has a different disk configuration. Please delete or update the existing PV to proceed", dep.Name)
	}

	reqLogger.Info("PV Found", "dep.Name", dep.Name, "dgType", dgType)

	return found.Name, ctrl.Result{}, nil
}

// createOrReplaceAsmPvC handles the ConfigMap variant of ASM PV reconcilation,
// ensuring consistent provisioning across configuration sources.
func (r *RacDatabaseReconciler) createOrReplaceAsmPvC(ctx context.Context, instance *racdb.RacDatabase,
	dep *corev1.PersistentVolumeClaim,
	dgType string,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	found := &corev1.PersistentVolumeClaim{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: instance.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	raccommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	if err != nil && apierrors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating a PVC")
		err = r.Create(ctx, dep)
		if err != nil {
			// Service creation failed
			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create Persistent Volume", "PVC.Namespace", dep.Namespace, "PersistentVolume.Name", dep.Name)
			return "", ctrl.Result{}, err
		} else {
			// Service creation was successful
			return dep.Name, ctrl.Result{}, nil
		}
	} else if err != nil {
		// Error that isn't due to the Service not existing
		reqLogger.Error(err, "Failed to find the persistent volume Claim details")
		return "", ctrl.Result{}, err
	}
	reqLogger.Info("PVC Found", "dep.Name", dep.Name, "dgType", dgType)

	return found.Name, ctrl.Result{}, nil
}

// ensureAsmStorageStatus initializes ASM DiskGroup details
// and restores ASM device and disk group info for each RAC instance.
// ensureAsmStorageStatus guarantees ASM disk group status structures exist
// before reconcile logic attempts to populate them with discovery results.
func (r *RacDatabaseReconciler) ensureAsmStorageStatus(racDatabase *racdb.RacDatabase) {
	r.Log.Info("Ensuring ASM DiskGroup status initialization")

	// Always initialize to avoid nil pointer issues
	if racDatabase.Status.AsmDiskGroups == nil {
		racDatabase.Status.AsmDiskGroups = []racdb.AsmDiskGroupStatus{}

		// If RAC instances exist, attempt to restore ASM devices and disk group metadata
		if len(racDatabase.Spec.InstDetails) > 0 {
			for idx, oraRacSpex := range racDatabase.Spec.InstDetails {
				// Skip deleted instances
				if strings.ToLower(oraRacSpex.IsDelete) == "true" {
					continue
				}

				podName := fmt.Sprintf("%s-%d", oraRacSpex.Name, 0) // assuming "-0" pattern for first pod
				r.Log.Info("Restoring ASM DiskGroup devices for instance", "Instance", oraRacSpex.Name)

				// Get CRS ASM and DB ASM device lists
				crsDeviceList := raccommon.GetcrsAsmDeviceList(
					racDatabase,
					&racdb.RacNodeStatus{},
					oraRacSpex,
					r.Client,
					r.kubeConfig,
					r.Log,
					r.kubeClient,
				)

				dbDeviceList := raccommon.GetdbAsmDeviceList(
					racDatabase,
					&racdb.RacNodeStatus{},
					oraRacSpex,
					r.Client,
					r.kubeConfig,
					r.Log,
					r.kubeClient,
				)

				// Update the ASM DiskGroups with CRS and DB device lists
				raccommon.SetAsmDiskGroupDevices(&racDatabase.Status.AsmDiskGroups, racdb.CrsAsmDiskDg, crsDeviceList)
				raccommon.SetAsmDiskGroupDevices(&racDatabase.Status.AsmDiskGroups, racdb.DbDataDiskDg, dbDeviceList)

				diskGroup := raccommon.GetAsmDiskgroup(
					podName,
					racDatabase,
					idx,
					r.kubeClient,
					r.kubeConfig,
					r.Log,
				)

				// If valid DG info returned, merge/update status
				if diskGroup != "" {
					for i, dgStatus := range racDatabase.Status.AsmDiskGroups {
						if dgStatus.Name == diskGroup {
							racDatabase.Status.AsmDiskGroups[i].Name = diskGroup
							break
						}
					}
				}
			}
		}

		r.Log.Info("ASM DiskGroup devices restored successfully",
			"DiskGroupsCount", len(racDatabase.Status.AsmDiskGroups))
	}
}

// ensureStatefulSetUpdated performs rolling updates on RAC StatefulSets when
// templates change, handling pod recreation and status monitoring.
func (r *RacDatabaseReconciler) ensureStatefulSetUpdated(ctx context.Context,
	reqLogger logr.Logger,
	racDatabase *racdb.RacDatabase,
	desired *appsv1.StatefulSet,
	asmAutoUpdate bool,
	// isDelete bool,
	req ctrl.Request, isOldStyle bool) error {
	timeout := 15 * time.Minute // Set a timeout for the update wait
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Fetch the existing StatefulSet
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: racDatabase.Namespace,
	}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the StatefulSet doesn't exist, create it
			reqLogger.Info("StatefulSet not found, creating new one", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
			return r.Create(ctx, desired)
		}
		reqLogger.Error(err, "Failed to get StatefulSet", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
		return err
	}

	// Compare the existing StatefulSet spec with the desired spec, sfs is replaced when ASM devices are added or removed
	if len(existing.Spec.Template.Spec.Containers[0].VolumeDevices) != len(desired.Spec.Template.Spec.Containers[0].VolumeDevices) {
		r.Log.Info("Change State to UPDATING")

		if isOldStyle && len(racDatabase.Spec.InstDetails) > 0 {
			for index := range racDatabase.Spec.InstDetails {
				r.updateRacInstStatus(
					racDatabase, ctx, req, racDatabase.Spec.InstDetails[index], index,
					string(racdb.RACProvisionState), r.Client, true,
				)
			}
		} else if racDatabase.Spec.ClusterDetails != nil && racDatabase.Spec.ClusterDetails.NodeCount > 0 {
			clusterSpec := racDatabase.Spec.ClusterDetails
			for index := 0; index < clusterSpec.NodeCount; index++ {
				r.updateRacNodeStatusForCluster(
					racDatabase, ctx, req, clusterSpec, index, string(racdb.RACProvisionState),
				)
			}
		}
		reqLogger.Info("StatefulSet spec differs for volume devices, updating StatefulSet (pods may be recreated)", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)

		// Perform the update
		err := r.Update(ctx, desired)
		if err != nil {
			reqLogger.Error(err, "Failed to update StatefulSet", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
			return err
		}

		reqLogger.Info("StatefulSet update applied, waiting for pod recreation", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)

		// Wait for the update to be applied
		for {
			select {
			case <-timeoutCtx.Done():
				reqLogger.Error(timeoutCtx.Err(), "Timed out waiting for StatefulSet update", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
				return timeoutCtx.Err()

			default:
				updated := &appsv1.StatefulSet{}
				err := r.Get(ctx, client.ObjectKey{
					Name:      desired.Name,
					Namespace: racDatabase.Namespace,
				}, updated)

				if err != nil {
					reqLogger.Error(err, "Failed to get StatefulSet after update", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
					return err
				}

				if reflect.DeepEqual(updated.Spec.Template.Spec.Containers[0].VolumeDevices, desired.Spec.Template.Spec.Containers[0].VolumeDevices) {
					reqLogger.Info("StatefulSet update is applied successfully", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
					return nil
				}

				reqLogger.Info("Waiting for StatefulSet update to be applied", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
				time.Sleep(5 * time.Second)
			}
		}
		// }
	} else {
		reqLogger.Info("StatefulSet matches for  ASM devices, SFS wont be updated", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
		return nil
	}
}

// diskGroupExists executes ASM queries against a pod to determine whether a
// specific disk group is present, aiding add/remove workflows.
func (r *RacDatabaseReconciler) diskGroupExists(podName, diskGroupName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *racdb.RacDatabase, logger logr.Logger) (bool, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	cmd := fmt.Sprintf("su - grid -c 'asmcmd lsdg | grep -w %s'", diskGroupName)
	stdout, _, err := raccommon.ExecCommand(podName, []string{"bash", "-c", cmd}, r.kubeClient, r.kubeConfig, instance, reqLogger)
	if err != nil {
		return false, err
	}
	if strings.Contains(stdout, diskGroupName) {
		return true, nil
	}
	return false, nil
}

// addDisks orchestrates the addition of new ASM disks by invoking helper
// jobs across RAC pods and waits for completion before proceeding.
func (r *RacDatabaseReconciler) addDisks(
	ctx context.Context,
	podList *corev1.PodList,
	instance *racdb.RacDatabase,
	diskGroupName string,
	deviceList []string,
) error {

	reqLogger := r.Log.WithValues(
		"Instance.Namespace", instance.Namespace,
		"Instance.Name", instance.Name,
	)

	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods available to add ASM disks")
	}

	// Pick exactly ONE pod to run once
	podName := podList.Items[0].Name
	reqLogger.Info("Using pod to add ASM disks", "Pod.Name", podName)

	if strings.HasPrefix(diskGroupName, "+") {
		diskGroupName = strings.TrimPrefix(diskGroupName, "+")
	}

	// Check disk group exists ONCE
	exists, err := r.diskGroupExists(
		podName,
		diskGroupName,
		r.kubeClient,
		r.kubeConfig,
		instance,
		reqLogger,
	)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("disk group %s does not exist", diskGroupName)
	}

	// Add all disks ONCE
	for _, disk := range deviceList {
		cmd := fmt.Sprintf(
			"python3 /opt/scripts/startup/scripts/main.py --updateasmdevices=\"diskname=%s;diskgroup=%s;processtype=addition\"",
			disk,
			diskGroupName,
		)

		reqLogger.Info("Executing command to add disk",
			"Pod.Name", podName,
			"Disk", disk,
			"DiskGroup", diskGroupName,
		)

		stdout, stderr, err := raccommon.ExecCommand(
			podName,
			[]string{"bash", "-c", cmd},
			r.kubeClient,
			r.kubeConfig,
			instance,
			reqLogger,
		)
		if err != nil {
			// tolerate "already added"
			if strings.Contains(stdout, "UPDATE_ASMDEVICES_NOT_UPDATED") {
				reqLogger.Info("ASM disk already exists, skipping",
					"Disk", disk,
					"DiskGroup", diskGroupName,
				)
				continue
			}

			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to execute command",
				"Stdout", stdout,
				"Stderr", stderr,
			)
			return err
		}
	}

	return nil
}

// checkRacDaemonSetStatusforRAC verifies disk-check DaemonSet readiness,
// returning a boolean that indicates whether discovery completed.
func checkRacDaemonSetStatusforRAC(ctx context.Context, r *RacDatabaseReconciler, racDatabase *racdb.RacDatabase) (bool, error) {
	timeout := time.After(2 * time.Minute)
	tick := time.NewTicker(10 * time.Second) // Poll every 10 seconds
	// Initial delay before starting checks
	time.Sleep(10 * time.Second)
	defer tick.Stop()
	// Sleep for 60 seconds
	for {
		select {
		case <-timeout:
			// Timeout reached
			ds := &appsv1.DaemonSet{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      "disk-check-daemonset",
				Namespace: racDatabase.Namespace,
			}, ds)
			if err != nil {
				return false, err
			}

			// Fetch the list of Pods managed by the DaemonSet
			pods, err := r.kubeClient.CoreV1().Pods(racDatabase.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=disk-check",
			})
			if err != nil {
				return false, err
			}

			// Check logs from each Pod
			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					// Pod is not running, check for logs and errors
					logs, err := r.kubeClient.CoreV1().Pods(racDatabase.Namespace).GetLogs(
						pod.Name,
						&corev1.PodLogOptions{},
					).DoRaw(ctx)
					if err != nil {
						return false, err
					}

					if bytes.Contains(logs, []byte("not a valid block device")) {
						// Disk validation failed
						return false, nil
					}
				}
			}

			// DaemonSet did not become ready or running within the timeout
			return false, fmt.Errorf("DaemonSet %s/%s did not become ready or running within 5 minutes", racDatabase.Namespace, "disk-check-daemonset")

		case <-tick.C:
			// Check DaemonSet status
			ds := &appsv1.DaemonSet{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      "disk-check-daemonset",
				Namespace: racDatabase.Namespace,
			}, ds)
			if err != nil {
				return false, err
			}

			// Check DaemonSet readiness
			if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled && ds.Status.NumberReady > 0 {
				// DaemonSet is running and ready
				return true, nil
			}

			// If DaemonSet is not ready, fetch the list of Pods managed by the DaemonSet
			pods, err := r.kubeClient.CoreV1().Pods(racDatabase.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=disk-check",
			})
			if err != nil {
				return false, err
			}

			// Check logs from each Pod
			for _, pod := range pods.Items {
				// Pod is not running, check for logs and errors
				logs, err := r.kubeClient.CoreV1().Pods(racDatabase.Namespace).GetLogs(
					pod.Name,
					&corev1.PodLogOptions{},
				).DoRaw(ctx)
				if err != nil {
					return false, err
				}

				if bytes.Contains(logs, []byte("not a valid block device")) {
					// Disk validation failed
					return false, nil
				}

			}
		}
	}
}

// createOrReplaceSfs reconciles per-instance StatefulSets ensuring pods match
// the desired spec, handling lookups, updates, and requeue timing.
func (r *RacDatabaseReconciler) createOrReplaceSfs(
	ctx context.Context,
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	dep *appsv1.StatefulSet,
	index int,
	isLast bool,
	oldState string,
	isOldStyle bool,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", racDatabase.Namespace, "Instance.Name", racDatabase.Name)

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: racDatabase.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	raccommon.LogMessages("DEBUG", string(jsn), nil, racDatabase, r.Log)

	if err != nil && apierrors.IsNotFound(err) {
		if isOldStyle {
			r.updateRacInstStatus(
				racDatabase, ctx, req, racDatabase.Spec.InstDetails[index], index,
				string(racdb.RACProvisionState), r.Client, true,
			)
		} else {
			clusterSpec := racDatabase.Spec.ClusterDetails
			r.updateRacNodeStatusForCluster(
				racDatabase, ctx, req,
				clusterSpec, index,
				string(racdb.RACProvisionState),
			)
		}
		reqLogger.Info("Creating a StatefulSet Normally", "StatefulSetName", dep.Name)
		err = r.Create(ctx, dep)

		if err != nil {
			// StatefulSet creation failed
			racDatabase.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create StatefulSet", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
			return ctrl.Result{}, err
		} else if !isLast {
			// StatefulSet creation was successful
			return ctrl.Result{}, nil
		}
	} else if err != nil {
		// Error that isn't due to the StatefulSet not existing
		reqLogger.Error(err, "Failed to find the StatefulSet details")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createOrReplaceSfsAsm coordinates StatefulSet changes when ASM updates are
// required, driving rolling restarts and handling auto-update flags.
func (r *RacDatabaseReconciler) createOrReplaceSfsAsm(ctx context.Context, req ctrl.Request, racDatabase *racdb.RacDatabase,
	dep *appsv1.StatefulSet, index int, isLast bool, oldSpec *racdb.RacDatabaseSpec, isOldStyle bool,
) (ctrl.Result, error) {
	asmAutoUpdate := true
	reqLogger := r.Log.WithValues("racDatabase.Namespace", racDatabase.Namespace, "racDatabase.Name", racDatabase.Name)

	found := &appsv1.StatefulSet{}

	// Check if the StatefulSet was found successfully
	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: racDatabase.Namespace,
	}, found)
	if err != nil {
		reqLogger.Error(err, "Failed to find existing StatefulSet to update")
		return ctrl.Result{}, err
	}

	addedAsmDisks, removedAsmDisks, err := r.computeDiskChanges(racDatabase, oldSpec)
	if err != nil {
		return ctrl.Result{}, err
	}
	inUse := false

	if len(removedAsmDisks) > 0 {
		// Step 1: Get the StatefulSet
		OraRacSpex := racDatabase.Spec.InstDetails[index]
		racSfSet, err := raccommon.CheckSfset(OraRacSpex.Name, racDatabase, r.Client)
		if err != nil {
			errMsg := fmt.Errorf("failed to retrieve StatefulSet for RAC database '%s': %w", OraRacSpex.Name, err)
			r.Log.Error(err, errMsg.Error())
			return reconcile.Result{}, errMsg
		}

		// Step 2: Get the Pod list
		podList, err := raccommon.GetPodList(racSfSet.Name, racDatabase, r.Client, OraRacSpex)
		if err != nil {
			errMsg := fmt.Errorf("failed to retrieve pod list for StatefulSet '%s': %w", racSfSet.Name, err)
			r.Log.Error(err, errMsg.Error())
			return reconcile.Result{}, errMsg
		}
		if len(podList.Items) == 0 {
			errMsg := fmt.Errorf("no pods found for StatefulSet '%s'", racSfSet.Name)
			r.Log.Error(errMsg, "Empty pod list")
			return reconcile.Result{}, errMsg
		}

		// Step 3: Use last pod to get ASM state
		podName := podList.Items[len(podList.Items)-1].Name
		racDatabase.Status.AsmDiskGroups = raccommon.GetAsmInstState(podName, racDatabase, 0, r.kubeClient, r.kubeConfig, r.Log)

		// Step 4: Check removed disks against disk groups

		asmDiskGroups := racDatabase.Status.AsmDiskGroups

		for _, removedAsmDisk := range removedAsmDisks {
			for _, diskgroup := range asmDiskGroups {
				for _, asmDiskStatus := range diskgroup.Disks {
					if removedAsmDisk == asmDiskStatus.Name {
						err := fmt.Errorf(
							"disk '%s' is part of diskgroup '%s' and must be manually removed before proceeding",
							removedAsmDisk, diskgroup.Name,
						)
						r.Log.Info(
							"Disk is in use and cannot be removed automatically",
							"disk", removedAsmDisk, "diskgroup", diskgroup.Name,
						)
						inUse = true
						return reconcile.Result{}, err
					}
				}
			}
		}
	}
	r.ensureAsmStorageStatus(racDatabase)

	// Ensure the StatefulSet is updated or re-created based on autoUpdate set to true/false
	// err = r.ensureStatefulSetUpdated(ctx, reqLogger, racDatabase, dep, autoUpdate, isDelete, req)
	err = r.ensureStatefulSetUpdated(ctx, reqLogger, racDatabase, dep, asmAutoUpdate, req, isOldStyle)
	if err != nil {
		racDatabase.Spec.IsFailed = true
		reqLogger.Error(err, "Failed to ensure StatefulSet is updated or created")
		if isOldStyle {
			r.updateRacInstStatus(racDatabase, ctx, req, racDatabase.Spec.InstDetails[index], index, string(racdb.RACProvisionState), r.Client, true)
		} else {
			clusterSpec := racDatabase.Spec.ClusterDetails
			for index := 0; index < clusterSpec.NodeCount; index++ {
				r.updateRacNodeStatusForCluster(
					racDatabase, ctx, req, clusterSpec, index, string(racdb.RACProvisionState),
				)
			}
		}
		return ctrl.Result{}, err
	}

	// Wait for all Pods to be created and running
	podList := &corev1.PodList{}
	allPodsRunning := true

	// Immediate check before starting ticker loop
	err = r.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabels(dep.Spec.Template.Labels))
	if err != nil {
		reqLogger.Error(err, "Failed to list Pods", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
		return reconcile.Result{}, err
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			allPodsRunning = false
			reqLogger.Info("Waiting for Pod to be running", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
			break
		}
	}

	if !allPodsRunning {
		// Only wait if at least one pod is not running
		timeout := time.After(2 * time.Minute) // 2-minute timeout
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

	waitLoop:
		for {
			select {
			case <-timeout:
				reqLogger.Info("Timed out waiting for all Pods to be created and running", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
				return reconcile.Result{}, fmt.Errorf("timed out waiting for all Pods to be created and running")
			case <-ticker.C:
				err = r.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabels(dep.Spec.Template.Labels))
				if err != nil {
					reqLogger.Error(err, "Failed to list Pods", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
					return reconcile.Result{}, err
				}

				allPodsRunning = true
				for _, pod := range podList.Items {
					if pod.Status.Phase != corev1.PodRunning {
						allPodsRunning = false
						reqLogger.Info("Waiting for Pod to be running", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
						break
					}
				}

				if allPodsRunning {
					reqLogger.Info("All Pods are running", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
					break waitLoop
				}
			}
		}
	}

	// Additional pod readiness check per your original code
	if allPodsRunning {
		const (
			podCheckInterval = 15 * time.Second // Interval between pod readiness checks
			podReadyTimeout  = 15 * time.Minute // Maximum wait time for pod readiness
		)

		timeoutCtx, cancel := context.WithTimeout(ctx, podReadyTimeout)
		defer cancel()

		// Wait for StatefulSet pods to be ready
		for i := 0; i < len(dep.Spec.Template.Spec.Containers); i++ {
			podName := fmt.Sprintf("%s-%d", dep.Name, i)
			var isPodReady bool

		waitForPodASM:
			for {
				select {
				case <-timeoutCtx.Done():
					reqLogger.Error(timeoutCtx.Err(), "Timed out waiting for pod to be ready", "Pod.Name", podName)
					if isOldStyle {
						r.updateRacInstStatus(racDatabase, ctx, req, racDatabase.Spec.InstDetails[index], index, string(racdb.RACProvisionState), r.Client, true)
					} else {
						clusterSpec := racDatabase.Spec.ClusterDetails
						for index := 0; index < clusterSpec.NodeCount; index++ {
							r.updateRacNodeStatusForCluster(
								racDatabase, ctx, req, clusterSpec, index, string(racdb.RACProvisionState),
							)
						}
					}
					return ctrl.Result{}, timeoutCtx.Err()
				default:
					podList, err := raccommon.GetPodList(dep.Name, racDatabase, r.Client, racDatabase.Spec.InstDetails[index])
					if err != nil {
						reqLogger.Error(err, "Failed to list pods")
						return ctrl.Result{}, err
					}
					isPodReady, _, _ = raccommon.PodListValidation(podList, dep.Name, racDatabase, r.Client)
					if isPodReady {
						reqLogger.Info("Pod is ready", "Pod.Name", podName)
						break waitForPodASM // Break out of the labeled loop
					} else {
						reqLogger.Info("Pod is not ready yet", "Pod.Name", podName)
						time.Sleep(podCheckInterval)
					}
				}
			}
		}
	}
	// Disk is not in use, proceed with PV and PVC deletion in last stage
	if isLast && !inUse {
		// Use raccommon.GetAsmPvcName and raccommon.getAsmPvName to generate PVC and PV names

		// Find and delete the corresponding PVC
		for _, diskName := range racDatabase.Status.RacNodes[index].NodeDetails.MountedDevices {
			for _, removedAsmDisk := range removedAsmDisks {
				if diskName == removedAsmDisk {
					pvcName := raccommon.GetAsmPvcName(diskName, racDatabase.Name)
					pvc := &corev1.PersistentVolumeClaim{}
					err := r.Get(ctx, client.ObjectKey{
						Name:      pvcName,
						Namespace: racDatabase.Namespace,
					}, pvc)
					if err != nil {
						if !apierrors.IsNotFound(err) {
							r.Log.Error(err, "Failed to get PVC", "PVC.Name", pvcName)
							return reconcile.Result{}, err
						}
						// PVC already deleted
					} else {
						err = r.Delete(ctx, pvc)
						if err != nil {
							r.Log.Error(err, "Failed to delete PVC", "PVC.Name", pvcName)
							return reconcile.Result{}, err
						}
						r.Log.Info("Successfully deleted PVC", "PVC.Name", pvcName)
					}

					// Find and delete the corresponding PV
					pvName := raccommon.GetAsmPvName(diskName, racDatabase.Name) // Use the existing function
					pv := &corev1.PersistentVolume{}
					err = r.Get(ctx, client.ObjectKey{
						Name: pvName,
					}, pv)
					if err != nil {
						if !apierrors.IsNotFound(err) {
							r.Log.Error(err, "Failed to get PV", "PV.Name", pvName)
							return reconcile.Result{}, err
						}
						// PV already deleted
					} else {
						err = r.Delete(ctx, pv)
						if err != nil {
							r.Log.Error(err, "Failed to delete PV", "PV.Name", pvName)
							return reconcile.Result{}, err
						}
						r.Log.Info("Successfully deleted PV", "PV.Name", pvName)
					}
				}
			}
		}
	}

	if isLast && asmAutoUpdate {
		// last iteration
		// update status column with configParams
		// Addition fo Disk Execution
		// Check each new disk against CrsAsmDeviceList, DbAsmDeviceList, RecoAsmDeviceList, RedoAsmDeviceList
		deviceDg := ""
		for _, disk := range addedAsmDisks {
			var cName, fName string

			if racDatabase.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
				cName = racDatabase.Spec.ConfigParams.GridResponseFile.ConfigMapName
			}
			if racDatabase.Spec.ConfigParams.GridResponseFile.Name != "" {
				fName = racDatabase.Spec.ConfigParams.GridResponseFile.Name
			}
			err := setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(racDatabase, r.Client, cName, fName)
			if err != nil {
				reqLogger.Error(err, "Failed to set disk group defaults")
				return ctrl.Result{}, err
			}

			deviceDg = getDeviceDG(
				disk,
				&racDatabase.Spec,
				reqLogger,
			)
		}
		if deviceDg != "" {
			// Add disks after POD recreation
			podList, err := raccommon.GetPodList(dep.Name, racDatabase, r.Client, racDatabase.Spec.InstDetails[index])
			if err != nil {
				reqLogger.Error(err, "Failed to list pods")
				return ctrl.Result{}, err
			}
			err = r.addDisks(ctx, podList, racDatabase, deviceDg, addedAsmDisks)
			if err != nil {
				return ctrl.Result{}, err
			}
			reqLogger.Info("New Disks added to CRS Disks Group")
		}
	}

	return ctrl.Result{}, nil
}

// createOrReplaceSfsAsmCluster manages new cluster-style StatefulSet updates
// when ASM configuration changes are detected across the RAC topology.
func (r *RacDatabaseReconciler) createOrReplaceSfsAsmCluster(
	ctx context.Context,
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	dep *appsv1.StatefulSet,
	index int,
	isLast bool,
	oldSpec *racdb.RacDatabaseSpec,
	isOldStyle bool,
) (ctrl.Result, error) {

	asmAutoUpdate := true
	reqLogger := r.Log.WithValues("racDatabase.Namespace", racDatabase.Namespace,
		"racDatabase.Name", racDatabase.Name)

	found := &appsv1.StatefulSet{}

	// ---------------------------------------------------------
	// STEP 1 — Retrieve existing StatefulSet
	// ---------------------------------------------------------
	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: racDatabase.Namespace,
	}, found)
	// debugger
	if err != nil {
		reqLogger.Error(err, "Failed to find existing StatefulSet to update")
		return ctrl.Result{}, err
	}

	// ---------------------------------------------------------
	// STEP 2 — Compute disk changes
	// ---------------------------------------------------------
	addedAsmDisks, removedAsmDisks, err := r.computeDiskChanges(racDatabase, oldSpec)
	if err != nil {
		return ctrl.Result{}, err
	}
	//debugger
	// addedAsmDisks = []string{"/dev/disk/by-partlabel/ocne_asm_disk_03"}

	inUse := false

	// ---------------------------------------------------------
	// STEP 3 — Check ASM disk usage before removal
	// ---------------------------------------------------------
	if len(removedAsmDisks) > 0 {

		clusterSpec := racDatabase.Spec.ClusterDetails
		nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, index+1)

		// 3a: Retrieve StatefulSet
		racSfSet, err := raccommon.CheckSfset(dep.Name, racDatabase, r.Client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to retrieve StatefulSet '%s': %w", dep.Name, err)
		}

		// 3b: Get pod list for this RAC node (cluster mode)
		podList, err := raccommon.GetPodList(
			racSfSet.Name,
			racDatabase,
			r.Client,
			racdb.RacInstDetailSpec{Name: nodeName},
		)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to retrieve pod list for StatefulSet '%s': %w",
				racSfSet.Name, err)
		}
		if len(podList.Items) == 0 {
			return ctrl.Result{}, fmt.Errorf("no pods found for StatefulSet '%s'", racSfSet.Name)
		}

		// 3c: Query ASM disk groups using last pod
		podName := podList.Items[len(podList.Items)-1].Name
		racDatabase.Status.AsmDiskGroups =
			raccommon.GetAsmInstState(podName, racDatabase, index, r.kubeClient, r.kubeConfig, r.Log)

	}

	// Sync ASM status
	r.ensureAsmStorageStatus(racDatabase)

	// ---------------------------------------------------------
	// STEP 4 — Apply StatefulSet ASM Update
	// ---------------------------------------------------------
	err = r.ensureStatefulSetUpdated(ctx, reqLogger, racDatabase, dep, asmAutoUpdate, req, isOldStyle)
	if err != nil {
		racDatabase.Spec.IsFailed = true
		reqLogger.Error(err, "Failed to ensure StatefulSet update")

		// Update cluster node statuses
		clusterSpec := racDatabase.Spec.ClusterDetails
		for nodeIdx := 0; nodeIdx < clusterSpec.NodeCount; nodeIdx++ {
			r.updateRacNodeStatusForCluster(
				racDatabase, ctx, req, clusterSpec, nodeIdx, string(racdb.RACProvisionState))
		}
		return ctrl.Result{}, err
	}

	// ---------------------------------------------------------
	// STEP 5 — Check if all pods are Running and Ready (non-blocking)
	// ---------------------------------------------------------
	podList := &corev1.PodList{}

	reqLogger.Info("Checking if all pods are Running and Ready")

	err = r.List(ctx, podList,
		client.InNamespace(dep.Namespace),
		client.MatchingLabels(dep.Spec.Template.Labels),
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(podList.Items) == 0 {
		reqLogger.Info("No pods found yet, requeueing")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	for _, pod := range podList.Items {

		if pod.Status.Phase != corev1.PodRunning {
			reqLogger.Info("Pod not Running yet, requeueing", "pod", pod.Name)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady &&
				cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}

		if !ready {
			reqLogger.Info("Pod not Ready yet, requeueing", "pod", pod.Name)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	reqLogger.Info("All pods are Running and Ready, proceeding with next steps")

	// ---------------------------------------------------------
	// STEP 6 — PVC/PV deletion for removed disks (last iteration)
	// ---------------------------------------------------------
	if isLast && !inUse {

		for _, removed := range removedAsmDisks {

			pvcName := raccommon.GetAsmPvcName(removed, racDatabase.Name)
			pvName := raccommon.GetAsmPvName(removed, racDatabase.Name)

			pvc := &corev1.PersistentVolumeClaim{}
			pv := &corev1.PersistentVolume{}

			// Delete PVC
			_ = r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: racDatabase.Namespace}, pvc)
			_ = r.Delete(ctx, pvc)

			// Delete PV
			_ = r.Get(ctx, client.ObjectKey{Name: pvName}, pv)
			_ = r.Delete(ctx, pv)

			r.Log.Info("Deleted ASM PVC & PV", "PVC", pvcName, "PV", pvName)
		}

	}

	// ---------------------------------------------------------
	// STEP 7 — Add new ASM disks after recreation (last iteration)
	// ---------------------------------------------------------
	if isLast && len(addedAsmDisks) > 0 {
		clusterSpec := racDatabase.Spec.ClusterDetails
		nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, index+1)

		podList, err := raccommon.GetPodList(dep.Name, racDatabase, r.Client,
			racdb.RacInstDetailSpec{Name: nodeName},
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Map disk group name to autoUpdate (bool)
		dgAutoUpdate := map[string]bool{}

		for _, dg := range racDatabase.Spec.AsmStorageDetails {
			if strings.EqualFold(dg.AutoUpdate, "true") {
				dgAutoUpdate[dg.Name] = true
			} else {
				// only set false if key does not exist yet
				if _, exists := dgAutoUpdate[dg.Name]; !exists {
					dgAutoUpdate[dg.Name] = false
				}
			}
		}

		// Group added disks by their diskgroup
		disksByDG := map[string][]string{}
		for _, disk := range addedAsmDisks {
			dg := getDeviceDG(disk, &racDatabase.Spec, reqLogger)
			if dg == "" {
				reqLogger.Info("No diskgroup found for disk, skipping", "disk", disk)
				continue
			}
			disksByDG[dg] = append(disksByDG[dg], disk)
		}

		// Add disks only for groups where autoUpdate == "true"
		for dg, disks := range disksByDG {
			if !dgAutoUpdate[dg] {
				reqLogger.Info("Skipping ASM disk add: autoUpdate is disabled", "diskgroup", dg)
				continue
			}
			if len(disks) == 0 {
				continue
			}
			err = r.addDisks(ctx, podList, racDatabase, dg, disks)
			if err != nil {
				return ctrl.Result{}, err
			}
			reqLogger.Info("New ASM disks added", "diskgroup", dg, "disks", disks)
		}
	}

	return ctrl.Result{}, nil
}

// getDeviceDG returns the disk group name for a given device by scanning
// the RAC spec, aiding logging and validations during reconcile.
func getDeviceDG(disk string, spec *racdb.RacDatabaseSpec, reqLogger logr.Logger) string {
	for _, dg := range spec.AsmStorageDetails {
		for _, dgDisk := range dg.Disks {
			if strings.TrimSpace(dgDisk) == strings.TrimSpace(disk) {
				reqLogger.Info("Disk found in ASM DiskGroup", "disk", disk, "diskGroup", dg.Name, "type", dg.Type)
				return dg.Name // or return dg.Type if you need type
			}
		}
	}
	return ""
}

// getRACDisksChangedSpecforRAC compares old vs. new specs to produce lists of
// added and removed disks, which guide follow-up reconcile operations.
func getRACDisksChangedSpecforRAC(racDatabase racdb.RacDatabase, oldSpec racdb.RacDatabaseSpec) ([]string, []string) {
	addedAsmDisks := []string{}
	removedAsmDisks := []string{}

	// If old spec is empty, do not treat this as disk changes
	if len(oldSpec.AsmStorageDetails) == 0 {
		return addedAsmDisks, removedAsmDisks
	}

	// Helper: disk slice to set
	diskSliceToSet := func(disks []string) map[string]bool {
		set := make(map[string]bool)
		for _, disk := range disks {
			if disk != "" {
				set[disk] = true
			}
		}
		return set
	}

	newGroupMap := make(map[string][]string)
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		groupKey := fmt.Sprintf("%s-%s", dg.Name, dg.Type)
		newGroupMap[groupKey] = dg.Disks
	}

	oldGroupMap := make(map[string][]string)
	for _, dg := range oldSpec.AsmStorageDetails {
		groupKey := fmt.Sprintf("%s-%s", dg.Name, dg.Type)
		oldGroupMap[groupKey] = dg.Disks
	}

	// Unique sets for additions/removals
	addedDiskSet := make(map[string]bool)
	removedDiskSet := make(map[string]bool)

	// 1. Check for added and removed disks per group
	for name, newDisks := range newGroupMap {
		oldDisks := oldGroupMap[name]
		// Added: in newDisks not in oldDisks
		oldSet := diskSliceToSet(oldDisks)
		for _, disk := range newDisks {
			if disk != "" && !oldSet[disk] {
				addedDiskSet[disk] = true
			}
		}
	}
	for name, oldDisks := range oldGroupMap {
		newDisks := newGroupMap[name]
		newSet := diskSliceToSet(newDisks)
		for _, disk := range oldDisks {
			if disk != "" && !newSet[disk] {
				removedDiskSet[disk] = true
			}
		}
	}

	// 2. Flatten all for top-level lists (de-duplicate)
	for disk := range addedDiskSet {
		addedAsmDisks = append(addedAsmDisks, disk)
	}
	for disk := range removedDiskSet {
		removedAsmDisks = append(removedAsmDisks, disk)
	}

	return addedAsmDisks, removedAsmDisks
}

// manageRacDatabaseDeletion orchestrates finalizer and cleanup logic when
// manageRacDatabaseDeletion orchestrates finalizer and cleanup logic when
// the RAC database resource is being deleted, including final status updates.
func (r *RacDatabaseReconciler) manageRacDatabaseDeletion(req ctrl.Request, ctx context.Context, racDatabase *racdb.RacDatabase, isOldStyle bool) error {
	log := r.Log.WithValues("manageRacDatabaseDeletion", req.NamespacedName)

	// Check if the RacDatabase instance is marked to be deleted
	isRacDatabaseMarkedToBeDeleted := racDatabase.GetDeletionTimestamp() != nil
	if isRacDatabaseMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(racDatabase, racDatabaseFinalizer) {
			// Run cleanup
			if err := r.cleanupRacDatabase(req, racDatabase, isOldStyle); err != nil {
				return err
			}

			// Remove finalizer
			if err := r.patchFinalizer(ctx, racDatabase, false); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return err
			}

			log.Info("Successfully removed RacDatabase finalizer")
			return errors.New("deletion pending")
		}

		// Finalizer already gone, just let K8s delete it
		return errors.New("deletion pending")
	}

	// Add finalizer for this CR if not present
	if !controllerutil.ContainsFinalizer(racDatabase, racDatabaseFinalizer) {
		if err := r.patchFinalizer(ctx, racDatabase, true); err != nil {
			log.Error(err, "Failed to add finalizer")
			return err
		}
	}
	return nil
}

// patchFinalizer updates the finalizer for the given resource
// patchFinalizer adds or removes the custom finalizer using a merge patch so
// the operator can manage cleanup semantics during deletion.
func (r *RacDatabaseReconciler) patchFinalizer(ctx context.Context, racDatabase *racdb.RacDatabase, add bool) error {
	var finalizers []string
	if add {
		finalizers = append(racDatabase.GetFinalizers(), racDatabaseFinalizer)
	} else {
		for _, finalizer := range racDatabase.GetFinalizers() {
			if finalizer != racDatabaseFinalizer {
				finalizers = append(finalizers, finalizer)
			}
		}
	}

	// Prepare patch payload
	patchData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers": finalizers,
		},
	}
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return err
	}

	patch := client.RawPatch(types.MergePatchType, patchBytes)
	return r.Client.Patch(ctx, racDatabase, patch, &client.PatchOptions{
		FieldManager: "rac-database-finalizer-manager",
	})
}

// cleanupRacDatabase handles resource teardown for RAC objects, including
// waiting for StatefulSets and services to drain before final removal.
func (r *RacDatabaseReconciler) cleanupRacDatabase(
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	isOldStyle bool) error {

	log := r.Log.WithValues("cleanupRacDatabase", req.NamespacedName)
	var err error

	if isOldStyle {
		// --- Old style (per-instance) cleanup ---
		var sfSetFound *appsv1.StatefulSet
		var svcFound *corev1.Service
		var i int32

		// Delete StatefulSets and ConfigMaps
		for i = 0; i < int32(len(racDatabase.Spec.InstDetails)); i++ {
			OraRacSpex := racDatabase.Spec.InstDetails[i]
			sfSetFound, err = raccommon.CheckSfset(OraRacSpex.Name, racDatabase, r.Client)
			if err != nil {
				return err
			}
			if sfSetFound != nil {
				log.Info("Deleting RAC Statefulset " + sfSetFound.Name)
				if err := r.Client.Delete(context.Background(), sfSetFound); err != nil {
					if apierrors.IsNotFound(err) {
						log.Info("StatefulSet already deleted")
					} else {
						return err
					}
				}
			}

			cmName := OraRacSpex.Name + racDatabase.Name + "-cmap"
			configMapFound, err := raccommon.CheckConfigMap(racDatabase, cmName, r.Client)
			if err == nil {
				log.Info("Deleting RAC Configmap " + configMapFound.Name)
				if err = r.Client.Delete(context.Background(), configMapFound); err != nil {
					return err
				}
			}
		}

		// Delete SW PVCs
		for index := range racDatabase.Spec.InstDetails {
			err = raccommon.DelRacSwPvc(racDatabase, racDatabase.Spec.InstDetails[index], r.Client, r.Log)
			if err != nil {
				return err
			}
		}

		// Delete DaemonSet
		daemonSetName := "disk-check-daemonset"
		daemonSet := &appsv1.DaemonSet{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      daemonSetName,
			Namespace: racDatabase.Namespace,
		}, daemonSet)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Log.Info("DaemonSet not found, skipping deletion", "DaemonSet.Name", daemonSetName)
			} else {
				r.Log.Error(err, "Failed to get DaemonSet", "DaemonSet.Name", daemonSetName)
				return err
			}
		} else {
			if err = r.Client.Delete(context.TODO(), daemonSet); err != nil {
				r.Log.Error(err, "Failed to delete DaemonSet", "DaemonSet.Name", daemonSetName)
				return err
			}
		}

		// Delete ASM PVCs and PVs
		if racDatabase.Spec.AsmStorageDetails != nil {
			for _, dg := range racDatabase.Spec.AsmStorageDetails {
				for _, diskName := range dg.Disks {
					err = raccommon.DelRacPvc(racDatabase, diskName, dg.Name, r.Client, r.Log)
					if err != nil {
						return err
					}
				}
			}

			if len(racDatabase.Spec.StorageClass) == 0 {
				processedDisks := make(map[string]bool)
				for _, dg := range racDatabase.Spec.AsmStorageDetails {
					for _, diskName := range dg.Disks {
						if processedDisks[diskName] {
							continue
						}
						err = raccommon.DelRacPv(racDatabase, diskName, dg.Name, r.Client, r.Log)
						if err != nil {
							return err
						}
						processedDisks[diskName] = true
					}
				}
			}
		}

		// Delete Services
		svcTypes := []string{"vip", "local", "onssvc", "lsnrsvc", "scansvc", "scan"}
		for _, svcType := range svcTypes {
			if err := r.deleteRacServices(req, racDatabase, svcType, isOldStyle); err != nil {
				return err
			}
		}
		// NodePort Services
		for i = 0; i < int32(len(racDatabase.Spec.InstDetails)); i++ {
			if len(racDatabase.Spec.InstDetails[i].NodePortSvc) != 0 {
				for index := range racDatabase.Spec.InstDetails[i].NodePortSvc {
					svcFound, err = raccommon.CheckRacSvc(
						racDatabase,
						"nodeport",
						racDatabase.Spec.InstDetails[i],
						racDatabase.Spec.InstDetails[i].NodePortSvc[index].SvcName,
						r.Client,
					)
					if err == nil {
						if err = r.Client.Delete(context.Background(), svcFound); err != nil {
							return err
						}
					}
				}
			}
		}
		// All done
		log.Info("Successfully cleaned up RacDatabase (old style)")
		return nil

	} else {
		// --- New style (cluster-level) cleanup ---
		log.Info("Running cleanup for RacDatabase (cluster style)")

		// Example: Cleanup using details from ClusterDetails
		// You'll need to adapt this based on how cluster-level objects are managed/deployed.
		// Pseudocode follows:

		cd := racDatabase.Spec.ClusterDetails
		if cd == nil {
			log.Error(nil, "ClusterDetails is nil in cleanup for new style spec")
			return fmt.Errorf("internal error: ClusterDetails is nil in new style cleanup")
		}

		// For StatefulSet: if you are using a pattern, such as racnode-0, racnode-1, etc:
		for i := 0; i < cd.NodeCount; i++ {
			nodeName := fmt.Sprintf("%s%d", cd.RacNodeName, i+1)
			sfSetFound, err := raccommon.CheckSfset(nodeName, racDatabase, r.Client)
			if err != nil && sfSetFound != nil {
				return err
			}
			if sfSetFound != nil {
				log.Info("Deleting RAC Statefulset " + sfSetFound.Name)
				if err := r.Client.Delete(context.Background(), sfSetFound); err != nil {
					if apierrors.IsNotFound(err) {
						log.Info("StatefulSet already deleted")
					} else {
						return err
					}
				}
			}
			// Cluster-wide configmap cleanup (optional, if used in new style)
			cmName := fmt.Sprintf("%s%s-cmap", nodeName, racDatabase.Name)
			configMapFound, err := raccommon.CheckConfigMap(racDatabase, cmName, r.Client)
			if err == nil {
				log.Info("Deleting RAC Configmap " + configMapFound.Name)
				if err = r.Client.Delete(context.Background(), configMapFound); err != nil {
					return err
				}
			}
		}

		daemonSetName := "disk-check-daemonset"
		daemonSet := &appsv1.DaemonSet{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      daemonSetName,
			Namespace: racDatabase.Namespace,
		}, daemonSet)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("DaemonSet not found, skipping deletion", "DaemonSet.Name", daemonSetName)
			} else {
				log.Error(err, "Failed to get DaemonSet", "DaemonSet.Name", daemonSetName)
				return err
			}
		} else {
			if err = r.Client.Delete(context.TODO(), daemonSet); err != nil {
				log.Error(err, "Failed to delete DaemonSet", "DaemonSet.Name", daemonSetName)
				return err
			}
		}
		// Example software PVC cleanup (adjust as per your naming conventions)
		for i := 0; i < cd.NodeCount; i++ {
			err := raccommon.DelRacSwPvcClusterStyle(racDatabase, cd, i, r.Client, r.Log)
			if err != nil {
				return err
			}
		}

		// ASM PVC/PV cleanup
		if racDatabase.Spec.AsmStorageDetails != nil {
			for _, dg := range racDatabase.Spec.AsmStorageDetails {
				for _, diskName := range dg.Disks {
					err = raccommon.DelRacPvc(racDatabase, diskName, dg.Name, r.Client, r.Log)
					if err != nil {
						return err
					}
					// PVs
					if len(racDatabase.Spec.StorageClass) == 0 {
						err = raccommon.DelRacPv(racDatabase, diskName, dg.Name, r.Client, r.Log)
						if err != nil {
							return err
						}
					}
				}
			}
		}

		// Example: Cleanup of services based on cluster-level naming
		svcTypes := []string{"vip", "local", "onssvc", "lsnrsvc", "scansvc", "scan"}
		for _, svcType := range svcTypes {
			if err := r.deleteRacServices(req, racDatabase, svcType, isOldStyle); err != nil {
				return err
			}
		}

		log.Info("Successfully cleaned up RacDatabase (cluster style)")
		return nil
	}
}

// deleteRacServices removes all RAC-related Services, cleaning up network
// endpoints as part of instance teardown routines.
func (r *RacDatabaseReconciler) deleteRacServices(
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	svcType string,
	isOldStyle bool,
) error {
	log := r.Log.WithValues("deleteRacServices", req.NamespacedName)

	if isOldStyle {
		// --- Old style: per-instance/service deletion ---
		for i := 0; i < len(racDatabase.Spec.InstDetails); i++ {
			svcFound, err := raccommon.CheckRacSvc(
				racDatabase,
				svcType,
				racDatabase.Spec.InstDetails[i],
				"",
				r.Client,
			)
			if err != nil {
				return err
			}
			if svcFound != nil {
				log.Info("Deleting RAC Service " + svcFound.Name)
				if err := r.Client.Delete(context.Background(), svcFound); err != nil {
					return client.IgnoreNotFound(err)
				}
			}
		}
	} else {
		// --- New style: cluster-level deletion using consistent naming ---
		cluster := racDatabase.Spec.ClusterDetails
		if cluster == nil {
			log.Error(nil, "ClusterDetails is nil during new style service deletion")
			return fmt.Errorf("ClusterDetails is nil")
		}

		// Handle standard per-node services
		for i := 0; i < cluster.NodeCount; i++ {
			// Use same service name construction as creation
			svcName := raccommon.GetClusterSvcName(racDatabase, cluster, i, svcType)
			svc := &corev1.Service{}
			err := r.Client.Get(context.TODO(), types.NamespacedName{
				Name:      svcName,
				Namespace: racDatabase.Namespace,
			}, svc)
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("Service not found, skipping", "Service.Name", svcName)
					continue
				}
				return err
			}
			log.Info("Deleting RAC Service " + svc.Name)
			if err := r.Client.Delete(context.Background(), svc); err != nil {
				return client.IgnoreNotFound(err)
			}
		}

		// Handle special cluster-wide services (e.g., "scansvc", "scan")
		if svcType == "scansvc" || svcType == "scan" {
			svcName := raccommon.GetClusterSvcName(racDatabase, cluster, 0, svcType)
			svc := &corev1.Service{}
			err := r.Client.Get(context.TODO(), types.NamespacedName{
				Name:      svcName,
				Namespace: racDatabase.Namespace,
			}, svc)
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("Cluster-wide Service not found, skipping", "Service.Name", svcName)
				} else {
					return err
				}
			} else {
				log.Info("Deleting Cluster-wide RAC Service " + svc.Name)
				if err := r.Client.Delete(context.Background(), svc); err != nil {
					return client.IgnoreNotFound(err)
				}
			}
		}
	}

	return nil
}

// cleanupRacInstance coordinates instance-level cleanup such as PVC removal,
// StatefulSet deletion, and ASM reconfiguration when instances are removed.
func (r *RacDatabaseReconciler) cleanupRacInstance(
	req ctrl.Request,
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
	isOldStyle bool,
	oldSpec *racdb.RacDatabaseSpec) (int32, error) {
	log := r.Log.WithValues("cleanupRacInstance", req.NamespacedName)
	var i int32
	var err error
	if oldSpec.ClusterDetails != nil && oldSpec.ClusterDetails.NodeCount > 0 {

		if isOldStyle {
			// Old style: Iterate InstDetails per-node
			if len(racDatabase.Spec.InstDetails) > 0 {
				for i = 0; i < int32(len(racDatabase.Spec.InstDetails)); i++ {
					OraRacSpex := racDatabase.Spec.InstDetails[i]
					if utils.CheckStatusFlag(OraRacSpex.IsDelete) {
						if len(racDatabase.Status.RacNodes) > 0 {
							for _, oraRacStatus := range racDatabase.Status.RacNodes {
								if strings.ToUpper(oraRacStatus.Name) == (strings.ToUpper(OraRacSpex.Name) + "-0") {
									if !utils.CheckStatusFlag(oraRacStatus.NodeDetails.IsDelete) {
										log.Info("Setting RAC status instance " + oraRacStatus.Name + " delete flag true")
										err = r.deleteRACInst(OraRacSpex, req, ctx, racDatabase, isOldStyle)
										if err != nil {
											log.Info("Error occurred RAC instance " + oraRacStatus.Name + " deletion")
											return i, err
										}
										oraRacStatus.NodeDetails.IsDelete = "true"
									}
								}
							}
						}
					}
				}
			}
			return i, nil
		} else {
			cd := racDatabase.Spec.ClusterDetails
			if cd == nil {
				log.Error(nil, "ClusterDetails is nil in cleanup instance")
				return 0, fmt.Errorf("internal error: ClusterDetails is nil in new style cleanup instance")
			}

			// 1. Build sets of previous and current nodes
			oldNodeNames := []string{}
			currentNodeNames := []string{}

			// Assume 'oldSpec' is obtained from annotation or similar
			if oldSpec.ClusterDetails.NodeCount > 0 {
				for idx := 0; idx < oldSpec.ClusterDetails.NodeCount; idx++ {
					oldNodeNames = append(oldNodeNames, fmt.Sprintf("%s%d", oldSpec.ClusterDetails.RacNodeName, idx+1))
				}
			}

			// Current node names
			for idx := 0; idx < cd.NodeCount; idx++ {
				currentNodeNames = append(currentNodeNames, fmt.Sprintf("%s%d", cd.RacNodeName, idx+1))
			}

			// 2. Create a lookup set for current nodes
			currentNodeSet := make(map[string]struct{}, len(currentNodeNames))
			for _, name := range currentNodeNames {
				currentNodeSet[name] = struct{}{}
			}

			// 3. Clean up only nodes in old but not in new
			deletedCount := 0
			for _, name := range oldNodeNames {
				if _, stillExists := currentNodeSet[name]; !stillExists {
					spec := racdb.RacInstDetailSpec{Name: name}
					log.Info("Starting delete RAC instance for node " + name)
					err := r.deleteRACInst(spec, req, ctx, racDatabase, false)
					if err != nil {
						log.Info(fmt.Sprintf("Error occurred during cluster node %s deletion", name))
						return int32(deletedCount), err
					}
					deletedCount++
				}
			}
			return int32(deletedCount), nil
		}
	} else {
		return 0, nil
	}

}

// deleteRACInst drives deletion logic for individual RAC instances, ensuring
// Kubernetes objects and ASM state are cleaned up safely.
func (r *RacDatabaseReconciler) deleteRACInst(
	OraRacSpex racdb.RacInstDetailSpec,
	req ctrl.Request,
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
	isOldStyle bool,
) error {
	log := r.Log.WithValues("deleteRACInst", req.NamespacedName)
	// log.Info("Sleeping for 60 minutes before continuing")
	// time.Sleep(60 * time.Minute)
	if isOldStyle {
		var nodeCount int
		var err error
		var cmName string
		var healthyNode string

		nodeCount, err = raccommon.GetHealthyNodeCounts(racDatabase)
		healthyNode, err = raccommon.GetHealthyNode(racDatabase)
		if err != nil {
			return fmt.Errorf("no healthy node found in the cluster to perform delete node operator. manual intervention required")
		}

		_, endp, err := raccommon.GetDBLsnrEndPoints(racDatabase)
		if err != nil {
			return fmt.Errorf("endpoint generation error in delete block")
		}

		sfSetFound, err := raccommon.CheckSfset(OraRacSpex.Name, racDatabase, r.Client)
		if err == nil && sfSetFound != nil {
			if strings.ToLower(OraRacSpex.IsDelete) != "force" {
				err = raccommon.DelRacNode(sfSetFound.Name+"-0", racDatabase, r.kubeClient, r.kubeConfig, r.Log)
				if err != nil {
					return err
				}
			}
			err = r.Client.Delete(context.Background(), sfSetFound)
			if err != nil {
				return err
			}
		}
		if !utils.CheckStatusFlag(OraRacSpex.IsKeepPVC) {
			err = raccommon.DelRacSwPvc(racDatabase, OraRacSpex, r.Client, r.Log)
			if err != nil {
				return err
			}
		}
		cmName = OraRacSpex.Name + racDatabase.Name + "-cmap"
		configMapFound, err := raccommon.CheckConfigMap(racDatabase, cmName, r.Client)
		if err == nil {
			err = r.Client.Delete(context.Background(), configMapFound)
			if err != nil {
				return err
			}
		}

		svcTypes := []string{"vip", "local", "onssvc", "lsnrsvc"}
		for _, svcType := range svcTypes {
			svcFound, err := raccommon.CheckRacSvc(racDatabase, svcType, OraRacSpex, "", r.Client)
			if err == nil && svcFound != nil {
				err = r.Client.Delete(context.Background(), svcFound)
				if err != nil {
					return err
				}
			}
		}

		if len(OraRacSpex.NodePortSvc) != 0 {
			for index := range OraRacSpex.NodePortSvc {
				svcFound, err := raccommon.CheckRacSvc(racDatabase, "nodeport", OraRacSpex, OraRacSpex.NodePortSvc[index].SvcName, r.Client)
				if err == nil && svcFound != nil {
					err = r.Client.Delete(context.Background(), svcFound)
					if err != nil {
						return err
					}
				}
			}
		}

		// ASM, Listener, and Scan endpoint update logic is the same per node -- no change needed
		if nodeCount == 3 {
			err = raccommon.UpdateAsmCount(racDatabase.Spec.ConfigParams.GridHome, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log)
			if err != nil {
				log.Info("error occurred while updating the asm count")
			} else {
				log.Info("Updated the asm cardinality successfully")
			}
		}
		log.Info("Updating the tcp listener endpoints after node deletion")
		lsnrname := "dblsnr"
		err = raccommon.UpdateTCPPort(racDatabase.Spec.ConfigParams.GridHome, endp, lsnrname, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log)
		if err != nil {
			log.Info("error occurred while updating the listener tcp ports")
		} else {
			log.Info("Updated the tcp listener endpoints successfully")
		}
		log.Info("Updating the scan end points after node deletion")
		err = raccommon.UpdateScanEP(racDatabase.Spec.ConfigParams.GridHome, racDatabase.Spec.ScanSvcName, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log)
		if err != nil {
			log.Info("error occurred while updating the scan end points")
		} else {
			log.Info("Updated scan end points successfully after node deletion")
		}
		log.Info("Updating the cdp after node deletion")
		err = raccommon.UpdateCDP(racDatabase.Spec.ConfigParams.GridHome, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log)
		if err != nil {
			log.Info("error occurred while updating the CDP")
		} else {
			log.Info("Updated cdp successfully after node deletion")
		}
	} else {
		// === New style (cluster, per-cluster naming, no InstDetails) ===
		nodeName := OraRacSpex.Name

		// Derive pod name, e.g. racnode-0-0 for statefulset racnode-0
		podName := nodeName + "-0"

		// Remove Oracle RAC instance from node _before_ deleting StatefulSet
		err := raccommon.DelRacNode(podName, racDatabase, r.kubeClient, r.kubeConfig, r.Log)
		if err != nil {
			return err
		}

		// Now it is safe to delete the StatefulSet
		sfSetFound, err := raccommon.CheckSfset(nodeName, racDatabase, r.Client)
		if err == nil && sfSetFound != nil {
			err = r.Client.Delete(context.Background(), sfSetFound)
			if err != nil {
				return err
			}
		}

		// 2. Delete ConfigMap by naming convention
		cmName := nodeName + racDatabase.Name + "-cmap"
		configMapFound, err := raccommon.CheckConfigMap(racDatabase, cmName, r.Client)
		if err == nil {
			err = r.Client.Delete(context.Background(), configMapFound)
			if err != nil {
				return err
			}
		}

		// 3. Delete VIP/LOCAL/ONSSVC/LSNR SVCs, also NodePort if naming applies
		svcTypes := []string{"vip", "local", "onssvc", "lsnrsvc", "nodeport"}
		nodeIndex := extractNodeIndexFromName(nodeName) // Ensure nodeName like "racnode-1"
		for _, svcType := range svcTypes {
			svcName := raccommon.GetClusterSvcName(racDatabase, racDatabase.Spec.ClusterDetails, nodeIndex, svcType)
			svcFound, err := raccommon.CheckRacSvcForCluster(
				racDatabase, racDatabase.Spec.ClusterDetails, nodeIndex, svcType, svcName, r.Client)
			if err == nil && svcFound != nil {
				err = r.Client.Delete(context.Background(), svcFound)
				if err != nil {
					return err
				}
				log.Info("Deleted Svc", "svcName", svcName)
			}
		}
		_, endp, err := raccommon.GetDBLsnrEndPointsForCluster(racDatabase)
		if err != nil {
			return fmt.Errorf("endpoint generation error in delete block")
		}
		// nodeCount, err := raccommon.GetHealthyNodeCounts(racDatabase)
		healthyNode, err := raccommon.GetHealthyNode(racDatabase)
		// ASM, TCP Listener, and Scan endpoint logic is usually cluster/global
		if racDatabase.Spec.ClusterDetails != nil && racDatabase.Spec.ClusterDetails.NodeCount == 3 {
			err := raccommon.UpdateAsmCount(
				racDatabase.Spec.ConfigParams.GridHome,
				healthyNode,
				racDatabase,
				r.kubeClient, r.kubeConfig, r.Log,
			)
			if err != nil {
				log.Info("error occurred while updating the asm count")
			} else {
				log.Info("Updated the asm cardinality successfully")
			}
		}

		if err != nil {
			return fmt.Errorf("no healthy node found in the cluster to perform delete node operator. manual intervention required")
		}

		log.Info("Updating the tcp listener endpoints after node deletion")
		lsnrname := "dblsnr"
		err = raccommon.UpdateTCPPort(
			racDatabase.Spec.ConfigParams.GridHome,
			endp,
			lsnrname,
			healthyNode,
			racDatabase,
			r.kubeClient, r.kubeConfig, r.Log,
		)
		if err != nil {
			log.Info("error occurred while updating the listener tcp ports")
		} else {
			log.Info("Updated the tcp listener endpoints successfully")
		}

		log.Info("Updating the scan end points after node deletion")
		log.Info(
			"Updating the scan end points after node deletion",
			"GridHome", racDatabase.Spec.ConfigParams.GridHome,
			"ScanSvcName", racDatabase.Spec.ScanSvcName,
			"HealthyNode", healthyNode,
			"RacDatabaseName", racDatabase.Name,
			"Namespace", racDatabase.Namespace,
		)

		err = raccommon.UpdateScanEP(
			racDatabase.Spec.ConfigParams.GridHome,
			racDatabase.Spec.ScanSvcName,
			healthyNode,
			racDatabase,
			r.kubeClient, r.kubeConfig, r.Log,
		)
		if err != nil {
			log.Info("error occurred while updating the scan end points")
		} else {
			log.Info("Updated scan end points successfully after node deletion")
		}
		log.Info("Updating the cdp after node deletion")
		err = raccommon.UpdateCDP(racDatabase.Spec.ConfigParams.GridHome, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log)
		if err != nil {
			log.Info("error occurred while updating the CDP")
		} else {
			log.Info("Updated cdp successfully after node deletion")
		}
	}
	log.Info("Successfully cleaned up RacInstance")
	return nil
}

// extractNodeIndexFromName parses the numeric suffix from a RAC node name and
// converts it to a zero-based index, accommodating optional pod ordinal suffixes.
func extractNodeIndexFromName(nodeName string) int {
	// Remove a "-0" pod ordinal suffix if present
	base := nodeName
	if strings.HasSuffix(nodeName, "-0") {
		base = strings.TrimSuffix(nodeName, "-0")
	}
	// Find the last group of digits in the base name
	re := regexp.MustCompile(`(\d+)$`)
	match := re.FindStringSubmatch(base)
	if len(match) > 1 {
		if idx, err := strconv.Atoi(match[1]); err == nil {
			return idx - 1
		}
	}
	return 0
}

// SetupWithManager sets up the controller with the Manager.
// SetupWithManager wires the reconciler into the controller manager so it
// watches RAC resources and their owned objects with configured concurrency.
func (r *RacDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&racdb.RacDatabase{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Pod{}).
		Owns(&appsv1.StatefulSet{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

const oldRacSpecAnnotation = "racdatabases.database.oracle.com/old-spec"

// GetOldSpec retrieves the old spec from annotations.
// Returns nil, nil if the annotation does not exist.
// GetOldSpec reads the previous RAC spec from annotations so reconcile can
// detect changes across iterations. Missing annotations yield a nil result.
func (r *RacDatabaseReconciler) GetOldSpec(racDatabase *racdb.RacDatabase) (*racdb.RacDatabaseSpec, error) {
	// Check if annotations exist
	annotations := racDatabase.GetAnnotations()
	if annotations == nil {
		r.Log.Info("No annotations found on RacDatabase")
		return nil, nil
	}

	// Retrieve the specific annotation
	val, ok := annotations[oldRacSpecAnnotation]
	if !ok {
		r.Log.Info("Old spec annotation not found")
		return nil, nil
	}

	// Unmarshal the old spec JSON string
	specBytes := []byte(val)
	var oldSpec racdb.RacDatabaseSpec
	if err := json.Unmarshal(specBytes, &oldSpec); err != nil {
		r.Log.Error(err, "Failed to unmarshal old spec from annotation")
		return nil, fmt.Errorf("failed to unmarshal old spec from annotation: %w", err)
	}

	// r.Log.Info("Successfully retrieved old spec from annotation", "spec", oldSpec)
	return &oldSpec, nil
}

// SetCurrentSpec stores the current spec as an annotation with retry logic, updating only if the annotation value has changed.
// SetCurrentSpec persists the current spec into annotations with conflict
// retries, enabling future reconciles to detect spec changes accurately.
func (r *RacDatabaseReconciler) SetCurrentSpecAndObservedGeneration(
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
	req ctrl.Request,
) error {

	var cName, fName string
	if racDatabase.Spec.ConfigParams != nil &&
		racDatabase.Spec.ConfigParams.GridResponseFile != nil {

		cName = racDatabase.Spec.ConfigParams.GridResponseFile.ConfigMapName
		fName = racDatabase.Spec.ConfigParams.GridResponseFile.Name
	}

	// Normalize DG defaults
	if err := setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(
		racDatabase, r.Client, cName, fName,
	); err != nil {
		r.Log.Info("Failed to set disk group defaults")
		return err
	}

	// Serialize spec
	currentSpecData, err := json.Marshal(racDatabase.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal current spec: %w", err)
	}
	currentSpecStr := string(currentSpecData)

	// Ensure annotations map
	if racDatabase.Annotations == nil {
		racDatabase.Annotations = map[string]string{}
	}

	// Detect change
	existingSpecStr := racDatabase.Annotations[oldRacSpecAnnotation]
	specChanged := existingSpecStr != currentSpecStr

	// -------------------------------
	// STEP 1: PATCH METADATA (ONLY)
	// -------------------------------
	if specChanged {
		original := racDatabase.DeepCopy()
		racDatabase.Annotations[oldRacSpecAnnotation] = currentSpecStr

		if err := r.Patch(ctx, racDatabase, client.MergeFrom(original)); err != nil {
			return err
		}
	}

	// -------------------------------
	// STEP 2: RE-FETCH OBJECT
	// -------------------------------
	if err := r.Get(ctx, req.NamespacedName, racDatabase); err != nil {
		return err
	}

	// -------------------------------
	// STEP 3: UPDATE STATUS
	// -------------------------------
	if racDatabase.Status.ObservedGeneration != racDatabase.Generation {
		original := racDatabase.DeepCopy()
		racDatabase.Status.ObservedGeneration = racDatabase.Generation

		if err := r.Status().Patch(ctx, racDatabase, client.MergeFrom(original)); err != nil {
			return err
		}
	}

	r.Log.Info("RAC spec annotation and observedGeneration updated successfully")
	return nil
}

// updateRacNodeStatusForCluster updates the RAC node status for a cluster with retry logic.
// It synchronizes node status changes from the working RacDatabase object to the API server,
// handling resource version conflicts and retrying up to maxRetries times with exponential backoff.
//
// Parameters:
//   - racDatabase: The RacDatabase instance containing updated node status information
//   - ctx: The context for API operations and cancellation
//   - req: The reconciliation request containing the namespaced name of the resource
//   - clusterSpec: The cluster specification details for the RAC database
//   - nodeIndex: The index of the node being updated within the cluster
//   - state: The new state to set for the RAC node
//
// The function performs the following steps:
//  1. Calls the cluster-style status update logic to prepare status data
//  2. Attempts to fetch the latest instance from the Kubernetes API
//  3. Syncs the RacNodes status from the working object to the latest instance
//  4. Updates the resource version and persists changes to the API server
//  5. Retries on conflicts (up to 5 times) with a 2-second delay between attempts
//  6. Logs errors and completion status for troubleshooting
func (r *RacDatabaseReconciler) updateRacNodeStatusForCluster(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	state string,
) {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	var lastErr error
	var failedUpdate bool

	// Call the cluster-style status update logic
	raccommon.UpdateRacNodeStatusDataForCluster(
		racDatabase, ctx, req, clusterSpec, nodeIndex, state,
		r.kubeClient, r.kubeConfig, r.Log, r.Client,
	)

	// Retry update with resource version logic, same as old style
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch the latest instance from the API
		latestInstance := &racdb.RacDatabase{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of RAC instance")
			lastErr = err
			continue // Continue to retry
		}
		// Sync RACNodes from working object
		latestInstance.Status.RacNodes = racDatabase.Status.RacNodes

		// If you need further merging/ensuring logic for cluster style, add here

		racDatabase.ResourceVersion = latestInstance.ResourceVersion
		err = r.Status().Update(ctx, racDatabase)
		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected in updateRacNodeStatusForCluster, retrying...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				failedUpdate = true
				continue // Retry
			}
			r.Log.Error(err, "Failed to update the RAC instance (cluster)")
			lastErr = err
			failedUpdate = true
			continue // Continue to retry
		}
		r.Log.Info("RAC Object updated with updateRacNodeStatusForCluster")
		failedUpdate = false
		break // Break if update succeeded
	}
	// Log if all retries exhausted
	if failedUpdate {
		r.Log.Info("Failed to update RAC instance (cluster) after 5 attempts", "lastErr", lastErr)
	}
}
func (r *RacDatabaseReconciler) waitForPodReady(ctx context.Context, podName string, namespace string, timeout time.Duration) error {
	t := time.After(timeout)
	for {
		select {
		case <-t:
			return fmt.Errorf("timed out waiting for pod %s to become ready", podName)
		default:
			pod := &corev1.Pod{}
			err := r.Get(ctx, client.ObjectKey{Name: podName, Namespace: namespace}, pod)
			if err != nil {
				// Pod might not exist yet - continue
				time.Sleep(5 * time.Second)
				continue
			}
			// Check for Ready condition
			ready := false
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			if ready {
				return nil
			}
			time.Sleep(5 * time.Second)
		}
	}
}
func (r *RacDatabaseReconciler) updateStatusWithRetry(
	ctx context.Context,
	req ctrl.Request,
	apply func(latest *racdb.RacDatabase),
) error {

	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
	)

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {

		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(ctx, req.NamespacedName, latest); err != nil {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}

		// Apply desired mutation on latest object
		apply(latest)

		if err := r.Status().Update(ctx, latest); err == nil {
			return nil
		} else if apierrors.IsConflict(err) {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		} else {
			return err
		}
	}

	return fmt.Errorf("status update failed after retries: %w", lastErr)
}
func (r *RacDatabaseReconciler) updateStatusNoGetRetry(
	ctx context.Context,
	obj *racdb.RacDatabase,
) error {

	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
	)

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {

		err := r.Status().Update(ctx, obj)
		if err == nil {
			return nil
		}

		if apierrors.IsConflict(err) {
			lastErr = err

			latest := &racdb.RacDatabase{}
			if getErr := r.Client.Get(
				ctx,
				types.NamespacedName{
					Name:      obj.Name,
					Namespace: obj.Namespace,
				},
				latest,
			); getErr != nil {
				return getErr
			}

			// --------------------------------------------------
			// Preserve ASM disk groups from latest object
			// --------------------------------------------------
			obj.Status.AsmDiskGroups = latest.Status.AsmDiskGroups

			// Refresh resourceVersion only
			obj.ResourceVersion = latest.ResourceVersion

			time.Sleep(retryDelay)
			continue
		}

		return err
	}

	return fmt.Errorf("status update failed after retries: %w", lastErr)
}
