// Copyright (c) 2019-2020, Oracle and/or its affiliates. All rights reserved.
//
// Support routines for managing TimesTen upgrades

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// effectively end an upgrade operation by setting zero-values
func resetUpgradeVars(instance *timestenv2.TimesTenClassic) error {

	instance.Status.ClassicUpgradeStatus.UpgradeState = ""
	instance.Status.ClassicUpgradeStatus.UpgradeStartTime = 0
	instance.Status.ClassicUpgradeStatus.PrevUpgradeState = ""
	instance.Status.ClassicUpgradeStatus.ActiveStatus = ""
	instance.Status.ClassicUpgradeStatus.ActiveStartTime = 0
	instance.Status.ClassicUpgradeStatus.StandbyStatus = ""
	instance.Status.ClassicUpgradeStatus.StandbyStartTime = 0
	instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch = 0
	instance.Status.ClassicUpgradeStatus.ImageUpdatePending = false
	return nil
}

// initiates standby upgrade; deletes the standby pod and sets upgrade state vars
func initUpgrade(ctx context.Context, kind string, instance *timestenv2.TimesTenClassic, client client.Client) error {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("initUpgrade entered")
	defer reqLogger.V(1).Info("initUpgrade ends")

	// kind is either ACTIVE or STANDBY
	kind = strings.ToUpper(kind)

	// make sure TTC object is in the 'Normal' state.

	if instance.Status.HighLevelState != "Normal" {
		errMsg := fmt.Sprintf("Cannot initiate the upgrade, HL state not 'Normal' (HL state=%v)", instance.Status.HighLevelState)
		reqLogger.V(1).Info("initUpgrade: " + errMsg)
		logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
		return errors.New(errMsg)

	} else {

		// upgrade only if ImageUpgradeStrategy is either not specified in manifest, empty or "auto"

		if instance.Spec.TTSpec.ImageUpgradeStrategy == nil ||
			*instance.Spec.TTSpec.ImageUpgradeStrategy == "Auto" ||
			*instance.Spec.TTSpec.ImageUpgradeStrategy == "" {

			reqLogger.V(1).Info("initUpgrade: proceed with automatic upgrade")

			var otherPodNo int
			var podNo int
			for podNo, _ = range instance.Status.PodStatus {
				if strings.ToUpper(instance.Status.PodStatus[podNo].IntendedState) == kind {
					reqLogger.V(1).Info(fmt.Sprintf("initUpgrade: starting upgrade of the %s on POD %d", kind, podNo))
					otherPodNo = 0
					if podNo == 0 {
						otherPodNo = 1
					}
					break
				}
			}
			podName := instance.Status.PodStatus[podNo].Name

			// if we haven't started the upgrade yet. verify that both the active and standby TimesTen instances
			// are running the same TimesTen release

			if instance.Status.ClassicUpgradeStatus.StandbyStatus == "" &&
				instance.Status.PodStatus[podNo].TimesTenStatus.Release != instance.Status.PodStatus[otherPodNo].TimesTenStatus.Release {
				errMsg := fmt.Sprintf("TimesTen version mismatch, %v=%v %v=%v, upgrade cancelled.", podNo,
					instance.Status.PodStatus[podNo].TimesTenStatus.Release, otherPodNo,
					instance.Status.PodStatus[otherPodNo].TimesTenStatus.Release)
				reqLogger.V(1).Info("initUpgrade: " + errMsg)
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				return errors.New(errMsg)
			}

			pod := &corev1.Pod{}
			err := client.Get(ctx, types.NamespacedName{Name: podName, Namespace: instance.Namespace}, pod)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				errMsg := fmt.Sprintf("Cannot fetch status of POD %v, upgrade aborted : %v", podName, errorMsg)
				reqLogger.Info("initUpgrade: " + errMsg)
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				resetUpgradeVars(instance)
				return errors.New(errMsg)
			} else {
				err = client.Delete(ctx, pod)
				if err != nil {

					//Checks if the error was because of lack of permission, if not, return the original message
					var authErrorMessage, isPermissionsProblem = verifyUnauthorizedError(err.Error())
					errMsg := fmt.Sprintf("Cannot delete %v POD %v : %v", kind, podName, authErrorMessage)
					if isPermissionsProblem {
						logTTEvent(ctx, client, instance, "FailedUpgrade", errMsg, true)
					} else {
						reqLogger.V(1).Info("initUpgrade: " + errMsg)
						// set ImageUpdatePending so we'll continue to retry until the pod delete operation succeeds
						instance.Status.ClassicUpgradeStatus.ImageUpdatePending = true
						reqLogger.Info("initUpgrade: setting ImageUpdatePending to true")
						logTTEvent(ctx, client, instance, "UpgradeError", errMsg, false)
					}
					return errors.New(errMsg)
				}

				reqLogger.V(1).Info("initUpgrade: deleted " + strings.ToUpper(kind) + " pod " + podName)

				// keep track of start times

				if instance.Status.ClassicUpgradeStatus.UpgradeStartTime == 0 {
					instance.Status.ClassicUpgradeStatus.UpgradeStartTime = time.Now().Unix()
					reqLogger.V(1).Info(fmt.Sprintf("initUpgrade: UpgradeStartTime set to %v", instance.Status.ClassicUpgradeStatus.UpgradeStartTime))
				}

				if kind == "ACTIVE" {
					instance.Status.ClassicUpgradeStatus.ActiveStartTime = time.Now().Unix()

					if instance.Status.ClassicUpgradeStatus.StandbyStatus == "" {
						// we should never have an empty StandbyStatus at ACTIVE upgrade invocation; go will choke on nil vals
						// in our SM map, so set to unknown
						instance.Status.ClassicUpgradeStatus.StandbyStatus = "unknown"
						reqLogger.V(1).Info("initUpgrade: WARN: StandbyStatus was empty, set to unknown")
					}

					instance.Status.ClassicUpgradeStatus.ActiveStatus = "deleteActive"
					reqLogger.V(1).Info("initUpgrade: ActiveStatus set to deleteActive")

					instance.Status.ClassicUpgradeStatus.UpgradeState = autoUpgradeStateMachine["UpgradingActive"][instance.Status.ClassicUpgradeStatus.ActiveStatus][instance.Status.ClassicUpgradeStatus.StandbyStatus]

				} else {

					instance.Status.ClassicUpgradeStatus.StandbyStartTime = time.Now().Unix()

					instance.Status.ClassicUpgradeStatus.ActiveStatus = "waiting"
					reqLogger.V(1).Info("initUpgrade: ActiveStatus set to waiting")

					instance.Status.ClassicUpgradeStatus.StandbyStatus = "deleteStandby"
					reqLogger.V(1).Info("initUpgrade: StandbyStatus set to deleteStandby")

					instance.Status.ClassicUpgradeStatus.UpgradeState = autoUpgradeStateMachine["UpgradingStandby"][instance.Status.ClassicUpgradeStatus.ActiveStatus][instance.Status.ClassicUpgradeStatus.StandbyStatus]

				}

				reqLogger.V(1).Info("initUpgrade: UpgradeState set to " + instance.Status.ClassicUpgradeStatus.UpgradeState)

				msg := "Deleted " + strings.ToLower(kind) + " pod " + podName + " during upgrade"
				logTTEvent(ctx, client, instance, "Upgrade", msg, false)

				// TODO: VERIFY that the standby successfully terminated

			}

		} else {
			reqLogger.V(1).Info("initUpgrade: ImageUpgradeStrategy=" + *instance.Spec.TTSpec.ImageUpgradeStrategy +
				"; set to 'Auto' for automatic upgrade")
		}

	}

	return nil

}

// monitors the progress of the ACTIVE upgrade
func checkUpgradeActive(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo, newHighLevelState string) (error, string) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("checkUpgradeActive entered")
	defer reqLogger.V(1).Info("checkUpgradeActive ends")

	var timeSinceLastState int64
	var newAutoUpgradeState string
	var activeHLState string
	var activePodNo int
	var standbyHLState string
	var standbyPodNo int
	var upgradeDownPodTimeout int = 600

	if instance.Spec.TTSpec.UpgradeDownPodTimeout != nil {
		upgradeDownPodTimeout = *instance.Spec.TTSpec.UpgradeDownPodTimeout
		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: upgradeDownPodTimeout=%v", upgradeDownPodTimeout))
	}

	reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: UpgradeState=%v PrevUpgradeState=%v",
		instance.Status.ClassicUpgradeStatus.UpgradeState, instance.Status.ClassicUpgradeStatus.PrevUpgradeState))
	reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: ActiveStatus=%v StandbyStatus=%v",
		instance.Status.ClassicUpgradeStatus.ActiveStatus, instance.Status.ClassicUpgradeStatus.StandbyStatus))

	for podNo, podStatus := range instance.Status.PodStatus {
		if strings.ToUpper(podStatus.IntendedState) == "ACTIVE" {
			activePodNo = podNo
			if podNo == 0 {
				standbyPodNo = 1
			} else {
				standbyPodNo = 0
			}
			activeHLState = instance.Status.PodStatus[activePodNo].HighLevelState
			standbyHLState = instance.Status.PodStatus[standbyPodNo].HighLevelState
			reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: ACTIVE pod=%d; HL state is %s", activePodNo, activeHLState))
			reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: STANDBY pod=%d; HL state is %s", standbyPodNo, standbyHLState))
			break
		}
	}

	if instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch != 0 {
		timeSinceLastState = time.Now().Unix() - instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch
		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: timeSinceLastState was %v secs", timeSinceLastState))

		// if the agent running in the pod hasn't come up (state=Down) within upgradeDownPodTimeout secs,
		// set standbyHLState to UpgradeFailed, which will become ManualInterventionRequired
		// upgradeDownPodTimeout=0 means never timeout

		if standbyHLState == "Down" {
			if int64(upgradeDownPodTimeout) == 0 {
				reqLogger.Info(fmt.Sprintf("checkUpgradeActive: checkUpgradeActive: standbyHLState has been down for %v secs, upgradeDownPodTimeout=0 [never]",
					timeSinceLastState))
			} else {
				if timeSinceLastState > int64(upgradeDownPodTimeout) {
					standbyHLState = "UpgradeFailed"
					reqLogger.Info(fmt.Sprintf("checkUpgradeActive: standbyHLState has been down for %v secs, setting standbyHLState to %v",
						timeSinceLastState, standbyHLState))
				} else {
					reqLogger.Info(fmt.Sprintf("checkUpgradeActive: checkUpgradeActive: standbyHLState has been down for %v secs, timeout in %v secs",
						timeSinceLastState, int64(upgradeDownPodTimeout)-timeSinceLastState))
				}
			}
		}
	}

	if instance.Status.HighLevelState != newHighLevelState {

		// there's a new HL state, let's check the progress of the standby upgrade

		if instance.Status.ClassicUpgradeStatus.ActiveStatus == "deleteActive" && activeHLState != "Healthy" {
			// we just deleted the active confirmed by the pod state not Healthy
			// the active upgrade is underway
			instance.Status.ClassicUpgradeStatus.ActiveStatus = "processing"
			reqLogger.V(1).Info("checkUpgradeActive: ActiveStatus set to " + instance.Status.ClassicUpgradeStatus.ActiveStatus +
				"; newHighLevelState is " + newHighLevelState)
		} else {
			reqLogger.V(1).Info("checkUpgradeActive: ActiveStatus=" + instance.Status.ClassicUpgradeStatus.ActiveStatus + " (unchanged)")
		}

		// wait for the recently deleted pod to change state (HL state should be 'Normal' until timeout)

		if strings.HasPrefix(instance.Status.ClassicUpgradeStatus.StandbyStatus, "delete") ||
			strings.HasPrefix(instance.Status.ClassicUpgradeStatus.ActiveStatus, "delete") &&
				newHighLevelState == "Normal" {

			// states expected are : ActiveDown->FAILOVER->ActiveTakeover->StandbyDown

			//TODO: how long are we willing to wait for failover?

			if instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch != 0 {
				timeSinceLastState = time.Now().Unix() - instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch
				reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: timeSinceLastState was %v secs", timeSinceLastState))
			}

		} else {

			if newHighLevelState != instance.Status.PrevHighLevelState {

				// we have a new HL state
				instance.Status.ClassicUpgradeStatus.PrevUpgradeState = instance.Status.ClassicUpgradeStatus.UpgradeState
				reqLogger.V(1).Info("checkUpgradeActive: PrevUpgradeState set to " + instance.Status.ClassicUpgradeStatus.PrevUpgradeState)
				instance.Status.ClassicUpgradeStatus.UpgradeState = "UpgradingActive"
				reqLogger.V(1).Info("checkUpgradeActive: UpgradeState set to " + instance.Status.ClassicUpgradeStatus.UpgradeState)
				timeSinceLastState = time.Now().Unix() - instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch
				reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: timeSinceLastState was %v secs", timeSinceLastState))
				instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch = time.Now().Unix()

			} else {
				// awaiting state change
			}

		}

	}

	reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: HighLevelState=%v newHighLevelState=%v", instance.Status.HighLevelState, newHighLevelState))

	// upgrade can be verified if the previous HL state was StandbyDown|StandbyStarting|StandbyCatchup and the new state is Normal

	if (instance.Status.HighLevelState == "StandbyDown" ||
		instance.Status.HighLevelState == "StandbyStarting" ||
		instance.Status.HighLevelState == "StandbyCatchup") &&
		newHighLevelState == "Normal" {

		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: POD 0 RUNNING TIMESTEN=%v", instance.Status.PodStatus[0].TimesTenStatus.Release))
		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: POD 1 RUNNING TIMESTEN=%v", instance.Status.PodStatus[1].TimesTenStatus.Release))

		// verify that rows inserted on the active are actually replicated to the standby

		err := verifyASReplication(ctx, instance, activePodNo, standbyPodNo, client, tts)
		if err != nil {
			instance.Status.ClassicUpgradeStatus.ActiveStatus = "failed"
			reqLogger.V(1).Info("checkUpgradeActive: ActiveStatus set to " + instance.Status.ClassicUpgradeStatus.ActiveStatus)
			logTTEvent(ctx, client, instance, "Error", err.Error(), true)
			return err, "ManualInterventionRequired"
		} else {
			// upgradeComplete
			instance.Status.ClassicUpgradeStatus.ActiveStatus = "success"
			reqLogger.V(1).Info("checkUpgradeActive: ActiveStatus set to " + instance.Status.ClassicUpgradeStatus.ActiveStatus)
		}
	}

	newAutoUpgradeState = autoUpgradeStateMachine[instance.Status.ClassicUpgradeStatus.UpgradeState][instance.Status.ClassicUpgradeStatus.ActiveStatus][instance.Status.ClassicUpgradeStatus.StandbyStatus]

	if newAutoUpgradeState != instance.Status.ClassicUpgradeStatus.UpgradeState {
		instance.Status.ClassicUpgradeStatus.UpgradeState = newAutoUpgradeState
		reqLogger.V(1).Info("checkUpgradeActive: UpgradeState set to " + instance.Status.ClassicUpgradeStatus.UpgradeState)
	}

	if instance.Status.ClassicUpgradeStatus.UpgradeState == "Complete" {
		elapsedTimeActive := time.Now().Unix() - instance.Status.ClassicUpgradeStatus.ActiveStartTime
		reqLogger.Info(fmt.Sprintf("checkUpgradeActive: active upgrade completed in %v secs", elapsedTimeActive))
	}

	reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeActive: UpgradeState=%v ActiveStatus=%v StandbyStatus=%v", instance.Status.ClassicUpgradeStatus.UpgradeState,
		instance.Status.ClassicUpgradeStatus.ActiveStatus, instance.Status.ClassicUpgradeStatus.StandbyStatus))

	return nil, newAutoUpgradeState

}

// monitors the progress of the STANDBY upgrade
func checkUpgradeStandby(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo, newHighLevelState string) (error, string) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("checkUpgradeStandby entered")
	defer reqLogger.V(1).Info("checkUpgradeStandby ends")

	var timeSinceLastState int64
	var newAutoUpgradeState string
	var standbyHLState string
	var standbyPodNo int
	var activeHLState string
	var activePodNo int
	var upgradeDownPodTimeout int = 600

	reqLogger.V(1).Info("checkUpgradeStandby newHighLevelState=" + newHighLevelState)

	if instance.Spec.TTSpec.UpgradeDownPodTimeout != nil {
		upgradeDownPodTimeout = *instance.Spec.TTSpec.UpgradeDownPodTimeout
		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: upgradeDownPodTimeout=%v", upgradeDownPodTimeout))
	}

	reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: UpgradeState=%v PrevUpgradeState=%v",
		instance.Status.ClassicUpgradeStatus.UpgradeState, instance.Status.ClassicUpgradeStatus.PrevUpgradeState))
	reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: ActiveStatus=%v StandbyStatus=%v",
		instance.Status.ClassicUpgradeStatus.ActiveStatus, instance.Status.ClassicUpgradeStatus.StandbyStatus))

	for podNo, podStatus := range instance.Status.PodStatus {
		if strings.ToUpper(podStatus.IntendedState) == "ACTIVE" {
			activePodNo = podNo
			if podNo == 0 {
				standbyPodNo = 1
			}
			activeHLState = instance.Status.PodStatus[activePodNo].HighLevelState
			standbyHLState = instance.Status.PodStatus[standbyPodNo].HighLevelState
			reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: ACTIVE pod=%d; HL state is %s", activePodNo, activeHLState))
			reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: STANDBY pod=%d; HL state is %s", standbyPodNo, standbyHLState))
			break
		}
	}

	if instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch != 0 {
		timeSinceLastState = time.Now().Unix() - instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch
		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: timeSinceLastState was %v secs", timeSinceLastState))

		// if the agent running in the pod hasn't come up (state=Down) within upgradeDownPodTimeout secs,
		// set standbyHLState to UpgradeFailed, which will become ManualInterventionRequired
		// upgradeDownPodTimeout=0 means never timeout

		if standbyHLState == "Down" {
			if int64(upgradeDownPodTimeout) == 0 {
				reqLogger.Info(fmt.Sprintf("checkUpgradeStandby: standbyHLState has been down for %v secs, upgradeDownPodTimeout=0 [never]",
					timeSinceLastState))
			} else {
				if timeSinceLastState > int64(upgradeDownPodTimeout) {
					standbyHLState = "UpgradeFailed"
					reqLogger.Info(fmt.Sprintf("checkUpgradeStandby: standbyHLState has been down for %v secs, setting standbyHLState to %v",
						timeSinceLastState, standbyHLState))
				} else {
					reqLogger.Info(fmt.Sprintf("checkUpgradeStandby: standbyHLState has been down for %v secs, timeout in %v secs",
						timeSinceLastState, int64(upgradeDownPodTimeout)-timeSinceLastState))
				}
			}
		}
	}

	if standbyHLState == "UpgradeFailed" {

		// mark upgrade HL state as failed and we should get ManualInterventionRequired
		instance.Status.ClassicUpgradeStatus.StandbyStatus = "failed"

		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: standby upgrade failed, set StandbyStatus to %v",
			instance.Status.ClassicUpgradeStatus.StandbyStatus))

		newAutoUpgradeState = autoUpgradeStateMachine[instance.Status.ClassicUpgradeStatus.UpgradeState][instance.Status.ClassicUpgradeStatus.ActiveStatus][instance.Status.ClassicUpgradeStatus.StandbyStatus]

		if instance.Status.ClassicUpgradeStatus.UpgradeState != newAutoUpgradeState {
			reqLogger.V(1).Info("checkUpgradeStandby: UpgradeState set to " + instance.Status.ClassicUpgradeStatus.UpgradeState)
		} else {
			reqLogger.V(1).Info("checkUpgradeStandby: UpgradeState=" + newAutoUpgradeState + " (unchanged)")
		}

		err := errors.New("standby upgrade unsuccessful")
		return err, newAutoUpgradeState

	}

	if instance.Status.HighLevelState != newHighLevelState {
		// there's a new HL state, let's check the progress of the standby upgrade

		if instance.Status.ClassicUpgradeStatus.StandbyStatus == "success" {
			if instance.Status.ClassicUpgradeStatus.ActiveStatus == "deleteActive" {
				// standby done, active pod has been deleted but has not failed over yet
				reqLogger.V(1).Info("checkUpgradeStandby: standby upgraded; waiting for active failover")
			} else {
				reqLogger.V(1).Info("checkUpgradeStandby: standby upgraded; active status not deleteActive (ActiveStatus=" +
					instance.Status.ClassicUpgradeStatus.ActiveStatus + ")")
			}
		} else if instance.Status.ClassicUpgradeStatus.StandbyStatus == "deleteStandby" && standbyHLState != "Healthy" {
			// we just deleted the standby confirmed by the pod state not Healthy
			// the standby upgrade is underway
			instance.Status.ClassicUpgradeStatus.StandbyStatus = "processing"
			reqLogger.V(1).Info("checkUpgradeStandby: StandbyStatus set to " + instance.Status.ClassicUpgradeStatus.StandbyStatus +
				"; newHighLevelState is " + newHighLevelState)
		} else {
			reqLogger.V(1).Info("checkUpgradeStandby: StandbyStatus=" + instance.Status.ClassicUpgradeStatus.StandbyStatus + " (unchanged)")
		}

		// wait for the recently deleted pod to change state (HL state should be 'Normal' until timeout)

		if strings.HasPrefix(instance.Status.ClassicUpgradeStatus.StandbyStatus, "delete") ||
			strings.HasPrefix(instance.Status.ClassicUpgradeStatus.ActiveStatus, "delete") &&
				newHighLevelState == "Normal" {

			// we're waiting for the standby to failover
			//TODO: how long are we willing to wait for failover?

		} else {

			if newHighLevelState != instance.Status.PrevHighLevelState {

				// we have a new HL state

				if instance.Status.ClassicUpgradeStatus.PrevUpgradeState != instance.Status.ClassicUpgradeStatus.UpgradeState {

					// we're exiting the deleteStandby state; new pod coming up

					instance.Status.ClassicUpgradeStatus.PrevUpgradeState = instance.Status.ClassicUpgradeStatus.UpgradeState
					reqLogger.V(1).Info("checkUpgradeStandby: PrevUpgradeState set to " + instance.Status.ClassicUpgradeStatus.PrevUpgradeState)

					instance.Status.ClassicUpgradeStatus.UpgradeState = "UpgradingStandby"
					reqLogger.V(1).Info("checkUpgradeStandby: UpgradeState set to " + instance.Status.ClassicUpgradeStatus.UpgradeState)

					if instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch != 0 {
						timeSinceLastState = time.Now().Unix() - instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch
						reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: timeSinceLastState was %v secs", timeSinceLastState))
					}
					instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch = time.Now().Unix()

				}

			} else {
				// no state change yet
				if instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch != 0 {
					timeSinceLastState = time.Now().Unix() - instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch
					reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: timeSinceLastState is %v secs", timeSinceLastState))
				}
			}

		}

	}

	// upgrade can be verified if the previous HL state was StandbyDown|StandbyStarting|StandbyCatchup and the new state is Normal

	if (instance.Status.HighLevelState == "StandbyDown" ||
		instance.Status.HighLevelState == "StandbyStarting" ||
		instance.Status.HighLevelState == "StandbyCatchup") &&
		newHighLevelState == "Normal" {

		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: POD 0 RUNNING TIMESTEN=%v", instance.Status.PodStatus[0].TimesTenStatus.Release))
		reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: POD 1 RUNNING TIMESTEN=%v", instance.Status.PodStatus[1].TimesTenStatus.Release))

		// verify that rows inserted on the active are actually replicated to the standby

		err := verifyASReplication(ctx, instance, activePodNo, standbyPodNo, client, tts)
		if err != nil {
			instance.Status.ClassicUpgradeStatus.StandbyStatus = "failed"
			reqLogger.V(1).Info("checkUpgradeStandby: StandbyStatus set to " + instance.Status.ClassicUpgradeStatus.StandbyStatus)
			logTTEvent(ctx, client, instance, "UpgradeError", err.Error(), true)
			return err, "ManualInterventionRequired"
		} else {
			instance.Status.ClassicUpgradeStatus.StandbyStatus = "success"
			reqLogger.V(1).Info("checkUpgradeStandby: StandbyStatus set to " + instance.Status.ClassicUpgradeStatus.StandbyStatus)
			elapsedTimeStandby := time.Now().Unix() - instance.Status.ClassicUpgradeStatus.StandbyStartTime
			reqLogger.Info(fmt.Sprintf("checkUpgradeStandby: standby upgrade completed in %v secs", elapsedTimeStandby))
		}

	}

	reqLogger.V(1).Info(fmt.Sprintf("checkUpgradeStandby: UpgradeState=%v ActiveStatus=%v StandbyStatus=%v", instance.Status.ClassicUpgradeStatus.UpgradeState,
		instance.Status.ClassicUpgradeStatus.ActiveStatus, instance.Status.ClassicUpgradeStatus.StandbyStatus))

	newAutoUpgradeState = autoUpgradeStateMachine[instance.Status.ClassicUpgradeStatus.UpgradeState][instance.Status.ClassicUpgradeStatus.ActiveStatus][instance.Status.ClassicUpgradeStatus.StandbyStatus]

	if instance.Status.ClassicUpgradeStatus.UpgradeState != newAutoUpgradeState {
		reqLogger.V(1).Info("checkUpgradeStandby: UpgradeState set to " + instance.Status.ClassicUpgradeStatus.UpgradeState)
	} else {
		reqLogger.V(1).Info("checkUpgradeStandby: UpgradeState=" + newAutoUpgradeState + " (unchanged)")
	}

	return nil, newAutoUpgradeState

}

// extract upgradeSchema version from upgrade.json file
func getUpgradeSchemaVer(ctx context.Context, upgradeListJson string) (*string, error) {
	reqLogger := log.FromContext(ctx)
	us := "getUpgradeSchemaVer"
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var x map[string]interface{}
	var jsonSchemaVer string

	err := json.Unmarshal([]byte(upgradeListJson), &x)
	if err != nil {
		errMsg := "error parsing upgrade compatibility list"
		reqLogger.V(2).Info(fmt.Sprintf("%s: %s", us, errMsg))
		return nil, errors.New(errMsg)
	}

	jsonSchemaVer, err = GetSimpleJsonString(&x, "schemaVersion")
	if err != nil {
		errMsg := "unable to determine schema version of upgrade compatibility list"
		reqLogger.V(2).Info(fmt.Sprintf("%s: %s", us, errMsg))
		return nil, errors.New(errMsg)
	} else {
		reqLogger.V(2).Info(fmt.Sprintf("%s: schema version of upgrade.json file is %v", us, jsonSchemaVer))
		return &jsonSchemaVer, nil
	}

}

// extract version of upgrade.json file
func getUpgradeListVer(ctx context.Context, upgradeListJson string) (*string, error) {
	reqLogger := log.FromContext(ctx)
	us := "getUpgradeListVer"
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var x map[string]interface{}
	var upgradeListVer string

	err := json.Unmarshal([]byte(upgradeListJson), &x)
	if err != nil {
		errMsg := "error parsing upgrade compatibility list"
		reqLogger.V(2).Info(fmt.Sprintf("%s: %s", us, errMsg))
		return nil, errors.New(errMsg)
	}

	upgradeListVer, err = GetSimpleJsonString(&x, "version")
	if err != nil {
		errMsg := "unable to determine version of upgrade compatibility list"
		reqLogger.V(2).Info(fmt.Sprintf("%s: %s", us, errMsg))
		return nil, errors.New(errMsg)
	} else {
		reqLogger.V(2).Info(fmt.Sprintf("%s: version of upgrade.json file is %v", us, upgradeListVer))
		return &upgradeListVer, nil
	}

}

// determine patch compatibility between an AS pair
func isPatchCompatible(ctx context.Context, instance *timestenv2.TimesTenClassic, upgradeListJson string) (*int, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("isPatchCompatible starts")
	defer reqLogger.V(2).Info("isPatchCompatible ends")

	var activePodName string
	var activePodNo int
	var activeVersion string
	var standbyPodName string
	var standbyPodNo int
	var standbyVersion string

	err, pairStates := getCurrActiveStandby(ctx, instance)
	if err != nil {
		errMsg := fmt.Sprintf("cannot determine current AS pod assignments : %v", err.Error())
		reqLogger.Error(err, fmt.Sprintf("isPatchCompatible: %s", errMsg))
		return nil, errors.New(errMsg)
	}
	reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: pairStates=%v", pairStates))

	if _, ok := pairStates["activePodNo"]; ok {
		activePodNo = pairStates["activePodNo"]
		reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: activePodNo=%d", activePodNo))
	} else {
		errMsg := "isPatchCompatible: getCurrActiveStandby did not return an activePodNo"
		reqLogger.Error(err, errMsg)
		return nil, errors.New(errMsg)
	}

	activePodName = instance.Status.PodStatus[activePodNo].Name

	activeVersion = instance.Status.PodStatus[activePodNo].TimesTenStatus.Release

	reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: activePodNo=%d", activePodNo))
	reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: activeVersion=%s", activeVersion))

	if _, ok := pairStates["standbyPodNo"]; !ok {
		msg := "isPatchCompatible: getCurrActiveStandby did not return a standbyPodNo"
		reqLogger.V(2).Info(msg)
		return nil, errors.New(msg)
	}

	standbyPodNo = pairStates["standbyPodNo"]
	reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: standbyPodNo=%d", standbyPodNo))

	standbyVersion = instance.Status.PodStatus[standbyPodNo].TimesTenStatus.Release

	standbyPodName = instance.Status.PodStatus[standbyPodNo].Name

	reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: standbyPodName=%v", standbyPodName))
	reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: standbyVersion=%v", standbyVersion))
	reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: active pod %v is running TimesTen %v", activePodName,
		activeVersion))
	reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: standby pod %v is running TimesTen %v", standbyPodName,
		standbyVersion))

	ttVersionActive := strings.Split(instance.Status.PodStatus[activePodNo].TimesTenStatus.Release, ".")
	if len(ttVersionActive) < 5 {
		errMsg := fmt.Sprintf("Cannot parse TimesTen release version for the active (POD %v)", activePodNo)
		errObj := errors.New(errMsg)
		reqLogger.Error(errObj, "isPatchCompatible: "+errMsg)
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: ttVersionActive=%v", ttVersionActive))
	}

	ttVersionStandby := strings.Split(instance.Status.PodStatus[standbyPodNo].TimesTenStatus.Release, ".")

	if len(ttVersionStandby) < 5 {
		errMsg := fmt.Sprintf("Cannot parse TimesTen release version for the standby (POD %v)", standbyPodNo)
		reqLogger.Info("isPatchCompatible: " + errMsg)
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: ttVersionStandby=%v", ttVersionStandby))
	}

	// get the json schema version of the upgrade.json file
	// given there are multiple schema versions of this file,
	// we need to know which struct to use when unmarshalling

	schemaVer, err := getUpgradeSchemaVer(ctx, upgradeListJson)
	if err != nil {
		errMsg := "unable to determine schema version of upgrade compatibility list"
		reqLogger.Info(fmt.Sprintf("isPatchCompatible: %s", errMsg))
	}

	patchCompat := 0
	knownSchemas := []string{"1", "2"}
	for _, s := range knownSchemas {

		if schemaVer != nil && *schemaVer != s {
			//reqLogger.Info(fmt.Sprintf("isPatchCompatible: schemaVer is %v, looking for %v, skip", *schemaVer, s))
			continue
		}

		switch s {
		case "1":

			reqLogger.Info(fmt.Sprintf("isPatchCompatible: parsing upgrade.json with schema version %s", s))
			var upgradeList upgradeCompat_v1
			err := json.Unmarshal([]byte(upgradeListJson), &upgradeList)
			if err != nil {
				errMsg := fmt.Sprintf("error parsing upgrade.json with schema version %s, err=%v", s, err)
				reqLogger.Info(fmt.Sprintf("isPatchCompatible: %s", errMsg))
				continue
			} else {

				for _, v := range upgradeList.ValidUpgrades {
					reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: from=%v to=%v is supported", v.From, v.To))
				}

				if activeVersion == standbyVersion {
					reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: active version %s is the same as standby version, allow upgrade", activeVersion))
					patchCompat = 1
				} else {
					reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: found version running on active, %s, check compat with standby %s",
						activeVersion, standbyVersion))
					for _, v := range upgradeList.ValidUpgrades {
						reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: from=%v to=%v is supported", v.From, v.To))
						if v.From == activeVersion || v.To == activeVersion {
							if v.To == standbyVersion || v.From == standbyVersion {
								if v.Classic == nil {
									// if the classic or scaleout field(s) are missing, the upgrade/downgrade pair is not supported
									reqLogger.Info(fmt.Sprintf("isPatchCompatible: json attrib classic not defined for upgrade/downgrade ver %s to %s", activeVersion, standbyVersion))
								} else {
									reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: active version %s supports upgrade/downgrade to standby version %s", activeVersion, standbyVersion))
									patchCompat = 1
									break
								}
							}
						}
					}
				}

				if patchCompat == 1 {
					reqLogger.Info(fmt.Sprintf("isPatchCompatible: active version %v is patch compatible with standby version %v",
						activeVersion, standbyVersion))
				} else {
					errMsg := fmt.Sprintf("Active version %v on POD %v is not patch compatible with standby version %v on POD %v; update image and delete pod %v",
						activeVersion, activePodNo, standbyVersion, standbyPodNo, standbyPodName)
					reqLogger.Info("isPatchCompatible: " + errMsg)
					return &patchCompat, errors.New(errMsg)
				}

				return &patchCompat, nil
			}

		case "2":

			reqLogger.Info(fmt.Sprintf("isPatchCompatible: parsing upgrade.json with schema version %s", s))
			var upgradeList upgradeCompat_v2
			err := json.Unmarshal([]byte(upgradeListJson), &upgradeList)
			if err != nil {
				errMsg := fmt.Sprintf("error parsing upgrade.json with schema version %s, err=%v", s, err)
				reqLogger.Info(fmt.Sprintf("isPatchCompatible: %s", errMsg))
				return nil, errors.New(errMsg)
			} else {
				if activeVersion == standbyVersion {
					reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: active version %s is the same as standby version, allow upgrade", activeVersion))
					patchCompat = 1
				} else {
					for _, v := range upgradeList.ValidUpgrades {
						reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: from=%v to=%v is supported", v.From, v.To))

						// the 'inverse' field was introduced in v2; if false, it means we can upgrade 'from->to',
						// but not back to 'from' once upgraded
						if v.Classic.Online.Inverse == nil || *v.Classic.Online.Inverse == false {
							reqLogger.Info(fmt.Sprintf("isPatchCompatible: inverse=false, %s can upgrade to %s but not the inverse", v.From, v.To))
							if v.From == activeVersion {
								reqLogger.V(2).Info(fmt.Sprintf("isPatchCompatible: found version running on active, %s, check compat with standby %s",
									activeVersion, standbyVersion))
								if v.To == standbyVersion {
									if v.Classic == nil {
										// if the classic or scaleout field(s) are missing, the upgrade/downgrade pair is not supported
										reqLogger.Info(fmt.Sprintf("isPatchCompatible: json attrib classic not defined for upgrade ver %s to %s", activeVersion, standbyVersion))
									} else {
										reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: active version %s supports upgrade to standby version %s", activeVersion, standbyVersion))
										patchCompat = 1
										break
									}
								}
							}
						} else {
							// allow upgrades/downgrades of a version pair (inverse)
							if v.From == activeVersion || v.To == activeVersion {
								reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: found version running on active, %s, check compat with standby %s",
									activeVersion, standbyVersion))
								if v.To == standbyVersion || v.From == standbyVersion {
									if v.Classic == nil {
										// if the classic or scaleout field(s) are missing, the upgrade/downgrade pair is not supported
										reqLogger.Info(fmt.Sprintf("isPatchCompatible: json attrib classic not defined for upgrade/downgrade ver %s to %s", activeVersion, standbyVersion))
									} else {
										reqLogger.V(1).Info(fmt.Sprintf("isPatchCompatible: active version %s supports upgrade/downgrade to standby version %s", activeVersion, standbyVersion))
										patchCompat = 1
										break
									}
								}
							}
						}
					}
				}

				if patchCompat == 1 {
					reqLogger.Info(fmt.Sprintf("isPatchCompatible: active version %v is patch compatible with standby version %v",
						activeVersion, standbyVersion))
				} else {
					errMsg := fmt.Sprintf("Active version %v on POD %v is not patch compatible with standby version %v on POD %v; to resolve, update image and delete pod %v",
						activeVersion, activePodNo, standbyVersion, standbyPodNo, standbyPodName)
					reqLogger.Info("isPatchCompatible: " + errMsg)
					return &patchCompat, errors.New(errMsg)
				}

				return &patchCompat, nil
			}

		default:
			errMsg := fmt.Sprintf("unknown value schema version %s found in upgrade.json", *schemaVer)
			reqLogger.Info("isPatchCompatible: " + errMsg)
			return nil, errors.New(errMsg)
		}

	}

	return &patchCompat, nil
}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-kind:nil */
/* End: */
