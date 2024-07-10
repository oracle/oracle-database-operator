// Copyright (c) 2019-2024, Oracle and/or its affiliates. All rights reserved.
//
// This is the main body of the TimesTen Kubernetes Operator

package controllers

import (
	"context"
	"regexp"
	"strconv"
	"time"

	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	//"regexp"
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	goRuntime "runtime"
	"strings"
	"sync"
)

var ttUser string // The OS user that TimesTen will run as

var jsonVer int = 1 // Update this for new versions of the Json protocol to the Agent

var universalInstanceGuid string = "63624bfd-67b9-11ea-81b7-0a580aed61c8"

// Normally we requeue events based on the "PollingInterval" specified
// in the TimesTenClassic by the user. But if it's not available
// we use this instead.

var defaultRequeueInterval time.Duration = 5 * time.Second // Seconds
var defaultRequeueMillisecs int64 = 5000                   // Milliseconds

var httpClients = map[string]*http.Client{}

var (
	mkdirMutex      sync.Mutex
	httpClientMutex sync.Mutex
)

// creates an http client object for communication with agents
func createHTTPClient(ctx context.Context, instance timestenv2.TimesTenObject, tts *TTSecretInfo) *http.Client {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("createHTTPClient entered")
	defer reqLogger.V(1).Info("createHTTPClient returns")

	tlsConfig := &tls.Config{
		RootCAs:       rootCAs,
		Certificates:  []tls.Certificate{tts.ClientCert},
		Renegotiation: tls.RenegotiateOnceAsClient,
		MinVersion:    tls.VersionTLS12,
	}
	tlsConfig.BuildNameToCertificate()

	// So establishing the TCP connection has a timeout (Client.Transport.Dial.Timeout)
	// And the TLS handshake has a timeout (Client.Transport.TLSHandshakeTimeout)
	// And the entire GET/POST request round trip (including the time on the server)
	// has a timeout (Client.Timeout).

	tcpTimeout := instance.GetAgentTCPTimeout()
	tlsTimeout := instance.GetAgentTLSTimeout()

	client := &http.Client{
		// we'll replace Timeout with getTimeout or postTimeout in getClient(),
		// called prior to the request
		Timeout: time.Second * time.Duration(60),
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout: time.Second * time.Duration(tcpTimeout),
			}).Dial,
			TLSClientConfig:     tlsConfig,
			TLSHandshakeTimeout: time.Second * time.Duration(tlsTimeout),
		},
	}

	return client

}

// creates and/or retrieves an http client object
func getHttpClient(ctx context.Context, instance timestenv2.TimesTenObject, dnsName string, method string,
	tts *TTSecretInfo, timeoutOverride *int) *http.Client {
	httpClientMutex.Lock()

	us := "getHttpClient"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")

	defer func() {
		httpClientMutex.Unlock()
		reqLogger.V(1).Info(us + " returns")
	}()

	if _, ok := httpClients[dnsName]; ok {
	} else {
		// create an http client object for each pod; this allows for persistent connections that should
		// survive container restarts
		httpClients[dnsName] = createHTTPClient(ctx, instance, tts)
		reqLogger.V(1).Info(fmt.Sprintf("%s: created http client for %s, all clients : %v", us, dnsName, httpClients))
	}

	client := httpClients[dnsName]

	if timeoutOverride == nil {
		if method == "POST" {
			postTimeout := instance.GetAgentPostTimeout()
			client.Timeout = time.Second * time.Duration(postTimeout)
		} else {
			// use getTimeout for all other methods (GET, etc)
			getTimeout := instance.GetAgentGetTimeout()
			client.Timeout = time.Second * time.Duration(getTimeout)
		}
	} else {
		client.Timeout = time.Second * time.Duration(*timeoutOverride)
	}
	reqLogger.V(2).Info(fmt.Sprintf("%s: client.Timeout=%v", us, client.Timeout))

	return client

}

// deletes an http client object
func deleteHTTPClient(ctx context.Context, namespace string, name string) error {
	us := "deleteHTTPClient"
	reqLogger := log.FromContext(ctx)

	reqLogger.V(1).Info(us + " entered")
	defer reqLogger.V(1).Info(us + " returns")

	for podNo := 0; podNo < 2; podNo++ {
		DNSName := name + "-" + strconv.Itoa(podNo) + "." + name + "." + namespace + ".svc.cluster.local"
		if _, ok := httpClients[DNSName]; ok {
			reqLogger.V(1).Info(fmt.Sprintf("%s deleted http client %s", us, DNSName))
			delete(httpClients, DNSName)
		}
	}

	return nil

}

// deletes cert files from /tmp
func deleteCerts(ctx context.Context, secretName string) error {
	us := "deleteCerts"
	reqLogger := log.FromContext(ctx)

	reqLogger.V(1).Info(us + " entered")
	defer reqLogger.V(1).Info(us + " returns")

	for _, ext := range [2]string{".priv", ".cert"} {
		file := "/tmp/" + secretName + ext
		if _, err := os.Stat(file); err == nil {
			e := os.Remove(file)
			if e != nil {
				errMsg := fmt.Sprintf("%s failed to delete %s", us, file)
				reqLogger.Error(errors.New(errMsg), errMsg)
			} else {
				reqLogger.V(2).Info(fmt.Sprintf("%s deleted %s", us, file))
			}
		}
	}

	return nil

}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func newBool(b bool) *bool {
	bb := b
	return &bb
}

func newString(s string) *string {
	ss := s
	return &ss
}

func newUID(s types.UID) *types.UID {
	ss := s
	return &ss
}

func newInt64(i int64) *int64       { return &i }
func newInt32(i int32) *int32       { return &i }
func newInt(i int) *int             { return &i }
func newFloat32(f float32) *float32 { return &f }

func safeMakeDir(dir string, mode int) error {
	mkdirMutex.Lock()
	defer mkdirMutex.Unlock()

	err := os.MkdirAll(dir, os.FileMode(mode))
	if err == nil {
		return nil
	}
	if os.IsExist(err) {
		stat, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !stat.IsDir() {
			return errors.New(dir + " exists but is not a directory")
		}
		return nil
	}
	return err
}

// Update the Status of this object
func updateStatus(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic) {
	us := "updateStatus"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(fmt.Sprintf("%s entered, resourceVersion='%s'", us, instance.ObjectMeta.ResourceVersion))
	defer reqLogger.V(2).Info("updateStatus returns")

	replicated := isReplicated(instance)

	// There are some summary fields in the 'status' that simply reflect other fields buried
	// more deeply in the status struct. These are so customer apps can more easily parse them.
	// (See bug 33135919). Set them here if they are relevant

	if replicated {
		if instance.Status.PodStatus != nil {
			standby := 0
			active := 1
			if instance.Status.PodStatus[1].IntendedState == "Standby" {
				active = 0
				standby = 1
			}

			instance.Status.ActiveRepAgent = instance.Status.PodStatus[active].ReplicationStatus.RepAgent
			instance.Status.ActiveCacheAgent = instance.Status.PodStatus[active].CacheStatus.CacheAgent
			instance.Status.StandbyRepAgent = instance.Status.PodStatus[standby].ReplicationStatus.RepAgent
			instance.Status.StandbyCacheAgent = instance.Status.PodStatus[standby].CacheStatus.CacheAgent

			if ps, ok := instance.Status.PodStatus[active].DbStatus.DbConfiguration["PermSize"]; ok {
				x, e := strconv.ParseInt(ps, 10, 64)
				if e == nil {
					instance.Status.ActivePermSize = newInt64(x)
				}
			}

			if ps, ok := instance.Status.PodStatus[active].DbStatus.Monitor["perm_in_use_size"]; ok {
				x, e := strconv.ParseInt(ps, 10, 64)
				if e == nil {
					instance.Status.ActivePermInUse = newInt64(x)
				}
			}

			if ps, ok := instance.Status.PodStatus[standby].DbStatus.DbConfiguration["PermSize"]; ok {
				x, e := strconv.ParseInt(ps, 10, 64)
				if e == nil {
					instance.Status.StandbyPermSize = newInt64(x)
				}
			}

			if ps, ok := instance.Status.PodStatus[standby].DbStatus.Monitor["perm_in_use_size"]; ok {
				x, e := strconv.ParseInt(ps, 10, 64)
				if e == nil {
					instance.Status.StandbyPermInUse = newInt64(x)
				}
			}

		}
	}

	err := client.Status().Update(ctx, instance)
	if err != nil {
		reqLogger.Error(err, "Error updating TimesTenClassic status: "+err.Error())
	}
	reqLogger.V(2).Info(fmt.Sprintf("After update resourceVersion='%s'", instance.ObjectMeta.ResourceVersion))
	reqLogger.Info("Reconcile", "Status", instance.Status) // Customer parses this, leave it Info (no debug level)
}

var eventDiscriminator int64 = 1
var lastEventTime int64

// Helps to know if an error thrown is due a lack of permission from Kubernetes and return the error.
func verifyUnauthorizedError(err string) (string, bool) {
	var permissionError = regexp.MustCompile(`.*forbidden.*`)
	var errorMessage = "Lack of permissions to perform Kubernetes action"
	if permissionError.MatchString(err) {
		return errorMessage, true
	}
	return err, false
}

// Create a Kubernetes "Event" for a particular occurrence
func logTTEvent(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject, reasonString string, msgString string, isWarning bool) {
	us := "logTTEvent"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")
	if len(msgString) == 0 {
		reqLogger.V(1).Info(us + " called with nil message")
		return
	}

	reqLogger.V(1).Info(msgString)

	instance.IncrLastEventNum()

	now := time.Now()
	unow := now.Unix()
	tm := metav1.MicroTime{Time: now}

	if unow == lastEventTime {
		eventDiscriminator++
	} else {
		eventDiscriminator = 0
		lastEventTime = unow
	}

	ts := fmt.Sprintf("%010d", unow)
	descriminator := fmt.Sprintf("%04d", eventDiscriminator)

	typeString := "Normal"
	if isWarning {
		typeString = "Warning"
	}

	msg := msgString
	if len(msg) > 1023 {
		msg = msg[0:1022] // Events can only be 1024 chars long (Kubernetes limitation)
	}

	evt := eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tt" + ts + "-" + descriminator,
			Namespace: instance.ObjectNamespace(),
		},
		Action:    "-",
		EventTime: tm,
		Note:      msg,
		Reason:    reasonString,
		Regarding: corev1.ObjectReference{
			Kind:      instance.ObjectType(),
			Name:      instance.ObjectName(),
			Namespace: instance.ObjectNamespace(),
			UID:       instance.ObjectUID(),
		},
		Related:             nil,
		ReportingController: os.Getenv("OPERATOR_NAME"),
		ReportingInstance:   os.Getenv("POD_NAME"),
		Series:              nil,
		Type:                typeString,
	}

	err := client.Create(ctx, &evt)
	if err != nil {
		reqLogger.Error(err, "Could not create event: "+err.Error())
		//Checks if the error was because of lack of permission, if not, return the original message
		var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
		if isPermissionsProblem {
			logTTEvent(ctx, client, instance, "FailedCreateEvent", "Failed to create event: "+errorMsg, true)
		}
	}
}

// Go through the stderr from a TimesTen command and create events for any TimesTen error msgs in it
func logTTEventsFromStderr(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject,
	what string, stderr *[]string) {
	for _, err := range *stderr {
		// Error Message: [TimesTen][TimesTen 18.1.3.2.0 ODBC Driver][TimesTen]TT1001: Blah blah -- file "ptSqlY.y", lineno 307, procedure "sbPtParseSql"
		if len(err) > 0 {
			if strings.HasPrefix(err, "Error Message: [TimesTen]") {
				x := strings.Index(err, "]")
				if x >= 0 {
					y := strings.Index(err[x+1:], "]")
					if y >= 0 {
						z := strings.Index(err[x+1+y+1:], "]")
						if z >= 0 {
							msg := err[x+1+y+1+z+1:]
							trailer := strings.Index(msg, "-- file")
							if trailer >= 0 {
								msg = msg[:trailer]
							}
							logTTEvent(ctx, client, instance, "Error", what+" error: "+msg, true)
						}
					}
				}
			}
		}
	}
}

// Go through the errors array from createDb and create events for any TimesTen error msgs in it
func logTTEventsFromErrors(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject,
	stderr *[]string) {
	for _, err := range *stderr {
		// Error setting cache uid / pwd: [TimesTen][TimesTen 18.1.3.2.0 ODBC Driver][TimesTen]TT1001: Blah blah -- file "ptSqlY.y", lineno 307, procedure "sbPtParseSql"
		w := strings.Index(err, "[TimesTen]")
		if w >= 0 {
			what := err[:w-1]
			x := strings.Index(err, "]")
			if x >= 0 {
				y := strings.Index(err[x+1:], "]")
				if y >= 0 {
					z := strings.Index(err[x+1+y+1:], "]")
					if z >= 0 {
						msg := err[x+1+y+1+z+1:]
						trailer := strings.Index(msg, "-- file")
						if trailer >= 0 {
							msg = msg[:trailer]
						}
						logTTEvent(ctx, client, instance, "Error", what+msg, true)
					}
				}
			}
		}
	}
}

// Report changes in pod state
//
// The logic here may seem overly complicated and random, but it's intended
// to make sure we generate a minimum of events during a normal startup
// but still report important changes in state when things go wrong (or get fixed)
// later

func reportChangesInPodState(ctx context.Context,
	podName string,
	prevPodStatus *timestenv2.TimesTenPodStatus,
	newPodStatus *timestenv2.TimesTenPodStatus,
	instanceHighLevelState string,
	client client.Client,
	instance timestenv2.TimesTenObject) {
	us := "reportChangesInPodState"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered for pod " + podName)
	defer reqLogger.V(2).Info(us + " returns")

	dbType := newPodStatus.TTPodType

	iState := instance.GetHighLevelState()

	if prevPodStatus.IntendedState != newPodStatus.IntendedState {
		reason := "StateChange"
		msg := "Pod " + podName + " Intended State '" + newPodStatus.IntendedState + "'"
		logTTEvent(ctx, client, instance, reason, msg, false)
	}

	if prevPodStatus.PodStatus.PodPhase != newPodStatus.PodStatus.PodPhase {
		reason := "StateChange"
		msg := "Pod " + podName + " Pod Phase '" + newPodStatus.PodStatus.PodPhase + "'"
		logTTEvent(ctx, client, instance, reason, msg, false)

		if prevPodStatus.HasBeenSeen == false && newPodStatus.PodStatus.PodPhase == "Pending" {
			// Nothing to see here (yet)
			return
		}
		if prevPodStatus.PodStatus.PodPhase == "Pending" && newPodStatus.PodStatus.PodPhase == "Running" {
			return
		}
	}

	if newPodStatus.PodStatus.Agent == "Unknown" {
		// No need to report anything else
		return
	}

	if prevPodStatus.PodStatus.Agent != newPodStatus.PodStatus.Agent {
		reason := "Info"
		msg := "Pod " + podName + " Agent " + newPodStatus.PodStatus.Agent
		logTTEvent(ctx, client, instance, reason, msg, false)
		if prevPodStatus.PodStatus.Agent == "Up" && newPodStatus.PodStatus.Agent == "Down" {
			// No need to report anything else
			return
		}

		if prevPodStatus.PodStatus.PodPhase == "Pending" && newPodStatus.PodStatus.PodPhase == "Running" &&
			newPodStatus.PodStatus.Agent == "Unknown" {
			return
		}
	}

	if newPodStatus.TimesTenStatus.Instance == "Exists" {
		if prevPodStatus.TimesTenStatus.Release != newPodStatus.TimesTenStatus.Release {
			reason := "Info"
			msg := "Pod " + podName + " Release " + newPodStatus.TimesTenStatus.Release
			logTTEvent(ctx, client, instance, reason, msg, false)
		}
	}

	if prevPodStatus.TimesTenStatus.Instance != newPodStatus.TimesTenStatus.Instance {
		reportIt := true
		if instance.ObjectType() == "TimesTenClassic" {
			if iState == "Initializing" && newPodStatus.TimesTenStatus.Instance == "Exists" {
				reportIt = false
			}
		}
		if instance.ObjectType() == "TimesTenScaleout" {
			if iState == "Initializing" && newPodStatus.TimesTenStatus.Instance == "Missing" {
				reportIt = false
			}
			if iState != "Initializing" && newPodStatus.TimesTenStatus.Instance == "Exists" {
				reportIt = false
			}
		}

		if iState == "ZookeeperReady" || iState == "GridCreated" {
			if newPodStatus.TimesTenStatus.Instance == "Missing" {
				reportIt = false
			}
		}

		if reportIt {
			reason := "Info"
			msg := "Pod " + podName + " Instance " + newPodStatus.TimesTenStatus.Instance
			logTTEvent(ctx, client, instance, reason, msg, false)
		}
	}

	reportDatabaseOrElementUnloaded := true

	if prevPodStatus.TimesTenStatus.Daemon != newPodStatus.TimesTenStatus.Daemon {
		reportIt := true
		if iState == "Initializing" ||
			iState == "InstancesCreated" ||
			iState == "GridCreated" ||
			iState == "ZookeeperReady" {
			if prevPodStatus.TimesTenStatus.Daemon == "Unknown" && newPodStatus.TimesTenStatus.Daemon == "Down" {
				reportIt = false
			}
		}
		if reportIt {
			reason := "Info"
			msg := "Pod " + podName + " Daemon " + newPodStatus.TimesTenStatus.Daemon
			logTTEvent(ctx, client, instance, reason, msg, false)
		}

		// If the daemon just came up then obviously the database/element it runs
		// isn't loaded. So don't report that later in that case

		if prevPodStatus.TimesTenStatus.Daemon == "Down" &&
			newPodStatus.TimesTenStatus.Daemon == "Up" {
			reportDatabaseOrElementUnloaded = false
		}
	}

	if prevPodStatus.DbStatus.Db != newPodStatus.DbStatus.Db {
		reportIt := true
		if newPodStatus.DbStatus.Db == "None" &&
			prevPodStatus.DbStatus.Db == "Unknown" &&
			(iState == "Initializing" ||
				iState == "InstancesCreated" ||
				iState == "DatabaseCreated" ||
				iState == "ZookeeperReady") {
			reportIt = false
		}
		if newPodStatus.DbStatus.Db == "Loaded" &&
			iState == "DatabaseCreated" {
			reportIt = false
		}

		if reportDatabaseOrElementUnloaded &&
			newPodStatus.DbStatus.Db == "Unloaded" {
			reportIt = false
		}

		if reportIt {
			reason := "Info"
			msg := "Pod " + podName + " " + dbType + " " + newPodStatus.DbStatus.Db
			logTTEvent(ctx, client, instance, reason, msg, false)
		}
	}

	if instance.ObjectType() == "TimesTenClassic" {
		if prevPodStatus.DbStatus.DbUpdatable != newPodStatus.DbStatus.DbUpdatable {
			switch newPodStatus.DbStatus.DbUpdatable {
			case "Yes":
				if iState != "Initializating" {
					reason := "Info"
					msg := "Pod " + podName + " " + dbType + " Updatable"
					logTTEvent(ctx, client, instance, reason, msg, false)
				}
			case "No":
				if iState == "Initializing" {
					if newPodStatus.IntendedState != "Standby" {
						reason := "Info"
						msg := "Pod " + podName + " " + dbType + " Not Updatable"
						logTTEvent(ctx, client, instance, reason, msg, false)
					}
				}
			default:
				reason := "Info"
				msg := "Pod " + podName + " " + dbType + " Updatable: " + newPodStatus.DbStatus.DbUpdatable
				logTTEvent(ctx, client, instance, reason, msg, false)
			}
		}
	}

	if newPodStatus.CacheGroupsFile { // Only display if there might actually be cachegroups
		if newPodStatus.TTPodType == "Database" || newPodStatus.TTPodType == "Element" {
			if prevPodStatus.CacheStatus.CacheAgent != newPodStatus.CacheStatus.CacheAgent {
				reportIt := true
				if iState == "Initializing" || iState == "DatabaseCreated" {
					if prevPodStatus.CacheStatus.CacheAgent == "Unknown" && newPodStatus.CacheStatus.CacheAgent == "Not Running" {
						reportIt = false
					}
				}

				if reportIt {
					reason := "Info"
					msg := "Pod " + podName + " CacheAgent " + newPodStatus.CacheStatus.CacheAgent
					logTTEvent(ctx, client, instance, reason, msg, false)
				}
			}
		}
	}

	if newPodStatus.TTPodType == "Database" || newPodStatus.TTPodType == "MgmtDb" {
		if prevPodStatus.ReplicationStatus.RepAgent != newPodStatus.ReplicationStatus.RepAgent {
			reportIt := true

			if iState == "Initializing" {
				if prevPodStatus.ReplicationStatus.RepAgent == "Unknown" && newPodStatus.ReplicationStatus.RepAgent == "Not Running" {
					reportIt = false
				}
			}

			if instance.ObjectType() == "TimesTenScaleout" {
				reportIt = false
			}

			if reportIt {
				reason := "Info"
				msg := "Pod " + podName + " RepAgent " + newPodStatus.ReplicationStatus.RepAgent
				logTTEvent(ctx, client, instance, reason, msg, false)
			}
		}

		if instance.ObjectType() == "TimesTenClassic" {
			ttc := instance.(timestenv2.TimesTenClassic)
			replicated := isReplicated(&ttc)

			if replicated {
				if prevPodStatus.ReplicationStatus.RepScheme != newPodStatus.ReplicationStatus.RepScheme {
					reportIt := true

					if iState == "Initializing" {
						if prevPodStatus.ReplicationStatus.RepScheme == "Unknown" && newPodStatus.ReplicationStatus.RepScheme == "None" {
							reportIt = false
						}
					}

					if instance.ObjectType() == "TimesTenScaleout" {
						reportIt = false
					}

					if reportIt {
						reason := "Info"
						msg := "Pod " + podName + " RepScheme " + newPodStatus.ReplicationStatus.RepScheme
						logTTEvent(ctx, client, instance, reason, msg, false)
					}
				}

				if newPodStatus.ReplicationStatus.RepScheme != "None" {
					if prevPodStatus.ReplicationStatus.RepState != newPodStatus.ReplicationStatus.RepState {
						reportIt := true

						if iState == "Initializing" {
							if newPodStatus.IntendedState != "Standby" {
								if prevPodStatus.ReplicationStatus.RepState == "Unknown" && newPodStatus.ReplicationStatus.RepState == "IDLE" {
									reportIt = false
								}
							}
						}

						if reportIt {
							reason := "StateChange"
							msg := "Pod " + podName + " RepState " + newPodStatus.ReplicationStatus.RepState
							logTTEvent(ctx, client, instance, reason, msg, false)
						}
					}
				}
			}
		}
	}

	if prevPodStatus.HighLevelState != newPodStatus.HighLevelState {
		if (newPodStatus.HighLevelState == "Unknown" || newPodStatus.HighLevelState == "Down") &&
			(prevPodStatus.HighLevelState == "Unknown" || prevPodStatus.HighLevelState == "Down") &&
			instanceHighLevelState == "Initializing" {
			// Don't mention
		} else {
			reason := "StateChange"
			msg := "Pod " + podName + " State '" + newPodStatus.HighLevelState + "'"
			logTTEvent(ctx, client, instance, reason, msg, false)
		}
	}

	if instance.ObjectType() == "TimesTenScaleout" && newPodStatus.TTPodType == "MgmtDb" {
		if newPodStatus.ScaleoutStatus.DbStatus != nil &&
			len(newPodStatus.ScaleoutStatus.DbStatus.Databases) > 0 &&
			len(newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary) >= 3 {

			if prevPodStatus.ScaleoutStatus.DbStatus != nil &&
				len(prevPodStatus.ScaleoutStatus.DbStatus.Databases) > 0 &&
				len(prevPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary) >= 3 {

				// We have an old and a new summary

				if prevPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[0] !=
					newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[0] {
					reason := "StateChange"
					msg := "Database Create State '" + newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[0] + "'"
					logTTEvent(ctx, client, instance, reason, msg, false)
				}
				if prevPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[1] !=
					newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[1] {
					reason := "StateChange"
					msg := "Database Load State '" + newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[1] + "'"
					logTTEvent(ctx, client, instance, reason, msg, false)
				}
				if newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[1] != "notLoaded" {
					if prevPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[2] !=
						newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[2] {
						reason := "StateChange"
						msg := "Database Open State '" + newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[2] + "'"
						logTTEvent(ctx, client, instance, reason, msg, false)
					}
				}

			} else {

				// We just have a new summary
				printIt := true

				if instanceHighLevelState == "DatabaseCreated" {
					// Don't display the normal first results

					if len(newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary) == 3 {
						if newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[0] == "created" &&
							newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[1] == "loaded-complete" &&
							newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[2] == "open" {
							printIt = false
						}
					} else {
						printIt = false
					}

					if printIt {
						reason := "StateChange"
						msg := "Database Create State '" + newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[0] + "'"
						logTTEvent(ctx, client, instance, reason, msg, false)

						msg = "Database Load State '" + newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[1] + "'"
						logTTEvent(ctx, client, instance, reason, msg, false)

						msg = "Database Open State '" + newPodStatus.ScaleoutStatus.DbStatus.Databases[0].Overall.Summary[2] + "'"
						logTTEvent(ctx, client, instance, reason, msg, false)
					}
				}
			}
		}
	}
	return
}

// Perform one-time initialization for a new TimesTenClassic object
func oneTimeInit(ctx context.Context, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo, client client.Client, scheme *runtime.Scheme) {
	us := "oneTimeInit"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	// Ideally we could specify defaults for everything in the Spec and
	// Kubernetes would automatically fill them in. But we can't, so
	// initialize any required fields here

	replicated, nReplicas, nSubs, maxNSubs, subName := replicationConfig(instance)

	// default behavior is to cleanup cache on object destruction
	if instance.Spec.TTSpec.CacheCleanup == nil {
		instance.Spec.TTSpec.CacheCleanup = newBool(true)
		//reqLogger.V(1).Info(fmt.Sprintf("%s: set CacheCleanup=%v, was nil", us, *instance.Spec.TTSpec.CacheCleanup))
	} else {
		//reqLogger.V(1).Info(fmt.Sprintf("%s: Spec.TTSpec.CacheCleanup=%v", us, *instance.Spec.TTSpec.CacheCleanup))
	}

	if instance.Spec.TTSpec.ImageUpgradeStrategy == nil {
		instance.Spec.TTSpec.ImageUpgradeStrategy = newString("Auto")
		//reqLogger.V(2).Info(us + ": set Spec.TTSpec.ImageUpgradeStrategy to " +
		//      *instance.Spec.TTSpec.ImageUpgradeStrategy + " (was nil)")
	} else {
		reqLogger.V(1).Info(us + ": instance.Spec.TTSpec.ImageUpgradeStrategy=" + *instance.Spec.TTSpec.ImageUpgradeStrategy)
	}

	if instance.Status.StatusVersion == "" {
		ttRel, _ := GetTTMajorRelease(ctx)
		if ttRel >= 22 {
		} else {
			if instance.Spec.TTSpec.Prometheus != nil {
				instance.Spec.TTSpec.Prometheus = nil
				logTTEvent(ctx, client, instance, "Create", "Ignoring 'prometheus' directive on TimesTen 18", true)
			}
		}
	}

	// Similarly if we have never seen this object before then we need to
	// initialize the Status of the object

	if instance.Status.HighLevelState == "Unknown" || instance.Status.HighLevelState == "" {
		reqLogger.V(1).Info(fmt.Sprintf("%s: HighLevelState '%s', set to Initializing", us, instance.Status.HighLevelState))
		instance.Status.HighLevelState = "Initializing"
		instance.Status.PrevStopManaging = ""
		instance.Status.PrevReexamine = ""
		if nSubs > 0 {
			instance.Status.Subscriber.HLState = "NoSubscribersReady"
		} else {
			instance.Status.Subscriber.HLState = "None"
		}
	}

	if instance.Status.PodStatus == nil {
		instance.Status.PodStatus = make([]timestenv2.TimesTenPodStatus, nReplicas+maxNSubs)
		updateTTClassicHighLevelState(ctx, instance, "Initializing", client)
		initOne := func(name string, s *timestenv2.TimesTenPodStatus, isSub bool) {
			if isSub {
				s.TTPodType = "Subscriber"
				s.IntendedState = "Subscriber"
			} else {
				s.TTPodType = "Database"
			}
			s.Initialized = true
			s.Name = name
			s.HighLevelState = "Initializing"
			s.PodStatus.PodPhase = "Unknown"
			s.PodStatus.Agent = "Down"
			s.PodStatus.PodIP = ""
			s.DbStatus.Db = "Unknown"
			s.DbStatus.DbUpdatable = "Unknown"
			s.DbStatus.DbId = 0
			s.TimesTenStatus.Release = "Unknown"
			s.TimesTenStatus.Instance = "Unknown"
			s.TimesTenStatus.Daemon = "Unknown"
			s.ReplicationStatus.RepAgent = "Unknown"
			s.ReplicationStatus.RepScheme = "Unknown"
			s.ReplicationStatus.RepState = "Unknown"
			s.ReplicationStatus.RepPeerPState = "Unknown"
			s.CacheStatus.CacheAgent = "Unknown"
			s.CacheStatus.NCacheGroups = 0
			return
		}

		iNum := 0
		for i := 0; i < nReplicas; i++ {
			name := fmt.Sprintf("%s-%d", instance.Name, i)
			initOne(name, &instance.Status.PodStatus[iNum], false)
			iNum++
		}
		for i := 0; i < maxNSubs; i++ {
			name := fmt.Sprintf("%s-%d", subName, i)
			initOne(name, &instance.Status.PodStatus[iNum], true)
			if i < nSubs {
				instance.Status.PodStatus[iNum].HighLevelState = "Initializing"
			} else {
				instance.Status.PodStatus[iNum].HighLevelState = "NotProvisioned"
			}
			iNum++
		}
	} else {
		podsInit := false
		for i := 0; i < len(instance.Status.PodStatus); i++ {
			if instance.Status.PodStatus[i].Initialized {
				podsInit = true
			}
		}
		if podsInit {
			return
		}
	}

	// Now initialize other fields in the status

	instance.Status.StatusVersion = "1.0"
	instance.Status.ActivePods = "unknown"

	if instance.Spec.TTSpec.RepPort == nil {
		instance.Status.RepPort = 4444
	} else {
		instance.Status.RepPort = *instance.Spec.TTSpec.RepPort
	}

	// If the name of the TimesTenClassic object contains a dash then
	// the DSN is truncated to the part before the dash. If that behavior
	// changes then this code as well as corresponding code in
	// starthost.pl and ttagent.go must be changed.

	dbName := instance.Name
	dash := strings.Index(dbName, "-")
	if dash > -1 {
		dbName = dbName[:dash]
	}

	if replicated {
		// Determine the create replication statement we'll use

		if instance.Spec.TTSpec.RepCreateStatement == nil {
			// Construct a statement

			repStoreAttribute := "PORT " + strconv.Itoa(instance.Status.RepPort) + " FAILTHRESHOLD 0"
			if instance.Spec.TTSpec.RepStoreAttribute != nil {
				repStoreAttribute = *instance.Spec.TTSpec.RepStoreAttribute
				// TODO Move the port out of this (change the spec)
			}

			repReturnServiceAttribute := "NO RETURN"
			if instance.Spec.TTSpec.RepReturnServiceAttribute != nil {
				repReturnServiceAttribute = *instance.Spec.TTSpec.RepReturnServiceAttribute
			}

			_, _, nSubs, maxNSubs, subName := replicationConfig(instance)

			instance.Status.RepCreateStatement =
				"create active standby pair " +
					"\"" + dbName + "\" on \"" + instance.Name + "-0." + instance.Name + "." +
					instance.Namespace + ".svc.cluster.local\", " +
					"\"" + dbName + "\" on \"" + instance.Name + "-1." + instance.Name + "." +
					instance.Namespace + ".svc.cluster.local\" "

			instance.Status.RepCreateStatement = instance.Status.RepCreateStatement +
				" " + repReturnServiceAttribute + " "

			if nSubs > 0 {
				instance.Status.RepCreateStatement = instance.Status.RepCreateStatement + " subscriber "
			}
			for s := 0; s < maxNSubs; s++ {
				if s > 0 {
					instance.Status.RepCreateStatement = instance.Status.RepCreateStatement + ", "
				}
				instance.Status.RepCreateStatement = instance.Status.RepCreateStatement +
					fmt.Sprintf(" \"%s\" on \"%s-%d.%s.%s.svc.cluster.local\" ", dbName, subName, s, instance.Name, instance.Namespace)
			}

			instance.Status.RepCreateStatement = instance.Status.RepCreateStatement +
				" store \"" + dbName + "\" on \"" + instance.Name + "-0." + instance.Name + "." +
				instance.Namespace + ".svc.cluster.local\" " +
				repStoreAttribute + " " +
				" store \"" + dbName + "\" on \"" + instance.Name + "-1." + instance.Name + "." +
				instance.Namespace + ".svc.cluster.local\" " +
				repStoreAttribute + " "

			for s := 0; s < maxNSubs; s++ {
				instance.Status.RepCreateStatement = instance.Status.RepCreateStatement +
					fmt.Sprintf(" store \"%s\" on \"%s-%d.%s.%s.svc.cluster.local\" %s ", dbName, subName, s, instance.Name, instance.Namespace, repStoreAttribute)
			}

		} else {
			instance.Status.RepCreateStatement = *instance.Spec.TTSpec.RepCreateStatement
		}

		// Now do variable substitution

		instance.Status.RepCreateStatement =
			strings.Replace(instance.Status.RepCreateStatement, "{{tt-name}}", dbName, -1)
		instance.Status.RepCreateStatement =
			strings.Replace(instance.Status.RepCreateStatement, "{{tt-rep-port}}", strconv.Itoa(instance.Status.RepPort), -1)
		instance.Status.RepCreateStatement =
			strings.Replace(instance.Status.RepCreateStatement, "{{tt-node-0}}", instance.Name+"-0."+
				instance.Name+"."+instance.Namespace+".svc.cluster.local", -1)
		instance.Status.RepCreateStatement =
			strings.Replace(instance.Status.RepCreateStatement, "{{tt-node-1}}", instance.Name+"-1."+
				instance.Name+"."+instance.Namespace+".svc.cluster.local", -1)
		instance.Status.RepCreateStatement =
			strings.Replace(instance.Status.RepCreateStatement, "{{tt-conflict-report-dir}}", "/tt/home/"+ttUser+"/conflict", -1)
		instance.Status.RepCreateStatement =
			strings.Replace(instance.Status.RepCreateStatement, "{{tt-agent-user}}", tts.AgentUID, -1)
	}

	if replicated {
		instance.Status.PodStatus[0].IntendedState = "Active"
		instance.Status.PodStatus[1].IntendedState = "Standby"
	} else {
		for i := 0; i < nReplicas; i++ {
			instance.Status.PodStatus[i].IntendedState = "Standalone"
		}
	}

}

// check for a pending async task
func checkPendingAsyncTask(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo) (pending bool, err error) {
	us := "checkPendingAsyncTask"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	pendingAsyncTask := false

	if instance.Status.AsyncStatus.Running == true {

		asyncStatusStr, _ := json.Marshal(instance.Status.AsyncStatus)
		reqLogger.V(2).Info(fmt.Sprintf("%s: asyncStatus=%s", us, string(asyncStatusStr)))

		reqLogger.V(2).Info(fmt.Sprintf("%s: calling getAsyncStatus for async requestId=%v host=%v type=%v", us,
			instance.Status.AsyncStatus.Id, instance.Status.AsyncStatus.Host, instance.Status.AsyncStatus.Type))
		asyncStatus, err := getAsyncStatus(ctx, instance, instance.Status.AsyncStatus.Host, tts, instance.Status.AsyncStatus.Id)
		if err != nil {
			reqLogger.Info(fmt.Sprintf("%s: %v", us, err))
			_, kErr := killAgent(ctx, client, instance, instance.Status.AsyncStatus.Host, instance.Status.AsyncStatus.PodName, tts, nil)
			if kErr != nil {
				reqLogger.V(2).Info(fmt.Sprintf("%s: error returned from killAgent : %v", us, kErr))
				return false, kErr
			}

			return false, err

		}

		asyncStatusStr, _ = json.Marshal(asyncStatus)
		reqLogger.V(2).Info(fmt.Sprintf("%s: getAsyncStatus returned : %s", us, string(asyncStatusStr)))

		if asyncStatus.Id == "" {

			reqLogger.V(2).Info(fmt.Sprintf("%s: no pending async task on %v, setting AsyncStatus.Running to false",
				us, instance.Status.AsyncStatus.PodName))
			instance.Status.AsyncStatus.Running = false

		} else {

			// if the task is complete, update the status object and continue with checkPendingAsyncTask
			if asyncStatus.Complete == true {

				instance.Status.AsyncStatus.Complete = true
				instance.Status.AsyncStatus.Running = false
				instance.Status.AsyncStatus.Updated = *asyncStatus.Updated
				instance.Status.AsyncStatus.Ended = *asyncStatus.Ended

				if instance.Status.AsyncStatus.Caller == "standbyDownStandbyAS" &&
					instance.Status.AsyncStatus.Id == instance.Status.StandbyDownStandbyAS.AsyncId &&
					instance.Status.AsyncStatus.Type == "repDuplicate" &&
					instance.Status.StandbyDownStandbyAS.Status == "pending" {

					// if this was a RepDuplicate task called by StandbyDownStandbyAS, mark RepDuplicate as done
					instance.Status.StandbyDownStandbyAS.RepDuplicate = true
					reqLogger.V(2).Info(fmt.Sprintf("%s: set instance.Status.StandbyDownStandbyAS.RepDuplicate to true", us))
				}

				reqLogger.V(1).Info(fmt.Sprintf("%s: aysnc task %s on %s complete", us, instance.Status.AsyncStatus.Id, instance.Status.AsyncStatus.Host))

			} else {

				pendingAsyncTask = true

				if asyncStatus.Started != nil {
					agentAsyncTimeout := instance.GetAgentAsyncTimeout()
					asyncTaskElapsed := time.Now().Unix() - *asyncStatus.Started
					asyncTaskTimeRemaining := int64(agentAsyncTimeout) - asyncTaskElapsed

					// if TT_DEBUG is set, enable unit tests to determine where we are via event messages
					_, ok := os.LookupEnv("TT_DEBUG")
					if ok {
						logMsg := fmt.Sprintf("%s: Async polling for %s, timeout in %v secs", us, instance.Status.AsyncStatus.Type, asyncTaskTimeRemaining)
						logTTEvent(ctx, client, instance, "Info", logMsg, false)
					}

					if int64(agentAsyncTimeout) < asyncTaskElapsed {

						errMsg := fmt.Sprintf("Async task %s timed out on %s", instance.Status.AsyncStatus.Type, instance.Status.AsyncStatus.Host)
						logTTEvent(ctx, client, instance, "TaskFailed", errMsg, true)

						reqLogger.V(2).Info(fmt.Sprintf("%s: async task timeout of %d secs exceeded, terminating pod %s",
							us, agentAsyncTimeout, instance.Status.AsyncStatus.PodName))

						if instance.Status.AsyncStatus.Host != "" {

							_, kErr := killAgent(ctx, client, instance, instance.Status.AsyncStatus.Host, instance.Status.AsyncStatus.PodName, tts, nil)
							if kErr != nil {
								reqLogger.V(2).Info(fmt.Sprintf("%s: error returned from killAgent : %v", us, kErr))
								return false, kErr
							}

						} else {
							// TODO: our status object doesn't know the podname!
							reqLogger.Info(fmt.Sprintf("%s: cannot determine the pod to terminate", us))
						}

						reqLogger.Info(fmt.Sprintf("%s: cancelling async task %v, pod was terminated", us, instance.Status.AsyncStatus.Id))
						instance.Status.AsyncStatus.Complete = true
						instance.Status.AsyncStatus.Running = false
						instance.Status.AsyncStatus.Errno = 10
						instance.Status.AsyncStatus.Errmsg = "task timed out"
						instance.Status.AsyncStatus.Updated = time.Now().Unix()

					} else {
						reqLogger.V(1).Info(fmt.Sprintf("%s: timeout=%v asyncTaskElapsed=%v asyncTaskTimeRemaining=%v",
							us, agentAsyncTimeout, asyncTaskElapsed, asyncTaskTimeRemaining))
					}
				}
			}
		}
	} // if an async task is pending

	return pendingAsyncTask, nil

}

// Given the state of the TimesTenClassic object, what flow should we run
// on a particular Pod?
func pickFlowToRun(ctx context.Context, instance *timestenv2.TimesTenClassic, podName string, isP *timestenv2.TimesTenPodStatus) (string, FSMFunc) {
	us := "pickFlowToRun"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + ": entered for POD " + podName)
	defer reqLogger.V(2).Info(us + " returns")

	if isP.TTPodType == "Subscriber" {
		return pickFlowToRunSubscriber(ctx, instance, podName, isP)
	}

	var funcToRun FSMFunc
	var funcName string

	reqLogger.V(1).Info(us + ": [classic obj] HighLevelState=" + instance.Status.HighLevelState)
	reqLogger.V(1).Info(us + ": POD " + podName + " HighLevelState=" + isP.HighLevelState)
	reqLogger.V(1).Info(us + ": POD " + podName + " IntendedState=" + isP.IntendedState)

	replicated := isReplicated(instance)

	if replicated == false {
		// Note that the pod high level states for nonreplicated objects are different than for a/s pairs
		switch hls := isP.HighLevelState; hls {
		case "Initializing":
			funcToRun = nonrepInitializing
			funcName = "nonrepInitializing"
		case "Normal":
			funcToRun = nonrepNormal
			funcName = "nonrepNormal"
		case "Terminal":
			funcToRun = nonrepTerminal
			funcName = "nonrepTerminal"
		case "Down":
			funcToRun = nonrepDown
			funcName = "nonrepDown"
		case "Reexamine":
			funcToRun = reexamineNonrep
			funcName = "reexamineNonrep"
		case "ManualInterventionRequired":
			reqLogger.V(1).Info("ERROR: " + us + " called for pod " + podName + ", pod in ManualInterventionRequired state")
			return "", nil
		case "Unknown":
			reqLogger.V(1).Info(us + ": pod state 'Unknown'")
		default:
			reqLogger.V(1).Info(us + ": unknown pod state " + hls + " for nonreplicated object")
		}

		reqLogger.V(1).Info(us + ": returning '" + funcName + "'")
		return funcName, funcToRun
	}

	// Replicated objects use this code

	switch instance.Status.HighLevelState {
	case "ConfiguringActive":
		switch isP.IntendedState {
		case "Active":
			if isP.PrevIntendedState == "Standby" {
				funcToRun = configureActiveActiveFromStandbyAS
				funcName = "configureActiveActiveFromStandbyAS"
			} else {
				funcToRun = configureActiveActiveFromActiveAS
				funcName = "configureActiveActiveFromActiveAS"
			}
		case "Standby":
			funcToRun = configureActiveStandbyAS
			funcName = "configureActiveStandbyAS"
		case "N/A":
		}

	case "Normal":
		switch isP.IntendedState {
		case "Active":
			funcToRun = checkNormalActiveAS
			funcName = "checkNormalActiveAS"
		case "Standby":
			funcToRun = checkNormalStandbyAS
			funcName = "checkNormalStandbyAS"
		case "N/A":
			// TODO: Active/Active Normal
			funcName = "NA"
			reqLogger.V(1).Info(us + ": IntendedState for pod " + podName + " is N/A, funcToRun not defined")
		}

	case "BothDown":
		switch isP.IntendedState {
		case "Active":
			funcName = "bothDownActiveAS"
			funcToRun = bothDownActiveAS
		case "Standby":
			funcName = "bothDownStandbyAS"
			funcToRun = bothDownStandbyAS
		case "N/A":
			// Should NEVER happen!
		}

	case "StandbyDown":
		switch isP.IntendedState {
		case "Active":
			funcToRun = checkNormalActiveAS
			funcName = "checkNormalActiveAS"
		case "Standby":
			funcToRun = standbyDownStandbyAS
			funcName = "standbyDownStandbyAS"
		case "N/A":
			// Should NEVER happen!
		}

	case "ActiveTakeover":
		switch isP.IntendedState {
		case "Active":
			funcToRun = activeTakeoverActiveAS
			funcName = "activeTakeoverActiveAS"
		case "Standby":
			funcToRun = killDeadStandbyAS
			funcName = "killDeadStandbyAS"
		case "N/A":
			// Should NEVER happen!
		}

	case "StandbyStarting":
		switch isP.IntendedState {
		case "Active":
			funcToRun = checkNormalActiveAS
			funcName = "checkNormalActiveAS"
		case "Standby":
			funcToRun = standbyStartingStandbyAS
			funcName = "standbyStartingStandbyAS"
		case "N/A":
			// Should NEVER happen!
		}

	case "StandbyCatchup":
		switch isP.IntendedState {
		case "Active":
			funcName = "checkNormalActiveAS"
			funcToRun = checkNormalActiveAS
		case "Standby":
			funcName = "standbyCatchupStandbyAS"
			funcToRun = standbyCatchupStandbyAS
		case "N/A":
			// Should NEVER happen!
		}

	case "Reexamine":
		switch isP.IntendedState {
		case "Active":
			if instance.Status.ClassicUpgradeStatus.UpgradeState != "" {
				reqLogger.V(1).Info(us + ": UpgradeState=" +
					instance.Status.ClassicUpgradeStatus.UpgradeState + "; this is the ACTIVE pod, looking for the STANDBY")
				return "", nil
			} else {
				funcName = "reexamineAS"
				funcToRun = reexamineAS
			}

		case "Standby":
			if instance.Status.ClassicUpgradeStatus.UpgradeState != "" {
				reqLogger.V(1).Info(us + ": StandbyStatus=" + instance.Status.ClassicUpgradeStatus.StandbyStatus)
				if instance.Status.ClassicUpgradeStatus.StandbyStatus == "CatchingUp" {
					// if we're CatchingUp then we've already performed the standby steps; proceed with reexamineAS
					funcName = "reexamineAS"
					funcToRun = reexamineAS
				} else {
					funcName = "standbyDownStandbyAS"
					funcToRun = standbyDownStandbyAS
				}
				reqLogger.V(1).Info(us + ": UpgradeState=" + instance.Status.ClassicUpgradeStatus.UpgradeState + "; calling " + funcName)
			} else {
				funcName = "reexamineAS"
				funcToRun = reexamineAS
			}

		case "N/A":
			// Should NEVER happen!
		}

	case "ManualInterventionRequired":
		reqLogger.V(1).Info("ERROR: " + us + " called in ManualInterventionRequired state")
		return "", nil

	case "WaitingForActive":
		switch isP.IntendedState {
		case "Active":
			funcName = "waitingActiveAS"
			funcToRun = waitingActiveAS
		case "Standby":
			funcName = "waitingStandbyAS"
			funcToRun = waitingStandbyAS
		case "N/A":
			// Should NEVER happen!
		}

	case "OneDown":
		// This state only happens in active/active pairs

		// Is this the pod that's down, or the one that's up?

		ourState := isP.HighLevelState
		var otherState string
		if strings.HasSuffix(podName, "0") {
			otherState = instance.Status.PodStatus[1].HighLevelState
		} else {
			otherState = instance.Status.PodStatus[0].HighLevelState
		}

		if ourState == "Down" || ourState == "Unknown" || otherState == "OtherDown" {
			// We're the dead one
			// TODO: Run something
		} else {
			// We're the alive one
			// TODO: Run something
		}

	case "ActiveDown":
		switch isP.IntendedState {
		case "Active":
			funcToRun = activeDownActiveAS // Deactivate this
			funcName = "activeDownActiveAS"
		case "Standby":
			funcToRun = checkActiveDownStandbyAS // Promote this
			funcName = "checkActiveDownStandbyAS"
		case "N/A":
			// Should NEVER happen!
		}

	case "Initializing":
		switch isP.IntendedState {
		case "Active":
			funcToRun = initializeActiveAS
			funcName = "initializeActiveAS"
		case "Standby":
			funcToRun = initializeStandbyAS
			funcName = "initializeStandbyAS"
		case "N/A": // Active/Active
			if strings.HasSuffix(podName, "0") {
				// TODO: Active/Active initialization of first db
			} else {
				// TODO: Active/Active initialization of second db
			}
		}

	}

	reqLogger.V(1).Info(us + ": returning " + funcName)
	return funcName, funcToRun
}

// Given the state of the TimesTenClassic object, what flow should we run
// on a particular subscriber?
func pickFlowToRunSubscriber(ctx context.Context, instance *timestenv2.TimesTenClassic, podName string, isP *timestenv2.TimesTenPodStatus) (string, FSMFunc) {
	us := "pickFlowToRunSubscriber"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + ": entered for POD " + podName)
	defer reqLogger.V(2).Info(us + " returns")

	var funcToRun FSMFunc
	var funcName string

	reqLogger.V(1).Info(us + ": [classic obj] HighLevelState=" + instance.Status.HighLevelState)
	reqLogger.V(1).Info(us + ": POD " + podName + " HighLevelState=" + isP.HighLevelState)
	reqLogger.V(1).Info(us + ": POD " + podName + " IntendedState=" + isP.IntendedState)

	switch isP.HighLevelState {
	case "Normal":
		funcName = "subscriberNormal"
		funcToRun = subscriberNormal

	case "NotProvisioned":
		funcName = "subscriberNotProvisioned"
		funcToRun = subscriberNotProvisioned

	case "Down":
		funcName = "subscriberDown"
		funcToRun = subscriberDown

		//case "Unknown":
		//funcName = "subscriberUnknown"
		//funcToRun = subscriberUnknown

	case "Terminal", "UpgradeFailed":
		funcName = "subscriberFailed"
		funcToRun = subscriberFailed

	case "CatchingUp":
		funcName = "subscriberCatchingUp"
		funcToRun = subscriberCatchingUp

	case "Initializing":
		funcName = "subscriberInitializing"
		funcToRun = subscriberInitializing

	case "OtherDown", "HealthyActive", "HealthyStandby": // Ones that can't happen to subscribers
		msg := us + ": called with unexpected state '" + isP.HighLevelState + "'"
		reqLogger.V(1).Info(msg)

	default:
		msg := us + ": called with unexpected state '" + isP.HighLevelState + "'"
		reqLogger.V(1).Info(msg)
	}

	reqLogger.V(1).Info(us + ": returning " + funcName)
	return funcName, funcToRun
}

// Get status of one Pod from Kubernetes
func getKubernetesPodStatus(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject, podName string) (error, *corev1.Pod) {
	//reqLogger := log.FromContext(ctx)
	pod := &corev1.Pod{}
	err := client.Get(ctx, types.NamespacedName{Name: podName, Namespace: instance.ObjectNamespace()}, pod)
	if err != nil {
		return err, nil
	}
	return nil, pod
}

// Get updated info about each pod
func getUpdatedInfoAboutPods(ctx context.Context, client client.Client, scheme *runtime.Scheme, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo) error {
	us := "getUpdatedInfoAboutPods"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	_ /* replicated */, nReplicas, _ /* nSubscribers */, maxNSubscribers, _ /* subName */ := replicationConfig(instance)

	// Let's get updated information about each pod

	// Carefully COPY the old status
	prevPodStatus := make([]timestenv2.TimesTenPodStatus, nReplicas+maxNSubscribers)
	for i, _ := range instance.Status.PodStatus {
		prevPodStatus[i] = instance.Status.PodStatus[i]
	}

	for i, _ := range instance.Status.PodStatus {
		isP := &instance.Status.PodStatus[i]
		podName := isP.Name
		err, ppod := getKubernetesPodStatus(ctx, client, instance, podName)
		if err == nil {
			isP.HasBeenSeen = true
		} else {
			if isP.HasBeenSeen {
				// If the pod used to exist then this is very strange
				reqLogger.Error(err, fmt.Sprintf("%s: Can not fetch status of pod %s : %v", us, podName, err.Error()))
			} else {
				// If the pod has never existed it's not a huge issue
				reqLogger.V(1).Info(fmt.Sprintf("%s: Can not fetch status of pod %s : %v", us, podName, err.Error()))
			}
			var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
			if isPermissionsProblem {
				//Checks if the error was because of lack of permission, if not, return the original message
				logTTEvent(ctx, client, instance, "FailedGetStatus", "Failed to get pod status: "+errorMsg, true)
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				return err
			}
			continue
		}
		pod := *ppod

		reqLogger.V(1).Info(fmt.Sprintf("%s: POD %s status : %v", us, podName, pod.Status))

		isP.PodStatus.PodIP = pod.Status.PodIP
		isP.PodStatus.PodPhase = string(pod.Status.Phase)

		err, podReplaced := checkStatusOfContainersInPod(ctx, client, instance, ppod, isP)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			logTTEvent(ctx, client, instance, "Failed", err.Error(), true)
			if instance.Status.HighLevelState == "Initializing" {
				// If things are dying and we haven't even finished starting up
				// let's just fail.
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
			}
		}

		podDNSName := podName + "." + instance.Name + "." + instance.Namespace + ".svc.cluster.local"

		reqLogger.V(1).Info(us + ": Pod " + podName + " Phase '" + isP.PodStatus.PodPhase + "'")

		// If the pod was replaced - i.e., we are now looking at a different pod
		// than we saw the last time - then the agent is surely down (how
		// would it have been started?).
		//
		// So we don't need to talk to the agent, we KNOW it's not running

		if podReplaced {
			isP.PodStatus.Agent = "Down"
			isP.DbStatus.Db = "Unknown"
			isP.DbStatus.DbUpdatable = "Unknown"
			isP.TimesTenStatus.Instance = "Unknown"
			isP.TimesTenStatus.Daemon = "Unknown"
			isP.ReplicationStatus.RepAgent = "Unknown"
			isP.ReplicationStatus.RepState = "Unknown"
			isP.ReplicationStatus.RepScheme = "Unknown"
			isP.ReplicationStatus.RepPeerPState = "Unknown"
			isP.ReplicationStatus.RepPeerPStateFetchErr = ""
			isP.CacheStatus.CacheAgent = "Unknown"
			isP.CacheStatus.AwtBehindMb = nil
		} else {
			reqLogger.V(1).Info(us + ": calling getPodStatus() on pod=" + podName + " podDNSName=" + podDNSName)
			getPodStatus(ctx, instance, podDNSName, isP, client, tts, nil)

			if prevPodStatus[i].ReplicationStatus.RepState != isP.ReplicationStatus.RepState {
				isP.ReplicationStatus.LastTimeRepStateChanged = time.Now().Unix()
			}
		}

		checkCGroupInfo(ctx, client, instance, &pod, isP)

	}
	return nil
}

// See if the exporter secret exists (if needed), and create it if it doesn't
func checkExporterSecrets(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, scheme *runtime.Scheme,
	exporterSecretName string) (err error) {
	us := "checkExporterSecrets"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	if instance.Spec.TTSpec.Prometheus != nil &&
		instance.Spec.TTSpec.Prometheus.Insecure != nil &&
		*instance.Spec.TTSpec.Prometheus.Insecure == true {
		return errors.New(fmt.Sprintf("%s called but no secret needed", us))
	}

	serverSecret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Name: exporterSecretName, Namespace: instance.Namespace}, serverSecret)
	if err != nil {
		var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
		if isPermissionsProblem {
			logTTEvent(ctx, client, instance, "FailedGetStatus", "Failed to get secret: "+errorMsg, true)
			updateTTClassicHighLevelState(ctx, instance, "Failed", client)
			return err
		}
		if k8sErrors.IsNotFound(err) {
			if instance.Status.ExporterSecret != nil {
				// the secret did exist as some point, but the client req failed to find it
				logTTEvent(ctx, client, instance, "Disappeared", fmt.Sprintf("Exporter secret %s no longer exists, replacing",
					exporterSecretName), true)
			}
			err, _ /*clientSecret*/ := makeExporterSecrets(ctx, instance, scheme, client, exporterSecretName)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				msg := fmt.Sprintf("%s: Could not create new exporter secret: %s", us, errorMsg)
				reqLogger.V(1).Info(msg)
				logTTEvent(ctx, client, instance, "FailedCreate", msg, true)
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				return err
			}
			instance.Status.ExporterSecret = newString(exporterSecretName)
			return nil
		} else {
			reqLogger.Error(err, us+": Could not fetch Secret from Kubernetes : "+err.Error())
			return err
		}
	} else {
		// secret returned from client
		if instance.Status.ExporterSecret == nil {
			instance.Status.ExporterSecret = newString(exporterSecretName)
		}
	}

	return nil
}

// See if our main StatefulSet is created and healthy, and create it if it doesn't exist
// If the user has updated the TimesTenClassic object we also update the StatefulSets to
// pass some of the updated values down to it
func checkMainStatefulSet(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, scheme *runtime.Scheme) (updatedImage bool, err error) {
	us := "checkMainStatefulSet"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var timeSinceLastState int64

	// Check if our main StatefulSet already exists; create it if not; update it if required
	foundSS := &appsv1.StatefulSet{}
	err = client.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundSS)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			err, ss := newStatefulSet(ctx, client, scheme, instance, false)
			if err != nil {
				reqLogger.V(1).Info(us + ": Could not create new prototype StatefulSet: " + err.Error())
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "FailedCreate", "StatefulSet creation failed: "+errorMsg, true)
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				return false, err
			}

			if err := controllerutil.SetControllerReference(instance, ss, scheme); err != nil {
				reqLogger.V(1).Info(us + ": Could not set StatefulSet controller reference: " + err.Error())
				return false, err
			}

			reqLogger.V(1).Info(us+": Creating a new StatefulSet", "Pod.Namespace", ss.Namespace, "StatefulSet.Name", ss.Name)
			err = client.Create(ctx, ss)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "FailedCreate", "StatefulSet creation failed: "+errorMsg, true)
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				return false, err
			}

			// StatefulSet created successfully - don't requeue
			logTTEvent(ctx, client, instance, "Create", "StatefulSet "+ss.Name+" created", false)
			return false, err
		} else {
			reqLogger.Error(err, us+": Could not fetch StatefulSet from Kubernetes : "+err.Error())
			return false, err
		}
	}

	// Does the statefulset have the same attributes as we do?  Our CRD may have been
	// updated since we created the statefulset. If so we need to push the modifications
	// to the statefulset as well.

	// For now the only attribute we act on is 'image'

	updatedImage = false

	for i, _ := range foundSS.Spec.Template.Spec.Containers {
		reqLogger.V(2).Info(us + ": examining " + foundSS.Spec.Template.Spec.Containers[i].Name)

		if foundSS.Spec.Template.Spec.Containers[i].Name == "tt" ||
			foundSS.Spec.Template.Spec.Containers[i].Name == "daemonlog" ||
			foundSS.Spec.Template.Spec.Containers[i].Name == "exporter" {
			reqLogger.V(2).Info(us + ": instance.Spec.TTSpec.Image=" + instance.Spec.TTSpec.Image)
			reqLogger.V(2).Info(fmt.Sprintf("%s: ImageUpdatePending=%v", us, instance.Status.ClassicUpgradeStatus.ImageUpdatePending))
			reqLogger.V(2).Info(fmt.Sprintf("%s: foundSS.Spec.Template.Spec.Containers[%d].Image=%v", us, i, foundSS.Spec.Template.Spec.Containers[i].Image))

			if instance.Spec.TTSpec.Image == "" {
				reqLogger.V(2).Info(fmt.Sprintf("%s: WARN instance.Spec.TTSpec.Image is empty (%d)", us, i))
			} else {
				// if existing image equals the new image
				if foundSS.Spec.Template.Spec.Containers[i].Image == instance.Spec.TTSpec.Image {
					// SS image is the same as the spec, nothing to do
				} else {

					reqLogger.Info(fmt.Sprintf("%s: instance.Status.PodStatus=%v", us, instance.Status.PodStatus))
					reqLogger.Info(fmt.Sprintf("%s: foundSS.Spec.Template.Spec.Containers[%v].Image=%v", us, i, foundSS.Spec.Template.Spec.Containers[i].Image))

					// set the PrevImage attrib for PodStatus to the old image
					for y, _ := range instance.Status.PodStatus {
						instance.Status.PodStatus[y].PrevImage = foundSS.Spec.Template.Spec.Containers[i].Image
						reqLogger.V(1).Info(fmt.Sprintf("%s: instance.Status.PodStatus[%d].PrevImage set to %v",
							us, y, instance.Status.PodStatus[y].PrevImage))
					}

					foundSS.Spec.Template.Spec.Containers[i].Image = instance.Spec.TTSpec.Image
					updatedImage = true
					reqLogger.Info(fmt.Sprintf("%s: image changed for container %v, image=%v", us, foundSS.Spec.Template.Spec.Containers[i].Name,
						foundSS.Spec.Template.Spec.Containers[i].Image))
				}
			}

			if instance.Status.ClassicUpgradeStatus.ImageUpdatePending == true &&
				*instance.Spec.TTSpec.ImageUpgradeStrategy != "Manual" {
				// add log entry if the current image is the same as the prev image
				for _, isP := range instance.Status.PodStatus {
					if isP.PrevImage != "" &&
						instance.Spec.TTSpec.Image == isP.PrevImage {
						reqLogger.V(1).Info(us + ": ImageUpdatePending=true, but the current image is the same " +
							"as the previous image for pod " + isP.Name)
						// set ImageUpdatePending=false if we ever want to disallow this
						//instance.Status.ClassicUpgradeStatus.ImageUpdatePending = false
					}
				}

				msg := "setting updatedImage=true since ImageUpdatePending=true and ImageUpgradeStrategy=Auto"
				reqLogger.V(1).Info(us + ": " + msg)
				updatedImage = true
			}

		} else {
			// For user-specified containers see if the user has modified the image in the
			// TimesTenClassic object

			if instance.Spec.Template != nil && instance.Spec.Template.Spec.Containers != nil {
				for j, _ := range instance.Spec.Template.Spec.Containers {
					if instance.Spec.Template.Spec.Containers[j].Name == foundSS.Spec.Template.Spec.Containers[i].Name {
						if instance.Spec.Template.Spec.Containers[j].Image == foundSS.Spec.Template.Spec.Containers[i].Image {
						} else {
							foundSS.Spec.Template.Spec.Containers[i].Image = instance.Spec.Template.Spec.Containers[j].Image
							updatedImage = true
							reqLogger.V(1).Info(us + ": Changing image for user container '" + foundSS.Spec.Template.Spec.Containers[i].Name +
								"' to '" + instance.Spec.Template.Spec.Containers[j].Image + "'")
						}
					}
				}
			}
		}
	}
	for i, _ := range foundSS.Spec.Template.Spec.InitContainers {
		if foundSS.Spec.Template.Spec.InitContainers[i].Name == "ttinit" {
			if instance.Spec.TTSpec.Image != "" &&
				foundSS.Spec.Template.Spec.InitContainers[i].Image == instance.Spec.TTSpec.Image {
			} else {
				reqLogger.Info(us + ": image changed for container ttinit, image=" + instance.Spec.TTSpec.Image)
				foundSS.Spec.Template.Spec.InitContainers[i].Image = instance.Spec.TTSpec.Image
				updatedImage = true
			}
		} else {
			// For user-specified containers see if the user has modified the image in the
			// TimesTenClassic object

			for j, _ := range instance.Spec.Template.Spec.InitContainers {
				if instance.Spec.Template.Spec.InitContainers[j].Name == foundSS.Spec.Template.Spec.InitContainers[i].Name {
					if instance.Spec.Template.Spec.InitContainers[j].Image == foundSS.Spec.Template.Spec.InitContainers[i].Image {
					} else {
						foundSS.Spec.Template.Spec.InitContainers[i].Image = instance.Spec.Template.Spec.InitContainers[j].Image
						updatedImage = true
						reqLogger.Info(us + ": image changed for user init container '" + foundSS.Spec.Template.Spec.InitContainers[i].Name +
							"', image=" + instance.Spec.Template.Spec.InitContainers[j].Image)
					}
				}
			}
		}
	}

	replicated := isReplicated(instance)
	if updatedImage && replicated == false {
		err = client.Update(ctx, foundSS)
		if err == nil {
			reqLogger.V(1).Info(us + ": updated StatefulSet with new image")
		} else {
			reqLogger.V(1).Info(us + ": could ot update StatefulSet with new image")
			//Checks if the error was because of lack of permission, if not, return the original message
			var errorMsg, _ = verifyUnauthorizedError(err.Error())
			logTTEvent(ctx, client, instance, "FailedUpdate", "StatefulSet update failed: "+errorMsg, true)
			return false, err
		}
	}

	if updatedImage && replicated == true {
		if instance.Status.ClassicUpgradeStatus.UpgradeState != "" {
			reqLogger.Info(fmt.Sprintf("%sc: updatedImage=%v but there is an upgrade in progress, UpgradeState=%v", us, updatedImage,
				instance.Status.ClassicUpgradeStatus.UpgradeState))
			updatedImage = false
			err = client.Update(ctx, foundSS)
			if err == nil {
				reqLogger.V(1).Info(us + ": updated StatefulSet with new image")
			} else {
				reqLogger.V(1).Info(us + ": could ot update StatefulSet with new image")
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "FailedUpdate", "StatefulSet update failed: "+errorMsg, true)
				return false, err
			}

			if instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch != 0 {
				timeSinceLastState = time.Now().Unix() - instance.Status.ClassicUpgradeStatus.LastUpgradeStateSwitch
				reqLogger.V(1).Info(fmt.Sprintf("%s: timeSinceLastState was %v secs", us, timeSinceLastState))

				// TODO: if we want to implement a timeout
				//
				//if timeSinceLastState > int64(upgradeDownPodTimeout) {
				//    errMsg := "Upgrade timeout, resetting upgrade attributes"
				//    reqLogger.V(1).Info(fmt.Sprintf("%s: %v, resetting upgrade vars", us, errMsg))
				//    logTTEvent(ctx, client, instance, "StateChange", errMsg, "Normal")
				//    resetUpgradeVars(instance)
				//} else {
				//    reqLogger.V(1).Info(fmt.Sprintf("%s: ongoing upgrade for %v secs, timeout in %v secs", us,
				//                               timeSinceLastState, int64(upgradeDownPodTimeout)-timeSinceLastState))
				//}
			}

			// TODO: if we want to automatically delete the standby once we detect and image change

			//if instance.Status.HighLevelState == "Reexamine" &&
			//   (instance.Status.PodStatus[0].HighLevelState == "UpgradeFailed" ||
			//    instance.Status.PodStatus[1].HighLevelState == "UpgradeFailed") {
			//
			//    updatedImage = true
			//    reqLogger.V(1).Info(fmt.Sprintf("%s: HighLevelState=%v, StandbyStatus=%v set updatedImage=%v", us,
			//                               instance.Status.HighLevelState, instance.Status.ClassicUpgradeStatus.StandbyStatus, updatedImage))
			//    return updatedImage, err
			//} else {
			//    updatedImage = false
			//    reqLogger.V(1).Info(fmt.Sprintf("%s: HighLevelState is not Reexamine, set updatedImage=%v", us, updatedImage))
			//}

		} else {
			err = client.Update(ctx, foundSS)
			if err == nil {
				reqLogger.Info(us + ": updated StatefulSet with new image")
				if *instance.Spec.TTSpec.ImageUpgradeStrategy == "Manual" {
					reqLogger.Info(us + ": ImageUpgradeStrategy set to Manual, not performing automatic upgrade")
					logTTEvent(ctx, client, instance, "Upgrade", "Image updated, automatic upgrade disabled", true)

					// ImageUpdatePending will be picked off if imageUpgradeStrategy changes to Auto
					instance.Status.ClassicUpgradeStatus.ImageUpdatePending = true
					reqLogger.Info(us + ": setting ImageUpdatePending to true")
					// we're not going to upgrade now, ImageUpgradeStrategy=Manual
					updatedImage = false

				} else {
					// auto is the default, so if it's not manual set it to auto
					if *instance.Spec.TTSpec.ImageUpgradeStrategy != "Auto" {
						*instance.Spec.TTSpec.ImageUpgradeStrategy = "Auto"
						reqLogger.V(1).Info(us + ": ImageUpgradeStrategy set to " + *instance.Spec.TTSpec.ImageUpgradeStrategy)
					}
					reqLogger.Info(us + ": performing automatic upgrade")
					logTTEvent(ctx, client, instance, "Upgrade", "Image updated, automatic upgrade started", false)
				}
			} else {
				reqLogger.Error(err, us+": could not update StatefulSet with new image: "+err.Error())
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "FailedUpdate", "Could not update StatefulSet with new image: "+errorMsg, true)
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				// TODO: something went wrong if we can't update the image!
			}
			// TODO: we're not exiting the reconcile here; that may have been expected from the above comment!
			return updatedImage, err
		}
	}

	return false, nil
}

// See if the StatefulSet for our subscribers is created and healthy, and create it if it doesn't exist
// TODO: This is a skeleton for now. The online upgrade code is largely stripped out for now, but needs
// to be added again at some point.
func checkSubscriberStatefulSet(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, scheme *runtime.Scheme) (updatedImage bool, err error) {
	us := "checkSubscriberStatefulSet"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	if instance.Spec.TTSpec.Subscribers == nil {
		return false, nil
	}
	if instance.Spec.TTSpec.Subscribers.Replicas == nil {
		return false, nil
	}

	setName := getSubscriberName(instance)

	// Check if the StatefulSet already exists; create it if not; update it if required
	foundSS := &appsv1.StatefulSet{}
	err = client.Get(ctx, types.NamespacedName{Name: setName, Namespace: instance.Namespace}, foundSS)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			err, ss := newStatefulSet(ctx, client, scheme, instance, true)
			if err != nil {
				reqLogger.V(1).Info(us + ": Could not create new prototype StatefulSet: " + err.Error())
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "FailedCreate", "StatefulSet creation failed: "+errorMsg, true)
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				return false, err
			}

			if err := controllerutil.SetControllerReference(instance, ss, scheme); err != nil {
				reqLogger.V(1).Info(us + ": Could not set StatefulSet controller reference: " + err.Error())
				return false, err
			}

			reqLogger.V(1).Info(us+": Creating a new StatefulSet", "Pod.Namespace", ss.Namespace, "StatefulSet.Name", ss.Name)
			err = client.Create(ctx, ss)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "FailedCreate", "StatefulSet creation failed: "+errorMsg, true)
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				return false, err
			}

			// StatefulSet created successfully - don't requeue
			logTTEvent(ctx, client, instance, "Create", "StatefulSet "+ss.Name+" created", false)
			return false, err
		} else {
			reqLogger.Error(err, us+": Could not fetch StatefulSet from Kubernetes : "+err.Error())
			return false, err
		}
	} else {
		// The StatefulSet already existed. Does it have the right options?

		if instance.Status.Subscriber.Surplusing == false {
			curNSubs := 0
			if foundSS.Spec.Replicas != nil {
				curNSubs = (int)(*foundSS.Spec.Replicas)
			}
			ttNSubs := 0
			if instance.Spec.TTSpec.Subscribers != nil {
				if instance.Spec.TTSpec.Subscribers.Replicas != nil {
					ttNSubs = *instance.Spec.TTSpec.Subscribers.Replicas
				}
			}

			if curNSubs != ttNSubs {
				if curNSubs > ttNSubs {
					// We have provisioned too many subscribers! We need to shut
					// some of them down.
					// But we won't do that here, we'll do it later. See
					// determineNewSubscriberHLState.

					logTTEvent(ctx, client, instance, "Change", fmt.Sprintf("Number of subscribers reduced from %d to %d, Surplusing begins", curNSubs, ttNSubs), false)
					instance.Status.Subscriber.Surplusing = true
					instance.Status.Subscriber.NewReplicas = ttNSubs
					instance.Status.Subscriber.PrevReplicas = curNSubs
				} else if curNSubs < ttNSubs {
					// We haven't provisioned enough subscribers! We need to tell
					// Kubernetes to make more of them.
					logTTEvent(ctx, client, instance, "Change", fmt.Sprintf("Number of subscribers increased from %d to %d", curNSubs, ttNSubs), false)
					foundSS.Spec.Replicas = newInt32(int32(ttNSubs))
					err = client.Update(ctx, foundSS)
					if err == nil {

					} else {
						//Checks if the error was because of lack of permission, if not, return the original message
						var errorMsg, _ = verifyUnauthorizedError(err.Error())
						logTTEvent(ctx, client, instance, "FailedUpdate", "Could not update StatefulSet "+getSubscriberName(instance)+": "+errorMsg, true)

					}
				}

			}
		}
	}

	// TODO: The online upgrade triggering code needs to be added here again, in some form, probably.

	return false, nil
}

// See if our Service is created and healthy, and create it if it doesn't exist
// TODO: We don't really check to see if it's 'healthy', whatever that means
func checkService(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, scheme *runtime.Scheme) (err error) {
	reqLogger := log.FromContext(ctx)

	// Check if our headless Service already exists; create if it not
	foundSrv := &corev1.Service{}
	err = client.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundSrv)
	if err != nil {
		var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
		if isPermissionsProblem {
			logTTEvent(ctx, client, instance, "FailedGetStatus", "Failed to get service status: "+errorMsg, true)
			return err
		}
		if k8sErrors.IsNotFound(err) {
			reqLogger.Info("Creating a new Service", "Service.Namespace", instance.Namespace, "Service.Name", instance.Name)
			// Define a headless Service object if there isn't already one

			srv := newHeadlessService(ctx, instance, scheme, client)

			// Set TimesTenClassic instance as the owner and controller
			if err := controllerutil.SetControllerReference(instance, srv, scheme); err != nil {
				reqLogger.Error(err, "Could not set service owner / controller: "+err.Error())
				return err
			}

			err = client.Create(ctx, srv)
			if err != nil {
				reqLogger.Error(err, "Service creation failed: "+err.Error())

				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "FailedCreate", "Service creation failed: "+errorMsg, true)

				return err
			}
			logTTEvent(ctx, client, instance, "Create", "Service "+srv.Name+" created", false)
		} else {
			reqLogger.Error(err, "Unexpected error : "+err.Error())
			return err
		}
	}
	return nil
}

func getCurrActiveStandby(ctx context.Context, instance *timestenv2.TimesTenClassic) (error, map[string]int) {
	reqLogger := log.FromContext(ctx)
	us := "getCurrActiveStandby"
	reqLogger.V(2).Info(us + " called")
	defer reqLogger.V(2).Info(us + " exits")

	pairStates := make(map[string]int)

	for podNo := 0; podNo < 2; podNo++ {
		name := fmt.Sprintf("%s-%d", instance.Name, podNo)
		reqLogger.V(2).Info(fmt.Sprintf("%s is '%s'", name, instance.Status.PodStatus[podNo].ReplicationStatus.RepState))
		if instance.Status.PodStatus[podNo].ReplicationStatus.RepState == "ACTIVE" {
			pairStates["activePodNo"] = podNo
			reqLogger.V(2).Info(fmt.Sprintf("%s: pod %s is the active", us, name))
		} else if instance.Status.PodStatus[podNo].ReplicationStatus.RepState == "STANDBY" {
			pairStates["standbyPodNo"] = podNo
			reqLogger.V(2).Info(fmt.Sprintf("%s: pod %s is the standby", us, name))
		}
	}

	a, aok := pairStates["activePodNo"]
	s, sok := pairStates["standbyPodNo"]
	if aok {
		if sok {
			// Great!
		} else {
			if a == 0 {
				s = 1
			} else {
				s = 0
			}
		}
	} else {
		if sok {
			if s == 0 {
				a = 1
			} else {
				a = 0
			}
		} else {
			msg := fmt.Sprintf("%s: could not determine active nor standby", us)
			reqLogger.V(1).Info(msg)
			return errors.New(msg), pairStates
		}
	}

	pairStates["activePodNo"] = a
	pairStates["standbyPodNo"] = s

	reqLogger.V(1).Info(fmt.Sprintf("%s: pairStates=%v", us, pairStates))

	return nil, pairStates
}

// cleanup cache objects, called by the cache cleanup finalizer
func cleanupTTCache(ctx context.Context, client client.Client, request reconcile.Request, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo) error {
	us := "cleanupTTCache"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("cleanupTTCache entered")
	defer reqLogger.V(2).Info("cleanupTTCache returns")

	var err error
	var activePodNo int
	var standbyPodNo int
	var activePodName string
	var standbyPodName string

	// TODO: need to do cleanup on subscribers, too

	for podNo := 0; podNo < getNReplicas(instance); podNo++ {
		podName := instance.Name + "-" + strconv.Itoa(podNo)
		if instance.Status.PodStatus[podNo].ReplicationStatus.RepState == "ACTIVE" {
			activePodNo = podNo
			activePodName = podName
			reqLogger.V(1).Info(us + ": pod " + activePodName + " is the ACTIVE")
		} else if instance.Status.PodStatus[podNo].ReplicationStatus.RepState == "STANDBY" {
			standbyPodNo = podNo
			standbyPodName = podName
			reqLogger.V(1).Info(us + ": pod " + standbyPodName + " is the STANDBY")
		}
	}

	reqLogger.V(1).Info(us + ": HighLevelState '" + instance.Status.HighLevelState + "'")

	switch hl := instance.Status.HighLevelState; hl {
	case "Normal":

		// ACTIVE

		reqParamsActive := make(map[string]string)
		reqParamsActive["hlState"] = hl
		reqParamsActive["repState"] = "active"
		reqParamsActive["activePodName"] = activePodName
		reqParamsActive["standbyPodName"] = standbyPodName
		reqParamsActive["dbName"] = request.Name

		reqLogger.V(1).Info(us + ": calling RunAction cleanupCache on the active (pod " + activePodName + ")")
		reqLogger.V(1).Info(fmt.Sprintf("%s: reqParamsActive=%v", us, reqParamsActive))

		err = RunAction(ctx, instance, activePodNo, "cleanupCache", reqParamsActive, client, tts, nil)
		if err != nil {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache failed on the active")
		} else {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache successful on the active")
		}

		// STANDBY

		reqParamsStandby := make(map[string]string)
		reqParamsStandby["hlState"] = hl
		reqParamsStandby["repState"] = "standby"
		reqParamsStandby["activePodName"] = activePodName
		reqParamsStandby["standbyPodName"] = standbyPodName
		reqParamsStandby["dbName"] = request.Name

		reqLogger.V(1).Info(us + ": calling RunAction cleanupCache on the standby (pod " + standbyPodName + ")")
		reqLogger.V(1).Info(fmt.Sprintf("%s: reqParamsStandby=%v", us, reqParamsStandby))

		err = RunAction(ctx, instance, standbyPodNo, "cleanupCache", reqParamsStandby, client, tts, nil)
		if err != nil {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache failed on the standby")
		} else {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache successful on the standby")
		}

	case "StandbyDown", "StandbyStarting", "ActiveTakeover":

		// ACTIVE

		reqParamsActive := make(map[string]string)

		reqParamsActive["hlState"] = hl
		reqParamsActive["repState"] = "active"
		reqParamsActive["activePodName"] = activePodName
		reqParamsActive["standbyPodName"] = standbyPodName
		reqParamsActive["dbName"] = request.Name

		// run the oracle cleanup scripts (we do not do this for hl state=NORMAL)
		reqParamsActive["runOraScript"] = "yes"

		// run the cleanup script for the previous standby
		if len(standbyPodName) > 0 {
			reqParamsActive["runOraScriptForOther"] = standbyPodName
		} else {
			if activePodNo == 0 {
				reqParamsActive["runOraScriptForOther"] = request.Name + "-1"
			} else if activePodNo == 1 {
				reqParamsActive["runOraScriptForOther"] = request.Name + "-0"
			} else {
				reqLogger.V(1).Info(us + ": unable to determine the standby podName, will not run oracle cleanup script on standby")
			}
		}

		reqLogger.V(1).Info(us + ": calling RunAction cleanupCache on the ACTIVE (pod " + activePodName + ")")
		reqLogger.V(1).Info(fmt.Sprintf("%s: reqParamsActive=%v", us, reqParamsActive))

		err = RunAction(ctx, instance, activePodNo, "cleanupCache", reqParamsActive, client, tts, nil)
		if err != nil {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache failed on the ACTIVE")
		} else {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache successful on the ACTIVE")
		}

	case "ActiveDown":

		// we are the STANDBY

		reqParamsStandby := make(map[string]string)
		reqParamsStandby["hlState"] = hl
		reqParamsStandby["repState"] = "standby"
		reqParamsStandby["activePodName"] = activePodName
		reqParamsStandby["standbyPodName"] = standbyPodName

		// run the oracle cleanup scripts (we do not do this for hl state=NORMAL)
		reqParamsStandby["runOraScript"] = "yes"

		// run the cleanup script for the previous active
		if len(activePodName) > 0 {
			reqParamsStandby["runOraScriptForOther"] = activePodName
		} else {
			if standbyPodNo == 0 {
				reqParamsStandby["runOraScriptForOther"] = request.Name + "-1"
			} else if standbyPodNo == 1 {
				reqParamsStandby["runOraScriptForOther"] = request.Name + "-0"
			} else {
				reqLogger.V(1).Info(us + ": unable to determine the previously active podName, will not run oracle cleanup script for that host")
			}
		}

		reqLogger.V(1).Info(us + ": calling RunAction cleanupCache on the STANDBY (pod " + standbyPodName + ")")
		reqLogger.V(1).Info(fmt.Sprintf("%s: reqParamsStandby=%v", us, reqParamsStandby))

		err = RunAction(ctx, instance, standbyPodNo, "cleanupCache", reqParamsStandby, client, tts, nil)
		if err != nil {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache failed on the STANDBY")
		} else {
			reqLogger.V(1).Info(us + ": RunAction cleanupCache successful on the STANDBY")
		}

	case "Initializing", "Failed", "BothDown":
		// TODO: perform cleanup from operator in future release (operator needs creds)
		reqLogger.V(1).Info(us + ": currentHighLevelState=" + hl + "; do nothing")

	default:
		reqLogger.V(1).Info(us + ": Unknown HighLevelState '" + hl + "'")
	}

	return nil
}

// newStatefulSet returns a StatefulSet pod with the same name/namespace as the cr
func newStatefulSet(ctx context.Context, client client.Client, scheme *runtime.Scheme, cr *timestenv2.TimesTenClassic, subscriber bool) (error, *appsv1.StatefulSet) {
	us := "newStatefulSet"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var ourerror error

	userSpecifiedTTConfig := false
	if cr.Spec.Template != nil && cr.Spec.Template.Spec.Volumes != nil {
		for _, v := range cr.Spec.Template.Spec.Volumes {
			if v.Name == "tt-config" {
				userSpecifiedTTConfig = true
			}
		}
	}

	labelMap := map[string]string{
		"app":                 cr.Name,
		"database.oracle.com": cr.Name,
	}

	ipp := corev1.PullIfNotPresent
	if cr.Spec.TTSpec.ImagePullPolicy != nil {
		if *cr.Spec.TTSpec.ImagePullPolicy == "Always" {
			ipp = corev1.PullAlways
		} else if *cr.Spec.TTSpec.ImagePullPolicy == "Never" {
			ipp = corev1.PullNever
		} else if *cr.Spec.TTSpec.ImagePullPolicy == "IfNotPresent" {
			ipp = corev1.PullIfNotPresent
		}
	}

	ttContainer := corev1.Container{
		Name:            "tt",
		Image:           cr.Spec.TTSpec.Image,
		ImagePullPolicy: ipp,
		Ports: []corev1.ContainerPort{
			{
				Name:          "agent",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "daemon",
				ContainerPort: 6624,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "cs",
				ContainerPort: 6625,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "repl",
				ContainerPort: int32(cr.Status.RepPort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             newBool(true),
			Privileged:               newBool(false),
			AllowPrivilegeEscalation: newBool(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Resources: corev1.ResourceRequirements{},
	}

	replicated := isReplicated(cr)
	if replicated == true {

		ttContainer.ReadinessProbe =
			&corev1.Probe{
				InitialDelaySeconds: 1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh", "-c", "cat /tmp/active"},
					},
				},
			}

	} else if replicated == false {

		ttContainer.ReadinessProbe =
			&corev1.Probe{
				InitialDelaySeconds: 1,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh", "-c", "cat /tmp/readiness"},
					},
				},
			}

		ttContainer.Lifecycle =
			&corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/bin/sh", "-c",
							"/usr/bin/perl /timesten/operator/operator/starthost.pl -quiesce > /tt/home/timesten/preStopHook.log 2>&1"},
					},
				},
			}
	}

	// If the user specified .spec.template.spec.containers[]
	// for the container named "tt" then use a number of
	// user specified attributes as defaults.

	userTemplate := (*cr).Spec.Template
	if subscriber {
		if (*cr).Spec.TTSpec.Subscribers != nil &&
			(*cr).Spec.TTSpec.Subscribers.Template != nil {
			userTemplate = (*cr).Spec.TTSpec.Subscribers.Template
		}
	}
	if userTemplate != nil {
		if userTemplate.Spec.Containers != nil {
			for _, c := range userTemplate.Spec.Containers {
				if c.Name == "tt" {
					ttContainer.Env = c.Env
					ttContainer.EnvFrom = c.EnvFrom
					if c.Lifecycle != nil {
						ttContainer.Lifecycle = c.Lifecycle
					}
					ttContainer.LivenessProbe = c.LivenessProbe
					ttContainer.ReadinessProbe = c.ReadinessProbe
					if c.SecurityContext != nil {
						ttContainer.SecurityContext = c.SecurityContext
					}
					ttContainer.StartupProbe = c.StartupProbe
					ttContainer.Resources = c.Resources
					ttContainer.VolumeMounts = c.VolumeMounts
					break
				}
			}
		}
	}

	// Set the container's resource requests

	err, res := setContainerResources(ctx, cr, client, "tt", ttContainer.Resources)
	if err != nil {
		ourerror = err
		return ourerror, nil
	}
	ttContainer.Resources = res

	ttContainer.VolumeMounts = append(ttContainer.VolumeMounts,
		corev1.VolumeMount{
			Name:      "tt-persistent",
			MountPath: "/tt",
		})
	ttContainer.VolumeMounts = append(ttContainer.VolumeMounts,
		corev1.VolumeMount{
			Name:      "tt-agent",
			MountPath: "/ttagent",
		})

	if cr.Spec.TTSpec.LogStorageSize != nil {
		ttContainer.VolumeMounts = append(ttContainer.VolumeMounts,
			corev1.VolumeMount{
				Name:      "tt-log",
				MountPath: "/ttlog",
			})
	}
	if userSpecifiedTTConfig ||
		cr.Spec.TTSpec.DbConfigMap != nil ||
		cr.Spec.TTSpec.DbSecret != nil {
		ttContainer.VolumeMounts = append(ttContainer.VolumeMounts,
			corev1.VolumeMount{
				Name:      "tt-config",
				MountPath: "/ttconfig",
			})
	}

	dbg, _ := os.LookupEnv("TT_DEBUG")
	if dbg == "1" {
		ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
			Name:  "TT_DEBUG",
			Value: "1",
		})
		reqLogger.V(1).Info(us + ": TT_DEBUG will be set in containers")
	}

	ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
		Name:  "TT_OPERATOR_MANAGED",
		Value: "1",
	})

	ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
		Name:  "TIMESTEN_HOME",
		Value: "/tt/home/" + ttUser + "/instances/instance1",
	})

	ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
		Name: "LD_LIBRARY_PATH",
		Value: "/tt/home/" + ttUser + "/instances/instance1/ttclasses/lib:" +
			"/tt/home/" + ttUser + "/instances/instance1/install/lib:" +
			"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient_11_2:" +
			"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient_19_17:" +
			"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient",
	})

	if cr.Spec.TTSpec.ReplicationCipherSuite != nil {
		ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
			Name:  "TT_REPLICATION_CIPHER_SUITE",
			Value: *cr.Spec.TTSpec.ReplicationCipherSuite,
		})
	}

	if cr.Spec.TTSpec.ReplicationSSLMandatory != nil {
		ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
			Name:  "TT_REPLICATION_SSL_MANDATORY",
			Value: strconv.Itoa(*cr.Spec.TTSpec.ReplicationSSLMandatory),
		})
	}

	ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
		Name:  "TT_DB_NAME",
		Value: (*cr).Name,
	})

	if subscriber {
		ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
			Name:  "TT_SUBSCRIBER",
			Value: "1",
		})
	}

	// old agents require TT_REPLICATION_TOPOLOGY
	reqLogger.V(1).Info(us + ": setting TT_REPLICATION_TOPOLOGY=activeStandbyPair in container env")
	ttContainer.Env = append(ttContainer.Env, corev1.EnvVar{
		Name:  "TT_REPLICATION_TOPOLOGY",
		Value: "activeStandbyPair",
	})

	// The init container will set up the TimesTen installation and
	// instance before the 'tt' container or any direct mode app
	// containers start up

	initContainer := ttContainer
	initContainer.Name = "ttinit"
	initContainer.Env = append(initContainer.Env, corev1.EnvVar{
		Name:  "TT_INIT_CONTAINER",
		Value: "1",
	},
		corev1.EnvVar{
			Name:  "TTC_UID",
			Value: string(cr.UID),
		},
	)
	initContainer.ReadinessProbe = nil
	initContainer.LivenessProbe = nil
	initContainer.StartupProbe = nil
	initContainer.Ports = nil
	initContainer.Lifecycle = nil

	// Create a StatefulSet; we'll then add things to it

	ssLabelMap := map[string]string{
		"app":                 cr.Name,
		"database.oracle.com": cr.Name,
	}
	for k, v := range cr.ObjectMeta.Labels {
		ssLabelMap[k] = v
	}

	ourAnnotations := cr.ObjectMeta.GetAnnotations()
	if ourAnnotations == nil {
		ourAnnotations = make(map[string]string)
	}
	ourAnnotations["TTC"] = string(cr.UID)

	nReps := getNReplicas(cr)
	if subscriber {
		nReps = getNSubscribers(cr)
	}

	var nReplicas int32 = int32(nReps)

	newSSName := cr.Name
	if subscriber {
		newSSName = getSubscriberName(cr)
	}

	newSS := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        newSSName,
			Namespace:   cr.Namespace,
			Labels:      ssLabelMap,
			Annotations: ourAnnotations,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labelMap,
			},
			ServiceName:         cr.Name,
			Replicas:            newInt32(nReplicas),
			PodManagementPolicy: "Parallel",
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: "OnDelete",
			},
		},
	}

	if *cr.Spec.TTSpec.ReplicationTopology == "none" {
		// TODO: customer provided value should override
		newSS.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: "RollingUpdate",
		}
	}

	if userTemplate == nil {
		newSS.Spec.Template = corev1.PodTemplateSpec{}
	} else {
		newSS.Spec.Template = *userTemplate

		newSS.Spec.Template.Spec.Containers = nil

		// Don't include any "tt" or "daemonlog" or "exporter" container the user
		// may have listed; we will add them ourselves later

		for _, c := range userTemplate.Spec.Containers {
			if c.Name == "tt" || c.Name == "daemonlog" || c.Name == "exporter" {
				// Skip copying it, we will add it later
			} else {
				newSS.Spec.Template.Spec.Containers = append(newSS.Spec.Template.Spec.Containers, c)
			}
		}
	}

	// If there are any user-specified containers, add the appropriate volumes to them

	for i, _ := range newSS.Spec.Template.Spec.Containers {
		newSS.Spec.Template.Spec.Containers[i].VolumeMounts = append(newSS.Spec.Template.Spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tt-persistent",
				MountPath: "/tt",
			})

		if cr.Spec.TTSpec.LogStorageSize != nil {
			newSS.Spec.Template.Spec.Containers[i].VolumeMounts = append(newSS.Spec.Template.Spec.Containers[i].VolumeMounts,
				corev1.VolumeMount{
					Name:      "tt-log",
					MountPath: "/ttlog",
				})
		}
	}

	// Add the TimesTen containers
	newSS.Spec.Template.Spec.Containers = append(newSS.Spec.Template.Spec.Containers, ttContainer)
	newSS.Spec.Template.Spec.InitContainers = append(newSS.Spec.Template.Spec.InitContainers, initContainer)

	// Unless the user asked us not to, let's add a sidecar container to handle the
	// TimesTen daemon logs

	if cr.Spec.TTSpec.DaemonLogSidecar == nil ||
		(cr.Spec.TTSpec.DaemonLogSidecar != nil && *cr.Spec.TTSpec.DaemonLogSidecar) {

		sidecar := corev1.Container{
			Name:            "daemonlog",
			Image:           cr.Spec.TTSpec.Image,
			ImagePullPolicy: ipp,
			Command: []string{
				"sh",
				"-c",
				"/bin/bash <<'EOF'\nif [ -e /timesten/operator/operator/daemonlogger ]; then\n/timesten/operator/operator/daemonlogger\nelse\nwhile [ 1 ] ; do\ntail --follow=name /tt/home/" + ttUser + "/instances/instance1/diag/ttmesg.log --max-unchanged-stats=5\nsleep 1\ndone\nfi\nexit 0\nEOF",
			},
		}

		sidecar.VolumeMounts = append(sidecar.VolumeMounts,
			corev1.VolumeMount{
				Name:      "tt-persistent",
				MountPath: "/tt",
			})

		if cr.Spec.TTSpec.LogStorageSize != nil {
			sidecar.VolumeMounts = append(sidecar.VolumeMounts,
				corev1.VolumeMount{
					Name:      "tt-log",
					MountPath: "/ttlog",
				})
		}

		// Set up environment variables for the "daemonlog" container

		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name:  "TIMESTEN_HOME",
			Value: "/tt/home/" + ttUser + "/instances/instance1",
		})

		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name:  "TT_OPERATOR_MANAGED",
			Value: "1",
		})

		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name: "LD_LIBRARY_PATH",
			Value: "/tt/home/" + ttUser + "/instances/instance1/ttclasses/lib:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/lib:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient_11_2:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient_19_17:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient",
		})

		// Set up resources for the daemonlog container

		sidecar.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
		}
		if (*cr).Spec.Template != nil {
			if (*cr).Spec.Template.Spec.Containers != nil {
				for _, c := range (*cr).Spec.Template.Spec.Containers {
					if c.Name == "daemonlog" {
						sidecar.Resources = c.Resources
					}
				}
			}
		}

		err, sidecar.Resources = setContainerResources(ctx, cr, client, "daemonlog", sidecar.Resources)

		// inherit SecurityContext from tt container

		if ttContainer.SecurityContext != nil {
			sidecar.SecurityContext = ttContainer.SecurityContext
		}

		newSS.Spec.Template.Spec.Containers = append(newSS.Spec.Template.Spec.Containers, sidecar)
	}

	// create the exporter sidecar container

	runExporter := shouldTTExporterRun(ctx, cr, client)
	pport := 8888
	if cr.Spec.TTSpec.Prometheus != nil && cr.Spec.TTSpec.Prometheus.Port != nil {
		pport = *cr.Spec.TTSpec.Prometheus.Port
	}
	if runExporter {
		sidecar := corev1.Container{
			Name:            "exporter",
			Image:           cr.Spec.TTSpec.Image,
			ImagePullPolicy: ipp,
			Ports: []corev1.ContainerPort{
				{
					Name:          "exporter",
					ContainerPort: int32(pport),
				},
			},
		}

		sidecar.VolumeMounts = append(sidecar.VolumeMounts,
			corev1.VolumeMount{
				Name:      "tt-persistent",
				MountPath: "/tt",
			})

		// Set up environment variables for the "exporter" container
		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name:  "TIMESTEN_HOME",
			Value: "/tt/home/" + ttUser + "/instances/instance1",
		})

		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name:  "TT_OPERATOR_MANAGED",
			Value: "1",
		})

		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name: "LD_LIBRARY_PATH",
			Value: "/tt/home/" + ttUser + "/instances/instance1/ttclasses/lib:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/lib:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient_11_2:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient_19_17:" +
				"/tt/home/" + ttUser + "/instances/instance1/install/ttoracle_home/instantclient",
		})

		// pass TT_DEBUG to the exporter container
		ttDebug, ok := os.LookupEnv("TT_DEBUG")
		if ok {
			sidecar.Env = append(sidecar.Env, corev1.EnvVar{
				Name:  "TT_DEBUG",
				Value: ttDebug,
			})
		}

		// tell starthost that we're the exporter container
		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name:  "TT_EXPORTER_CONTAINER",
			Value: "1",
		})

		// starthost will pass this as -port to ttExporter
		sidecar.Env = append(sidecar.Env, corev1.EnvVar{
			Name:  "TT_EXPORTER_PORT",
			Value: strconv.Itoa(pport),
		})

		if cr.Spec.TTSpec.Prometheus != nil &&
			cr.Spec.TTSpec.Prometheus.LimitRate != nil {
			// starthost will pass this as -limitRate to ttExporter
			sidecar.Env = append(sidecar.Env, corev1.EnvVar{
				Name:  "TT_EXPORTER_LIMIT_RATE",
				Value: strconv.Itoa(*cr.Spec.TTSpec.Prometheus.LimitRate),
			})
		}

		// Hierarchy:
		// If user specified certSecret then insecure is ignored
		// If user doesn't specify certSecret then insecure:true turns on http
		// Anything else has us generate a cert for the user

		if cr.Spec.TTSpec.Prometheus != nil &&
			cr.Spec.TTSpec.Prometheus.CertSecret != nil {
			// CertSecret
			exporterVol := corev1.Volume{
				Name: *cr.Spec.TTSpec.Prometheus.CertSecret,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: *cr.Spec.TTSpec.Prometheus.CertSecret,
					},
				},
			}
			newSS.Spec.Template.Spec.Volumes = append(newSS.Spec.Template.Spec.Volumes, exporterVol)

			sidecar.VolumeMounts = append(sidecar.VolumeMounts,
				corev1.VolumeMount{
					Name:      *cr.Spec.TTSpec.Prometheus.CertSecret,
					MountPath: "/ttconfig",
				})
		} else {
			if cr.Status.ExporterSecret != nil {
				exporterVol := corev1.Volume{
					Name: "exporterwallet",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: *cr.Status.ExporterSecret,
						},
					},
				}
				newSS.Spec.Template.Spec.Volumes = append(newSS.Spec.Template.Spec.Volumes, exporterVol)
				sidecar.VolumeMounts = append(sidecar.VolumeMounts,
					corev1.VolumeMount{
						Name:      "exporterwallet",
						MountPath: "/ttconfig",
					})
			}
		}

		// Set up resources for the exporter container

		sidecar.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
		}
		if userTemplate != nil {
			if userTemplate.Spec.Containers != nil {
				for _, c := range userTemplate.Spec.Containers {
					if c.Name == "exporter" {
						sidecar.Resources = c.Resources
					}
				}
			}
		}

		err, sidecar.Resources = setContainerResources(ctx, cr, client, "exporter", sidecar.Resources)

		// inherit SecurityContext from tt container
		if ttContainer.SecurityContext != nil {
			sidecar.SecurityContext = ttContainer.SecurityContext
		}

		newSS.Spec.Template.Spec.Containers = append(newSS.Spec.Template.Spec.Containers, sidecar)
	}

	// There are some attributes that we need to set, even if it overwrites
	// some setting that the user might have made.

	newSS.Spec.Template.Spec.ShareProcessNamespace = newBool(true)

	// pass any labels and annotations to the SS and the resulting pods
	newSS.Spec.Template.ObjectMeta = metav1.ObjectMeta{
		Labels:      ssLabelMap,
		Annotations: ourAnnotations,
	}

	if replicated == false {
		newSS.Spec.Template.Spec.TerminationGracePeriodSeconds = newInt64(300)
	} else {
		newSS.Spec.Template.Spec.TerminationGracePeriodSeconds = newInt64(10)
	}
	reqLogger.V(2).Info(fmt.Sprintf("%s: TerminationGracePeriodSeconds=%d", us, newSS.Spec.Template.Spec.TerminationGracePeriodSeconds))

	// We need to get the PV mounted as the proper user. But we don't
	// know what uid that user is. So we impose the restriction that the
	// operator must run as the same uid.

	var ourUid int64 = 333
	var ourGid int64 = 333

	opUser, err := user.Current()
	if err == nil {
		ourUid, _ = strconv.ParseInt(opUser.Uid, 10, 32)
		ourGid, _ = strconv.ParseInt(opUser.Gid, 10, 32)
		reqLogger.Info("Operator running as uid " + fmt.Sprintf("%d", ourUid) + " gid " + fmt.Sprintf("%d", ourGid))
	}

	if userTemplate == nil {
		newSS.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
	} else {
		if userTemplate.Spec.SecurityContext == nil {
			newSS.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
		} else {
			newSS.Spec.Template.Spec.SecurityContext = userTemplate.Spec.SecurityContext
		}
	}
	newSS.Spec.Template.Spec.SecurityContext.FSGroup = newInt64(ourGid)

	// We have to merge the attributes that the TimesTen containers need
	// into whatever attributes the user may have specified.

	newSS.Spec.Template.Spec.ImagePullSecrets = append(newSS.Spec.Template.Spec.ImagePullSecrets,
		corev1.LocalObjectReference{
			Name: cr.Spec.TTSpec.ImagePullSecret,
		})

	if cr.Spec.VolumeClaimTemplates != nil {
		newSS.Spec.VolumeClaimTemplates = append(newSS.Spec.VolumeClaimTemplates, *cr.Spec.VolumeClaimTemplates...)
	}

	// Fill in Volumes

	agentVol := corev1.Volume{
		Name: "tt-agent",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "tt" + string(cr.UID),
			},
		},
	}
	newSS.Spec.Template.Spec.Volumes = append(newSS.Spec.Template.Spec.Volumes, agentVol)

	// Did the user put a tt-secret volume in the template?
	// If so we'll use theirs.  Perhaps they are filling it in with an init
	// container.
	// If they didn't specify one, AND they specified a config map / secret
	// then we will use those.

	if userSpecifiedTTConfig == false &&
		(cr.Spec.TTSpec.DbConfigMap != nil || cr.Spec.TTSpec.DbSecret != nil) {
		cf := corev1.Volume{
			Name: "tt-config",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{},
			},
		}

		if cr.Spec.TTSpec.DbConfigMap != nil {
			for _, name := range *cr.Spec.TTSpec.DbConfigMap {
				vp := corev1.VolumeProjection{
					ConfigMap: &corev1.ConfigMapProjection{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: name,
						},
					},
				}

				cf.VolumeSource.Projected.Sources = append(cf.VolumeSource.Projected.Sources, vp)
			}
		}

		if cr.Spec.TTSpec.DbSecret != nil {
			for _, name := range *cr.Spec.TTSpec.DbSecret {
				vp := corev1.VolumeProjection{
					Secret: &corev1.SecretProjection{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: name,
						},
					},
				}

				cf.VolumeSource.Projected.Sources = append(cf.VolumeSource.Projected.Sources, vp)
			}
		}

		newSS.Spec.Template.Spec.Volumes = append(newSS.Spec.Template.Spec.Volumes, cf)
	}

	// Fill in the VolumeClaimTemplates

	storageRequest := corev1.ResourceList{}
	if cr.Spec.TTSpec.StorageSize == nil {
		storageRequest[corev1.ResourceStorage] = *resource.NewQuantity(50*1024*1024*1024, resource.DecimalSI)
	} else {
		storageRequest[corev1.ResourceStorage], _ = resource.ParseQuantity(*cr.Spec.TTSpec.StorageSize)
	}

	var newPVC corev1.PersistentVolumeClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tt-persistent",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: newString(cr.Spec.TTSpec.StorageClassName),
			Resources: corev1.VolumeResourceRequirements{
				Limits:   corev1.ResourceList{},
				Requests: storageRequest,
			},
		},
	}

	if cr.Spec.TTSpec.StorageSelector != nil {
		newPVC.Spec.Selector = cr.Spec.TTSpec.StorageSelector
	}

	newSS.Spec.VolumeClaimTemplates = append(newSS.Spec.VolumeClaimTemplates, newPVC)

	if cr.Spec.TTSpec.LogStorageSize != nil {
		var scn string
		if cr.Spec.TTSpec.LogStorageClassName == nil {
			scn = cr.Spec.TTSpec.StorageClassName
		} else {
			scn = *cr.Spec.TTSpec.LogStorageClassName
		}

		storageRequest2 := corev1.ResourceList{}
		storageRequest2[corev1.ResourceStorage], _ = resource.ParseQuantity(*cr.Spec.TTSpec.LogStorageSize)

		var newLogPVC corev1.PersistentVolumeClaim = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "tt-log",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: newString(scn),
				Resources: corev1.VolumeResourceRequirements{
					Limits:   corev1.ResourceList{},
					Requests: storageRequest2,
				},
			},
		}

		if cr.Spec.TTSpec.LogStorageSelector != nil {
			newLogPVC.Spec.Selector = cr.Spec.TTSpec.LogStorageSelector
		}

		newSS.Spec.VolumeClaimTemplates = append(newSS.Spec.VolumeClaimTemplates, newLogPVC)
	}

	// Will set affinity for arch to match operator's.
	oparch := goRuntime.GOARCH
	ourerror = addArchToTemplate(ctx, oparch, &newSS.Spec.Template)
	if ourerror != nil {
		return ourerror, nil
	}

	return ourerror, &newSS
}

// addArchToTemplate - add a user-supplied architecture definition (arm64, amd64, etc)
// to a StatefulSet.spec.template. This adds a fairly complicated set of stuff, something
// like:
//
//	template:
//	  spec:
//	    affinity:
//	      nodeAffinity:
//	        requiredDuringSchedulingIgnoredDuringExecution:
//	          nodeSelectorTerms:
//	            - matchExpressions:
//	              - key: "kubernetes.io/arch"
//	                operator: In
//	                values: ["arm64"]
//
// If the user already specified nodeAffinity then we need to try to weave our entries
// into theirs.
func addArchToTemplate(ctx context.Context, arch string, tmpl *corev1.PodTemplateSpec) error {
	reqLogger := log.FromContext(ctx)
	us := "addArchToTemplate"
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")
	var err error

	if arch != "amd64" && arch != "arm64" {
		return errors.New(fmt.Sprintf("Unsupported architecture '%s' specified", arch))
	}

	nsr := corev1.NodeSelectorRequirement{
		Key:      "kubernetes.io/arch",
		Operator: "In",
		Values:   []string{arch},
	}

	nsterm := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{nsr},
	}

	if tmpl.Spec.Affinity == nil {
		tmpl.Spec.Affinity = &corev1.Affinity{}
	}

	tmpl.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{nsterm},
		},
	}

	return err
}

// newHeadlessService returns a headless service. This is used to
// set up stable domain names for the servers, of the form
// pod.<cr>-server.<namespace>.svc.cluster.local
func newHeadlessService(ctx context.Context, cr *timestenv2.TimesTenClassic, scheme *runtime.Scheme, client client.Client) *corev1.Service {
	reqLogger := log.FromContext(ctx)
	us := "newHeadlessService"
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	labelMap := map[string]string{
		"app": cr.Name,
	}

	headlessService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    labelMap,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "cs",
					Port:       6625,
					TargetPort: intstr.FromInt(6625),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector:                 labelMap,
			ClusterIP:                "None", // Makes this a 'headless' service
			PublishNotReadyAddresses: true,
		},
	}

	if shouldTTExporterRun(ctx, cr, client) {
		reqLogger.V(2).Info(us + ": appending prometheus port to headless service")
		port := 8888
		if cr.Spec.TTSpec.Prometheus != nil && cr.Spec.TTSpec.Prometheus.Port != nil {
			port = *cr.Spec.TTSpec.Prometheus.Port
		}
		headlessService.Spec.Ports = append(headlessService.Spec.Ports, []corev1.ServicePort{
			{
				Name:       "exporter",
				Port:       int32(port),
				TargetPort: intstr.FromInt(port),
				Protocol:   corev1.ProtocolTCP,
			}}...)
	}

	return headlessService

}

// Make sure there's a TimesTen instance ready for us to use
func setupTimesTen(ctx context.Context) (string, error) {
	us := "setupTimesTen"
	reqLogger := log.FromContext(ctx)

	timestenHome, ok := os.LookupEnv("TIMESTEN_HOME")
	if ok {
		reqLogger.Info(us + ": using TimesTen instance " + timestenHome)
		return timestenHome, nil
	}

	// We want to use an instance in $HOME/instance1

	instanceName := "instance1"

	// See if there are any TimesTen distributions we can use

	homeDir, ok := os.LookupEnv("HOME")
	if !ok {
		err := errors.New(us + ": TimesTen not configured and HOME directory not set")
		reqLogger.Error(err, us+": TimesTen not configured and HOME directory not set")
		return "", err
	}

	fullInstanceName := homeDir + "/" + instanceName
	st, err := os.Stat(fullInstanceName)
	if err == nil {
		if st.IsDir() {
			// Let's go ahead and use it
			reqLogger.Info(us + ": found TimesTen instance " + fullInstanceName)
			os.Setenv("TIMESTEN_HOME", fullInstanceName)
			return fullInstanceName, nil
		} else {
			e := errors.New(fullInstanceName + " exists but is not a directory")
			reqLogger.Error(e, "Could not make TimesTen instance")
			panic(e)
		}
	} else {
		if os.IsNotExist(err) {
			// Great, let's make it. First we have to find an installation we can use
			reqLogger.Info(us + ": TimesTen instance not found")
			timestenHome, err := makeTimesTenInstance(ctx, homeDir, instanceName)
			if err == nil {
				return timestenHome, nil
			} else {
				panic(err)
			}

		} else {
			e := errors.New(fullInstanceName + " exists but is not readable")
			reqLogger.Error(e, "Could not make TimesTen instance")
			panic(e)
		}
	}
}

// Make a TimesTen instance
func makeTimesTenInstance(ctx context.Context, location string, instanceName string) (string, error) {
	reqLogger := log.FromContext(ctx)

	installation, err := findTimesTenInstallation(ctx, location, true)
	if err != nil {
		return "", err
	}

	rc, stdout, stderr := runShellCommand(ctx, []string{installation + "/bin/ttInstanceCreate", "-location", location, "-name", instanceName})
	if rc != 0 {
		err := errors.New("Error " + strconv.Itoa(rc) + " creating TimesTen instance")
		reqLogger.Error(err, "Error creating instance", "stdout", stdout, "stderr", stderr)
		return "", err
	}

	// We have to modify the instance guid to a well known value so the
	// agents can read the Oracle Wallets that we create.

	ttc, err := ioutil.ReadFile(location + "/" + instanceName + "/conf/timesten.conf")
	if err != nil {
		reqLogger.Error(err, "Error reading "+location+"/"+instanceName+"/conf/timesten.conf")
		return "", err
	}

	ttclines := strings.Split(string(ttc), "\n")
	for i, l := range ttclines {
		if strings.HasPrefix(l, "instance_guid=") {
			ttclines[i] = "instance_guid=" + universalInstanceGuid
		}
	}

	err = ioutil.WriteFile(location+"/"+instanceName+"/conf/timesten.conf", []byte(strings.Join(ttclines, "\n")), 0600)
	if err != nil {
		reqLogger.Error(err, "Error writing "+location+"/"+instanceName+"/conf/timesten.conf")
		return "", err
	}

	return location + "/" + instanceName, nil
}

// See if a TimesTen installation already exists and create one if not
func findTimesTenInstallation(ctx context.Context, location string, makeNewOnePlease bool) (string, error) {
	us := "findTimesTenInstallation"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	xArgs := []string{"-c", "ls -d /*/installation"}
	loc, err := exec.Command("/bin/bash", xArgs...).Output()
	if err != nil {
		reqLogger.Error(err, us+": cannot find installation: "+string(loc))
		panic(err)
	}
	loca := strings.TrimSpace(string(loc))
	reqLogger.V(2).Info(us + ": returning '" + loca + "'")
	return loca, nil
}
func makeTimesTenInstallation(ctx context.Context, location string, source string) error {
	reqLogger := log.FromContext(ctx)

	cwd, _ := os.Getwd()

	reqLogger.Info(fmt.Sprintf("makeTimesTenInstallation: called with location=%v source=%v", location, source))
	defer reqLogger.V(1).Info("makeTimesTenInstallation returns")
	defer os.Chdir(cwd)

	os.Chdir(location)
	rc, out, err := runShellCommand(ctx, []string{"unzip", source})
	if rc != 0 {
		erro := errors.New(fmt.Sprintf("%v", err))
		reqLogger.Error(erro, fmt.Sprintf("Error %v unzipping %v, stdout=%v err=%v", strconv.Itoa(rc), source, out, err))
		return erro
	}
	// Worked! Caller has to find the install
	return nil
}

// Command to run a shell command, returning the output to the caller
func runShellCommand(ctx context.Context, cmd []string) (int, []string, []string) {
	reqLogger := log.FromContext(ctx)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var ourRc int

	c := exec.Command(cmd[0])
	c.Args = cmd[0:]
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err == nil {
		// Command ran with exit code 0
		ourRc = 0
	} else {
		exitError, ok := err.(*exec.ExitError)
		if ok {
			ourRc = exitError.ExitCode()
		} else {
			reqLogger.Error(err, "Error starting process: "+err.Error())
			return 254, []string{}, []string{}
		}
	}

	printCmd := ""
	for _, l := range cmd {
		printCmd = printCmd + l + " "
	}
	reqLogger.V(2).Info("runShellCommand '" + printCmd + "': rc " + strconv.Itoa(ourRc))

	var outStdout []string
	for _, l := range strings.Split(stdout.String(), "\n") {
		outStdout = append(outStdout, l)
	}

	var outStderr []string
	for _, l := range strings.Split(stderr.String(), "\n") {
		outStderr = append(outStderr, l)
	}

	return ourRc, outStdout, outStderr
}

// Update the high level state of the TT Classic object. Carefully remember the previous
// state and when the state switch occurred
func updateTTClassicHighLevelState(ctx context.Context, instance *timestenv2.TimesTenClassic, newState string, client client.Client) {
	us := "updateTTClassicHighLevelState"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered with newState=" + newState)
	defer reqLogger.V(2).Info(us + " returns")

	if newState != "Initializing" {
		warning := false
		if instance.Status.HighLevelState == "Normal" {
			warning = true
		}
		msg := "TimesTenClassic was " + instance.Status.HighLevelState + ", now " + newState
		reqLogger.V(1).Info(us + ": " + msg)
		kind := "StateChange"
		if newState == "Failed" {
			kind = "FailedCreate"
		}
		logTTEvent(ctx, client, instance, kind, msg, warning)
	}
	instance.Status.PrevHighLevelState = instance.Status.HighLevelState
	if instance.Status.PrevHighLevelState != "" {
		reqLogger.Info(us + ": PrevHighLevelState set to " + instance.Status.PrevHighLevelState)
	}
	instance.Status.HighLevelState = newState
	reqLogger.Info(us + ": HighLevelState set to " + instance.Status.HighLevelState)
	instance.Status.LastHighLevelStateSwitch = time.Now().Unix()
}

// Update the high level state of the subscribers in a TT Classic object. Carefully remember the previous
// state and when the state switch occurred
func updateSubscriberHighLevelState(ctx context.Context, instance *timestenv2.TimesTenClassic, newState string, client client.Client) {
	us := "updateSubscriberHighLevelState"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered with newState=" + newState)
	defer reqLogger.V(2).Info(us + " returns")

	warning := false
	if instance.Status.HighLevelState == "AllSubscribersReady" {
		warning = true
	}
	msg := "Subscriber state was " + instance.Status.Subscriber.HLState + ", now " + newState
	reqLogger.V(1).Info(us + ": " + msg)
	kind := "StateChange"
	logTTEvent(ctx, client, instance, kind, msg, warning)

	instance.Status.Subscriber.PrevHLState = instance.Status.Subscriber.HLState
	if instance.Status.PrevHighLevelState != "" {
		reqLogger.Info(us + ": PrevHLState set to " + instance.Status.PrevHighLevelState)
	}
	instance.Status.Subscriber.HLState = newState
	reqLogger.Info(us + ": HighLevelState set to " + instance.Status.HighLevelState)
	instance.Status.Subscriber.LastHLStateSwitch = time.Now().Unix()
}

func updatePodHighLevelState(ctx context.Context, instance *timestenv2.TimesTenClassic, isP *timestenv2.TimesTenPodStatus, newState string, client client.Client) {
	us := "updatePodHighLevelState"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered with newState=" + newState)
	defer reqLogger.V(2).Info(us + " returns")

	if isP.HighLevelState == newState {
		return
	}

	isP.PrevHighLevelState = isP.HighLevelState
	isP.LastHighLevelStateSwitch = time.Now().Unix()
	isP.HighLevelState = newState

	if isReplicated(instance) {
		// We don't generate events for these. But we could...
	} else {
		warning := false
		if newState == "Terminal" || newState == "Down" {
			warning = true
		}
		msg := fmt.Sprintf("Pod %s state was %s, now %s", isP.Name, isP.PrevHighLevelState, newState)
		reqLogger.V(1).Info(us + ": " + msg)
		kind := "StateChange"
		logTTEvent(ctx, client, instance, kind, msg, warning)
	}
}

// process finalizers, tasks to be performed during ttc object destruction
func processFinalizers(ctx context.Context, client client.Client, request reconcile.Request, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo) (string, error) {
	reqLogger := log.FromContext(ctx)
	currentFinalizers := instance.GetFinalizers()

	if len(currentFinalizers) > 0 {
		reqLogger.V(1).Info(fmt.Sprintf("processFinalizers: currentFinalizers=%v", currentFinalizers))
	}

	// define the cache finalizer, but don't set it yet
	finalizer := "cleancache.finalizers.database.oracle.com"

	// examine DeletionTimestamp to determine if we're under deletion
	if instance.DeletionTimestamp.IsZero() {

		// we're not deleting this resource, set a finalizer if appropriate
		if !containsString(currentFinalizers, finalizer) {
			// if we have cachegroups, add a finalizer
			if len(instance.Status.PodStatus) > 0 {
				ncg := 0
				for i := 0; i < len(instance.Status.PodStatus); i++ {
					ncg += instance.Status.PodStatus[i].CacheStatus.NCacheGroups
				}

				// if the agent detected cache groups, set the cache cleanup finalizer
				if ncg > 0 {

					if instance.Spec.TTSpec.CacheCleanup != nil && *instance.Spec.TTSpec.CacheCleanup == false {
						reqLogger.V(1).Info("processFinalizers: CacheCleanup set to false, not setting finalizer " + finalizer)
					} else {
						reqLogger.V(1).Info("processFinalizers: setting finalizer " + finalizer + ", calling client.Update")
						instance.SetFinalizers(append(instance.GetFinalizers(), finalizer))
						// update the object so it knows about the finalizer
						if err := client.Update(ctx, instance); err != nil {

							reqLogger.V(1).Info("processFinalizers: failed to register finalizer " + finalizer + " err=" + err.Error())
							//Checks if the error was because of lack of permission, if not, return the original message
							var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
							if isPermissionsProblem {
								logTTEvent(ctx, client, instance, "FailedRegister", "processFinalizers: failed to register finalizer "+finalizer+": "+errorMsg, true)
								updateTTClassicHighLevelState(ctx, instance, "Failed", client)
							}
							return "", err
						} else {
							reqLogger.V(1).Info("processFinalizers: successfully registered finalizer " + finalizer)
						}
					}
				}
			} else {
				reqLogger.V(1).Info("not setting finalizer since we don't have pod status")
			}
		} else if instance.Spec.TTSpec.CacheCleanup != nil && *instance.Spec.TTSpec.CacheCleanup == false {
			// user applied an update, wants to turn cache cleanup OFF
			reqLogger.V(1).Info("processFinalizers: finalizers set but Spec.TTSpec.CacheCleanup is false; remove finalizer" +
				" (calling client.Update)")
			instance.Finalizers = removeString(instance.Finalizers, finalizer)
			if err := client.Update(ctx, instance); err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
				if isPermissionsProblem {
					logTTEvent(ctx, client, instance, "FailedFinalizer", "Failed to remove finalizer "+finalizer+": "+errorMsg, true)
					updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				}
				reqLogger.V(1).Info("failed to remove finalizer " + finalizer + " err=" + err.Error())
				return "", err
			}
		}

	} else {

		// resource is being deleted

		if containsString(currentFinalizers, finalizer) {
			reqLogger.V(1).Info("process finalizer " + finalizer)
			if err := cleanupTTCache(ctx, client, request, instance, tts); err != nil {
				// if we fail to clean up cache objects, return with error
				// so that it can be retried
				reqLogger.Info("cleanupTTCache failed to cleanup cache objects")
				return "", err
			}

			reqLogger.V(1).Info("removing finalizer " + finalizer)

			// remove the finalizer and update to allow garbage collection to proceed
			instance.Finalizers = removeString(instance.Finalizers, finalizer)
			if err := client.Update(ctx, instance); err != nil {
				reqLogger.V(1).Info("error removing finalizer " + finalizer + " err=" + err.Error())
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
				if isPermissionsProblem {
					logTTEvent(ctx, client, instance, "FailedFinalizer", "Error removing finalizer "+finalizer+": "+errorMsg, true)
					updateTTClassicHighLevelState(ctx, instance, "Failed", client)
				}
				return "", err
			}

			// NOTE: the [deferred] call to Update may fail, presumably since the object(s)
			// are being deleted, error is 'Storage Error: invalid object, Code: 4'

			return "done", nil
		}

	}

	return "", nil
}

// bothDownAction - determine the state of the world when both dbs go down
// podThatShouldBeTheNewActive = bothDownAction(instance)
// Return value is the best - and worst - pod names, or "" (couldn't tell)
func bothDownAction(ctx context.Context, instance *timestenv2.TimesTenClassic) (int, int) {
	reqLogger := log.FromContext(ctx)
	var whoseAhead string
	pod0 := 0
	pod1 := 1

	if instance.Status.UsingTwosafe {
		// In twosafe in general the standby is ahead of the active,
		// since transactions are committed on the standby and only then
		// on the active. But that depends on the previous state.

		switch instance.Status.PrevHighLevelState {
		case "ActiveDown":
			whoseAhead = "Standby"
		case "ActiveTakeover", "StandbyStarting", "StandbyDown":
			whoseAhead = "Active"
		case "Normal":
			if instance.Status.BothDownRecoveryIneligible {
				whoseAhead = "Unknown"
			} else {
				whoseAhead = "Standby"
			}
		default:
			reqLogger.V(1).Info("bothDownAction: unknown PrevHighLevelState '" + instance.Status.PrevHighLevelState + "'")
			whoseAhead = "Unknown"
		}

	} else {
		// If we're async or anything other than twosafe then the active is
		// ALWAYS ahead of the standby
		whoseAhead = "Active"
	}

	switch whoseAhead {
	case "Active":
		if instance.Status.PodStatus[0].IntendedState == "Active" {
			return pod0, pod1
		} else {
			return pod1, pod0
		}
	case "Standby":
		if instance.Status.PodStatus[0].IntendedState == "Standby" {
			return pod0, pod1
		} else {
			return pod1, pod0
		}
	case "Unknown":
		return -1, -1
	}

	return -1, -1
}

// quiesce database and ensure that standby has caught up to active
func quiesceForUpgrade(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, tts *TTSecretInfo) error {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("quiesceForUpgrade entered")
	defer reqLogger.V(2).Info("quiesceForUpgrade ends")

	// the standard procedure follows :
	// NOTE: we will not do an -abort, risks data loss
	//  1. Close the db so nobody can establish a new connection
	//  2. Do a -transactional disconnect
	//  3. Wait 'a while'
	//  4. Do an -immediate disconnect
	//  5. Wait 'a while'
	//  6. Do an -abort disconnect

	// Close the db to prevent new connections

	err := closeDb(ctx, instance, client, tts)
	if err != nil {
		reqLogger.Info("quiesceForUpgrade: closeDb failed")
	}

	// Do a -transactional disconnect
	// -transactional - Allows any open transactions to be committed or rolled back
	// before disconnecting. Does not affect idle connections.

	err = forceDisconnect(ctx, "transactional", instance, client, tts)
	if err != nil {
		reqLogger.Info("quiesceForUpgrade: forceDisconnect -transactional failed")
	}

	reqLogger.Info("quiesceForUpgrade: wait 30 secs before -immediate disconnect")
	time.Sleep(30 * time.Second)

	// Do an -immediate disconnect
	// -immediate - Rolls back any open transactions before immediately disconnecting.
	// This also disconnects idle connections.

	err = forceDisconnect(ctx, "immediate", instance, client, tts)
	if err != nil {
		reqLogger.Info("quiesceForUpgrade: forceDisconnect -immediate failed")
	}

	err = repAdminWait(ctx, 30, instance, client, tts)
	if err != nil {
		reqLogger.Info("quiesceForUpgrade: repAdminWait failed")
		return err
	}

	return nil

}

// init() functions are called before main() and can be used for initialization
// NOTE that this is called so early in initialization that logging doesn't work!
func init() {
	rootCAs, _ = x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	// Determine if we are running in an original or 2022-style 'official'
	// image and other misc details
	ttUser = "timesten"
}

// Is this TimesTenClassic object replicated? How many replicas are there?
// Are there any subscribers?
// replicated, nReplicas, nSubscribers, maxNSubscribers, subName = replicationConfig(instance)
func replicationConfig(instance *timestenv2.TimesTenClassic) (bool, int, int, int, string) {
	nReplicas := 2
	replicated := true
	nSubs := 0
	maxNSubs := 0
	subName := ""

	if instance.Spec.TTSpec.ReplicationTopology == nil {
		replicated = true
		nReplicas = 2
	} else {
		if *instance.Spec.TTSpec.ReplicationTopology == "none" {
			replicated = false
			nReplicas = 1
			if instance.Spec.TTSpec.Replicas != nil {
				nReplicas = *instance.Spec.TTSpec.Replicas
			}
		} else {
			replicated = true
			nReplicas = 2
		}
	}

	if replicated {
		if instance.Spec.TTSpec.Subscribers != nil {
			if instance.Spec.TTSpec.Subscribers.Replicas == nil {
			} else {
				nSubs = *instance.Spec.TTSpec.Subscribers.Replicas
				if instance.Spec.TTSpec.Subscribers.MaxReplicas == nil {
					maxNSubs = nSubs // Can't add more later, then
				} else {
					maxNSubs = *instance.Spec.TTSpec.Subscribers.MaxReplicas
				}
			}
			if instance.Spec.TTSpec.Subscribers.Name == nil {
				subName = instance.Name + "-sub"
			} else {
				subName = *instance.Spec.TTSpec.Subscribers.Name
			}
		}
	}
	return replicated, nReplicas, nSubs, maxNSubs, subName
}

// Getter functions for each item
func getSubscriberName(instance *timestenv2.TimesTenClassic) string {
	_, _, _, _, subName := replicationConfig(instance)
	return subName
}

func getNSubscribers(instance *timestenv2.TimesTenClassic) int {
	_, _, nSubs, _, _ := replicationConfig(instance)
	return nSubs
}

func getMaxNSubscribers(instance *timestenv2.TimesTenClassic) int {
	_, _, _, maxSubs, _ := replicationConfig(instance)
	return maxSubs
}

func getNReplicas(instance *timestenv2.TimesTenClassic) int {
	_, nReps, _, _, _ := replicationConfig(instance)
	return nReps
}

func isReplicated(instance *timestenv2.TimesTenClassic) bool {
	rep, _, _, _, _ := replicationConfig(instance)
	return rep
}

// For active standby pairs, determine the next high level state
// newHLState := determineNextHLState(ctx, instance)
func determineNextHLState(ctx context.Context, client client.Client, tts *TTSecretInfo, instance *timestenv2.TimesTenClassic) (string, bool) {
	us := "determineNextHLState"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	upgradeActive := false

	var err error

	newHighLevelState := "???"
	newAutoUpgradeState := "???"

	// Special case: If the pair is in BothDown state then we need to decide
	// which instance, if any, should become the active when things start back
	// up. It's possible that we can determine that, in which case
	// we set things up for that and switch to WaitingForActive to wait for it
	// to come up. If we can't determine then the user has to sort it out,
	// and we switch to ManualInterventionRequired. In any case the current
	// state of the instances is irrelevant, so we use this special code here.

	if instance.Status.HighLevelState == "BothDown" {
		newHighLevelState := "ManualInterventionRequired"
		if instance.Spec.TTSpec.BothDownBehavior == nil ||
			*instance.Spec.TTSpec.BothDownBehavior == "Best" {
			bestPod, worstPod := bothDownAction(ctx, instance)
			reqLogger.V(2).Info(fmt.Sprintf("bothDownAction returns %d", bestPod))
			if bestPod < 0 {
				// We can't tell which pod should be the active
				logTTEvent(ctx, client, instance, "StateChange", "Operator cannot determine Best database", true)
			} else {
				// We know which pod should be the new active. Wait for it to come up.
				newHighLevelState = "WaitingForActive"

				instance.Status.PodStatus[bestPod].PrevIntendedState = instance.Status.PodStatus[bestPod].IntendedState
				instance.Status.PodStatus[bestPod].IntendedState = "Active"
				instance.Status.PodStatus[worstPod].PrevIntendedState = instance.Status.PodStatus[worstPod].IntendedState
				instance.Status.PodStatus[worstPod].IntendedState = "Standby"
				logTTEvent(ctx, client, instance, "StateChange", fmt.Sprintf("Based on replication configuration %s will be the new 'active'; its previous role was %s", instance.Status.PodStatus[bestPod].Name, instance.Status.PodStatus[bestPod].PrevIntendedState), true)
			}
		} else if *instance.Spec.TTSpec.BothDownBehavior == "Manual" {
			newHighLevelState = "ManualInterventionRequired"
		} else {
			reqLogger.V(1).Info("ERROR: Unsupported bothDownBehavior '" + *instance.Spec.TTSpec.BothDownBehavior + "'")
			newHighLevelState = "ManualInterventionRequired"
		}
		return newHighLevelState, false
	}

	nReplicas := getNReplicas(instance)

	s0 := instance.Status.PodStatus[0].HighLevelState
	i0 := instance.Status.PodStatus[0].IntendedState
	s1 := instance.Status.PodStatus[1].HighLevelState
	i1 := instance.Status.PodStatus[1].IntendedState

	// Figure out what the next high level state of the pair will be
	for podNo := 0; podNo < nReplicas; podNo++ {
		reqLogger.V(1).Info(fmt.Sprintf("POD %d HighLevelState=%v IntendedState=%v", podNo, instance.Status.PodStatus[podNo].HighLevelState, instance.Status.PodStatus[podNo].IntendedState))
	}

	switch i0 {
	case "Active":
		if i1 == "Standby" {

			newHighLevelState = highLevelStateMachine[instance.Status.HighLevelState][s0][s1]

			if instance.Status.ClassicUpgradeStatus.UpgradeState != "" { // if we're performing an upgrade

				reqLogger.V(1).Info(fmt.Sprintf("%s: ActiveStatus=%v StandbyStatus=%v UpgradeState=%v", us,
					instance.Status.ClassicUpgradeStatus.ActiveStatus,
					instance.Status.ClassicUpgradeStatus.StandbyStatus,
					instance.Status.ClassicUpgradeStatus.UpgradeState))

				if instance.Status.ClassicUpgradeStatus.StandbyStatus == "deleteStandby" ||
					instance.Status.ClassicUpgradeStatus.StandbyStatus == "processing" {

					// STANDBY upgrade in progress

					err, newAutoUpgradeState := checkUpgradeStandby(ctx, client, instance, tts, newHighLevelState)
					if err != nil {
						reqLogger.V(1).Info(fmt.Sprintf("%s: checkUpgradeStandby returned newAutoUpgradeState=%v err=%v",
							us, newAutoUpgradeState, err))
					}

					if newAutoUpgradeState == "UpgradingActive" {
						upgradeActive = true
						reqLogger.V(1).Info(us + ": standby upgrade complete; upgradeActive set to true")
						logTTEvent(ctx, client, instance, "Upgrade", "Upgrade of standby complete", false)
					} else if newAutoUpgradeState == "ManualInterventionRequired" {
						newHighLevelState = "ManualInterventionRequired"
						reqLogger.V(1).Info(us + ": AS HighLevelState set to " + newHighLevelState)
						logTTEvent(ctx, client, instance, "UpgradeError", err.Error(), true)
					}

				} else if instance.Status.ClassicUpgradeStatus.ActiveStatus == "deleteActive" ||
					instance.Status.ClassicUpgradeStatus.ActiveStatus == "processing" {

					// ACTIVE upgrade in progress
					err, newAutoUpgradeState = checkUpgradeActive(ctx, client, instance, tts, newHighLevelState)
					if err != nil {
						reqLogger.V(1).Info(fmt.Sprintf("%s: checkUpgradeActive returned newAutoUpgradeState=%v err=%v",
							us, newAutoUpgradeState, err))
					}

					if newAutoUpgradeState == "Complete" {
						upgradeTime := time.Now().Unix() - instance.Status.ClassicUpgradeStatus.UpgradeStartTime
						msg := fmt.Sprintf("Upgrade completed in %v secs", upgradeTime)
						reqLogger.V(1).Info(us + ": " + msg)
						logTTEvent(ctx, client, instance, "Upgrade", msg, false)
						resetUpgradeVars(instance)

						// open the db that we closed during upgrade prep
						err = openDb(ctx, instance, client, tts)
						if err != nil {
							reqLogger.V(1).Info(us + ": openDb failed")
						}
					} else if newAutoUpgradeState == "ManualInterventionRequired" {
						newHighLevelState = "ManualInterventionRequired"
						reqLogger.V(1).Info(us + ": AS HighLevelState set to " + newHighLevelState)
						logTTEvent(ctx, client, instance, "UpgradeError", err.Error(), true)
					}

				} else {
					var errMsg string
					reqLogger.V(1).Info(fmt.Sprintf("%s: ActiveStatus=%v StandbyStatus=%v UpgradeState=%v", us,
						instance.Status.ClassicUpgradeStatus.ActiveStatus,
						instance.Status.ClassicUpgradeStatus.StandbyStatus,
						instance.Status.ClassicUpgradeStatus.UpgradeState))

					if instance.Status.ClassicUpgradeStatus.ActiveStatus == "failed" {
						errMsg = "Upgrade unsuccessful on the active pod, entering ManualInterventionRequired"
					} else if instance.Status.ClassicUpgradeStatus.StandbyStatus == "failed" {
						errMsg = "Upgrade unsuccessful on the standby pod, entering ManualInterventionRequired"
					} else {
						errMsg = "Error determining upgrade state, entering ManualInterventionRequired"
					}
					reqLogger.V(1).Info(us + ": error, " + errMsg)
					newHighLevelState = "ManualInterventionRequired"
					reqLogger.V(1).Info(us + ": AS HighLevelState set to " + newHighLevelState)
					logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
				}
			}

		} else {
			reqLogger.V(1).Info("Intended States are '" + i0 + "' and '" + i1 + "'")
		}

	case "Standby":
		if i1 == "Active" {

			newHighLevelState = highLevelStateMachine[instance.Status.HighLevelState][s1][s0]

			if instance.Status.ClassicUpgradeStatus.UpgradeState != "" { // if we're performing an upgrade

				if instance.Status.ClassicUpgradeStatus.ActiveStatus == "deleteActive" ||
					instance.Status.ClassicUpgradeStatus.ActiveStatus == "processing" {

					err, newAutoUpgradeState = checkUpgradeActive(ctx, client, instance, tts, newHighLevelState)
					if err != nil {
						reqLogger.V(1).Info(fmt.Sprintf("%s: checkUpgradeActive returned newAutoUpgradeState=%v err=%v",
							us, newAutoUpgradeState, err))
					}

					if newAutoUpgradeState == "Complete" {
						logTTEvent(ctx, client, instance, "Upgrade", "Upgrade of active complete", false)
						upgradeTime := time.Now().Unix() - instance.Status.ClassicUpgradeStatus.UpgradeStartTime
						msg := fmt.Sprintf("Upgrade completed in %v secs", upgradeTime)
						reqLogger.V(1).Info(us + ": " + msg)
						logTTEvent(ctx, client, instance, "Upgrade", msg, false)
						resetUpgradeVars(instance)

						// open the db that we closed during upgrade prep
						err = openDb(ctx, instance, client, tts)
						if err != nil {
							reqLogger.V(1).Info(us + ": openDb failed")
						}
					} else if newAutoUpgradeState == "ManualInterventionRequired" {
						newHighLevelState = "ManualInterventionRequired"
						reqLogger.V(1).Info(us + ": AS HighLevelState set to " + newHighLevelState)
						logTTEvent(ctx, client, instance, "UpgradeError", err.Error(), true)
					}

				} else if instance.Status.ClassicUpgradeStatus.StandbyStatus == "deleteStandby" ||
					instance.Status.ClassicUpgradeStatus.StandbyStatus == "processing" {

					// STANDBY upgrade in progress
					err, newAutoUpgradeState = checkUpgradeStandby(ctx, client, instance, tts, newHighLevelState)
					if err != nil {
						reqLogger.V(1).Info(fmt.Sprintf("%s: checkUpgradeStandby returned newAutoUpgradeState=%v err=%v",
							us, newAutoUpgradeState, err))
					}

					if newAutoUpgradeState == "UpgradingActive" {
						upgradeActive = true
						reqLogger.V(1).Info(us + ": STANDBY upgrade complete; upgradeActive set to true")
						logTTEvent(ctx, client, instance, "Upgrade", "Upgrade of standby complete", false)
					} else if newAutoUpgradeState == "ManualInterventionRequired" {
						newHighLevelState = "ManualInterventionRequired"
						reqLogger.V(1).Info(us + ": AS HighLevelState set to " + newHighLevelState)
						logTTEvent(ctx, client, instance, "UpgradeError", err.Error(), true)
					}
				} else {

					reqLogger.V(1).Info(fmt.Sprintf("%s: ActiveStatus=%v StandbyStatus=%v UpgradeState=%v", us,
						instance.Status.ClassicUpgradeStatus.ActiveStatus,
						instance.Status.ClassicUpgradeStatus.StandbyStatus,
						instance.Status.ClassicUpgradeStatus.UpgradeState))

					if instance.Status.ClassicUpgradeStatus.ActiveStatus == "failed" ||
						instance.Status.ClassicUpgradeStatus.StandbyStatus == "failed" {
						reqLogger.V(1).Info(us + ": error, one or more pods failed to upgrade")
					} else {
						reqLogger.V(1).Info(us + ": error, unknown upgrade state")

					}
					newHighLevelState = "ManualInterventionRequired"
					reqLogger.V(1).Info(us + ": AS HighLevelState set to " + newHighLevelState)
					logTTEvent(ctx, client, instance, "UpgradeError", "Failure during upgrade", true)
				}
			}

		} else {
			reqLogger.V(1).Info("Intended States are '" + i0 + "' and '" + i1 + "'")
		}

	default:
		reqLogger.V(1).Info("Intended States are '" + i0 + "' and '" + i1 + "'")
	}
	return newHighLevelState, upgradeActive
}

// See if a pod has become ready or become not ready
// x := updateReadiness(ctx, client, instance, podNo)
func updateReadiness(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo, podNo int, ready bool) {
	us := "updateReadiness"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	podName := instance.Status.PodStatus[podNo].Name

	// See if there was a 'readiness' transition. Note that we have a couple of possible
	// definitions of 'readiness', so this is done in a couple of ways

	// This is the original one that Fidelity wanted ... any TimesTen db that is up is 'ready'

	// If we don't know the new state then we don't change any readiness states - yet.

	instance.Status.PodStatus[podNo].PrevReady = instance.Status.PodStatus[podNo].Ready
	instance.Status.PodStatus[podNo].Ready = ready
	if instance.Status.PodStatus[podNo].PrevReady != instance.Status.PodStatus[podNo].Ready {
		action := "setReadiness"
		if instance.Status.PodStatus[podNo].Ready == false {
			action = "clearReadiness"
		}
		shortTimeout := newInt(5) // It shouldn't take long to clear/set readiness
		err := RunAction(ctx, instance, podNo, action, nil, client, tts, shortTimeout)
		if err != nil {
			reqLogger.V(1).Info(fmt.Sprintf("%s: Error running %s : %s", us, action, err.Error()))
		}
		if instance.Status.PodStatus[podNo].Ready {
			logTTEvent(ctx, client, instance, "StateChange", fmt.Sprintf("Pod %s is Ready", podName), false)
		} else {
			logTTEvent(ctx, client, instance, "StateChange", fmt.Sprintf("Pod %s is Not Ready", podName), false)
		}
	}

	// Another way to think about 'ready' is "what database is 'active'"?
	// Where should incoming db connections be steered?

	active := false
	if ready {
		if instance.Status.PodStatus[podNo].IntendedState == "Active" {
			active = true
		}
	}

	instance.Status.PodStatus[podNo].PrevActive = instance.Status.PodStatus[podNo].Active
	instance.Status.PodStatus[podNo].Active = active
	if instance.Status.PodStatus[podNo].PrevActive != instance.Status.PodStatus[podNo].Active {
		action := "setActive"
		if instance.Status.PodStatus[podNo].Ready == false {
			action = "clearActive"
		}
		shortTimeout := newInt(5) // It shouldn't take long to clear/set active readiness
		err := RunAction(ctx, instance, podNo, action, nil, client, tts, shortTimeout)
		if err != nil {
			reqLogger.V(1).Info(fmt.Sprintf("%s: Error running %s : %s", us, action, err.Error()))
		}
		if instance.Status.PodStatus[podNo].Ready {
			logTTEvent(ctx, client, instance, "StateChange", fmt.Sprintf("Pod %s is Active Ready", podName), false)
		} else {
			logTTEvent(ctx, client, instance, "StateChange", fmt.Sprintf("Pod %s is Not Active Ready", podName), false)
		}
	}
}

// If the user sets 'Reexamine' then see if they successfully fixed things
// newHLState := handleReexamine(ctx, instance)
func handleReexamine(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic) string {
	us := "handleReexamine"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	newHighLevelState := "???"

	switch instance.Status.PodStatus[0].HighLevelState {
	case "HealthyActive", "Healthy":
		// NOTE: for failed upgrades, where the active has not been touched (standby upgraded first), the status
		// will be healthy; we want to match that and return to HL state Normal
		if instance.Status.PodStatus[1].HighLevelState == "HealthyStandby" {
			newHighLevelState = "Normal"
			instance.Status.PodStatus[0].PrevIntendedState = instance.Status.PodStatus[0].IntendedState
			instance.Status.PodStatus[0].IntendedState = "Active"
			instance.Status.PodStatus[1].PrevIntendedState = instance.Status.PodStatus[1].IntendedState
			instance.Status.PodStatus[1].IntendedState = "Standby"
			if instance.Status.ClassicUpgradeStatus.UpgradeState != "" {
				upgradeTime := time.Now().Unix() - instance.Status.ClassicUpgradeStatus.UpgradeStartTime
				reqLogger.V(1).Info(fmt.Sprintf("%s: failed upgrade resolved in %v secs", us, upgradeTime))
				errMsg := "Recovery from upgrade failure complete"
				reqLogger.V(1).Info(fmt.Sprintf("%s: %v, resetting upgrade vars", us, errMsg))
				logTTEvent(ctx, client, instance, "Upgrade", errMsg, false)
				resetUpgradeVars(instance)
			}
		} else if instance.Status.PodStatus[1].HighLevelState == "CatchingUp" {
			// newHighLevelState = "ManualInterventionRequired"
			newHighLevelState = "Reexamine"
			instance.Status.ClassicUpgradeStatus.StandbyStatus = "CatchingUp"
			reqLogger.V(1).Info(fmt.Sprintf("%s: set StandbyStatus to %v", us, instance.Status.ClassicUpgradeStatus.StandbyStatus))
			reqLogger.V(1).Info(fmt.Sprintf("%s: set newHighLevelState=%v", us, newHighLevelState))
		} else {
			newHighLevelState = "ManualInterventionRequired"
			if instance.Status.ClassicUpgradeStatus.UpgradeState != "" {
				errMsg := "Upgrade: Waiting for standby, set reexamine attrib and try again"
				reqLogger.V(1).Info(us + ": " + errMsg)
				logTTEvent(ctx, client, instance, "UpgradeError", errMsg, true)
			}
		}
	case "HealthyStandby":
		if instance.Status.PodStatus[1].HighLevelState == "Healthy" ||
			instance.Status.PodStatus[1].HighLevelState == "HealthyActive" {

			newHighLevelState = "Normal"
			instance.Status.PodStatus[1].PrevIntendedState = instance.Status.PodStatus[1].IntendedState
			instance.Status.PodStatus[1].IntendedState = "Active"
			instance.Status.PodStatus[0].PrevIntendedState = instance.Status.PodStatus[0].IntendedState
			instance.Status.PodStatus[0].IntendedState = "Standby"
			if instance.Status.ClassicUpgradeStatus.UpgradeState != "" {
				upgradeTime := time.Now().Unix() - instance.Status.ClassicUpgradeStatus.UpgradeStartTime
				reqLogger.V(1).Info(fmt.Sprintf("%s: failed upgrade resolved in %v secs", us, upgradeTime))
				errMsg := "Recovery from upgrade failure complete"
				reqLogger.V(1).Info(fmt.Sprintf("%s: %v, resetting upgrade vars", us, errMsg))
				logTTEvent(ctx, client, instance, "Upgrade", errMsg, false)
				resetUpgradeVars(instance)
			}
		} else {
			newHighLevelState = "ManualInterventionRequired"
		}
	case "HealthyIdle":
		if instance.Status.PodStatus[1].HighLevelState == "Down" {
			// We can fix it!
			instance.Status.PodStatus[0].PrevIntendedState = instance.Status.PodStatus[0].IntendedState
			instance.Status.PodStatus[0].IntendedState = "Active"
			instance.Status.PodStatus[1].PrevIntendedState = instance.Status.PodStatus[1].IntendedState
			instance.Status.PodStatus[1].IntendedState = "Standby"
			newHighLevelState = "ConfiguringActive"
		} else {
			newHighLevelState = "ManualInterventionRequired"
			// Explain this to the user
		}
	case "Down":
		if instance.Status.PodStatus[1].HighLevelState == "HealthyIdle" {
			// We can fix it!
			instance.Status.PodStatus[1].PrevIntendedState = instance.Status.PodStatus[1].IntendedState
			instance.Status.PodStatus[1].IntendedState = "Active"
			instance.Status.PodStatus[0].PrevIntendedState = instance.Status.PodStatus[0].IntendedState
			instance.Status.PodStatus[0].IntendedState = "Standby"
			newHighLevelState = "ConfiguringActive"
		} else {
			newHighLevelState = "ManualInterventionRequired"
			// Explain this to the user
		}
	default:
		newHighLevelState = "ManualInterventionRequired"
	}
	return newHighLevelState
}

func determineNewHLStateReplicated(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, someoneOtherDown bool, someoneHealthy bool, upgradeActive bool, tts *TTSecretInfo) (error, bool) {
	us := "determineNewHLStateReplicated"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	// Now that we've assessed the state of each individual pod,
	// let's reassess the high level health / state of the pair

	reqLogger.V(1).Info(fmt.Sprintf("%s: TTC HLS %s, pod HLS %s and %s.",
		us,
		instance.Status.HighLevelState,
		instance.Status.PodStatus[0].HighLevelState,
		instance.Status.PodStatus[1].HighLevelState))

	newHighLevelState := "???"

	// Odd special case: consider a StandbyDown case where we just fixed the standby, but the active had
	// decided that it was down (active is in OtherDown state). If the -0 pod is active and the -1 pod standby
	// then when we get here the overall state is StandbyDown, the -0 pod state is OtherDown and the -1 pod
	// state is Healthy. But that's misleading; we should have re-examined the -0 pod's state after fixing -1.
	// Because if we DID we'd now find that the -0 state is healthy, too.
	//
	// SO, if one pod is in OtherDown and we then fix the 'other' we won't finish re-assessing the
	// overall state; we'll just return and let the next execution of Reconcile sort it out. That one
	// should see both pods as Healthy.

	if instance.Status.HighLevelState == "StandbyDown" && someoneOtherDown && someoneHealthy {
		reqLogger.V(1).Info(us + ": Special OtherDown Handling: returning")
		instance.Status.PodStatus[0].HighLevelState = "Unknown"
		instance.Status.PodStatus[1].HighLevelState = "Unknown"
		newHighLevelState = "Normal"
	} else {

		// Did the user successfully fix a pair that required manual intervention?

		if instance.Status.HighLevelState == "Reexamine" {
			newHighLevelState = handleReexamine(ctx, client, instance)
		} else {
			newHighLevelState, upgradeActive = determineNextHLState(ctx, client, tts, instance)
		}

		// If the next state is "FAILOVER" then it's time to flip the "intended
		// states" of the pods

		if newHighLevelState == "FAILOVER" {
			newHighLevelState = "ActiveTakeover"
			if instance.Status.PodStatus[0].IntendedState == "Active" {
				instance.Status.PodStatus[0].PrevIntendedState = instance.Status.PodStatus[0].IntendedState
				instance.Status.PodStatus[0].IntendedState = "Standby"
				instance.Status.PodStatus[1].PrevIntendedState = instance.Status.PodStatus[1].IntendedState
				instance.Status.PodStatus[1].IntendedState = "Active"
			} else {
				instance.Status.PodStatus[0].PrevIntendedState = instance.Status.PodStatus[0].IntendedState
				instance.Status.PodStatus[0].IntendedState = "Active"
				instance.Status.PodStatus[1].PrevIntendedState = instance.Status.PodStatus[1].IntendedState
				instance.Status.PodStatus[1].IntendedState = "Standby"
			}
		}
	}

	// Now we've determined what the next high level state of the pair will be
	// ... make sure that it's a legit state
	if _, ok := ClassicHLStates[newHighLevelState]; !ok {
		if newHighLevelState != "FAILOVER" {
			reqLogger.V(1).Info("ERROR: Unsupported newHighLevelState of '" + newHighLevelState + "' current pair state '" + instance.Status.HighLevelState + "'")
		}
	}

	if instance.Status.HighLevelState != newHighLevelState {
		updateTTClassicHighLevelState(ctx, instance, newHighLevelState, client)
	} else {
		reqLogger.V(1).Info("High Level State for this TimesTenClassic: '" + newHighLevelState + "' (unchanged)")
	}

	// Now that we know the new high level state of the pair we can
	// figure out any last pair-wide data.

	// We need to update the "awtBehindMB" datum in the pair's status, which
	// is taken from a pod's value. Which one? Depends on the pair's state.
	// Normally the standby pushes data to Oracle, but if the standby's down
	// then the active will do it.

	var standby int
	var active int
	if instance.Status.PodStatus[1].IntendedState == "Standby" {
		active = 0
		standby = 1
	} else {
		active = 1
		standby = 0
	}

	switch hl := instance.Status.HighLevelState; hl {
	case "Normal", "ActiveDown": // The standby is pushing data to oracle
		if instance.Status.PodStatus[standby].CacheStatus.AwtBehindMb == nil {
			instance.Status.AwtBehindMb = nil
		} else {
			instance.Status.AwtBehindMb = newInt(*instance.Status.PodStatus[standby].CacheStatus.AwtBehindMb)
		}

	case "StandbyDown", "StandbyStarting", "StandbyCatchup":
		if instance.Status.PodStatus[active].CacheStatus.AwtBehindMb == nil {
			instance.Status.AwtBehindMb = nil
		} else {
			instance.Status.AwtBehindMb = newInt(*instance.Status.PodStatus[active].CacheStatus.AwtBehindMb)
		}

	case "Failed", "Initializing", "BothDown", "ManualInterventionRequired", "WaitingForActive":
		instance.Status.AwtBehindMb = nil
	default:
		instance.Status.AwtBehindMb = nil
	}

	return nil, upgradeActive
}

func determineNewHLStateNonReplicated(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo) error {
	us := "determineNewHLStateNonReplicated"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	newHighLevelState := "NoReplicasReady"

	nReplicas := getNReplicas(instance)

	readyCt := 0
	for _, isP := range instance.Status.PodStatus {
		if isP.Ready {
			readyCt++
		}
	}

	reqLogger.V(2).Info(fmt.Sprintf("replicas %d ready %d", nReplicas, readyCt))

	if readyCt == nReplicas {
		newHighLevelState = "AllReplicasReady"
	} else if readyCt == 0 {
		newHighLevelState = "NoReplicasReady"
	} else {
		newHighLevelState = "SomeReplicasReady"
	}

	if instance.Status.HighLevelState != newHighLevelState {
		updateTTClassicHighLevelState(ctx, instance, newHighLevelState, client)
	} else {
		reqLogger.V(1).Info("High Level State for this TimesTenClassic: '" + newHighLevelState + "' (unchanged)")
	}

	return nil
}

func determineNewSubscriberHLState(ctx context.Context, client client.Client, instance *timestenv2.TimesTenClassic, tts *TTSecretInfo) error {
	us := "determineNewSubscriberHLState"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	newHighLevelState := "NoSubscribersReady"

	nSubs := getNSubscribers(instance)

	readyCt := 0
	for _, isP := range instance.Status.PodStatus {
		if isP.TTPodType != "Subscriber" {
			continue // Only look at subscribers
		}
		if isP.Ready {
			readyCt++
		}
	}

	reqLogger.V(2).Info(fmt.Sprintf("readyCt %d nSubs %d", readyCt, nSubs))

	if readyCt == nSubs {
		newHighLevelState = "AllSubscribersReady"
	} else if readyCt == 0 {
		newHighLevelState = "NoSubscribersReady"
	} else {
		newHighLevelState = "SomeSubscribersReady"
	}

	if instance.Status.Subscriber.HLState != newHighLevelState {
		updateSubscriberHighLevelState(ctx, instance, newHighLevelState, client)
	} else {
		reqLogger.V(1).Info("Subscriber High Level State for this TimesTenClassic: '" + newHighLevelState + "' (unchanged)")
	}

	return nil
}

// Reconcile reads the state of the cluster for a TimesTenClassic object and makes changes based on the state
// and what is in the TimesTenClassic.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func ReconcileClassic(ctx context.Context, request reconcile.Request, client client.Client, scheme *runtime.Scheme) (reconcile.Result, error) {
	var err error
	reqLogger := log.FromContext(ctx)

	//reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info(fmt.Sprintf("Reconciling TimesTenClassic %s", request.Name))
	defer reqLogger.Info("Reconcile complete")

	// Fetch the TimesTenClassic instance
	instance := &timestenv2.TimesTenClassic{}
	err = client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			// We were asked to handle an object that no longer exists.
			// Guess we're completely done with it, then.
			reqLogger.V(1).Info("TimesTenClassic object " + request.Namespace + "." + request.Name + " does not exist")

			deleteHTTPClient(ctx, request.Namespace, request.Name)

			secretName, err := ioutil.ReadFile("/tmp/" + request.Namespace + ".ttc." + request.Name)
			if err != nil {
				reqLogger.V(1).Info("Reconcile: /tmp/" + request.Namespace + ".ttc." + request.Name +
					" does not exist, not deleting secrets/certs")
			} else {

				if string(secretName) != "" {
					reqLogger.V(2).Info("Reconcile: found secret file, object uid was " + string(secretName))
					reqLogger.V(1).Info("Reconcile: calling deleteSecret")
					ttSecrets.deleteSecret(ctx, string(secretName), request.Namespace, request.Name)
					reqLogger.V(1).Info("Reconcile: calling deleteCerts")
					deleteCerts(ctx, string(secretName))
				} else {
					reqLogger.V(1).Info("Reconcile: /tmp/" + request.Namespace + ".ttc." + request.Name +
						" does not contain a valid object uid, not deleting secrets/certs")
				}

			}

			// TODO remove / revoke rootCAs

			deleteClassicMetrics(ctx, request.Name)
			return reconcile.Result{}, nil // Won't requeue to us
		}
		// Error reading the object - requeue the request.
		reqLogger.V(1).Info("Unknown error reading TimesTenClassic " + request.Namespace + "." +
			request.Name + ": " + err.Error())
		return reconcile.Result{RequeueAfter: defaultRequeueInterval}, err
	}

	replicated, _ /* nReplicas */, _ /* nSubscribers */, maxNSubscribers, _ /* subName */ := replicationConfig(instance)

	// Should we even attempt to look at the object?  Perhaps not. Note that when WE update our
	// object's "status", that modifies the object ... resulting in our watch on the object calling
	// Reconcile again. Which is silly. So we only process the object if something has actually
	// changed, or if sufficient time has gone by since the last time we processed it, or if we're
	// in the middle of anything important.

	// Have we already processed this generation of the spec?

	processMe := false
	thisOp := os.Getenv("POD_NAME")
	t := time.Now().UnixMilli() // Milliseconds since epoch

	var ourPollingInterval time.Duration = defaultRequeueInterval

	var ourPollingIntervalMillisecs int64 = defaultRequeueMillisecs

	if instance.Spec.TTSpec.PollingInterval != nil &&
		*instance.Spec.TTSpec.PollingInterval > 0 {
		ourPollingIntervalMillisecs = int64(*instance.Spec.TTSpec.PollingInterval) * 1000
	}

	ourPollingInterval = time.Duration(ourPollingIntervalMillisecs * int64(time.Millisecond))

	reqLogger.V(2).Info(fmt.Sprintf("ObservedGeneration=%d; current=%d; resourceVersion='%s'", instance.Status.ObservedGeneration, instance.ObjectMeta.Generation, instance.ObjectMeta.ResourceVersion))
	reqLogger.V(2).Info(fmt.Sprintf("LastReconcilingOperator='%s'; current='%s'", instance.Status.LastReconcilingOperator, thisOp))
	reqLogger.V(2).Info(fmt.Sprintf("ttcuid=%s LastReconcileTime=%d; now=%d; diff=%d", string(instance.UID), instance.Status.LastReconcileTime, t, t-instance.Status.LastReconcileTime))

	if instance.ObjectMeta.Generation != instance.Status.ObservedGeneration {
		processMe = true
		instance.Status.ObservedGeneration = instance.ObjectMeta.Generation
		instance.Status.LastReconcileTime = t
		instance.Status.LastReconcilingOperator = thisOp
	} else {
		if instance.Status.LastReconcilingOperator != thisOp {
			processMe = true
			instance.Status.LastReconcilingOperator = thisOp
			instance.Status.LastReconcileTime = t
		} else {
			if t >= instance.Status.LastReconcileTime+ourPollingIntervalMillisecs {
				processMe = true
				instance.Status.LastReconcileTime = t
			} else {
				//if instance.Status.HighLevelState != "Normal" && instance.Status.HighLevelState != "AllReplicasReady" {
				//    // reqLogger.V(2).Info(fmt.Sprintf("instance.Status.HighLevelState %s != Normal, process", instance.Status.HighLevelState))
				//    processMe = true
				//    instance.Status.LastReconcileTime = t
				//} else {
				// Do NOT update lastReconcileTime! We want to wait the appropriate time from the last ACTUAL reconcile
				millisLeftToWait := ourPollingIntervalMillisecs - (t - instance.Status.LastReconcileTime)
				reqLogger.V(2).Info(fmt.Sprintf("ttcuid=%s Waiting for %d more milliseconds, requeing", string(instance.UID), millisLeftToWait))
				newPollingInterval := time.Duration(millisLeftToWait * int64(time.Millisecond))
				return reconcile.Result{RequeueAfter: newPollingInterval}, nil
				//}
			}
		}
	}

	if processMe {
		reqLogger.V(2).Info("Processing Reconcile for ttcuid=" + string(instance.UID))
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("Waiting for impactful change or requeue interval (%v)", ourPollingInterval))
		return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
	}

	// OK, let's go ahead and do whatever we need to do to this object

	// Whatever we do, update the status before we leave

	defer updateClassicMetrics(ctx, instance)

	defer updateStatus(ctx, client, instance)

	// Get or make the secret info we will use to control this TimesTenClassic

	secretName := "tt" + string(instance.ObjectMeta.UID)
	exporterSecretName := fmt.Sprintf("%s-metrics", instance.ObjectName())

	tts, err := getKey(ctx, instance, client, scheme, secretName)
	if err != nil {
		reqLogger.Error(err, "GetKey failed: "+err.Error()+", we cannot manage this TimesTenClassic.")
		// This object is dead as far as we are concerned.
		// We can't talk to the agents in it, so there's nothing we can do.

		return reconcile.Result{RequeueAfter: defaultRequeueInterval}, err
	}

	// process finalizers (cleanup before object destruction)
	status, err := processFinalizers(ctx, client, request, instance, tts)
	if err != nil {
		logTTEvent(ctx, client, instance, "FailedFinalizer", "Finalizer not processed: "+err.Error(), true)
		return reconcile.Result{}, err
	} else if status == "done" {
		// Stop reconciliation as the item was deleted
		return reconcile.Result{}, nil
	}

	// If this is the first time we've seen this TimesTenClassic then we'll have to do some
	// initialization of it's status first

	oneTimeInit(ctx, instance, tts, client, scheme)

	pendingAsyncTask, asyncErr := checkPendingAsyncTask(ctx, client, instance, tts)
	if asyncErr != nil {
		reqLogger.Info(fmt.Sprintf("Reconcile: error checking async status, asyncErr=%v", asyncErr))
		// TODO: don't do this indefinately, at some point enter manual intervention
		return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
	}

	if pendingAsyncTask == false {
		reqLogger.V(2).Info("Reconcile: no pending async tasks")
	} else {
		// Stop reconciliation; we're waiting for the task to complete
		reqLogger.Info("Reconcile: there are pending async tasks, requeue reconcile")
		return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
	}

	// For testing we can cause Reconcile to return immediately. This
	// can be done in several ways.

	if shouldOpDoNothing(ctx, instance, instance.Status.HighLevelState) {
		return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
	}

	// If the pair is 'Failed' then we don't even bother to examine it. It's dead and will
	// never come back.

	if instance.Status.HighLevelState == "Failed" {
		reqLogger.V(1).Info("TimesTenClassic in 'Failed' state. User must delete.")
		return reconcile.Result{}, nil
	}

	// If .spec.ttspec.stopManaging has changed then we are supposed to effectively ignore this object.

	if instance.Spec.TTSpec.StopManaging != instance.Status.PrevStopManaging &&
		instance.Status.HighLevelState != "ManualInterventionRequired" {
		updateTTClassicHighLevelState(ctx, instance, "ManualInterventionRequired", client)
		instance.Status.PrevStopManaging = instance.Spec.TTSpec.StopManaging
		return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
	}

	instance.Status.PrevStopManaging = instance.Spec.TTSpec.StopManaging

	// If .spec.ttspec.reexamine has changed then we are supposed to start managing
	// the object again

	if instance.Spec.TTSpec.Reexamine != instance.Status.PrevReexamine {
		instance.Status.PrevReexamine = instance.Spec.TTSpec.Reexamine

		if instance.Status.HighLevelState == "SomeReplicasReady" || instance.Status.HighLevelState == "NoReplicasReady" {
			// reqLogger.V(1).Info("Reconcile: Reexamine set in nonrep config, instance.Status.HighLevelState=SomeReplicasReady")
			// find the pods in manualintervention and reexamine them
			for podNo, _ := range instance.Status.PodStatus {
				isP := &instance.Status.PodStatus[podNo]
				if isP.HighLevelState == "ManualInterventionRequired" {
					updatePodHighLevelState(ctx, instance, isP, "Reexamine", client)
				}
			}
		}

		if instance.Status.HighLevelState == "ManualInterventionRequired" {
			reqLogger.V(1).Info("Reconcile: Reexamine set, calling updateTTClassicHighLevelState")
			updateTTClassicHighLevelState(ctx, instance, "Reexamine", client)
			instance.Status.ActivePods = "None"
			return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
		}
	}
	// Verify that the exporter secret exists if needed; create it if not

	ttRel, _ := GetTTMajorRelease(ctx)

	if instance.Spec.TTSpec.Prometheus != nil &&
		instance.Spec.TTSpec.Prometheus.Insecure != nil &&
		*instance.Spec.TTSpec.Prometheus.Insecure == true {
		// No secret needed
	} else {
		if ttRel > 18 {
			err = checkExporterSecrets(ctx, instance, client, scheme, exporterSecretName)
			if err != nil {
				return reconcile.Result{RequeueAfter: ourPollingInterval}, err
			}
		} else {
			// No secret needed
		}
	}

	// Make sure the object's service exists; create it if not
	err = checkService(ctx, instance, client, scheme)
	if err != nil {
		if instance.Status.HighLevelState == "Initializing" {
			updateTTClassicHighLevelState(ctx, instance, "Failed", client)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{RequeueAfter: ourPollingInterval}, err
	}

	var updatedImage bool
	var upgradeStandby bool
	var upgradeActive bool

	// if the user patches with ResetUpgradeState, reset upgrade task variables

	if instance.Spec.TTSpec.ResetUpgradeState != instance.Status.ClassicUpgradeStatus.PrevResetUpgradeState {
		instance.Status.ClassicUpgradeStatus.PrevResetUpgradeState = instance.Spec.TTSpec.ResetUpgradeState
		errMsg := "Resetting upgrade state"
		reqLogger.V(1).Info(fmt.Sprintf("Reconcile: %v", errMsg))
		logTTEvent(ctx, client, instance, "Upgrade", errMsg, true)
		resetUpgradeVars(instance)
	}

	updatedImage, err = checkMainStatefulSet(ctx, instance, client, scheme)
	if err != nil {
		reqLogger.V(1).Info(fmt.Sprintf("Reconcile: checkMainStatefulSet failed, %v", err))
		return reconcile.Result{RequeueAfter: ourPollingInterval}, err
	} else if updatedImage == true {
		if *instance.Spec.TTSpec.ImageUpgradeStrategy != "Manual" {
			upgradeStandby = true
			reqLogger.V(1).Info("Reconcile: upgradeStandby set to true")

			// TODO: if we want to automatically delete the standby once we detect and image change
			//if instance.Status.HighLevelState == "Reexamine" {
			//    err := initUpgrade("STANDBY", instance, client, reqLogger)
			//    if err != nil {
			//        reqLogger.V(1).Info("Reconcile: ERROR=" + err.Error())
			//    }
			//    return reconcile.Result{RequeueAfter: ourPollingInterval}, err
			//}
		}
	}

	updatedImage, err = checkSubscriberStatefulSet(ctx, instance, client, scheme)
	if err != nil {
		reqLogger.V(1).Info(fmt.Sprintf("Reconcile: checkSubscriberStatefulSet failed, %v", err))
		return reconcile.Result{RequeueAfter: ourPollingInterval}, err
	} else if updatedImage == true {
		if *instance.Spec.TTSpec.ImageUpgradeStrategy != "Manual" {
			upgradeStandby = true
			reqLogger.V(1).Info("Reconcile: upgradeStandby set to true")

			// TODO: if we want to automatically delete the standby once we detect and image change
			//if instance.Status.HighLevelState == "Reexamine" {
			//    err := initUpgrade("STANDBY", instance, client, reqLogger)
			//    if err != nil {
			//        reqLogger.V(1).Info("Reconcile: ERROR=" + err.Error())
			//    }
			//    return reconcile.Result{RequeueAfter: ourPollingInterval}, err
			//}
		}
	}

	// See if our PodMonitor exists, and create it if it doesn't

	if instance.Spec.TTSpec.Prometheus == nil ||
		instance.Spec.TTSpec.Prometheus.CreatePodMonitors == nil ||
		*instance.Spec.TTSpec.Prometheus.CreatePodMonitors {

		if instance.Spec.TTSpec.Prometheus == nil ||
			instance.Spec.TTSpec.Prometheus.CertSecret == nil {
			checkPodMonitor(ctx, instance, scheme, client)
		}
	}

	// Get information from the agent in each pod about the state of things (if possible)
	// Also looks at the pod itself - is it the same one we saw last time?
	// Did any containers get oom killed? Etc.

	err = getUpdatedInfoAboutPods(ctx, client, scheme, instance, tts)
	if err != nil {
		reqLogger.V(1).Info("Error from getUpdatedInfoAboutPods: " + err.Error())
		return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
	}

	// If the object is 'ManualInterventionRequired' then we, uh, don't take any action...

	if instance.Status.HighLevelState == "ManualInterventionRequired" {
		reqLogger.V(1).Info("Reconcile: HighLevelState=ManualInterventionRequired; taking no action")
		return reconcile.Result{RequeueAfter: ourPollingInterval}, nil
	}

	// See if the A/S pair can be recovered in the case of a simultaneous BothDown situation

	if replicated && instance.Status.BothDownRecoveryIneligible == false {
		for podNo := 0; podNo < 2; podNo++ {
			if instance.Status.PodStatus[podNo].UsingTwosafe != nil &&
				*instance.Status.PodStatus[podNo].UsingTwosafe {
				instance.Status.UsingTwosafe = true
				if instance.Status.PodStatus[podNo].DisableReturn != nil &&
					*instance.Status.PodStatus[podNo].DisableReturn {
					instance.Status.BothDownRecoveryIneligible = true
				}
				if instance.Status.PodStatus[podNo].LocalCommit != nil &&
					*instance.Status.PodStatus[podNo].LocalCommit {
					instance.Status.BothDownRecoveryIneligible = true
				}
			}
		}
	}

	// Verify that TimesTen in each pod is running the same major release as
	// TimesTen in the operator pod

	reqLogger.V(2).Info(fmt.Sprintf("Operator has access to TimesTen release %d", ttRel))
	for podName, _ := range instance.Status.PodStatus {
		if instance.Status.PodStatus[podName].PodStatus.Agent == "Up" {
			components := strings.Split(instance.Status.PodStatus[podName].TimesTenStatus.Release, ".")
			if len(components) > 1 {
				if components[0] == fmt.Sprintf("%d", ttRel) {
					//reqLogger.V(2).Info(fmt.Sprintf("%s also has TimesTen release %d", instance.Status.PodStatus[podNo].Name, ttRel))
				} else {
					msg := fmt.Sprintf("%s is TimesTen release %s but operator is release %d", instance.Status.PodStatus[podName].Name, components[0], ttRel)
					reqLogger.V(1).Info(msg)
					if instance.GetHighLevelState() == "Initializing" {
						failMsg := fmt.Sprintf("v%d operator not patch compatible with v%s object; HLState set to Failed", ttRel, components[0])
						reqLogger.V(1).Info(failMsg)
						updateTTClassicHighLevelState(ctx, instance, "Failed", client)
						logTTEvent(ctx, client, instance, "Fatal", failMsg, true)
					} else {
						failMsg := fmt.Sprintf("v%d operator not patch compatible with v%s object; HLState set to ManualInterventionRequired", ttRel, components[0])
						reqLogger.V(1).Info(failMsg)
						updateTTClassicHighLevelState(ctx, instance, "ManualInterventionRequired", client)
						logTTEvent(ctx, client, instance, "Fatal", failMsg, true)
					}
					return reconcile.Result{}, nil
				}
			}
		}
	}

	// Let's assess the state of each pod (excluding subscribers)
	// This normally may also take actions in order to change the state - for
	// example, starting the daemon, duplicating a database, etc.
	// If we are simply 'examining' the state of the pair we don't take any
	// explicit actions, we'll just see what's going on

	reqLogger.V(1).Info("Reconcile: assessing the state of each primary database pod")

	someoneOtherDown := false
	someoneHealthy := false

	for podNo, _ := range instance.Status.PodStatus {
		isP := &instance.Status.PodStatus[podNo]
		podName := isP.Name

		if isP.TTPodType != "Database" {
			continue // Skip subscribers for this
		}

		// Based on the state of the state of the pair figure out which state machine / flowchart
		// to run on the pod

		var answer FSMAnswer
		var funcToRun FSMFunc
		var funcName string
		var ready bool

		if replicated == false {
			reqLogger.Info(fmt.Sprintf("Pod %v (non-replicated) Object HighLevelState=%v, Pod HighLevelState=%v, Pod IntendedState=%v", podName, instance.Status.HighLevelState,
				isP.HighLevelState, isP.IntendedState))
		} else {
			reqLogger.Info(fmt.Sprintf("Pod %v Pair HighLevelState=%v, Pod IntendedState=%v", podName, instance.Status.HighLevelState, isP.IntendedState))
		}
		funcName, funcToRun = pickFlowToRun(ctx, instance, podName, isP)

		// Now that we've determined what flow we should run (if any), let's run it

		if funcToRun == nil {
			if replicated == false && isP.HighLevelState == "ManualInterventionRequired" {
				msg := "Pod " + podName + " state is ManualInterventionRequired"
				reqLogger.V(1).Info("Reconcile: " + msg)
			} else {
				msg := "Not running any SM for pod " + podName
				reqLogger.V(1).Info("Reconcile: " + msg)
				logTTEvent(ctx, client, instance, "Error", msg, true)
			}
		} else {
			answer, err, ready = funcToRun(ctx, client, instance, podNo, tts)
			if err == nil {
				reqLogger.V(1).Info(fmt.Sprintf("Reconcile: "+funcName+" returned ANSWER: '"+string(answer)+"', ready: %v", ready))
			} else {
				reqLogger.V(1).Info(fmt.Sprintf("Reconcile: "+funcName+" returned ANSWER: '"+string(answer)+"' ERROR '"+err.Error()+"', ready: %v", ready))
			}
			if err != nil {
				if instance.Status.HighLevelState == "Reexamine" {
					if isP.IntendedState == "Active" {
						logTTEvent(ctx, client, instance, "Error", fmt.Sprintf("Pod %s-%d: Active error: %s", instance.Name, podNo, err.Error()), true)
					} else {
						logTTEvent(ctx, client, instance, "Error", fmt.Sprintf("Pod %s-%d: Standby error: %s", instance.Name, podNo, err.Error()), true)
					}
				} else {
					logTTEvent(ctx, client, instance, "Error", fmt.Sprintf("Pod %s-%d %s", instance.Name, podNo, err.Error()), true)
				}
			}

			// Update the readiness or lack thereof of this pod

			if answer != "Unknown" {
				updateReadiness(ctx, client, instance, tts, podNo, ready)
			}

			isP = &instance.Status.PodStatus[podNo]

			if !replicated && answer == "Unknown" {
				// For non-replicated databases we don't make the pod state 'unknown' when
				// we don't know what the state of the pod is ... rather we keep the previous
				// state.
			} else {
				updatePodHighLevelState(ctx, instance, isP, string(answer), client)
			}

			if string(answer) == "OtherDown" {
				someoneOtherDown = true
			}
			if string(answer) == "Healthy" {
				someoneHealthy = true
			}
		}

	} // end loop (looping through each pod in the object)

	if replicated {
		_, upgradeActive = determineNewHLStateReplicated(ctx, client, instance, someoneOtherDown, someoneHealthy, upgradeActive, tts)
	} else {
		determineNewHLStateNonReplicated(ctx, client, instance, tts)
	}

	// Now that we've updated the new high level state of the pair
	// we can do any reporting that's necessary

	switch hl := instance.Status.HighLevelState; hl {
	case "Failed", "Initializing", "BothDown", "ActiveDown", "ManualInterventionRequired", "WaitingForActive", "ConfiguringActive":
		instance.Status.ActivePods = "None"

	case "Normal", "StandbyDown", "ActiveTakeover", "StandbyStarting", "StandbyCatchup":
		if instance.Status.PodStatus[0].Initialized && instance.Status.PodStatus[1].Initialized {
			if instance.Status.PodStatus[0].IntendedState == "Active" {
				instance.Status.ActivePods = instance.Status.PodStatus[0].Name
			}
			if instance.Status.PodStatus[1].IntendedState == "Active" {
				instance.Status.ActivePods = instance.Status.PodStatus[1].Name
			}
		}

	case "OneDown":
		ap := "None"
		if instance.Status.PodStatus[0].HighLevelState == "Healthy" {
			ap = instance.Status.PodStatus[0].Name
			if instance.Status.PodStatus[1].HighLevelState == "Healthy" {
				ap = ap + "," + instance.Status.PodStatus[1].Name
			}
		} else {
			if instance.Status.PodStatus[1].HighLevelState == "Healthy" {
				ap = instance.Status.PodStatus[1].Name
			}
		}
		instance.Status.ActivePods = ap

	case "NoReplicasReady", "SomeReplicasReady", "AllReplicasReady":
		instance.Status.ActivePods = "N/A"

	default:
		reqLogger.V(1).Info("Unknown instance.Status.HighLevelState '" + instance.Status.HighLevelState + "' found in activePods determination")
	}

	if upgradeStandby == true {

		err := initUpgrade(ctx, "STANDBY", instance, client)
		if err != nil {
			// initUpgrade wrote msg to event log
			reqLogger.Error(err, "Reconcile: initUpgrade for standby failed")
		}

	} else if upgradeActive == true {

		// quiesce and make sure the standby has caught up before killing the active pod
		err = quiesceForUpgrade(ctx, instance, client, tts)
		if err != nil {
			eventLogMsg := "Could not quiesce the database, standby behind active"
			reqLogger.Error(err, "Reconcile: "+eventLogMsg)
			logTTEvent(ctx, client, instance, "UpgradeError", eventLogMsg, true)
			logTTEvent(ctx, client, instance, "UpgradeError", "Upgrade aborted", true)

			newHighLevelState := "ManualInterventionRequired"
			updateTTClassicHighLevelState(ctx, instance, newHighLevelState, client)
			reqLogger.V(1).Info("Reconcile: AS HighLevelState set to " + newHighLevelState)

			// open the database (previously closed during upgrade prep)
			err = openDb(ctx, instance, client, tts)
			if err != nil {
				reqLogger.Error(err, "Reconcile: openDb failed")
			}

		} else {
			reqLogger.Info("Reconcile: quiesceForUpgrade complete, proceed with upgrade")

			err := initUpgrade(ctx, "ACTIVE", instance, client)
			if err != nil {
				reqLogger.Error(err, "Reconcile: initUpgrade for active failed")
			}
		}
	}

	// Now that we have sorted out the status of the main databases
	// (replicated a/s pair or standalones), if there are any subscribers
	// we need to handle them.

	// Let's assess the state of each subscriber pod
	// This normally may also take actions in order to change the state - for
	// example, starting the daemon, duplicating a database, etc.

	if maxNSubscribers > 0 {
		reqLogger.V(1).Info("Reconcile: assessing the state of each subscriber pod")

		for podNo, _ := range instance.Status.PodStatus {
			isP := &instance.Status.PodStatus[podNo]
			podName := isP.Name

			if isP.TTPodType != "Subscriber" {
				continue // Only look at subscribers
			}

			// Based on the state of the state of the pair figure out which state machine / flowchart
			// to run on the pod

			var answer FSMAnswer
			var funcToRun FSMFunc
			var funcName string
			var ready bool

			reqLogger.Info(fmt.Sprintf("Pod %v subscriber replicated %t Pair HL State=%v, this pod IntendedState=%v", podName, replicated, instance.Status.HighLevelState, isP.IntendedState))
			funcName, funcToRun = pickFlowToRun(ctx, instance, podName, isP)

			// Now that we've determined what flow we should run (if any), let's run it

			if funcToRun == nil {
				msg := "Reconcile: Not running any SM for subscriber pod " + podName
				reqLogger.V(1).Info(msg)
				logTTEvent(ctx, client, instance, "Error", msg, true)
			} else {
				answer, err, ready = funcToRun(ctx, client, instance, podNo, tts)
				if err == nil {
					reqLogger.V(1).Info(fmt.Sprintf("Reconcile: "+funcName+" returned ANSWER: '"+string(answer)+"', ready: %v", ready))
				} else {
					reqLogger.V(1).Info(fmt.Sprintf("Reconcile: "+funcName+" returned ANSWER: '"+string(answer)+"' ERROR '"+err.Error()+"', ready: %v", ready))
				}
				if instance.Status.HighLevelState == "Reexamine" && err != nil {
					logTTEvent(ctx, client, instance, "Error", "Subscriber error: "+err.Error(), true)
				}

				// Update the readiness or lack thereof of this pod

				if answer != "Unknown" {
					updateReadiness(ctx, client, instance, tts, podNo, ready)
					isP.PrevHighLevelState = isP.HighLevelState
					isP.LastHighLevelStateSwitch = time.Now().Unix()
					isP.HighLevelState = string(answer)
					reqLogger.V(1).Info(fmt.Sprintf("Reconcile: Subscriber POD [%s] HighLevelState set to %v", isP.Name, isP.HighLevelState))
				}

				if string(answer) == "OtherDown" {
					someoneOtherDown = true
				}
				if string(answer) == "Healthy" {
					someoneHealthy = true
				}

			}

		} // end loop (looping through each pod in the object)

		determineNewSubscriberHLState(ctx, client, instance, tts)

	}

	return reconcile.Result{RequeueAfter: ourPollingInterval}, nil

}

// Test harness: tests can create files to tell the operator to stop processing
// a TimesTenClassic object in various high level states.
func shouldOpDoNothing(ctx context.Context, instance *timestenv2.TimesTenClassic, stateToCheck string) bool {
	us := "shouldOpDoNothing"
	reqLogger := log.FromContext(ctx)

	_, err := os.Stat("/tmp/donothing")
	if err == nil {
		reqLogger.V(2).Info(us + ": Operator should stop processing object, /tmp/donothing file found")
		return true
	}

	if stateToCheck == "" {
		// Skip state check
		return false
	}

	// If the user has created a file called /tmp/donothingStates inside
	// the operator's container then we will read the file. It should contain
	// one or more lines of text; each line contains the name of a top level
	// state for the TimesTenClassic object (Normal, ActiveDown, etc).
	// If the TimesTenClassic object is in one of the states listed in the
	// file then the operator will immediately return.

	_, err = os.Stat("/tmp/donothingStates")
	if err == nil {
		pleaseStop := false
		fd, err := os.Open("/tmp/donothingStates")
		if err == nil {
			scan := bufio.NewScanner(fd)
			for scan.Scan() {
				if strings.EqualFold(scan.Text(), stateToCheck) {
					pleaseStop = true
					reqLogger.V(2).Info(us + ": Operator should stop processing object, state " + stateToCheck + " found in /tmp/donothingStates")
				}
			}
			fd.Close()
			if pleaseStop {
				return true
			}
		} else {
			reqLogger.V(2).Info(us + ": /tmp/donothingStates found but cannot be opened: " + err.Error())
		}
	}
	return false
}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
