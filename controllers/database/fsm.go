// Copyright (c) 2019-2020, Oracle and/or its affiliates. All rights reserved.
//
// Finite State Machines /  Flowcharts for managing TimesTen

package controllers

import (
	"errors"

	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var highLevelStateMachine = map[string]map[string]map[string]string{}
var autoUpgradeStateMachine = map[string]map[string]map[string]string{}
var podstates = map[string]int{}
var upgradestates = map[string]int{}
var upgradeHLstates = map[string]int{}

//var hlstates = map[string]int{}

// Is a TimesTen container running? Meaning:
// 1. Does Kubernetes say that the Pod is running?
func isPodRunning(instance *timestenv2.TimesTenClassic, podNo int) (bool, error) {
	if instance.Status.PodStatus[podNo].PodStatus.PodPhase == "Running" {
		return true, errors.New("Pod phase " + instance.Status.PodStatus[podNo].PodStatus.PodPhase)
	}
	return false, nil
}

// Is a TimesTen container reachable? Meaning:
// 1. Does Kubernetes say that the Pod is running?
// 2. Is the TimesTen Agent in the TimesTen container in that pod returning data to us?
// If both of those are true then the container is reachable.
func isPodReachable(ctx context.Context, instance *timestenv2.TimesTenClassic, podNo int) (bool, error) {
	us := "isPodReachable"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	if ok, err := isPodRunning(instance, podNo); ok {
		if instance.Status.PodStatus[podNo].PodStatus.Agent == "Up" {
			instance.Status.PodStatus[podNo].PodStatus.LastTimeReachable = time.Now().Unix()
			reqLogger.V(2).Info(fmt.Sprintf("%s : %d", us, instance.Status.PodStatus[podNo].PodStatus.LastTimeReachable))
			return true, nil // It's reachable!
		} else {
			// NOT reachable
			return false, errors.New(fmt.Sprintf("%s: pod %d agent %s", us, podNo, instance.Status.PodStatus[podNo].PodStatus.Agent))
		}
	} else {
		// It's NOT reachable!
		msg := fmt.Sprintf("%s: pod %v:  %s", us, podNo, err.Error())
		reqLogger.V(2).Info(msg)
		return false, err
	}
}

// Is a TimesTen container quiescing? Meaning that a preStop hook is shutting it down?
// During this time the database is being unloaded, and we shouldn't try to do anything
// to repair it ... the pod will vanish soon. We can try to fix it's replacement when it
// comes back up, but not now.
func isPodQuiescing(ctx context.Context, instance *timestenv2.TimesTenClassic, podNo int) (bool, error) {
	us := "isPodQuiescing"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	if ok, err := isPodReachable(ctx, instance, podNo); ok {
		if instance.Status.PodStatus[podNo].Quiescing {
			reqLogger.V(2).Info(fmt.Sprintf("%s: pod %d quiescing", us, podNo))
		}
		return instance.Status.PodStatus[podNo].Quiescing, nil
	} else {
		// It's NOT reachable!
		msg := fmt.Sprintf("%s: pod %v:  %s", us, podNo, err.Error())
		reqLogger.V(2).Info(msg)
		return false, err
	}
}

// Has an unreachable Pod been unreachable longer than the specified timeout?
func isReachableTimeoutExceeded(ctx context.Context, instance *timestenv2.TimesTenClassic, podNo int) (bool, error) {
	us := "isReachableTimeoutExceeded"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")

	// If the agent in this pod hasn't started yet then the timeout hasn't been hit
	if instance.Status.PodStatus[podNo].PodStatus.LastTimeReachable == 0 {
		return false, nil
	}

	var timeout time.Duration = 30 * time.Second
	if instance.Spec.TTSpec.UnreachableTimeout != nil &&
		*instance.Spec.TTSpec.UnreachableTimeout > 0 {
		timeout = time.Duration(int64(*instance.Spec.TTSpec.UnreachableTimeout) * int64(time.Second))
	}
	reqLogger.V(2).Info(fmt.Sprintf("%s: timeout set to %d secs", us, int(timeout.Seconds())))

	now := time.Now().Unix()
	t := now - instance.Status.PodStatus[podNo].PodStatus.LastTimeReachable

	if float64(t) > timeout.Seconds() {
		reqLogger.V(2).Info(fmt.Sprintf("%s: timed out waiting for pod %v t=%d ltr=%d", us, podNo, t, instance.Status.PodStatus[podNo].PodStatus.LastTimeReachable))
		return true, errors.New(fmt.Sprintf("Unreachable for %d seconds", int(t)))
	} else {
		reqLogger.V(2).Info(fmt.Sprintf("%s: podno %v not reachable, timeout in %v secs",
			us, podNo, timeout.Seconds()-float64(t)))
		return false, nil
	}
}

// Has a pod's replication state been in a bad state for longer than the timeout?
func isRepStateTimeoutExceeded(ctx context.Context, instance *timestenv2.TimesTenClassic, podNo int) bool {
	reqLogger := log.FromContext(ctx)

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		return false
	}
	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return false
	}

	var timeout time.Duration = 30 * time.Second
	if instance.Spec.TTSpec.RepStateTimeout != nil &&
		*instance.Spec.TTSpec.RepStateTimeout > 0 {
		timeout = time.Duration(int64(*instance.Spec.TTSpec.RepStateTimeout) * int64(time.Second))
	}

	now := time.Now().Unix()
	t := now - instance.Status.PodStatus[podNo].ReplicationStatus.LastTimeRepStateChanged
	reqLogger.Info("repStateTimeout last " + strconv.FormatInt(instance.Status.PodStatus[podNo].ReplicationStatus.LastTimeRepStateChanged, 10) + " now " + strconv.FormatInt(now, 10) + " delta " + strconv.FormatInt(t, 10) + " timeout " + strconv.FormatInt(int64(timeout.Seconds()), 10))
	if float64(t) > timeout.Seconds() {
		return true
	} else {
		return false
	}
}

// ----------------------------------------------------------------------
// FSMFunc is a function which implements a flow
// Return values:
// The current state of TimesTen in this pod (the answer)
// Any error
// Boolean: whether TimesTen in this pod is "Ready" or not (aka
// Kubernetes 'readiness probes' style "Ready")
// ----------------------------------------------------------------------
type FSMFunc func(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool)

// ----------------------------------------------------------------------
// An "Answer" is returned by a Question or an Action
// ----------------------------------------------------------------------
type FSMAnswer string

// ----------------------------------------------------------------------
// Check for NORMAL in an NON-REPLICATED OBJECT
// ----------------------------------------------------------------------
func nonrepNormal(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "nonrepNormal"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Normal"), nil, true
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Normal"), errors.New("Instance status " + is), true
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon Down"), false
	case "Unknown":
		return FSMAnswer("Normal"), errors.New("Daemon status 'Unknown'"), true
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded", "Loading", "Transitioning":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Unknown":
		return FSMAnswer("Normal"), errors.New("Db status " + dbs), true
	case "Loaded":
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		return FSMAnswer("Down"), errors.New("Db closed"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "None":
	case "Exists":
		return FSMAnswer("Down"), errors.New("Unexpected repscheme"), false
	case "Unknown":
		return FSMAnswer("Normal"), errors.New("Repscheme " + rsch), true
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected RepScheme " + rsch), false

	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

//----------------------------------------------------------------------
// When a non-replicated pod becomes terminal, that means forever
// (at least for now)
//----------------------------------------------------------------------

func nonrepTerminal(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "nonrepTerminal"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	return FSMAnswer("Terminal"), nil, false
}

// ----------------------------------------------------------------------
// Check for NOT PROVISIONED in an SUBSCRIBER
// ----------------------------------------------------------------------
func subscriberNotProvisioned(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "subscriberNotProvisioned"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	if ok, _ := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("NotProvisioned"), nil, false
	}
	return FSMAnswer("Down"), nil, false
}

// ----------------------------------------------------------------------
// Check for NORMAL in an SUBSCRIBER
// ----------------------------------------------------------------------
func subscriberNormal(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "subscriberNormal"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance status " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon is down"), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon status 'Unknown'"), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		return FSMAnswer("Down"), errors.New("No database"), false
	case "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Db status " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		return FSMAnswer("Down"), errors.New("Db closed"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "None":
		return FSMAnswer("Down"), errors.New("No repscheme"), false
	case "Exists":
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Repscheme " + rsch), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme " + rsch), false
	}

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		return FSMAnswer("Down"), errors.New("No repagent"), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent status " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

// ----------------------------------------------------------------------
// Check for DOWN in an SUBSCRIBER
// ----------------------------------------------------------------------
func subscriberDown(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "subscriberDown"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error
	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance status " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon status 'Unknown'"), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloaded":
		if instance.Status.HighLevelState == "Normal" {
			// Destroy database just in case
			_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)

			// Duplicate database
			err = RunAction(ctx, instance, podNo, "repDuplicate", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				// Fall through, this is awesome
			}
		} else {
			return FSMAnswer("Down"), errors.New("Db status " + dbs), false
		}
	case "Unloading":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Db status " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "None":
		return FSMAnswer("Down"), errors.New("No repscheme"), false
	case "Exists":
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Repscheme " + rsch), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme " + rsch), false

	}

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		// Start the replication agent
		err := RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			instance.Status.RepStartFailCount++
			instance.Status.StandbyDownStandbyAS.Status = "complete"
			errMsg := fmt.Sprintf("Standby: Starting replication failed. Count: %d", instance.Status.RepStartFailCount)
			reqLogger.Info(errMsg)
			logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
			return FSMAnswer("Down"), err, false
		} else {
			instance.Status.StandbyDownStandbyAS.StartRepAgent = true
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent status " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

// ----------------------------------------------------------------------
// Check for UNKNOWN in an SUBSCRIBER
// ----------------------------------------------------------------------
func subscriberUnknown(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "subscriberUnknown"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error
	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), err, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance status " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon status 'Unknown'"), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		if instance.Status.HighLevelState == "Normal" {
			// Destroy database just in case
			_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)

			// Duplicate database
			err = RunAction(ctx, instance, podNo, "repDuplicate", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				return FSMAnswer("Unknown"), nil, false
			}
		} else {
			return FSMAnswer("Down"), errors.New("Db status " + dbs), false
		}
	case "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Db status " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "None":
	case "Exists":
		return FSMAnswer("Down"), errors.New("Unexpected repscheme"), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Repscheme " + rsch), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme " + rsch), false

	}

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		// Start the replication agent
		err := RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			instance.Status.RepStartFailCount++
			instance.Status.StandbyDownStandbyAS.Status = "complete"
			errMsg := fmt.Sprintf("Standby: Starting replication failed. Count: %d", instance.Status.RepStartFailCount)
			reqLogger.Info(errMsg)
			logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
			return FSMAnswer("Down"), err, false
		} else {
			instance.Status.StandbyDownStandbyAS.StartRepAgent = true
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent status " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

// ----------------------------------------------------------------------
// Check for FAILED in an SUBSCRIBER
// ----------------------------------------------------------------------
func subscriberFailed(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "subscriberFailed"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error
	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance status " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon status 'Unknown'"), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		if instance.Status.HighLevelState == "Normal" {
			// Destroy database just in case
			_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)

			// Duplicate database
			err = RunAction(ctx, instance, podNo, "repDuplicate", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				return FSMAnswer("Unknown"), nil, false
			}
		} else {
			return FSMAnswer("Down"), errors.New("Db status " + dbs), false
		}
	case "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Db status " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "None":
	case "Exists":
		return FSMAnswer("Down"), errors.New("Unexpected repscheme"), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Repscheme " + rsch), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme " + rsch), false

	}

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		// Start the replication agent
		err := RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			instance.Status.RepStartFailCount++
			instance.Status.StandbyDownStandbyAS.Status = "complete"
			errMsg := fmt.Sprintf("Standby: Starting replication failed. Count: %d", instance.Status.RepStartFailCount)
			reqLogger.Info(errMsg)
			logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
			return FSMAnswer("Down"), err, false
		} else {
			instance.Status.StandbyDownStandbyAS.StartRepAgent = true
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent status " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

// ----------------------------------------------------------------------
// Check for CATCHING UP in an SUBSCRIBER
// ----------------------------------------------------------------------
func subscriberCatchingUp(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "subscriberCatchingUp"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error
	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), errors.New("Pod not reachable"), false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance status " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon status 'Unknown'"), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		if instance.Status.HighLevelState == "Normal" {
			// Destroy database just in case
			_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)

			// Duplicate database
			err = RunAction(ctx, instance, podNo, "repDuplicate", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				return FSMAnswer("Unknown"), nil, false
			}
		} else {
			return FSMAnswer("Down"), errors.New("Db status " + dbs), false
		}
	case "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Db status " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "None":
	case "Exists":
		return FSMAnswer("Down"), errors.New("Unexpected repscheme"), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Repscheme " + rsch), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme " + rsch), false

	}

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		// Start the replication agent
		err := RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			instance.Status.RepStartFailCount++
			instance.Status.StandbyDownStandbyAS.Status = "complete"
			errMsg := fmt.Sprintf("Standby: Starting replication failed. Count: %d", instance.Status.RepStartFailCount)
			reqLogger.Info(errMsg)
			logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
			return FSMAnswer("Down"), err, false
		} else {
			instance.Status.StandbyDownStandbyAS.StartRepAgent = true
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent status " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

// ----------------------------------------------------------------------
// Check for INITIALIZING in an SUBSCRIBER
// ----------------------------------------------------------------------
func subscriberInitializing(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "subscriberInitializing"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error
	if ok, _ := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Initializing"), nil, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		return FSMAnswer("Initializing"), nil, false
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Initializing"), nil, false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		} else {
			return FSMAnswer("Initializing"), nil, false
		}
	case "Unknown":
		return FSMAnswer("Initializing"), errors.New("Daemon status 'Unknown'"), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		if instance.Status.HighLevelState == "Normal" {
			// Destroy database just in case
			_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)

			// Duplicate database
			err = RunAction(ctx, instance, podNo, "repDuplicate", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				// Fall through, this is good!
			}
		} else {
			// If the active standby pair isn't ready then no sense
			// trying to duplicate from it yet.
			return FSMAnswer("Initializing"), errors.New("Active Standby Pair status " + instance.Status.HighLevelState), false
		}
	case "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Initializing"), errors.New("Db status " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "None":
	case "Exists":
		return FSMAnswer("Down"), errors.New("Unexpected repscheme"), false
	case "Unknown":
		return FSMAnswer("Initializing"), errors.New("Repscheme " + rsch), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme " + rsch), false

	}

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		// Start the replication agent
		err := RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			instance.Status.RepStartFailCount++
			instance.Status.StandbyDownStandbyAS.Status = "complete"
			errMsg := fmt.Sprintf("Standby: Starting replication failed. Count: %d", instance.Status.RepStartFailCount)
			reqLogger.Info(errMsg)
			logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
			return FSMAnswer("Down"), err, false
		} else {
			instance.Status.StandbyDownStandbyAS.StartRepAgent = true
		}
	case "Unknown":
		return FSMAnswer("Initializing"), errors.New("RepAgent status " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

// ----------------------------------------------------------------------
// Check for NORMAL ACTIVE in an ACTIVE STANDBY PAIR
// ----------------------------------------------------------------------
func checkNormalActiveAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("checkNormalActiveAS starts")
	defer reqLogger.V(2).Info("checkNormalActiveAS ends")

	var err error

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance status " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon Down"), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon status 'Unknown'"), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Db status " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Db status " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	switch rsch := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rsch {
	case "Exists":
	case "None":
		return FSMAnswer("Down"), errors.New("No repscheme"), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Repscheme " + rsch), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme " + rsch), false

	}

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		return FSMAnswer("Down"), errors.New("RepAgent status " + ra), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent status " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
	case "STANDBY", "FAILED", "IDLE":
		return FSMAnswer("Down"), errors.New("RepState " + rs), false
	case "RECOVERING":
		if isRepStateTimeoutExceeded(ctx, instance, podNo) {
			return FSMAnswer("Down"), errors.New("RepState RECOVERING and timeout exceeded"), false
		} else {
			return FSMAnswer("Unknown"), errors.New("RepState RECOVERING"), false
		}
	case "ACTIVE":
	case "Unknown":
		return "", errors.New("RepState " + rs), false
	default:
		return "", errors.New("Unexpected RepState value '" + rs + "'"), false
	}

	switch rps := instance.Status.PodStatus[podNo].ReplicationStatus.RepPeerPState; rps {
	case "stop", "failed":
		return FSMAnswer("OtherDown"), errors.New("Peer state " + rps), true // WE are fine
	case "pause":
		err = RunAction(ctx, instance, podNo, "setSubStateStart", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Down"), err, false
		}
		return FSMAnswer("Unknown"), nil, false // BUT may be ready next time
	case "start":
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Peer state " + rps), false
	default:
		return FSMAnswer("Down"), errors.New("Unexpected RepPeerPState value '" + rps + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		return FSMAnswer("Down"), errors.New("Db closed"), false
	}

	return FSMAnswer("Healthy"), nil, true
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for STANDBY DOWN in an ACTIVE STANDBY PAIR
// This is run on the dead standby to bring it back to life
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func standbyDownStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "standbyDownStandbyAS"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	podName := instance.Status.PodStatus[podNo].Name

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	// Does instance exist?
	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance status " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	// Is daemon up? Start it if not.
	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		reqLogger.V(1).Info(fmt.Sprintf("%s: TimesTenStatus.Daemon returned DOWN, running startDaemon", us))
		err := RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Down"), err, false
		} else {
			reqLogger.V(1).Info(us + ": startDaemon succeeded")
			// if we're not upgrading, preserve previous behavior and return Unknown
			// if we are upgrading, we want to continue
			if instance.Status.ClassicUpgradeStatus.UpgradeState == "" {
				return FSMAnswer("Unknown"), nil, false
			}
		}
	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	if instance.Status.ClassicUpgradeStatus.UpgradeState != "" {

		// check the patch compatibility of the new standby before proceeding with upgrade

		if strings.HasPrefix(instance.Status.ClassicUpgradeStatus.UpgradeState, "Upgrading") {

			//var upgradeListStandby *string
			//var upgradeListActive *string
			//var patchCompat *int
			var activePodNo int
			var activePodName string
			var activeVersion string
			var activePodDNSName string
			var standbyPodNo int
			var standbyPodName string
			var standbyVersion string
			var standbyPodDNSName string
			var activeUpgradeListVer *string
			var standbyUpgradeListVer *string

			useLocalUpgradeList := 0
			reqParams := make(map[string]string)
			reqParams["include"] = "upgradeList"

			err, pairStates := getCurrActiveStandby(ctx, instance)
			if err != nil {
				errMsg := fmt.Sprintf("cannot determine current AS pod assignments : %v", err.Error())
				reqLogger.Error(err, errMsg)
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				return FSMAnswer("UpgradeFailed"), err, false
			}

			reqLogger.V(2).Info(fmt.Sprintf("%s: pairStates=%v", us, pairStates))

			if _, ok := pairStates["activePodNo"]; ok {
				reqLogger.V(2).Info(fmt.Sprintf("%s: activePodNo=%d", us, pairStates["activePodNo"]))
			} else {
				errMsg := us + ": getCurrActiveStandby did not return an activePodNo"
				reqLogger.Error(err, errMsg)
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				return FSMAnswer("UpgradeFailed"), err, false
			}

			activePodNo = pairStates["activePodNo"]
			activePodName = instance.Name + "-" + strconv.Itoa(activePodNo)
			activeVersion = instance.Status.PodStatus[activePodNo].TimesTenStatus.Release
			activePodDNSName = activePodName + "." + instance.Name + "." + instance.Namespace + ".svc.cluster.local"

			reqLogger.V(2).Info(fmt.Sprintf("%s: activePodName=%s", us, activePodName))
			reqLogger.V(2).Info(fmt.Sprintf("%s: activeVersion=%s", us, activeVersion))

			if _, ok := pairStates["standbyPodNo"]; ok {
				reqLogger.V(2).Info(fmt.Sprintf("%s: standbyPodNo=%d", us, pairStates["standbyPodNo"]))
			} else {
				reqLogger.V(2).Info(us + ": getCurrActiveStandby did not return a standbyPodNo")
				// SAMDRAKE Why isn't this logic in getCurrActiveStandby?
				// figure out the standby pod
				//for pName, p := range instance.Status.PodStatus
				//    if pName != podNo != activePodNo {
				//         standbyPodNo = podNo
				//         reqLogger.V(2).Info(fmt.Sprintf("%s: standbyPodNo set to %v", us, standbyPodNo))
				//         break
				//    }
				//}
			}

			standbyPodNo = pairStates["standbyPodNo"]
			standbyPodName = instance.Name + "-" + strconv.Itoa(standbyPodNo)
			standbyVersion = instance.Status.PodStatus[standbyPodNo].TimesTenStatus.Release
			standbyPodDNSName = standbyPodName + "." + instance.Name + "." + instance.Namespace + ".svc.cluster.local"

			reqLogger.V(2).Info(fmt.Sprintf("%s: standbyPodName=%s", us, standbyPodName))
			reqLogger.V(2).Info(fmt.Sprintf("%s: standbyVersion=%s", us, standbyVersion))
			reqLogger.V(1).Info(fmt.Sprintf("%s: active pod %v is running TimesTen %v", us, activePodName, activeVersion))
			reqLogger.V(1).Info(fmt.Sprintf("%s: standby pod %v is running TimesTen %v", us, standbyPodName, standbyVersion))

			// get the active ip address for the call to getTTAgentOut

			activePodStatus := &corev1.Pod{}
			err = client.Get(ctx, types.NamespacedName{Name: activePodName, Namespace: instance.Namespace}, activePodStatus)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
				errMsg := fmt.Sprintf("%s: cannot fetch status of pod %s : %v", us, activePodName, errorMsg)
				reqLogger.Error(err, errMsg)
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				if isPermissionsProblem {
					updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				}
				return FSMAnswer("UpgradeFailed"), err, false
			}

			// get the standby ip address for the call to getTTAgentOut

			standbyPodStatus := &corev1.Pod{}
			err = client.Get(ctx, types.NamespacedName{Name: standbyPodName, Namespace: instance.Namespace}, standbyPodStatus)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
				errMsg := fmt.Sprintf("%s: cannot fetch status of pod %s : %v", us, activePodName, errorMsg)
				reqLogger.Error(err, errMsg)
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				if isPermissionsProblem {
					updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				}
				return FSMAnswer("UpgradeFailed"), err, false
			}

			// get the standby's upgrade list

			standbyAgentOut, err, _ := getTTAgentOut(ctx, instance, string(instance.ObjectMeta.UID), standbyPodDNSName, standbyPodStatus.Status.PodIP,
				client, tts, reqParams)
			if err != nil {
				reqLogger.V(2).Info(fmt.Sprintf("%s: getTTAgentOut for standby %v returned err=%v", us, standbyPodName, err))
				reqLogger.Error(err, "Could not fetch ttAgentOut: "+err.Error())
				errMsg := "error reading upgrade compatibility list on standby"
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
			} else {
				reqLogger.V(2).Info(fmt.Sprintf("%s: getTTAgentOut for standby %v returned : %v", us, standbyPodName, standbyAgentOut))
			}

			if standbyAgentOut.UpgradeList == nil {
				// the agent response did not include the UpgradeList field
				// that probably means that the remote agent does not support the retrieval of the upgrade list (it's old),
				// otherwise it would have returned an empty string or string containing an error message
				reqLogger.V(1).Info(fmt.Sprintf("%s: No upgrade list from standby", us))
			} else {
				if len(*standbyAgentOut.UpgradeList) == 0 {
					reqLogger.V(1).Info(fmt.Sprintf("%s: Empty upgrade list from standby", us))
				} else {
					reqLogger.V(2).Info(fmt.Sprintf("%s: standbyAgentOut.UpgradeList=%v", us, *standbyAgentOut.UpgradeList))
					if strings.HasPrefix(*standbyAgentOut.UpgradeList, "error") {
						errMsg := "error reading upgrade compatibility list from standby"
						logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
					} else {
						//upgradeListStandby = standbyAgentOut.UpgradeList
						reqLogger.Info("Unimplemented getUpgradeListVer")
						/**
						standbyUpgradeListVer, err = getUpgradeListVer(ctx, *upgradeListStandby)
						if err != nil {
							errMsg := "unable to determine version of standby upgrade compatibility list"
							reqLogger.Info(fmt.Sprintf("%s: %s", us, errMsg))
						}
						reqLogger.Info(fmt.Sprintf("%s: standbyUpgradeListVer=%s", us, *standbyUpgradeListVer))
						*/
					}
				}
			}

			// get the active's upgrade list

			activeAgentOut, err, _ := getTTAgentOut(ctx, instance, string(instance.ObjectMeta.UID), activePodDNSName, activePodStatus.Status.PodIP, client, tts, reqParams)
			if err != nil {
				reqLogger.V(2).Info(fmt.Sprintf("%s: getTTAgentOut for active %v returned err=%v", us, activePodName, err))
				reqLogger.Error(err, "Could not fetch ttAgentOut: "+err.Error())
				errMsg := "error reading upgrade compatibility list on active"
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
			} else {
				reqLogger.V(2).Info(fmt.Sprintf("%s: getTTAgentOut for active %v returned : %v", us, activePodName, activeAgentOut))
			}

			if activeAgentOut.UpgradeList == nil {
				// the agent response did not include the UpgradeList field
				// that probably means that the remote agent does not support the retrieval of the upgrade list (it's old),
				// otherwise it would have returned an empty string or string containing an error message
				// in this case, we'll use the upgrade.json file in the operator's local distro
				reqLogger.V(1).Info(fmt.Sprintf("%s: active agent did not return upgrade list", us))
			} else {
				reqLogger.V(2).Info(fmt.Sprintf("%s: activeAgentOut.UpgradeList=%v", us, *activeAgentOut.UpgradeList))

				if strings.HasPrefix(*activeAgentOut.UpgradeList, "error") {
					errMsg := "error reading upgrade compatibility list from active"
					logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				} else {
					reqLogger.Info("Unimplemented getUpgradeListVer")
					/**upgradeListActive = activeAgentOut.UpgradeList
					// get version of upgrade list
					activeUpgradeListVer, err = getUpgradeListVer(ctx, *upgradeListActive)
					if err != nil {
						errMsg := "unable to determine version of active upgrade compatibility list"
						reqLogger.Info(fmt.Sprintf("%s: %s", us, errMsg))
					}
					reqLogger.Info(fmt.Sprintf("%s: activeUpgradeListVer=%s", us, *activeUpgradeListVer))
					*/
				}
			}

			var upgradeLists []string

			if standbyUpgradeListVer != nil && activeUpgradeListVer != nil {
				// use the latest upgrade list, unless we're 18x, in which case we have to check both lists
				if *standbyUpgradeListVer == *activeUpgradeListVer &&
					strings.HasPrefix(activeVersion, "18") {
					// older tt releases don't increment the upgradeList version string, so we don't
					// know which release is newer; process both active and standby lists
					upgradeLists = append(upgradeLists, *activeAgentOut.UpgradeList, *standbyAgentOut.UpgradeList)
				} else if *standbyUpgradeListVer > *activeUpgradeListVer {
					upgradeLists = append(upgradeLists, *standbyAgentOut.UpgradeList)
				} else {
					upgradeLists = append(upgradeLists, *activeAgentOut.UpgradeList)
				}
			} else if standbyUpgradeListVer != nil {
				upgradeLists = append(upgradeLists, *standbyAgentOut.UpgradeList)
			} else if activeUpgradeListVer != nil {
				upgradeLists = append(upgradeLists, *activeAgentOut.UpgradeList)
			}

			if len(upgradeLists) == 0 {
				reqLogger.Info(fmt.Sprintf("%s: no upgrade lists were pulled, using upgrade list from operator", us))
				useLocalUpgradeList = 1
			}

			if useLocalUpgradeList == 1 {
				upgradeFile := "/timesten/instance1/install/info/upgrade.json"
				upgradeListLocal, err := ioutil.ReadFile(upgradeFile)
				if err != nil {
					reqLogger.Error(err, us+": Could not read "+upgradeFile+": "+err.Error())
					errMsg := fmt.Sprintf("error processing local upgrade file %s", upgradeFile)
					logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
					return FSMAnswer("UpgradeFailed"), errors.New(errMsg), false
				} else {
					upgradeLists = append(upgradeLists, string(upgradeListLocal))
				}
			}

			// now process upgrade.json to determine if we have patch compatibility

			versionMatch := false
			for i, upgradeList := range upgradeLists {
				reqLogger.V(2).Info(fmt.Sprintf("%s: upgradeList %d : %+v", us, i, upgradeList))
				reqLogger.V(2).Info("Unimplemented isPatchCompatible, returning always false")
				versionMatch = false
				/**
				patchCompat, err = isPatchCompatible(ctx, instance, upgradeList)

				if err != nil {
					reqLogger.V(2).Info(fmt.Sprintf("%s: no version match in upgradeList %d, not patch compatible: %s", us, i, err.Error()))
				} else if *patchCompat == 1 {
					reqLogger.V(1).Info(fmt.Sprintf("%s: version match found in upgradeList %d, upgrade image is patch compatible", us, i))
					versionMatch = true
					break
				} else {
					reqLogger.V(2).Info(fmt.Sprintf("%s: no version match in upgradeList %d, not patch compatible", us, i))
				}
				*/

			}

			if versionMatch == true {
				reqLogger.V(1).Info(fmt.Sprintf("%s: upgrade image is patch compatible", us))
			} else {
				reqLogger.V(2).Info(fmt.Sprintf("%s: no version match in upgrade lists, not patch compatible", us))
				errMsg := "error determining upgrade patch compatibility"
				reqLogger.Info(fmt.Sprintf("%s: %s", us, errMsg))
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				return FSMAnswer("UpgradeFailed"), errors.New(errMsg), false
			}
		}
	}

	j, _ := json.Marshal(instance.Status.StandbyDownStandbyAS)
	reqLogger.V(2).Info(fmt.Sprintf("%s: instance.Status.StandbyDownStandbyAS=%s", us, string(j)))

	k, _ := json.Marshal(instance.Status.AsyncStatus)
	reqLogger.V(2).Info(fmt.Sprintf("%s: instance.Status.AsyncStatus=%s", us, string(k)))

	// get the current async status from the agent and make sure it knows about our async id
	// if the pod died during the async task, the TTC status object may not have updated and may think an operation
	// is pending after a pod restart (we can resume an operation after an operator restart, not a pod restart)

	asyncStatusUpdated := false
	asyncPodMatch := false

	reqLogger.V(2).Info(fmt.Sprintf("%s: get async status from %s", us, instance.Status.AsyncStatus.Host))
	currAgentAsyncStatus, err := getAsyncStatus(ctx, instance, instance.Status.AsyncStatus.Host, tts, instance.Status.AsyncStatus.Id)
	l, _ := json.Marshal(currAgentAsyncStatus)
	reqLogger.V(2).Info(fmt.Sprintf("%s: currAgentAsyncStatus=%s", us, string(l)))
	if err != nil {
		reqLogger.Info(fmt.Sprintf("%s: getAsyncStatus returned an error, err=%v", us, err))
	} else {
		asyncStatusUpdated = true
	}

	// in order to resume an async task where the operator restarted and we re-entered StandbyDownStandbyAS,
	// the following must be true :
	// 1) we have current async task data from the agent
	// 2) the async id associated with the StandbyDownStandbyAS task matches the most recent async request id
	// 3) the matched async id did not return an error
	// 4) the async task on the agent is not currently running (reconcile should have polled until completion)
	// 5) the pod we're trying to bring back up is the same pod associated with the async task

	if instance.Status.AsyncStatus.PodName == podName {
		// make sure the async data matches the pod we're evaluating
		asyncPodMatch = true
	}

	if asyncStatusUpdated == true &&
		asyncPodMatch == true &&
		instance.Status.StandbyDownStandbyAS.Status == "pending" &&
		instance.Status.StandbyDownStandbyAS.AsyncId == currAgentAsyncStatus.Id &&
		currAgentAsyncStatus.Errno == nil &&
		currAgentAsyncStatus.Running == false {

		if instance.Status.StandbyDownStandbyAS.DestroyDb == true {
			reqLogger.V(1).Info(fmt.Sprintf("%s: task %v already destroyed db", us, instance.Status.StandbyDownStandbyAS.Id))
		} else {
			// Destroy the database
			reqLogger.V(1).Info(us + ": destroyDb")

			_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)
			instance.Status.StandbyDownStandbyAS.DestroyDb = true
		}

	} else {

		reqLogger.V(2).Info("standbyDownStandbyAS: creating a new StandbyDownStandbyAS task")

		// the TimesTenClassicStatus.StandbyDownStandbyAS data structure defines the
		// steps we need to complete : DestroyDb, RepDuplicate, StartRepAgent

		instance.Status.StandbyDownStandbyAS.Status = "pending"
		instance.Status.StandbyDownStandbyAS.Id = uuid.New().String()
		instance.Status.StandbyDownStandbyAS.AsyncId = ""
		instance.Status.StandbyDownStandbyAS.PodName = podName
		instance.Status.StandbyDownStandbyAS.DestroyDb = false
		instance.Status.StandbyDownStandbyAS.RepDuplicate = false
		instance.Status.StandbyDownStandbyAS.StartRepAgent = false

		// Destroy the database
		reqLogger.V(1).Info("standbyDownStandbyAS: destroyDb")

		_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)
		// TODO : we need to check for success/fail

		// set DestroyDb to true, we have just completed a task!
		instance.Status.StandbyDownStandbyAS.DestroyDb = true

	}

	// determine if we need to run repDuplicate

	doRepDuplicate := true
	if asyncPodMatch == true &&
		instance.Status.StandbyDownStandbyAS.AsyncId == currAgentAsyncStatus.Id {
		if instance.Status.StandbyDownStandbyAS.RepDuplicate == true {
			reqLogger.V(1).Info(fmt.Sprintf("standbyDownStandbyAS: task %v already completed RepDuplicate", instance.Status.StandbyDownStandbyAS.Id))
			doRepDuplicate = false
		} else {
			reqLogger.V(2).Info(fmt.Sprintf("standbyDownStandbyAS: task %v has not completed RepDuplicate", instance.Status.StandbyDownStandbyAS.Id))
		}
	}

	if doRepDuplicate == true {
		// Duplicate the database from the active
		err := RunAction(ctx, instance, podNo, "repDuplicate", nil, client, tts, nil)
		if err != nil {
			instance.Status.RepStartFailCount++
			instance.Status.StandbyDownStandbyAS.Status = "complete"
			errMsg := fmt.Sprintf("Standby: Duplicate unsuccessful. Count: %d", instance.Status.RepStartFailCount)
			reqLogger.Info(errMsg)
			logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
			return FSMAnswer("Down"), err, false
		} else {
			reqLogger.Info("standbyDownStandbyAS: repDuplicate succesful, now startRepAgent")

			instance.Status.StandbyDownStandbyAS.RepDuplicate = true
		}
	}

	doStartRepAgent := true
	if asyncPodMatch == true &&
		instance.Status.StandbyDownStandbyAS.AsyncId == currAgentAsyncStatus.Id {
		if instance.Status.StandbyDownStandbyAS.StartRepAgent == true {
			reqLogger.V(1).Info(fmt.Sprintf("standbyDownStandbyAS: task %v already completed StartRepAgent", instance.Status.StandbyDownStandbyAS.Id))
			doStartRepAgent = false
		} else {
			reqLogger.V(2).Info(fmt.Sprintf("standbyDownStandbyAS: task %v has not completed StartRepAgent", instance.Status.StandbyDownStandbyAS.Id))
		}
	}

	if doStartRepAgent == true {
		// Start the replication agent
		err := RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			instance.Status.RepStartFailCount++
			instance.Status.StandbyDownStandbyAS.Status = "complete"
			errMsg := fmt.Sprintf("Standby: Starting replication failed. Count: %d", instance.Status.RepStartFailCount)
			reqLogger.Info(errMsg)
			logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
			return FSMAnswer("Down"), err, false
		} else {
			instance.Status.StandbyDownStandbyAS.StartRepAgent = true
		}
	} else {
		reqLogger.V(2).Info(us + ": Not starting repagent")
	}

	// mark async task as complete; done with tasks
	instance.Status.StandbyDownStandbyAS.Status = "complete"

	// What's our repstate?
	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
	case "ACTIVE", "FAILED":
		instance.Status.RepStartFailCount++
		errMsg := fmt.Sprintf("Standby: Replication state incorrect (%s). Count: %d", rs, instance.Status.RepStartFailCount)
		reqLogger.Info(errMsg)
		logTTEvent(ctx, client, instance, "StateChange", errMsg, true)
		return FSMAnswer("Down"), errors.New("RepState " + rs), false
	case "RECOVERING", "IDLE":
		return FSMAnswer("CatchingUp"), nil, false
	case "STANDBY":
		if instance.Status.RepStartFailCount > 0 {
			instance.Status.RepStartFailCount = 0
			msg := "Standby: Replication started successfully"
			reqLogger.Info(msg)
			logTTEvent(ctx, client, instance, "StateChange", msg, false)
		}
		// if we're recovering from an upgrade failure, return HealthyStandby
		if instance.Status.ClassicUpgradeStatus.UpgradeState != "" &&
			strings.ToUpper(instance.Status.HighLevelState) == "REEXAMINE" {
			reqLogger.V(1).Info(fmt.Sprintf("standbyDownStandbyAS: this is an upgrade, returning HealthyStandby"))
			return FSMAnswer("HealthyStandby"), nil, true
		}

		return FSMAnswer("Healthy"), nil, true

	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	default:
		return "", errors.New("Unexpected RepState value '" + rs + "'"), false
	}
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for ACTIVE DOWN on the ACTIVE in an A/S PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func activeDownActiveAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("activeDownActiveAS starts")
	defer reqLogger.V(2).Info("activeDownActiveAS ends")

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	// Does instance exist?
	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	// Is daemon up?
	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// Is repagent running?
	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
		err := RunAction(ctx, instance, podNo, "stopRepAgent", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Down"), err, false
		}

	case "Not Running":
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Repagent " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	// Deactivate the active.

	_ = RunAction(ctx, instance, podNo, "repDeactivate", nil, client, tts, nil)

	// That was a last ditch best effort. Whether it worked or not the
	// node is dead.
	return FSMAnswer("Down"), nil, false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Let's check a Standby when state is StandbyStarting.
// The database has been duplicated and replication started;
// we need to wait until the repstate switches to STANDBY
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func standbyStartingStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("standbyStartingStandbyAS starts")
	defer reqLogger.V(2).Info("standbyStartingStandbyAS ends")

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	// Does instance exist?
	reqLogger.Info("TimesTen instance '" + instance.Status.PodStatus[podNo].TimesTenStatus.Instance + "'")
	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	// Is daemon up?
	reqLogger.Info("Daemon " + instance.Status.PodStatus[podNo].TimesTenStatus.Daemon)
	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	reqLogger.Info("Database " + instance.Status.PodStatus[podNo].DbStatus.Db)
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// Is repagent running?
	reqLogger.Info("repagent " + instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent)
	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		return FSMAnswer("Down"), errors.New("RepAgent " + ra), false
	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	// What's our repstate?
	reqLogger.Info("repstate " + instance.Status.PodStatus[podNo].ReplicationStatus.RepState)
	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
	case "ACTIVE", "FAILED":
		return FSMAnswer("Down"), errors.New("RepState " + rs), false
	case "RECOVERING", "IDLE":
		return FSMAnswer("CatchingUp"), nil, false
	case "STANDBY":
		return FSMAnswer("Healthy"), nil, false // Not all the way up yet
	case "Unknown":
		return FSMAnswer("Unknown"), nil, false
	default:
		return "", errors.New("Unexpected RepState value '" + rs + "'"), false
	}
}

func standbyCatchupStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("standbyCatchupStandbyAS starts")
	defer reqLogger.V(2).Info("standbyCatchupStandbyAS ends")

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	// Does instance exist?
	reqLogger.Info("TimesTen instance '" + instance.Status.PodStatus[podNo].TimesTenStatus.Instance + "'")
	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	// Is daemon up?
	reqLogger.Info("Daemon " + instance.Status.PodStatus[podNo].TimesTenStatus.Daemon)
	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	reqLogger.Info("Database " + instance.Status.PodStatus[podNo].DbStatus.Db)
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// Is repagent running?
	reqLogger.Info("repagent " + instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent)
	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		return FSMAnswer("Down"), errors.New("RepAgent " + ra), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	// What's our repstate?
	reqLogger.Info("repstate " + instance.Status.PodStatus[podNo].ReplicationStatus.RepState)
	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
	case "ACTIVE", "FAILED":
		return FSMAnswer("Down"), errors.New("RepState " + rs), false
	case "RECOVERING", "IDLE":
		return FSMAnswer("CatchingUp"), nil, false
	case "STANDBY":
		return FSMAnswer("Healthy"), nil, false // Not all the way up yet
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepState " + rs), false
	default:
		return "", errors.New("Unexpected RepState value '" + rs + "'"), false
	}
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for ACTIVE DOWN on the STANDBY in an A/S PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func checkActiveDownStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("checkActiveDownStandbyAS starts")
	defer reqLogger.V(2).Info("checkActiveDownStandbyAS ends")

	var err error

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
	case "STANDBY", "IDLE":
	case "FAILED", "RECOVERING", "ACTIVE":
		return FSMAnswer("Down"), errors.New("RepState " + rs), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepState " + rs), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepState value '" + rs + "'"), false
	}

	err = RunAction(ctx, instance, podNo, "repStateSetActive", nil, client, tts, nil)
	if err != nil {
		return FSMAnswer("Down"), err, false
	} else {
		return FSMAnswer("Healthy"), nil, false // wait until next time
	}

}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Check for NORMAL STANDBY in an ACTIVE STANDBY PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func checkNormalStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("checkNormalStandbyAS starts")
	defer reqLogger.V(2).Info("checkNormalStandbyAS ends")
	var err error

	podName := instance.Status.PodStatus[podNo].Name

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	} else {
		reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: pod %s is reachable", podName))
	}

	reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: instance.Status.PodStatus[%s].TimesTenStatus.Instance=%v", podName,
		instance.Status.PodStatus[podNo].TimesTenStatus.Instance))

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Down"), errors.New("Instance " + is), false
	case
		"Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: instance.Status.PodStatus[%s].TimesTenStatus.Daemon=%v", podName,
		instance.Status.PodStatus[podNo].TimesTenStatus.Daemon))

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?

	reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: instance.Status.PodStatus[%s].DbStatus.Db=%v", podName,
		instance.Status.PodStatus[podNo].DbStatus.Db))

	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		return FSMAnswer("Down"), errors.New("Db closed"), false
	}

	reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: instance.Status.PodStatus[%s].ReplicationStatus.RepScheme=%v", podName,
		instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme))

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rs {
	case "Exists":
	case "None":
		return FSMAnswer("Down"), errors.New("RepScheme " + rs), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepScheme " + rs), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme value '" + rs + "'"), false

	}

	reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: instance.Status.PodStatus[%s].ReplicationStatus.RepAgent=%v", podName,
		instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent))

	switch ras := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ras {
	case "Running":
	case "Not Running":
		return FSMAnswer("Down"), errors.New("RepAgent " + ras), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent " + ras), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ras + "'"), false
	}

	reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: instance.Status.PodStatus[%s].ReplicationStatus.RepState=%v", podName,
		instance.Status.PodStatus[podNo].ReplicationStatus.RepState))

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
	case "STANDBY":
	case "RECOVERING":
		if isRepStateTimeoutExceeded(ctx, instance, podNo) {
			return FSMAnswer("Down"), errors.New("RepState " + rs + " and timeout exceeded"), false
		} else {
			return FSMAnswer("Unknown"), errors.New("RepState " + rs), false
		}
	case "IDLE", "FAILED", "ACTIVE":
		return FSMAnswer("Down"), errors.New("RepState " + rs), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepState " + rs), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepState value '" + rs + "'"), false
	}

	reqLogger.V(2).Info(fmt.Sprintf("checkNormalStandbyAS: instance.Status.PodStatus[%s].ReplicationStatus.RepPeerPState=%v", podName,
		instance.Status.PodStatus[podNo].ReplicationStatus.RepPeerPState))

	switch rps := instance.Status.PodStatus[podNo].ReplicationStatus.RepPeerPState; rps {
	case "stop":
		return FSMAnswer("Down"), errors.New("RepPeerState " + rps), false
	case "failed":
		return FSMAnswer("OtherDown"), errors.New("RepPeerState " + rps), true // WE are fine
	case "pause":
		err = RunAction(ctx, instance, podNo, "setSubStartStart", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	case "start":
		return FSMAnswer("Healthy"), nil, true
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepPeerState " + rps), false
	default:
		return FSMAnswer("Down"), errors.New("Unexpected RepPeerPState value '" + rps + "'"), false
	}
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for CONFIGURE ACTIVE on the ACTIVE
// in an ACTIVE STANDBY PAIR. This is used when the instance
// that will now be 'active' was previous the 'active'
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func configureActiveActiveFromActiveAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "configureActiveActiveFromActiveAS"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "Unloaded":
		err = RunAction(ctx, instance, podNo, "loadDb", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "None", "Unloading":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// Is there a replication scheme? Create one if not.

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rs {
	case "Exists":
	case "None":
		err = RunAction(ctx, instance, podNo, "createRepScheme", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}

		err = RunAction(ctx, instance, podNo, "createRepEpilog", nil, client, tts, nil)
		if err != nil {
			reqLogger.Info(fmt.Sprintf("%s: RunAction createRepEpilog failed, err=%v", us, err))
			// We ignore the error
		} else {
			reqLogger.Info(us + ": RunAction createRepEpilog successful")
		}

	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepScheme " + rs), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme value '" + rs + "'"), false

	}

	// Is the repagent running? Start it if not.

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		err = RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	err = RunAction(ctx, instance, podNo, "repStateSetActive", nil, client, tts, nil)
	if err != nil {
		return FSMAnswer("Down"), err, false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	if instance.Status.PodStatus[podNo].CacheStatus.NCacheGroups > 0 {
		err = RunAction(ctx, instance, podNo, "startCacheAgent", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Healthy"), nil, false
		}
	}

	return FSMAnswer("Healthy"), nil, false

}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for CONFIGURE ACTIVE on the ACTIVE
// in an ACTIVE STANDBY PAIR. This is used when the instance
// that will now be 'active' was previous the 'standby'
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func configureActiveActiveFromStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "configureActiveActiveFromStandbyAS"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "Unloaded":
		err = RunAction(ctx, instance, podNo, "loadDb", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "None", "Unloading":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// If the repagent is running stop it

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
		err = RunAction(ctx, instance, podNo, "stopRepAgent", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Not Running":
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	// If the cache agent is running stop it

	switch ca := instance.Status.PodStatus[podNo].CacheStatus.CacheAgent; ca {
	case "Running":
		err = RunAction(ctx, instance, podNo, "stopCacheAgent", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Not Running":
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("CacheAgent " + ca), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected CacheAgent value '" + ca + "'"), false
	}

	// Drop the active standby pair

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rs {
	case "Exists":
		err = RunAction(ctx, instance, podNo, "dropRepScheme", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "None":
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepScheme " + rs), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepScheme value '" + rs + "'"), false
	}

	// Drop all cache groups

	if instance.Status.PodStatus[podNo].CacheStatus.NCacheGroups > 0 {
		err = RunAction(ctx, instance, podNo, "dropCg", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	}

	// Re-create the cache groups (re-run the user's cachegroups.sql file)

	if instance.Status.PodStatus[podNo].CacheGroupsFile {
		err = RunAction(ctx, instance, podNo, "createCg", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	}

	// Create the replication scheme

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rs {
	case "Exists":
		return FSMAnswer("Terminal"), errors.New("Repscheme existed and should not"), false
	case "None":
		err = RunAction(ctx, instance, podNo, "createRepScheme", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}

		err = RunAction(ctx, instance, podNo, "createRepEpilog", nil, client, tts, nil)
		if err != nil {
			reqLogger.Info(fmt.Sprintf(us+": RunAction createRepEpilog failed, err=%v", err))
			// We ignore the error
		} else {
			reqLogger.Info(us + ": RunAction createRepEpilog successful")
		}

	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepScheme " + rs), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme value '" + rs + "'"), false

	}

	// Make this the active db

	err = RunAction(ctx, instance, podNo, "repStateSetActive", nil, client, tts, nil)
	if err != nil {
		return FSMAnswer("Down"), err, false
		//SAMDRAKE 2022-05-20. New operator-sdk v1 noted that the code below here
		//SAMDRAKE is unreachable. Good bug catch by the sdk!
		//SAMDRAKE    } else {
		//SAMDRAKEreturn FSMAnswer("Healthy"), nil, false
	}

	// Is the repagent running? Start it if not.

	switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
	case "Running":
	case "Not Running":
		err = RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("RepAgent " + ra), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ra + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	// Start the cache agent if appropriate

	switch ca := instance.Status.PodStatus[podNo].CacheStatus.CacheAgent; ca {
	case "Running":
	case "Not Running":
		if instance.Status.PodStatus[podNo].CacheStatus.NCacheGroups > 0 {
			err = RunAction(ctx, instance, podNo, "startCacheAgent", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Down"), err, false
			}
		}
	case "Unknown":
	default:
		return FSMAnswer(""),
			errors.New("Unexpected CacheAgent value '" + ca + "'"), false
	}

	return FSMAnswer("Healthy"), nil, false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for CONFIGURE ACTIVE on the STANDBY
// in an ACTIVE STANDBY PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func configureActiveStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("configureActiveStandbyAS starts")
	defer reqLogger.V(2).Info("configureActiveStandbyAS ends")

	return FSMAnswer("Unknown"), errors.New("Active not configured yet"), false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for INITIALIZE ACTIVE in an ACTIVE STANDBY PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func initializeActiveAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "initializeActiveAS"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error

	if ok, _ := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), nil, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		return FSMAnswer("Down"), nil, false
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		err = RunAction(ctx, instance, podNo, "createDb", nil, client, tts, nil)
		if err != nil {
			// RunAction should have already logged this
			//logTTEvent(ctx, client, instance, "Error", "Error creating database: " + err.Error(), true)
			return FSMAnswer("Terminal"), err, false
		}
		// Did the db actually create?
		switch newdbs := instance.Status.PodStatus[podNo].DbStatus.Db; newdbs {
		case "Unknown":
			logTTEvent(ctx, client, instance, "Error", fmt.Sprintf("Db created but status 'Unknown'"), true)
			return FSMAnswer("Terminal"), err, false
		default:
		}
	case "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	if instance.Status.PodStatus[podNo].CacheStatus.NCacheGroups == 0 {
		if instance.Status.PodStatus[podNo].CacheGroupsFile {
			err = RunAction(ctx, instance, podNo, "createCg", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				return FSMAnswer("Unknown"), nil, false
			}
		} else {
			// No cache groups file, so nothing to do
		}
	}

	if instance.Spec.TTSpec.ReplicationTopology != nil &&
		*instance.Spec.TTSpec.ReplicationTopology == "none" {
		reqLogger.Info(us + ": not creating replication scheme due to replicationTopology")
		return FSMAnswer("Healthy"), nil, false
	} else {

		// Is there a replication scheme? Create one if not.

		switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rs {
		case "Exists":
		case "None":
			err = RunAction(ctx, instance, podNo, "createRepScheme", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			}

			err = RunAction(ctx, instance, podNo, "createRepEpilog", nil, client, tts, nil)
			if err != nil {
				reqLogger.Info(fmt.Sprintf("%s: RunAction createRepEpilog failed, err=%v", us, err))
				// We ignore the error
			} else {
				reqLogger.Info(us + ": RunAction createRepEpilog successful")
			}

		case "Unknown":
			return FSMAnswer("Unknown"), errors.New("RepScheme " + rs), false
		default:
			return FSMAnswer(""), errors.New("Unexpected RepScheme value '" + rs + "'"), false

		}

		// Is the repagent running? Start it if not.

		switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
		case "Running":
		case "Not Running":
			return FSMAnswer("Down"), errors.New("RepAgent " + ra), false
		case "Unknown":
			// IS THIS RIGHT?
			err = RunAction(ctx, instance, podNo, "createRepScheme", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			}
		default:
			return FSMAnswer(""),
				errors.New("Unexpected RepAgent value '" + ra + "'"), false
		}

		// Do we think our peer is OK?

		switch rps := instance.Status.PodStatus[podNo].ReplicationStatus.RepPeerPState; rps {
		case "stop", "failed", "pause":
			return FSMAnswer("Unknown"), errors.New("RepPeerState " + rps), false
		case "start":
			return FSMAnswer("Healthy"), nil, false // Wait until next time
		case "Unknown":
			return FSMAnswer("Unknown"), errors.New("RepPeerState " + rps), false
		default:
			return FSMAnswer("Down"), errors.New("Unexpected RepPeerPState value '" + rps + "'"), false
		}
	}
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for INITIALIZE in an NON-REPLICATED OBJECT
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func nonrepInitializing(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "nonrepInitializing"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error

	if ok, _ := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Initializing"), nil, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		// DO NOT call isReachableTimeoutExceeded here; it compares current time to last
		// pod reachable time but we're initializing and we have a never been reachable
		return FSMAnswer("Initializing"), nil, false
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Initializing"), errors.New("Instance " + is), false
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		}
	case "Unknown":
		return FSMAnswer("Initializing"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		err = RunAction(ctx, instance, podNo, "createDb", nil, client, tts, nil)
		if err != nil {
			// RunAction should have already logged this
			//logTTEvent(ctx, client, instance, "Error", "Error creating database: " + err.Error(), "Error")
			return FSMAnswer("Terminal"), err, false
		}
	case "Unloading", "Unloaded":
		return FSMAnswer("Terminal"), errors.New("Database " + dbs), false // Why would that happen during initialization?
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Initializing"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// Is db open?

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	if instance.Status.PodStatus[podNo].CacheStatus.NCacheGroups == 0 {
		if instance.Status.PodStatus[podNo].CacheGroupsFile {
			err = RunAction(ctx, instance, podNo, "createCg", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			}
		} else {
			// No cache groups file, so nothing to do
		}
	}

	return FSMAnswer("Normal"), nil, true
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for DOWN in an NON-REPLICATED OBJECT
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func nonrepDown(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "nonrepDown"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	var err error

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		// Last time we saw it it was down, so we still consider it down
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Down"), nil, false
		}
	}

	if instance.Status.PodStatus[podNo].NonRepUpgradeFailed == true {
		// TODO: read the contents of the failure file and issue a more accurate event desc
		errMsg := fmt.Sprintf("Upgrade Failed on POD %d", podNo)
		reqLogger.Info(us + ": " + errMsg)
		logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)

		return FSMAnswer("ManualInterventionRequired"), err, false
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Down"), errors.New("Instance " + is), false
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		} else {
			return FSMAnswer("Down"), nil, false
		}
	case "Unknown":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		return FSMAnswer("Terminal"), errors.New("No database, where did it go?"), false
	case "Unloading":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Unloaded":
		reqLogger.V(2).Info(fmt.Sprintf("%s DbStatus.Db for pod %d is %s, call loadDb", us, podNo, dbs))
		err = RunAction(ctx, instance, podNo, "loadDb", nil, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s loadDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("ManualInterventionRequired"), err, false
		}
		// Fall through to starting the cache agent
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// Is db open?

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("ManualInterventionRequired"), err, false
		}
	}

	// SAMDRAKE Cache Agent?

	return FSMAnswer("Normal"), nil, true
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for REEXAMINE
// in a non-replicated STANDALONE database configuration
//
// The pair was in "ManualInterventionRequired" state, and the user
// fiddled with it in some way (failed upgrade). We have no idea what
// the state of the databases / instances is. Whatever it is we need to
// react to it; if the database is healthy then we will switch back to "Normal".
// If anything is wrong we'll flip back to "ManualInterventionRequired".
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func reexamineNonrep(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	us := "reexamineNonrep"
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " ends")

	var err error

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	if instance.Status.PodStatus[podNo].NonRepUpgradeFailed == true {

		// if the TT version of the image matches the installed version, then starthost did not mark the upgrade as failed
		if instance.Status.PodStatus[podNo].ImageRelease == instance.Status.PodStatus[podNo].InstallRelease {
			reqLogger.Info(fmt.Sprintf("%s: pod %d ImageRelease %s = InstallRelease %s, remove nonRepUpgradeFailed file",
				us, podNo, instance.Status.PodStatus[podNo].ImageRelease, instance.Status.PodStatus[podNo].InstallRelease))

			err = RunAction(ctx, instance, podNo, "removeNonRepUpgradeFailedFile", nil, client, tts, nil)
			if err != nil {
				reqLogger.V(2).Info(fmt.Sprintf("%s: failed to remove nonRepUpgradeFailed file on pod %d", us, podNo))
				return FSMAnswer("ManualInterventionRequired"), err, false
			}
			reqLogger.Info(fmt.Sprintf("%s: removed nonRepUpgradeFailed file on pod %d", us, podNo))
		} else {

			reqLogger.Info(fmt.Sprintf("%s: pod %d ImageRelease %s != InstallRelease %s, did not remove nonRepUpgradeFailed file",
				us, podNo, instance.Status.PodStatus[podNo].ImageRelease, instance.Status.PodStatus[podNo].InstallRelease))

			// TODO: event should indicate the version mismatch
			errMsg := fmt.Sprintf("Upgrade Failed on POD %d", podNo)
			reqLogger.Info(fmt.Sprintf("%s: %s", us, errMsg))
			logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)

			return FSMAnswer("ManualInterventionRequired"), err, false
		}

	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Down"), errors.New("Instance " + is), false
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		} else {
			return FSMAnswer("Down"), nil, false
		}
	case "Unknown":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		return FSMAnswer("Terminal"), errors.New("No database, where did it go?"), false
	case "Unloading":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Unloaded":
		reqLogger.V(2).Info(fmt.Sprintf("%s DbStatus.Db for pod %d is %s, call loadDb", us, podNo, dbs))
		err = RunAction(ctx, instance, podNo, "loadDb", nil, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s loadDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("ManualInterventionRequired"), err, false
		}
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	// Is db open?

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.V(2).Info(fmt.Sprintf("%s openDb for pod %d returned an err %v, return ManualInterventionRequired", us, podNo, err))
			return FSMAnswer("Terminal"), err, false
		}
	}

	return FSMAnswer("Normal"), nil, true

}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for ACTIVE in ActiveTakeover state
// in an ACTIVE STANDBY PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func activeTakeoverActiveAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("activeTakeoverActiveAS starts")
	defer reqLogger.V(2).Info("activeTakeoverActiveAS ends")

	_ = RunAction(ctx, instance, podNo, "repStateSave", nil, client, tts, nil)

	return FSMAnswer("Healthy"), nil, true
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for REEXAMINE
// in an ACTIVE STANDBY PAIR
//
// The pair was in "ManualInterventionRequired" state, and the user
// fiddled with it in some way. We have no idea what the state of the
// databases / instances is. Whatever it is we need to react to it;
// if they are both healthy then we will switch back to "Normal".
// If anything is wrong we'll flip back to "ManualInterventionRequired".
//
// This action is called on both the (former) active and the (former) standby.
// Our job is to report whether this is a sane active, a sane standby,
// or a broken anything.
//
// We return some Answers that aren't used for anything else:
//
// HealthyActive  - This is indeed a healthy active. If the corresponding standby
//                  were also to be healthy then this is a Normal pair
// HealthyStandby - This is indeed a healthy standby. If the corresponding active
//                  were also to be healthy then this is a Normal pair
// HealthyIdle   -  This is a loaded and usable TimesTen database but it's 'idle'
//                  in the replication sense. If it's the only such database
//                  in the pair then it would be a good candidate as the new 'active'
//
// ... in addition to normal ones like Down
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func reexamineAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("reexamineAS starts")
	defer reqLogger.V(2).Info("reexamineAS ends")

	var thisIs string

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Down"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Down"), errors.New("Instance " + is), false
	default:
		return "", errors.New("Unexpected Instance value '" + is + "'"), false
	}

	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	case "Unknown":
		return FSMAnswer("Down"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist (and is it loaded)?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None", "Unloading", "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loading", "Transitioning", "Unknown":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false
	case "Loaded":
	default:
		return FSMAnswer(""), errors.New("Unexpected Db value '" + dbs + "'"), false
	}

	if instance.Status.PodStatus[podNo].DbStatus.DbOpen {
		// Great!
	} else {
		return FSMAnswer("Down"), errors.New("Db closed"), false
	}

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepScheme; rs {
	case "Exists":
	case "None":
		// If the other conditions match then this will be "HealthyIdle".
		// If this is the only db in the pair then we will eventually make
		// it the new 'active'

		if instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent == "Not Running" &&
			instance.Status.PodStatus[podNo].ReplicationStatus.RepState == "IDLE" {
			return FSMAnswer("HealthyIdle"), nil, false
		} else {
			return FSMAnswer("Down"), errors.New("RepScheme " + rs), false
		}
	case "Unknown":
		return FSMAnswer("Down"), errors.New("RepScheme " + rs), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepScheme value '" + rs + "'"), false

	}

	switch ras := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ras {
	case "Running":
	case "Not Running":
		return FSMAnswer("Down"), errors.New("RepAgent " + ras), false
	case "Unknown":
		return FSMAnswer("Down"), errors.New("RepAgent " + ras), false
	default:
		return FSMAnswer(""),
			errors.New("Unexpected RepAgent value '" + ras + "'"), false
	}

	switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
	case "ACTIVE":
		thisIs = "Active"
	case "STANDBY":
		thisIs = "Standby"
	case "RECOVERING", "IDLE", "FAILED", "Unknown":
		return FSMAnswer("Down"), errors.New("RepState " + rs), false
	default:
		return FSMAnswer(""), errors.New("Unexpected RepState value '" + rs + "'"), false
	}

	switch rps := instance.Status.PodStatus[podNo].ReplicationStatus.RepPeerPState; rps {
	case "start":
		return FSMAnswer("Healthy" + thisIs), nil, false
	case "stop", "failed", "pause", "Unknown":
		return FSMAnswer("Down"), errors.New("RepPeerState " + rps), false
	default:
		return FSMAnswer("Down"), errors.New("Unexpected RepPeerPState value '" + rps + "'"), false
	}

}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for STANDBY in ActiveTakeover state
// in an ACTIVE STANDBY PAIR
//
// The standby needs to commit suicide, the active has declared
// it dead.
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func killDeadStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("killDeadStandbyAS starts")
	defer reqLogger.V(2).Info("killDeadStandbyAS ends")

	if ok, _ := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), nil, false
	}

	// This will also kill EVERYTHING running in the pod, including
	// any direct mode applications in customer containers. See bug 32214419.

	_ = RunAction(ctx, instance, podNo, "stopDaemon", nil, client, tts, nil)

	return FSMAnswer("Down"), nil, false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for INITIALIZE STANDBY in an ACTIVE STANDBY PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func initializeStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "initializeStandbyAS"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	podName := instance.Status.PodStatus[podNo].Name

	var err error
	var otherPodNo int
	if strings.HasSuffix(podName, "-0") {
		otherPodNo = 1
	} else {
		otherPodNo = 0
	}

	if ok, _ := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), nil, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		return FSMAnswer("Down"), nil, false
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), err, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Instance value '" + is + "'"), false
	}

	// Daemon running?
	switch ds := instance.Status.PodStatus[podNo].TimesTenStatus.Daemon; ds {
	case "Up":
	case "Down":
		err = RunAction(ctx, instance, podNo, "startDaemon", nil, client, tts, nil)
		if err != nil {
			return FSMAnswer("Terminal"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}

	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Daemon " + ds), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Daemon value '" + ds + "'"), false
	}

	// Does db exist?
	switch dbs := instance.Status.PodStatus[podNo].DbStatus.Db; dbs {
	case "None":
		// Is the 'active' ready?
		switch otherhls := instance.Status.PodStatus[otherPodNo].HighLevelState; otherhls {
		case "Healthy", "HealthyActive":
			// Destroy database just in case
			_ = RunAction(ctx, instance, podNo, "destroyDb", nil, client, tts, nil)

			// Duplicate database
			err = RunAction(ctx, instance, podNo, "repDuplicate", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				return FSMAnswer("Unknown"), nil, false
			}
		case "Terminal":
			return FSMAnswer("Terminal"), errors.New("Active pod is Terminal"), false
		case "Down", "OtherDown":
			return FSMAnswer("Down"), nil, false // No need to report that the active isn't up yet
		case "Unknown":
			return FSMAnswer("Unknown"), nil, false
		default:
			return FSMAnswer("Terminal"), errors.New("OtherHLS unexpected value " + otherhls), false
		}
	case "Loaded":
		// Is repagent running?
		switch ra := instance.Status.PodStatus[podNo].ReplicationStatus.RepAgent; ra {
		case "Running":
			switch rs := instance.Status.PodStatus[podNo].ReplicationStatus.RepState; rs {
			case "STANDBY":
				return FSMAnswer("Healthy"), nil, false
			case "FAILED", "RECOVERING", "ACTIVE":
				return FSMAnswer("Down"), errors.New("RepState " + rs), false
			case "IDLE", "Unknown":
				return FSMAnswer("Unknown"), errors.New("RepState " + rs), false
			default:
				return FSMAnswer("Terminal"), errors.New("Unexpected RepState value '" + rs + "'"), false
			}

		case "Not Running":
			err = RunAction(ctx, instance, podNo, "startRepAgent", nil, client, tts, nil)
			if err != nil {
				return FSMAnswer("Terminal"), err, false
			} else {
				return FSMAnswer("Unknown"), nil, false
			}
		case "Unknown":
			return FSMAnswer("Unknown"), errors.New("RepAgent " + ra), false
		default:
			return FSMAnswer("Terminal"),
				errors.New("Unexpected RepAgent value '" + ra + "'"), false
		}

	case "Unloaded":
		return FSMAnswer("Down"), errors.New("Database " + dbs), false

	case "Loading", "Transitioning", "Unloading", "Unknown":
		return FSMAnswer("Unknown"), errors.New("Database " + dbs), false
	default:
		return FSMAnswer("Terminal"), errors.New("Unexpected Db value '" + dbs + "'"), false
	}
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for WAITING FOR ACTIVE on ACTIVE in an A/S PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func waitingActiveAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("waitingActiveAS starts")
	defer reqLogger.V(2).Info("waitingActiveAS ends")

	// We'll wait until the pod is reachable and the agent in it is up.
	// Until then we'll stay in WaitingForActive state. Once the agent is
	// up we'll switch to ConfiguringActive state.

	if ok, err := isPodRunning(instance, podNo); !ok {
		return FSMAnswer("Down"), err, false
	}

	// Is this pod reachable?
	if ok, _ := isPodReachable(ctx, instance, podNo); !ok {
		if ok, err := isReachableTimeoutExceeded(ctx, instance, podNo); ok {
			return FSMAnswer("Down"), err, false
		} else {
			return FSMAnswer("Unknown"), nil, false
		}
	}

	if ok, _ := isPodQuiescing(ctx, instance, podNo); ok {
		return FSMAnswer("Unknown"), nil, false // Take no action until it finally dies
	}

	switch is := instance.Status.PodStatus[podNo].TimesTenStatus.Instance; is {
	case "Exists":
	case "Missing":
		return FSMAnswer("Terminal"), errors.New("Instance " + is), false
	case "Unknown":
		return FSMAnswer("Unknown"), errors.New("Instance " + is), false
	default:
		return FSMAnswer(""), errors.New("Unexpected Instance value '" + is + "'"), false
	}

	return FSMAnswer("Healthy"), nil, false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for WAITING FOR ACTIVE on STANDBY in an A/S PAIR
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func waitingStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("waitingStandbyAS starts")
	defer reqLogger.V(2).Info("waitingStandbyAS ends")

	//var err error
	//us := "waitingStandbyAS"
	//otherPodNo := 0
	//if podNo == 0 {
	//  otherPodNo = 1
	//}

	// If we're 'waiting for active' then the state of the standby is irrelevant

	return FSMAnswer("Unknown"), errors.New("Waiting for active"), false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for BOTH DOWN on ACTIVE in an A/S PAIR
// Note that the real work for figuring out what to do after BothDown
// is in determineNewHLStateReplicated. There we have to decide which
// instance is eligible to be the new 'active' (if any), and will switch
// to a new high level state there.
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func bothDownActiveAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	us := "bothDownActiveAS"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	return FSMAnswer(instance.Status.PodStatus[podNo].HighLevelState), nil, false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//
// Flowchart / state machine for BOTH DOWN on STANDBY in an A/S PAIR
// Note that the real work for figuring out what to do after BothDown
// is in determineNewHLStateReplicated. There we have to decide which
// instance is eligible to be the new 'active' (if any), and will switch
// to a new high level state there.
//
//----------------------------------------------------------------------
//----------------------------------------------------------------------
//----------------------------------------------------------------------

func bothDownStandbyAS(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, podNo int, tts *TTSecretInfo) (FSMAnswer, error, bool) {
	reqLogger := log.FromContext(ctx)
	us := "bothDownStandbyAS"
	reqLogger.V(2).Info(us + " starts")
	defer reqLogger.V(2).Info(us + " ends")

	return FSMAnswer(instance.Status.PodStatus[podNo].HighLevelState), nil, false
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
// Create the high level state transition table
//----------------------------------------------------------------------
//----------------------------------------------------------------------

// This is the definitive list of all possible high level states for replicated
// TimesTenClassic objects

var ClassicHLStates map[string]int = map[string]int{
	"Initializing":               1,
	"Normal":                     1,
	"ActiveDown":                 1,
	"StandbyDown":                1,
	"OneDown":                    1,
	"BothDown":                   1,
	"StandbyStarting":            1,
	"Failed":                     1,
	"ActiveTakeover":             1,
	"StandbyCatchup":             1,
	"ManualInterventionRequired": 1,
	"WaitingForActive":           1,
	"Reexamine":                  1,
	"ConfiguringActive":          1,
}

// This is the definitive list of all possible high level states for pods in standalone
// TimesTenClassic objects

var StandaloneHLStates map[string]int = map[string]int{
	"Initializing":               1,
	"Normal":                     1,
	"Down":                       1,
	"Failed":                     1,
	"ManualInterventionRequired": 1,
	"Reexamine":                  1,
}

func init() {

	// This is the definitive list of all states of a pod in a replicated object

	podstates = map[string]int{
		"Healthy":        1,
		"Down":           1,
		"OtherDown":      1,
		"Unknown":        1,
		"Terminal":       1,
		"UpgradeFailed":  1,
		"CatchingUp":     1,
		"HealthyActive":  1,
		"HealthyStandby": 1,
	}

	upgradeHLstates = map[string]int{
		"UpgradingActive":  1,
		"UpgradingStandby": 1,
		"Complete":         1,
	}

	upgradestates = map[string]int{
		"deleteStandby": 1,
		"deleteActive":  1,
		"processing":    1, // pod initialization in progress, waiting for AS hlstates Standbydown->Normal
		"CatchingUp":    1,
		"failed":        1,
		"success":       1,
		"waiting":       1, // the active is waiting for the standby to finish an upgrade
		"unknown":       1,
	}

	// This structure controls how replicated active standby pairs are
	// managed.
	// This data structure defines the new high level state of the pair
	// given three items:
	// - The current high level state of the pair
	// - The current state of the ACTIVE pod
	// - The current state of the STANDBY pod
	//
	// Note that if we're in an active/active environment the pod states are
	// in random order

	// Multi-dimensional maps are weird, so first we initialize everything
	highLevelStateMachine = make(map[string]map[string]map[string]string)
	for hl, _ := range ClassicHLStates {
		highLevelStateMachine[hl] = make(map[string]map[string]string)
		for p1, _ := range podstates {
			highLevelStateMachine[hl][p1] = make(map[string]string)
			for p2, _ := range podstates {
				highLevelStateMachine[hl][p1][p2] = ""
			}
		}
	}

	// From the "Failed" state you can only get to the "Failed" state.
	// It's a one-way trip

	for p1, _ := range podstates {
		for p2, _ := range podstates {
			highLevelStateMachine["Failed"][p1][p2] = "Failed"
		}
	}

	highLevelStateMachine["Initializing"]["Healthy"]["Healthy"] = "Normal"
	highLevelStateMachine["Initializing"]["Healthy"]["Down"] = "Initializing"
	highLevelStateMachine["Initializing"]["Healthy"]["OtherDown"] = "Initializing"
	highLevelStateMachine["Initializing"]["Healthy"]["Unknown"] = "Initializing"
	highLevelStateMachine["Initializing"]["Healthy"]["Terminal"] = "Failed"
	highLevelStateMachine["Initializing"]["Down"]["Healthy"] = "Initializing"
	highLevelStateMachine["Initializing"]["Down"]["Down"] = "Initializing"
	highLevelStateMachine["Initializing"]["Down"]["OtherDown"] = "Initializing"
	highLevelStateMachine["Initializing"]["Down"]["Unknown"] = "Initializing"
	highLevelStateMachine["Initializing"]["Down"]["Terminal"] = "Failed"
	highLevelStateMachine["Initializing"]["Down"]["CatchingUp"] = ""
	highLevelStateMachine["Initializing"]["OtherDown"]["Healthy"] = "Initializing"
	highLevelStateMachine["Initializing"]["OtherDown"]["Down"] = "Initializing"
	highLevelStateMachine["Initializing"]["OtherDown"]["OtherDown"] = "Initializing"
	highLevelStateMachine["Initializing"]["OtherDown"]["Unknown"] = "Initializing"
	highLevelStateMachine["Initializing"]["OtherDown"]["Terminal"] = "Failed"
	highLevelStateMachine["Initializing"]["OtherDown"]["CatchingUp"] = ""
	highLevelStateMachine["Initializing"]["Unknown"]["Healthy"] = "Initializing"
	highLevelStateMachine["Initializing"]["Unknown"]["Down"] = "Initializing"
	highLevelStateMachine["Initializing"]["Unknown"]["OtherDown"] = "Initializing"
	highLevelStateMachine["Initializing"]["Unknown"]["Unknown"] = "Initializing"
	highLevelStateMachine["Initializing"]["Unknown"]["Terminal"] = "Failed"
	highLevelStateMachine["Initializing"]["Unknown"]["CatchingUp"] = "Failed"
	highLevelStateMachine["Initializing"]["Terminal"]["Healthy"] = "Failed"
	highLevelStateMachine["Initializing"]["Terminal"]["Down"] = "Failed"
	highLevelStateMachine["Initializing"]["Terminal"]["OtherDown"] = "Failed"
	highLevelStateMachine["Initializing"]["Terminal"]["Unknown"] = "Failed"
	highLevelStateMachine["Initializing"]["Terminal"]["Terminal"] = "Failed"
	highLevelStateMachine["Initializing"]["Terminal"]["CatchingUp"] = "Failed"
	highLevelStateMachine["Initializing"]["CatchingUp"]["Healthy"] = "Failed"
	highLevelStateMachine["Initializing"]["CatchingUp"]["Down"] = "Failed"
	highLevelStateMachine["Initializing"]["CatchingUp"]["OtherDown"] = "Failed"
	highLevelStateMachine["Initializing"]["CatchingUp"]["Unknown"] = "Failed"
	highLevelStateMachine["Initializing"]["CatchingUp"]["Terminal"] = "Failed"
	highLevelStateMachine["Initializing"]["CatchingUp"]["CatchingUp"] = "Failed"

	highLevelStateMachine["StandbyStarting"]["Healthy"]["Healthy"] = "Normal"
	highLevelStateMachine["StandbyStarting"]["Healthy"]["Down"] = "StandbyDown"
	highLevelStateMachine["StandbyStarting"]["Healthy"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["Healthy"]["Unknown"] = "StandbyStarting"
	highLevelStateMachine["StandbyStarting"]["Healthy"]["Terminal"] = "StandbyDown"
	highLevelStateMachine["StandbyStarting"]["Healthy"]["CatchingUp"] = "StandbyCatchup"

	//
	highLevelStateMachine["StandbyStarting"]["Down"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["Down"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["StandbyStarting"]["Down"]["Unknown"] = "StandbyStarting"

	//
	highLevelStateMachine["StandbyStarting"]["Down"]["Terminal"] = "ManualInterventionRequired"
	//

	// See bug 33328303. Previously this was "WaitingForActive", which makes sense but doesnt
	// quite work. At some point it should. Until then make the human fix this.
	// The standby is NOT the best db (it's just "catching up", after all), so we need to wait
	// for the active to come back and make it be the active again - else we lose data.
	highLevelStateMachine["StandbyStarting"]["Down"]["CatchingUp"] = "ManualInterventionRequired"

	//
	highLevelStateMachine["StandbyStarting"]["OtherDown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["StandbyStarting"]["OtherDown"]["Unknown"] = "StandbyStarting"

	//
	highLevelStateMachine["StandbyStarting"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["Unknown"]["Healthy"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["StandbyStarting"]["Unknown"]["Down"] = "StandbyStarting"
	highLevelStateMachine["StandbyStarting"]["Unknown"]["OtherDown"] = "StandbyStarting"
	highLevelStateMachine["StandbyStarting"]["Unknown"]["Unknown"] = "StandbyStarting"
	highLevelStateMachine["StandbyStarting"]["Unknown"]["Terminal"] = "StandbyStarting"
	highLevelStateMachine["StandbyStarting"]["Unknown"]["CatchingUp"] = "StandbyStarting"

	//
	highLevelStateMachine["StandbyStarting"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["StandbyStarting"]["Terminal"]["Unknown"] = "StandbyStarting"

	//
	highLevelStateMachine["StandbyStarting"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["StandbyStarting"]["CatchingUp"]["Unknown"] = "StandbyStarting"

	//
	highLevelStateMachine["StandbyStarting"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyStarting"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//

	// These entries for BothDown aren't really used. BothDown handling is processed
	// in determineNewHLStateReplicated, which decides if the pair CAN be recovered,
	// and which instance should be the 'active' if so, and either switches to
	// WaitingForActive or ManualInterventionRequired depending on what it finds out
	// So these entries will never be used.
	highLevelStateMachine["BothDown"]["Healthy"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Healthy"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Healthy"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Healthy"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Healthy"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Down"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Down"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Down"]["Unknown"] = "BothDown" // Stay where we are until we know more
	highLevelStateMachine["BothDown"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["OtherDown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["OtherDown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Unknown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Unknown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Unknown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Unknown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Unknown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Unknown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Terminal"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["CatchingUp"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["BothDown"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"

	//
	highLevelStateMachine["OneDown"]["Healthy"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Healthy"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Healthy"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Healthy"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Healthy"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Down"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Down"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Down"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["OtherDown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["OtherDown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Unknown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Unknown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Unknown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Unknown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Unknown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Unknown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Terminal"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["CatchingUp"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["OneDown"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["StandbyDown"]["Healthy"]["Healthy"] = "Normal"
	highLevelStateMachine["StandbyDown"]["Healthy"]["Down"] = "StandbyDown"
	highLevelStateMachine["StandbyDown"]["Healthy"]["OtherDown"] = "ActiveDown"
	highLevelStateMachine["StandbyDown"]["Healthy"]["Unknown"] = "StandbyDown"

	//
	highLevelStateMachine["StandbyDown"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["StandbyDown"]["Healthy"]["CatchingUp"] = "StandbyStarting"

	// auto upgrade - StandbyDown is the common state during upgrades, we're always waiting for the new pod
	// UpgradeFailed : patch incompatible
	highLevelStateMachine["StandbyDown"]["Healthy"]["UpgradeFailed"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["UpgradeFailed"]["Healthy"] = "ManualInterventionRequired"

	highLevelStateMachine["StandbyDown"]["Down"]["Healthy"] = "ActiveDown"
	highLevelStateMachine["StandbyDown"]["Down"]["Down"] = "BothDown"
	highLevelStateMachine["StandbyDown"]["Down"]["OtherDown"] = "ActiveDown"
	highLevelStateMachine["StandbyDown"]["Down"]["Unknown"] = "StandbyDown"
	//
	highLevelStateMachine["StandbyDown"]["Down"]["Terminal"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyDown"]["Down"]["CatchingUp"] = "Failed" // Seen in Igor's 1000 connection test 5/22/2020
	highLevelStateMachine["StandbyDown"]["OtherDown"]["Healthy"] = "StandbyDown"
	highLevelStateMachine["StandbyDown"]["OtherDown"]["Down"] = "StandbyDown"
	//
	highLevelStateMachine["StandbyDown"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyDown"]["OtherDown"]["Unknown"] = "StandbyDown"
	//
	highLevelStateMachine["StandbyDown"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyDown"]["OtherDown"]["CatchingUp"] = "StandbyStarting"
	highLevelStateMachine["StandbyDown"]["Unknown"]["Healthy"] = "StandbyDown"
	highLevelStateMachine["StandbyDown"]["Unknown"]["Down"] = "StandbyDown"
	highLevelStateMachine["StandbyDown"]["Unknown"]["OtherDown"] = "StandbyDown"
	highLevelStateMachine["StandbyDown"]["Unknown"]["Unknown"] = "StandbyDown"
	highLevelStateMachine["StandbyDown"]["Unknown"]["Terminal"] = "StandbyDown"
	highLevelStateMachine["StandbyDown"]["Unknown"]["CatchingUp"] = "StandbyStarting"
	//
	highLevelStateMachine["StandbyDown"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyDown"]["Terminal"]["Unknown"] = "StandbyDown"
	//
	highLevelStateMachine["StandbyDown"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyDown"]["CatchingUp"]["Unknown"] = "StandbyDown"
	//
	highLevelStateMachine["StandbyDown"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyDown"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyCatchup"]["Healthy"]["Healthy"] = "Normal"
	highLevelStateMachine["StandbyCatchup"]["Healthy"]["Down"] = "StandbyDown"
	highLevelStateMachine["StandbyCatchup"]["Healthy"]["OtherDown"] = "ActiveDown"
	highLevelStateMachine["StandbyCatchup"]["Healthy"]["Unknown"] = "StandbyCatchup"
	highLevelStateMachine["StandbyCatchup"]["Healthy"]["Terminal"] = "StandbyDown"
	highLevelStateMachine["StandbyCatchup"]["Healthy"]["CatchingUp"] = "StandbyCatchup"
	highLevelStateMachine["StandbyCatchup"]["Down"]["Healthy"] = "ActiveDown"
	highLevelStateMachine["StandbyCatchup"]["Down"]["Down"] = "BothDown"
	highLevelStateMachine["StandbyCatchup"]["Down"]["OtherDown"] = "ActiveDown"
	highLevelStateMachine["StandbyCatchup"]["Down"]["Unknown"] = "StandbyCatchup"
	//
	highLevelStateMachine["StandbyCatchup"]["Down"]["Terminal"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyCatchup"]["Down"]["CatchingUp"] = "Failed"
	highLevelStateMachine["StandbyCatchup"]["OtherDown"]["Healthy"] = "StandbyCatchup"
	highLevelStateMachine["StandbyCatchup"]["OtherDown"]["Down"] = "StandbyDown"
	highLevelStateMachine["StandbyCatchup"]["OtherDown"]["OtherDown"] = "Failed"
	highLevelStateMachine["StandbyCatchup"]["OtherDown"]["Unknown"] = "StandbyCatchup"
	highLevelStateMachine["StandbyCatchup"]["OtherDown"]["Terminal"] = "StandbyDown"
	highLevelStateMachine["StandbyCatchup"]["OtherDown"]["CatchingUp"] = "StandbyCatchup"
	highLevelStateMachine["StandbyCatchup"]["Unknown"]["Healthy"] = "StandbyCatchup"
	highLevelStateMachine["StandbyCatchup"]["Unknown"]["Down"] = "StandbyDown"
	//
	highLevelStateMachine["StandbyCatchup"]["Unknown"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyCatchup"]["Unknown"]["Unknown"] = "StandbyCatchup"
	highLevelStateMachine["StandbyCatchup"]["Unknown"]["Terminal"] = "StandbyDown"
	highLevelStateMachine["StandbyCatchup"]["Unknown"]["CatchingUp"] = "StandbyCatchup"
	//
	highLevelStateMachine["StandbyCatchup"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyCatchup"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyCatchup"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyCatchup"]["Terminal"]["Unknown"] = "StandbyCatchup"
	//
	highLevelStateMachine["StandbyCatchup"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyCatchup"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyCatchup"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyCatchup"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyCatchup"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["StandbyCatchup"]["CatchingUp"]["Unknown"] = "StandbyCatchup"
	//
	highLevelStateMachine["StandbyCatchup"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["StandbyCatchup"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//
	//
	highLevelStateMachine["ManualInterventionRequired"]["Healthy"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Healthy"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Healthy"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Healthy"]["Unknown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Healthy"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Down"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Down"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Down"]["Unknown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["OtherDown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["OtherDown"]["Unknown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Unknown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Unknown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Unknown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Unknown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Unknown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Unknown"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Terminal"]["Unknown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["CatchingUp"]["Unknown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ManualInterventionRequired"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ManualInterventionRequired"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//

	// Note: in "WaitingForActive" state "Healthy" for the active
	// means "reachable and with an instance"

	highLevelStateMachine["WaitingForActive"]["Healthy"]["Healthy"] = "ConfiguringActive"
	highLevelStateMachine["WaitingForActive"]["Healthy"]["Down"] = "ConfiguringActive"
	highLevelStateMachine["WaitingForActive"]["Healthy"]["OtherDown"] = "ConfiguringActive"
	highLevelStateMachine["WaitingForActive"]["Healthy"]["Unknown"] = "ConfiguringActive"
	highLevelStateMachine["WaitingForActive"]["Healthy"]["Terminal"] = "ConfiguringActive"
	highLevelStateMachine["WaitingForActive"]["Healthy"]["CatchingUp"] = "ConfiguringActive"
	highLevelStateMachine["WaitingForActive"]["Down"]["Healthy"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Down"]["Down"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Down"]["OtherDown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Down"]["Unknown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Down"]["Terminal"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Down"]["CatchingUp"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["OtherDown"]["Healthy"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["OtherDown"]["Down"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["OtherDown"]["OtherDown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["OtherDown"]["Unknown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["OtherDown"]["Terminal"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["OtherDown"]["CatchingUp"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Unknown"]["Healthy"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Unknown"]["Down"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Unknown"]["OtherDown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Unknown"]["Unknown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Unknown"]["Terminal"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Unknown"]["CatchingUp"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Terminal"]["Healthy"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Terminal"]["Down"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Terminal"]["OtherDown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Terminal"]["Unknown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Terminal"]["Terminal"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["Terminal"]["CatchingUp"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["CatchingUp"]["Healthy"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["CatchingUp"]["Down"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["CatchingUp"]["OtherDown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["CatchingUp"]["Unknown"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["CatchingUp"]["Terminal"] = "WaitingForActive"
	highLevelStateMachine["WaitingForActive"]["CatchingUp"]["CatchingUp"] = "WaitingForActive"

	highLevelStateMachine["Reexamine"]["Healthy"]["Healthy"] = "Normal"
	highLevelStateMachine["Reexamine"]["Healthy"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Healthy"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Healthy"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Healthy"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Down"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Down"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Down"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["OtherDown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["OtherDown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Unknown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Unknown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Unknown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Unknown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Unknown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Unknown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Terminal"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["CatchingUp"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Reexamine"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"

	highLevelStateMachine["ConfiguringActive"]["Healthy"]["Healthy"] = "Normal"
	highLevelStateMachine["ConfiguringActive"]["Healthy"]["Down"] = "StandbyDown"
	highLevelStateMachine["ConfiguringActive"]["Healthy"]["OtherDown"] = "StandbyDown"
	highLevelStateMachine["ConfiguringActive"]["Healthy"]["Unknown"] = "StandbyDown"
	highLevelStateMachine["ConfiguringActive"]["Healthy"]["Terminal"] = "StandbyDown"
	highLevelStateMachine["ConfiguringActive"]["Healthy"]["CatchingUp"] = "StandbyDown"
	highLevelStateMachine["ConfiguringActive"]["Down"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Down"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Down"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["OtherDown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["OtherDown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Unknown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Unknown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Unknown"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Unknown"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Unknown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Unknown"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Terminal"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["CatchingUp"]["Unknown"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ConfiguringActive"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"

	highLevelStateMachine["Normal"]["Healthy"]["Healthy"] = "Normal"
	highLevelStateMachine["Normal"]["Healthy"]["Down"] = "ActiveTakeover"
	highLevelStateMachine["Normal"]["Healthy"]["OtherDown"] = "ActiveDown"
	highLevelStateMachine["Normal"]["Healthy"]["Unknown"] = "Normal"
	//
	highLevelStateMachine["Normal"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["Healthy"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["Normal"]["Down"]["Healthy"] = "ActiveDown"
	highLevelStateMachine["Normal"]["Down"]["Down"] = "BothDown"
	//
	highLevelStateMachine["Normal"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["Normal"]["Down"]["Unknown"] = "Normal"
	//
	highLevelStateMachine["Normal"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["Normal"]["OtherDown"]["Healthy"] = "ActiveTakeover" // Believe the Active
	//
	highLevelStateMachine["Normal"]["OtherDown"]["Down"] = "StandbyDown"
	highLevelStateMachine["Normal"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["Normal"]["OtherDown"]["Unknown"] = "Normal"
	//
	highLevelStateMachine["Normal"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["Normal"]["Unknown"]["Healthy"] = "Normal"
	highLevelStateMachine["Normal"]["Unknown"]["Down"] = "Normal"
	highLevelStateMachine["Normal"]["Unknown"]["OtherDown"] = "Normal"
	highLevelStateMachine["Normal"]["Unknown"]["Unknown"] = "Normal"
	highLevelStateMachine["Normal"]["Unknown"]["Terminal"] = "Normal"
	highLevelStateMachine["Normal"]["Unknown"]["CatchingUp"] = "Normal"
	//
	highLevelStateMachine["Normal"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["Normal"]["Terminal"]["Unknown"] = "Normal"
	//
	highLevelStateMachine["Normal"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["Normal"]["CatchingUp"]["Unknown"] = "Normal"
	//
	highLevelStateMachine["Normal"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["Normal"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//

	highLevelStateMachine["ActiveDown"]["Healthy"]["Healthy"] = "FAILOVER"
	//
	highLevelStateMachine["ActiveDown"]["Healthy"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["Healthy"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["Healthy"]["Unknown"] = "ActiveDown"
	//
	highLevelStateMachine["ActiveDown"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["Healthy"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["Down"]["Healthy"] = "FAILOVER"
	highLevelStateMachine["ActiveDown"]["Down"]["Down"] = "BothDown"
	//
	highLevelStateMachine["ActiveDown"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["Down"]["Unknown"] = "ActiveDown"
	//
	highLevelStateMachine["ActiveDown"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["OtherDown"]["Healthy"] = "FAILOVER"
	//
	highLevelStateMachine["ActiveDown"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["OtherDown"]["Unknown"] = "ActiveDown"
	//
	highLevelStateMachine["ActiveDown"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["Unknown"]["Healthy"] = "FAILOVER"
	highLevelStateMachine["ActiveDown"]["Unknown"]["Down"] = "BothDown"
	highLevelStateMachine["ActiveDown"]["Unknown"]["OtherDown"] = "ActiveDown"
	highLevelStateMachine["ActiveDown"]["Unknown"]["Unknown"] = "ActiveDown"
	highLevelStateMachine["ActiveDown"]["Unknown"]["Terminal"] = "ActiveDown"
	highLevelStateMachine["ActiveDown"]["Unknown"]["CatchingUp"] = "ActiveDown"
	highLevelStateMachine["ActiveDown"]["Terminal"]["Healthy"] = "FAILOVER"
	highLevelStateMachine["ActiveDown"]["Terminal"]["Down"] = "BothDown"
	//
	highLevelStateMachine["ActiveDown"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["Terminal"]["Unknown"] = "ActiveDown"
	//
	highLevelStateMachine["ActiveDown"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["CatchingUp"]["Healthy"] = "FAILOVER"
	//
	highLevelStateMachine["ActiveDown"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveDown"]["CatchingUp"]["Unknown"] = "ActiveDown"
	//
	highLevelStateMachine["ActiveDown"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveDown"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//

	//
	highLevelStateMachine["ActiveTakeover"]["Healthy"]["Healthy"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveTakeover"]["Healthy"]["Down"] = "StandbyDown"
	//
	highLevelStateMachine["ActiveTakeover"]["Healthy"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveTakeover"]["Healthy"]["Unknown"] = "ActiveTakeover"
	//
	highLevelStateMachine["ActiveTakeover"]["Healthy"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Healthy"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Down"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Down"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Down"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveTakeover"]["Down"]["Unknown"] = "ActiveTakeover"
	//
	highLevelStateMachine["ActiveTakeover"]["Down"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Down"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["OtherDown"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["OtherDown"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["OtherDown"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveTakeover"]["OtherDown"]["Unknown"] = "ActiveTakeover"
	//
	highLevelStateMachine["ActiveTakeover"]["OtherDown"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["OtherDown"]["CatchingUp"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveTakeover"]["Unknown"]["Healthy"] = "ActiveTakeover"
	highLevelStateMachine["ActiveTakeover"]["Unknown"]["Down"] = "ActiveTakeover"
	highLevelStateMachine["ActiveTakeover"]["Unknown"]["OtherDown"] = "ActiveTakeover"
	highLevelStateMachine["ActiveTakeover"]["Unknown"]["Unknown"] = "ActiveTakeover"
	highLevelStateMachine["ActiveTakeover"]["Unknown"]["Terminal"] = "ActiveTakeover"
	highLevelStateMachine["ActiveTakeover"]["Unknown"]["CatchingUp"] = "ActiveTakeover"
	//
	highLevelStateMachine["ActiveTakeover"]["Terminal"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Terminal"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Terminal"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveTakeover"]["Terminal"]["Unknown"] = "ActiveTakeover"
	//
	highLevelStateMachine["ActiveTakeover"]["Terminal"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["Terminal"]["CatchingUp"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["CatchingUp"]["Healthy"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["CatchingUp"]["Down"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["CatchingUp"]["OtherDown"] = "ManualInterventionRequired"
	//
	highLevelStateMachine["ActiveTakeover"]["CatchingUp"]["Unknown"] = "ActiveTakeover"
	//
	highLevelStateMachine["ActiveTakeover"]["CatchingUp"]["Terminal"] = "ManualInterventionRequired"
	highLevelStateMachine["ActiveTakeover"]["CatchingUp"]["CatchingUp"] = "ManualInterventionRequired"
	//

	autoUpgradeStateMachine = make(map[string]map[string]map[string]string)
	for hl, _ := range upgradeHLstates {
		autoUpgradeStateMachine[hl] = make(map[string]map[string]string)
		for p1, _ := range upgradestates {
			autoUpgradeStateMachine[hl][p1] = make(map[string]string)
			for p2, _ := range upgradestates {
				autoUpgradeStateMachine[hl][p1][p2] = ""
			}
		}
	}

	autoUpgradeStateMachine["UpgradingStandby"]["waiting"]["deleteStandby"] = "UpgradingStandby"
	autoUpgradeStateMachine["UpgradingStandby"]["waiting"]["processing"] = "UpgradingStandby"
	// should never have a UpgradingStandby HL state with success for active
	autoUpgradeStateMachine["UpgradingStandby"]["waiting"]["success"] = "UpgradingActive"
	autoUpgradeStateMachine["UpgradingStandby"]["success"]["success"] = "Complete"
	// unknown states indicate a problem
	autoUpgradeStateMachine["UpgradingStandby"]["unknown"]["success"] = "ManualInterventionRequired"
	autoUpgradeStateMachine["UpgradingStandby"]["success"]["unknown"] = "ManualInterventionRequired"
	autoUpgradeStateMachine["UpgradingStandby"]["deleteActive"]["success"] = "UpgradingActive"

	autoUpgradeStateMachine["UpgradingActive"]["deleteActive"]["success"] = "UpgradingActive"
	autoUpgradeStateMachine["UpgradingActive"]["processing"]["success"] = "UpgradingActive"
	autoUpgradeStateMachine["UpgradingActive"]["success"]["success"] = "Complete"

	autoUpgradeStateMachine["UpgradingStandby"]["waiting"]["failed"] = "ManualInterventionRequired"
	autoUpgradeStateMachine["UpgradingStandby"]["failed"]["waiting"] = "ManualInterventionRequired"
	autoUpgradeStateMachine["UpgradingStandby"]["failed"]["failed"] = "ManualInterventionRequired"

	autoUpgradeStateMachine["UpgradingActive"]["waiting"]["failed"] = "ManualInterventionRequired"
	autoUpgradeStateMachine["UpgradingActive"]["failed"]["waiting"] = "ManualInterventionRequired"
	autoUpgradeStateMachine["UpgradingActive"]["failed"]["failed"] = "ManualInterventionRequired"

	// unknown states indicate a problem
	autoUpgradeStateMachine["UpgradingActive"]["unknown"]["success"] = "ManualInterventionRequired"
	autoUpgradeStateMachine["UpgradingActive"]["success"]["unknown"] = "ManualInterventionRequired"

}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
