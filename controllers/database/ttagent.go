// Copyright (c) 2019-2021, Oracle and/or its affiliates. All rights reserved.
//
// Communication with the TimesTen Agent

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	logr "github.com/go-logr/logr"
	"github.com/google/uuid"
	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type asyncStatus struct {
	running  bool
	complete bool
	errNo    int
}

// Ask the Agent to execute an action on our behalf by sending a POST request
// This generic version works for Classic and Scaleout

func RunActionG(ctx context.Context, instance timestenv2.TimesTenObject, isP *timestenv2.TimesTenPodStatus, action string, params interface{}, client client.Client, tts *TTSecretInfo, timeoutOverride *int, caller string) error {
	podName := isP.Name
	reqLogger := log.FromContext(ctx)
	us := "RunActionG"
	what := fmt.Sprintf("%s: calling action %s on %s", us, action, podName)
	reqLogger.V(1).Info(what)
	defer reqLogger.V(2).Info(what + " returns")

	ourDNSName := podName + "." + instance.ObjectName() + "." + instance.ObjectNamespace() + ".svc.cluster.local"

	ourUrl := "https://" + ourDNSName + ":8443/agent"
	reqLogger.V(2).Info("RunAction: target url is " + ourUrl)

	// If the TimesTen object's name contains a hyphen (for example, "xyz-abc")
	// The code in starthost.pl will truncate the DSN / data store path to "xyz".
	// This code needs to do the same thing. If you change this, change starthost.pl
	// as well as the code that creates the RepCreateStatement.

	dbName := instance.ObjectName()
	dash := strings.Index(dbName, "-")
	if dash > -1 {
		dbName = dbName[:dash]
	}
	v := url.Values{}
	v.Set("verb", action)
	v.Set("dbName", dbName)
	v.Set("ourDNSName", ourDNSName)
	v.Set("objectName", instance.ObjectName())
	v.Set("objectNamespace", instance.ObjectNamespace())

	v.Set("zzAgentDebugInfo", strconv.Itoa(instance.GetAgentDebugInfo()))
	if ti := instance.GetAgentTestInfo(); ti != nil {
		v.Set("zzTestInfo", *ti)
	}

	switch params.(type) {
	case map[string]string:
		for key, val := range params.(map[string]string) {
			//reqLogger.V(2).Info("RunAction: set " + key + "=" + val)
			v.Set(key, val)
		}
	case map[string]interface{}:
		for key, vval := range params.(map[string]interface{}) {
			switch vval.(type) {
			case string:
				val := vval.(string)
				//reqLogger.V(2).Info("RunAction: set " + key + "=" + val)
				v.Set(key, val)
			case []string:
				for _, vvv := range vval.([]string) {
					//reqLogger.V(2).Info("RunAction: add " + key + "=" + vvv)
					v.Add(key, vvv)
				}
			default:
			}
		}

	default:
	}

	fetchStatusAgain := true // After an action we usually do a GET to fetch updated status. Usually.

	switch action {
	case "setReadiness", "clearReadiness", "setActive", "clearActive":
		fetchStatusAgain = false
	default:
	}

	// Classic vs Scaleout specific datum

	switch tt := instance.(type) {
	case *timestenv2.TimesTenClassic:

		// Is this a subscriber? (Find the active and standby while we're at it)

		isSub := false
		active := -1
		standby := -1
		for n, iP := range tt.Status.PodStatus {
			switch iP.TTPodType {
			case "Subscriber":
				if iP.Name == podName {
					isSub = true // We are a subscriber
				}

			case "Database":
				switch iP.IntendedState {
				case "Active":
					active = n
				case "Standby":
					standby = n
				case "Standalone":
					// Ignore it
				default:
					reqLogger.Info(fmt.Sprintf("Unexpected intendedState '%s'", iP.IntendedState))
				}

			default:
				reqLogger.Info(fmt.Sprintf("Unexpected podType '%s'", iP.TTPodType))
			}
		}

		if active >= 0 {
			v.Set("activeDNSName", tt.Status.PodStatus[active].Name+"."+instance.ObjectName()+"."+instance.ObjectNamespace()+".svc.cluster.local")
		}
		if standby >= 0 {
			v.Set("standbyDNSName", tt.Status.PodStatus[standby].Name+"."+instance.ObjectName()+"."+instance.ObjectNamespace()+".svc.cluster.local")
		}

		// Who should we duplicate FROM?

		if isSub {
			v.Set("weAreSubscriber", "1")
		} else {
			if isReplicated(tt) {
				// Part of an active/standby pair
				if active < 0 {
					reqLogger.Info(fmt.Sprintf("Could not determine active! %v %d %d", isSub, active, standby))
				} else {
					v.Set("otherDNSName", tt.Status.PodStatus[active].Name+"."+instance.ObjectName()+"."+instance.ObjectNamespace()+".svc.cluster.local")
				}
			}
		}

		if action == "createRepScheme" {
			v.Set("repCreateStatement", tt.Status.RepCreateStatement)
		}

	default:
		reqLogger.Error(errors.New("RunAction: Unknown type "+fmt.Sprintf("%T", tt)), "Could not insert type-specific items")
	}

	reqLogger.Info("RunAction: request params=" + (v.Encode())) // Parsed by customer, leave info not debug

	req, err := http.NewRequest("POST", ourUrl, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")
	req.SetBasicAuth(tts.HttpUID, tts.HttpPWD)
	req.Close = false

	asyncTasks := []string{"repDuplicate", "createCg"}

	var requestId string

	if sliceContains(asyncTasks, action) {

		reqLogger.V(2).Info(fmt.Sprintf("RunAction: action %s will be async", action))

		// generate a UUID for this async request and pass it in the header

		requestId = uuid.New().String()
		req.Header.Set("X-Request-Id", requestId)

		switch tt := instance.(type) {
		case *timestenv2.TimesTenClassic:

			if strings.ToUpper(caller) == strings.ToUpper("StandbyDownStandbyAS") {
				// only set asyncid if caller was StandbyDownStandbyAS, not initializeStandbyAS
				// for initializeStandbyAS, we're standing up TT for the first time and we're not going to try and recover
				// from operator restarts
				reqLogger.V(2).Info(fmt.Sprintf("RunAction: caller was %s, set Status.StandbyDownStandbyAS.AsyncId=%s", caller, requestId))
				tt.Status.StandbyDownStandbyAS.AsyncId = requestId
			}

			j, _ := json.Marshal(tt.Status.StandbyDownStandbyAS)
			reqLogger.V(1).Info(fmt.Sprintf("RunAction: tt.Status.StandbyDownStandbyAS=%s", string(j)))

			tt.Status.AsyncStatus.Id = requestId
			tt.Status.AsyncStatus.Type = action
			tt.Status.AsyncStatus.Caller = caller
			tt.Status.AsyncStatus.Host = ourDNSName
			tt.Status.AsyncStatus.PodName = podName

			reqLogger.V(1).Info(fmt.Sprintf("RunAction: tt.Status.AsyncStatus.Id=%v", requestId))

			// update the Status of this object in etcd
			// SAMDRAKE This is a latent bug; time can go backwards

			updateStatus(ctx, client, tt)

		}

	}

	// get our saved http client object (has persistent connection to pod)
	klient := getHttpClient(ctx, instance, ourDNSName, "POST", tts, timeoutOverride)

	resp, err := klient.Do(req)
	if err != nil {
		reqLogger.V(2).Info(err.Error())
		errmsg := err.Error()
		postRE := regexp.MustCompile(`Post ".*": (.*$)`)
		errs := postRE.FindStringSubmatch(err.Error())
		if len(errs) >= 2 {
			errmsg = errs[1]

			timeoutRE := regexp.MustCompile(`^context deadline exceeded.*$`)
			if timeoutRE.MatchString(errmsg) {
				errmsg = "agentPostTimeout exceeded"
			}
		}
		reqLogger.Error(err, "RunAction: Communicating with Agent returns error: "+errmsg)
		return errors.New(errmsg)
	}

	defer func() {
		resp.Body.Close()
		reqLogger.V(2).Info("RunAction returns")
	}()

	body, err := ioutil.ReadAll(resp.Body)

	e := *new(error)

	if sliceContains(asyncTasks, action) {

		reqLogger.V(2).Info(fmt.Sprintf("RunAction: %v is an async task", action))

		if resp.StatusCode == 200 {

			reqLogger.V(1).Info(fmt.Sprintf("RunAction: agent returned http 200, async not supported"))

			// the agent is running an older version that does not support async
			switch tt := instance.(type) {
			case *timestenv2.TimesTenClassic:
				tt.Status.AsyncStatus.Id = ""
				tt.Status.AsyncStatus.Type = action
				tt.Status.AsyncStatus.Host = ourDNSName
				tt.Status.AsyncStatus.PodName = podName
				tt.Status.AsyncStatus.Running = false
				tt.Status.AsyncStatus.Complete = true

				getPodStatus(ctx, instance, ourDNSName, isP, client, tts, nil)

				// SAMDRAKE This is a latent bug. Time can appear to go backwards.

				updateStatus(ctx, client, tt) // And update the Status of this object in etcd
			}

		} else if resp.StatusCode == 202 {

			reqLogger.V(1).Info(fmt.Sprintf("RunAction: agent accepted async task (http 202), begin polling requestId %v on %v",
				requestId, ourDNSName))

			if respId := resp.Header.Get("X-Request-Id"); respId != "" {

				switch tt := instance.(type) {
				case *timestenv2.TimesTenClassic:
					if respId != tt.Status.AsyncStatus.Id {
						reqLogger.V(2).Info("RunAction: warning, agent responded with unknown X-Request-Id=" + respId)
					} else {
						reqLogger.V(2).Info("RunAction: agent responded with matching X-Request-Id=" + respId)
					}
				}

			} else {
				reqLogger.Info("RunAction: warning, agent did not return an X-Request-Id")
			}

			switch tt := instance.(type) {
			case *timestenv2.TimesTenClassic:
				tt.Status.AsyncStatus.Running = true
				tt.Status.AsyncStatus.Complete = false
				getPodStatus(ctx, instance, ourDNSName, isP, client, tts, nil)
				// SAMDRAKE This is a latent bug; time can appear to go backwards
				updateStatus(ctx, client, tt)
			}

			asyncStatus, err := pollAsyncTask(ctx, instance, client, ourDNSName, ourUrl, requestId, tts)

			if err != nil {

				reqLogger.V(1).Info("RunAction: error from pollAsyncTask : " + err.Error())

				switch tt := instance.(type) {
				case *timestenv2.TimesTenClassic:

					reqLogger.V(2).Info(fmt.Sprintf("RunAction: cancelling async task %v", tt.Status.AsyncStatus.Id))

					connErr, _ := regexp.MatchString(`connection refused`, err.Error())

					if connErr == true {
						tt.Status.AsyncStatus.Errno = 11
						tt.Status.AsyncStatus.Errmsg = "agent connect failed"
						logTTEvent(ctx, client, instance, "TaskFailed", tt.Status.AsyncStatus.Errmsg, false)
					} else {
						if asyncStatus.Errno == nil {
							tt.Status.AsyncStatus.Errno = 10
						} else {
							tt.Status.AsyncStatus.Errno = *asyncStatus.Errno
						}
						if asyncStatus.Errmsg == nil {
							tt.Status.AsyncStatus.Errmsg = "task timed out"
							eventMsg := fmt.Sprintf("Async task %s timed out on %s", tt.Status.AsyncStatus.Type, ourDNSName)
							logTTEvent(ctx, client, instance, "TaskFailed", eventMsg, false)
						} else {
							tt.Status.AsyncStatus.Errmsg = *asyncStatus.Errmsg
							eventMsg := fmt.Sprintf("Async task %s failed on %s: Error %d: %s", tt.Status.AsyncStatus.Type, ourDNSName, tt.Status.AsyncStatus.Errno, tt.Status.AsyncStatus.Errmsg)
							logTTEvent(ctx, client, instance, "TaskFailed", eventMsg, false)
						}
					}

					tt.Status.AsyncStatus.Running = false
					tt.Status.AsyncStatus.Complete = true

					// update the status obj
					getPodStatus(ctx, instance, ourDNSName, isP, client, tts, nil)
					// SAMDRAKE This is a latent bug; time can appear to go backwards
					updateStatus(ctx, client, tt)

					// kill the agent

					_, kErr := killAgent(ctx, client, instance, ourDNSName, podName, tts, timeoutOverride)
					if kErr != nil {
						reqLogger.V(2).Info(fmt.Sprintf("RunAction: error returned from killAgent : %v", kErr))
					}
				}
				return err
			}

			if asyncStatus.Complete == true {
				switch tt := instance.(type) {
				case *timestenv2.TimesTenClassic:
					tt.Status.AsyncStatus.Running = false
					tt.Status.AsyncStatus.Complete = true
					reqLogger.V(1).Info(fmt.Sprintf("RunAction: aysnc task %s complete", asyncStatus.Type))

					asyncStatusStr, _ := json.Marshal(asyncStatus)
					reqLogger.V(2).Info(fmt.Sprintf("RunAction: pollAsyncTask returned : %s", string(asyncStatusStr)))
					var asyncTaskOut AsyncTask
					err = json.Unmarshal(asyncStatusStr, &asyncTaskOut)
					if err != nil {
						return errors.New("error unmarshalling async status")
					}
					reqLogger.V(2).Info(fmt.Sprintf("RunAction: asyncTaskOut : %v", asyncTaskOut))

					switch action {
					case "createCg":
						if asyncTaskOut.AgentCreateCgReply.CgErr != nil {
							reqLogger.V(1).Info(fmt.Sprintf("RunAction: asyncTaskOut.AgentCreateCgReply.CgErr=%v", asyncTaskOut.AgentCreateCgReply.CgErr))
							logTTEventsFromStderr(ctx, client, instance, "Cachegroups", asyncTaskOut.AgentCreateCgReply.CgErr)
						}
					case "repDuplicate":
						if len(asyncTaskOut.AgentDuplicateReply.DupErr) > 0 {
							reqLogger.V(1).Info(fmt.Sprintf("RunAction: asyncTaskOut.AgentDuplicateReply.DupErr=%v", asyncTaskOut.AgentDuplicateReply.DupErr))
							logTTEventsFromStderr(ctx, client, instance, "Repduplicate", &asyncTaskOut.AgentDuplicateReply.DupErr)
						}
					}
				}
			}

			if asyncStatus.Errno != nil {
				reqLogger.Info(fmt.Sprintf("RunAction: async task %s failed with errno=%v", asyncStatus.Type, *asyncStatus.Errno))

				switch tt := instance.(type) {
				case *timestenv2.TimesTenClassic:
					tt.Status.AsyncStatus.Errno = *asyncStatus.Errno
				}

				if *asyncStatus.Errmsg != "" {
					switch tt := instance.(type) {
					case *timestenv2.TimesTenClassic:
						tt.Status.AsyncStatus.Errmsg = *asyncStatus.Errmsg
					}
					e = errors.New(*asyncStatus.Errmsg)
				} else {
					e = errors.New(fmt.Sprintf("aysnc task %s failed with errno=%v", asyncStatus.Type, *asyncStatus.Errno))
				}
			}

		} else {

			// we didn't get HTTP 202 (ACCEPTED)

			switch tt := instance.(type) {
			case *timestenv2.TimesTenClassic:
				tt.Status.AsyncStatus.Running = false
				tt.Status.AsyncStatus.Complete = true
				tt.Status.AsyncStatus.Errno = 11
				tt.Status.AsyncStatus.Errmsg = fmt.Sprintf("agent returned http %d, expecting 202", resp.StatusCode)
				getPodStatus(ctx, instance, ourDNSName, isP, client, tts, nil)
				// SAMDRAKE This is a latent bug. Time can appear to go backwards
				updateStatus(ctx, client, tt)
			}
			return errors.New(fmt.Sprintf("agent rejected task, http code %d", resp.StatusCode))
		}

	} else {

		// NOT AN ASYNC TASK

		// seems like debugging
		if action != "runGridAdmin" {
			reqLogger.Info("RunAction: response=" + Redact(string(body))) // Parsed by customer, leave info not debug
		}

		var agentPostReply TTAgentPostReply
		var agentPostGenericReply TTAgentGenericReply
		var agentCreateDbReply TTAgentCreateDbReply
		var agentCreateCgReply TTAgentCreateCgReply
		var agentRepAdminWaitReply TTAgentRepAdminWaitReply
		var agentOpenDbReply TTAgentOpenDbReply
		var agentCloseDbReply TTAgentCloseDbReply

		humanReadableAction := ""

		p := json.Unmarshal(body, &agentPostReply)
		if p != nil {
			reqLogger.V(1).Info("RunAction: Error parsing reply from agent: " + p.Error())
		}

		if agentPostReply.JsonVer == nil || *agentPostReply.JsonVer != jsonVer {
			errmsg := "RunAction: Error parsing reply from agent: jsonVer not valid"
			reqLogger.V(1).Info(errmsg)
			e = errors.New(errmsg)
		} else {
			if agentPostReply.Errno == nil {
				errmsg := "RunAction: Error parsing reply from agent; Errno not specified"
				reqLogger.V(1).Info(errmsg)
				e = errors.New(errmsg)
			} else {
				// Now that we know we got a semi-valid response from the agent
				// Let's parse the details that it returned to us into an action-specific struct

				if agentPostReply.Errno != nil && *agentPostReply.Errno != 0 {
					reqLogger.V(1).Info(fmt.Sprintf("RunAction: agentPostReply.Errno=%v", *agentPostReply.Errno))
				}

				if agentPostReply.Errmsg != nil {
					reqLogger.V(1).Info(fmt.Sprintf("RunAction: agentPostReply.Errmsg=%v", *agentPostReply.Errmsg))
				}

				switch action {
				case "createDb":
					humanReadableAction = "creating db"
					ee := json.Unmarshal(body, &agentCreateDbReply)
					if ee != nil {
						reqLogger.V(1).Info("RunAction: Error parsing createDb reply from agent: " + ee.Error())
					} else {
						if agentCreateDbReply.Create1Err != nil {
							logTTEventsFromStderr(ctx, client, instance, "CreateDb", &agentCreateDbReply.Create1Err)
						}
						if agentCreateDbReply.Create2Err != nil {
							logTTEventsFromStderr(ctx, client, instance, "Cache UID Set", &agentCreateDbReply.Create2Err)
						}
						if agentCreateDbReply.SchemaErr != nil {
							logTTEventsFromStderr(ctx, client, instance, "Schema", &agentCreateDbReply.SchemaErr)
						}
						if agentCreateDbReply.Errors != nil {
							logTTEventsFromErrors(ctx, client, instance, &agentCreateDbReply.Errors)
						}
						if agentCreateDbReply.Errno != nil && *agentCreateDbReply.Errno != 0 {
							if agentCreateDbReply.Errmsg != nil {
								logTTEvent(ctx, client, instance, "Error", "Error creating database: "+*agentCreateDbReply.Errmsg, true)
							}
						}
						if len(agentCreateDbReply.DebugMsgs) > 0 {
							for _, x := range agentCreateDbReply.DebugMsgs {
								reqLogger.V(1).Info("DEBUG: " + x)
							}
						}
					}

				case "createCg":
					humanReadableAction = "creating cache groups"
					ee := json.Unmarshal(body, &agentCreateCgReply)
					reqLogger.V(1).Info(fmt.Sprintf("RunAction: agentCreateCgReply=%v", agentCreateCgReply))
					if ee != nil {
						reqLogger.V(1).Info("RunAction: Error parsing createCg reply from agent: " + ee.Error())
					} else {
						if agentCreateCgReply.CgErr != nil {
							reqLogger.V(1).Info(fmt.Sprintf("RunAction: agentCreateCgReply.CgErr=%v", agentCreateCgReply.CgErr))
							logTTEventsFromStderr(ctx, client, instance, "Cachegroups", agentCreateCgReply.CgErr)
						}
					}

				case "createRepEpilog":
					humanReadableAction = "running epilog.sql"
					ee := json.Unmarshal(body, &agentPostGenericReply)
					if ee != nil {
						reqLogger.V(1).Info(us + ": Error parsing createRepEpilog reply from agent: " + ee.Error())
					} else {
						if agentPostGenericReply.Stdout != nil && len(*agentPostGenericReply.Stdout) > 0 {
							reqLogger.V(1).Info(fmt.Sprintf("%s: createRepEpilog agentPostGenericReply.stdout=%v", us, *agentPostGenericReply.Stdout))
						}
						if agentPostGenericReply.Stderr != nil && len(*agentPostGenericReply.Stderr) > 0 {
							reqLogger.V(1).Info(fmt.Sprintf("%s: createRepEpilog agentPostGenericReply.stderr=%v", us, *agentPostGenericReply.Stderr))
							logTTEventsFromStderr(ctx, client, instance, "CreateRepEpilog", agentPostGenericReply.Stderr)
						}
					}

				case "closeDb":
					humanReadableAction = "closing db"
					ee := json.Unmarshal(body, &agentCloseDbReply)

					reqLogger.V(1).Info(fmt.Sprintf("%s: closeDb agentCloseDbReply=%v", us, agentCloseDbReply))

					if ee != nil {
						reqLogger.V(1).Info(us + ": Error parsing closeDb reply from agent: " + ee.Error())
					} else {
						if agentCloseDbReply.CmdRc != nil {
							logMsg := []string{agentCloseDbReply.Msg}
							logTTEventsFromStderr(ctx, client, instance, "CloseDb", &logMsg)
						}
					}

				case "openDb":
					humanReadableAction = "opening db"
					ee := json.Unmarshal(body, &agentOpenDbReply)

					reqLogger.V(1).Info(fmt.Sprintf("%s: closeDb agentOpenDbReply=%v", us, agentOpenDbReply))

					if ee != nil {
						reqLogger.V(1).Info(us + ": Error parsing openDb reply from agent: " + ee.Error())
					} else {
						if agentOpenDbReply.CmdRc != nil {
							logMsg := []string{agentOpenDbReply.Msg}
							logTTEventsFromStderr(ctx, client, instance, "OpenDb", &logMsg)
						}
					}

				case "doRepAdminWait":
					humanReadableAction = "calling repAdminWait"
					ee := json.Unmarshal(body, &agentRepAdminWaitReply)
					if ee != nil {
						reqLogger.V(1).Info(us + ": Error parsing doRepAdminWait reply from agent: " + ee.Error())
					} else {
						if agentRepAdminWaitReply.Stdout != nil && len(*agentRepAdminWaitReply.Stdout) > 0 {
							reqLogger.V(1).Info(fmt.Sprintf("%s: doRepAdminWait agentRepAdminWaitReply.stdout=%v", us, *agentRepAdminWaitReply.Stdout))
						}
						if agentRepAdminWaitReply.Stderr != nil && len(*agentRepAdminWaitReply.Stderr) > 0 {
							reqLogger.V(1).Info(fmt.Sprintf("%s: doRepAdminWait agentRepAdminWaitReply.stderr=%v", us, *agentRepAdminWaitReply.Stderr))
							logTTEventsFromStderr(ctx, client, instance, "doRepAdminWait", agentRepAdminWaitReply.Stderr)
						}

						if agentRepAdminWaitReply.Errno != nil {
							reqLogger.V(1).Info(fmt.Sprintf("%s: doRepAdminWait agentRepAdminWaitReply.Errno=%v", us, *agentRepAdminWaitReply.Errno))
							if *agentRepAdminWaitReply.Errno == 9 {
								reqLogger.V(1).Info(fmt.Sprintf("%s: doRepAdminWait : replication catching up", us))
							} else if *agentRepAdminWaitReply.Errno == 1 {
								reqLogger.V(1).Info(fmt.Sprintf("%s: doRepAdminWait : unknown response from ttRepAdmin", us))
							}
						}
					}

				case "createRepScheme":
					humanReadableAction = "creating replication scheme"
					ee := json.Unmarshal(body, &agentPostGenericReply)
					if ee != nil {
						reqLogger.V(1).Info(us + ": Error parsing createRepScheme reply from agent: " + ee.Error())
					} else {
						if agentPostGenericReply.Errmsg != nil {
							reqLogger.V(1).Info(fmt.Sprintf("%s: createRepScheme error: %s", us, *agentPostGenericReply.Errmsg))
						} else {
							if agentPostGenericReply.Stderr != nil && len(*agentPostGenericReply.Stderr) > 0 {
								reqLogger.V(1).Info(fmt.Sprintf("%s: createRepScheme agentPostReply.stderr=%v", us, *agentPostGenericReply.Stderr))

								logTTEventsFromStderr(ctx, client, instance, "CreateRepScheme", agentPostGenericReply.Stderr)
							}
						}
						if agentPostGenericReply.Stdout != nil && len(*agentPostGenericReply.Stdout) > 0 {
							reqLogger.V(1).Info(fmt.Sprintf("%s: createRepScheme agentPostReply.stdout=%v", us, *agentPostGenericReply.Stdout))
						}
					}

				default:
					humanReadableAction = "Running " + action
				}

				// dump the json to the log
				agentPostReplyStr, _ := json.Marshal(agentPostReply)
				reqLogger.V(2).Info(fmt.Sprintf(us+": agentPostReply : %s", string(agentPostReplyStr)))

				if *agentPostReply.Errno == 0 {
					e = nil
				} else {
					if agentPostReply.Errmsg == nil {
						e = errors.New(fmt.Sprintf("Error %d %s: NO ERRMSG RETURNED", *agentPostReply.Errno, humanReadableAction))
					} else {
						e = errors.New(fmt.Sprintf("Error %d %s: %s", *agentPostReply.Errno, humanReadableAction, *agentPostReply.Errmsg))
					}
				}
			}
		}

	} // not async

	// After running the action re-fetch the status again
	// Usually

	switch tt := instance.(type) {
	case *timestenv2.TimesTenClassic:
		if fetchStatusAgain {
			getPodStatus(ctx, instance, ourDNSName, isP, client, tts, nil)
		}
		// SAMDRAKE This is a latent bug.
		updateStatus(ctx, client, tt)
	default:
		reqLogger.Error(errors.New(us+": Unknown type "+fmt.Sprintf("%T", tt)), "Could not get pod status")
	}

	return e
}

// Ask the Agent to execute an action on our behalf by sending a POST request
// Only useful in Classic
func RunAction(ctx context.Context, instance *timestenv2.TimesTenClassic, podNo int, action string, params interface{}, client client.Client, tts *TTSecretInfo, timeoutOverride *int) error {
	us := "RunAction"
	podName := instance.Status.PodStatus[podNo].Name
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info(fmt.Sprintf("%s: calling action %s on %s", us, action, podName))
	defer reqLogger.V(2).Info(us + " returns")

	caller := "unknown"
	pc, _, _, ok := runtime.Caller(1)
	calledBy := runtime.FuncForPC(pc)
	if ok && calledBy != nil {
		caller = stringAfter(calledBy.Name(), "controllers.")
		if caller == "" {
			caller = calledBy.Name()
		}
		reqLogger.V(1).Info(fmt.Sprintf("%s: called by %s", us, caller))
	}

	err := RunActionG(ctx, instance, &instance.Status.PodStatus[podNo], action, params, client, tts, timeoutOverride, caller)
	return err
}

// check whether the non-replicated database is quiescing
func checkNonRepQuiesce(ctx context.Context, instance *timestenv2.TimesTenClassic, podNo int, client client.Client, tts *TTSecretInfo) (error, string) {
	us := "checkNonRepQuiesce"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info(us + " entered")
	defer reqLogger.V(1).Info(us + " returns")

	podName := instance.Status.PodStatus[podNo].Name
	ourDNSName := podName + "." + instance.ObjectName() + "." + instance.ObjectNamespace() + ".svc.cluster.local"

	ourUrl := "https://" + ourDNSName + ":8443/agent"
	reqLogger.V(2).Info(us + " target url is " + ourUrl)

	v := url.Values{}
	v.Set("verb", "checkNonRepQuiesce")
	v.Set("ourDNSName", ourDNSName)
	v.Set("objectName", instance.ObjectName())
	v.Set("objectNamespace", instance.ObjectNamespace())

	v.Set("zzAgentDebugInfo", strconv.Itoa(instance.GetAgentDebugInfo()))
	if ti := instance.GetAgentTestInfo(); ti != nil {
		v.Set("zzTestInfo", *ti)
	}

	req, err := http.NewRequest("POST", ourUrl, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")
	req.SetBasicAuth(tts.HttpUID, tts.HttpPWD)
	req.Close = false

	// get our saved http client object (has persistent connection to pod)
	klient := getHttpClient(ctx, instance, ourDNSName, "POST", tts, nil)

	resp, err := klient.Do(req)
	if err != nil {
		reqLogger.Error(err, us+": Communicating with Agent returns error: "+err.Error())
		return err, "Unknown"
	}

	defer func() {
		resp.Body.Close()
		reqLogger.V(2).Info(us + " returns")
	}()

	body, err := ioutil.ReadAll(resp.Body)

	var reply TTAgentDoCheckQuiesce

	ee := json.Unmarshal(body, &reply)
	if ee != nil {
		reqLogger.V(1).Info(us + ": Error parsing reply from agent: " + ee.Error())
		return ee, "Unknown"
	}

	// Now that we know we got a semi-valid response from the agent
	// Let's parse the details that it returned to us into an action-specific struct

	if reply.Errno != nil && *reply.Errno != 0 {
		msg := fmt.Sprintf("%s: errno=%v", us, *reply.Errno)
		reqLogger.V(1).Info(msg)
		return errors.New(msg), "unknown"
	}

	if reply.Errmsg != nil {
		msg := fmt.Sprintf("%s: errmsg=%v", us, *reply.Errmsg)
		reqLogger.V(1).Info(msg)
		return errors.New(msg), "unknown"
	}

	if *reply.Quiescing == true {
		msg := fmt.Sprintf("%s: quiescing", us)
		reqLogger.V(1).Info(msg)
		return nil, "quiescing"
	}

	return nil, "notQuiescing"
}

// initiate polling of an agent running an async task
func pollAsyncTask(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client, ourDNSName string, ourUrl string, requestId string, tts *TTSecretInfo) (*AsyncTask, error) {

	us := "pollAsyncTask"

	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " called")

	// TODO: async tasks are generally going to be long[er]-running tasks, is the default (agentPostTimeout) secs adequate?
	timeout := instance.GetAgentAsyncTimeout()

	reqLogger.V(2).Info(fmt.Sprintf("%s: async task timeout=%v secs", us, timeout))

	ctxTimer, cancelFunc := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)

	defer func() {
		// cancelFunc() allows you to cancel the poll before a timeout
		// in this case, just defer cancel it

		cancelFunc()
		reqLogger.V(2).Info(us + " exits")
	}()

	// poll every 3 seconds
	var pollEverySecs time.Duration
	pollEverySecs = 3

	status, err := pollAsyncStatus(instance, client, ctxTimer, requestId, pollEverySecs*time.Second, ourDNSName, ourUrl, tts, reqLogger)

	return status, err

}

// pollAsyncStatus will keep 'pinging' the status API until timeout is reached or status returned is complete
func pollAsyncStatus(instance timestenv2.TimesTenObject, client client.Client, ctx context.Context, requestId string, pollInterval time.Duration, ourDNSName string, ourUrl string, tts *TTSecretInfo, reqLogger logr.Logger) (*AsyncTask, error) {

	us := "pollAsyncStatus"

	reqLogger.Info(fmt.Sprintf(us+": called with async requestId=%v", requestId))

	taskType := "unknown"

	ticker := time.NewTicker(pollInterval)

	defer func() {
		ticker.Stop()
		reqLogger.V(2).Info(us + " exits")
	}()

	tickerCounter := 0

	for {
		select {
		case <-ctx.Done():
			// task timed out
			errMsg := fmt.Sprintf("Async task %s timed out on %s", taskType, ourDNSName)
			return nil, errors.New(errMsg)

		case tick := <-ticker.C:
			reqLogger.V(2).Info(fmt.Sprintf(us+": polled at %v", tick))
			status, err := getAsyncStatus(ctx, instance, ourDNSName, tts, requestId)
			//reqLogger.V(1).Info(fmt.Sprintf(us + ": getAsyncStatus returned %v", status))
			if err != nil {
				reqLogger.Info(fmt.Sprintf(us + ": getAsyncStatus returned an error, exiting"))
				return nil, err
			}

			tickerCounter++

			// dump the json to the log
			asyncStatusStr, _ := json.Marshal(status)
			reqLogger.V(2).Info(fmt.Sprintf(us+": getAsyncStatus returned : %s", string(asyncStatusStr)))

			if status.Type != "" {
				taskType = status.Type
			}

			if status.Errno != nil {
				errMsg := fmt.Sprintf("getAsyncStatus returned errno %v", *status.Errno)
				reqLogger.V(2).Info(us + ": " + errMsg)
				err := errors.New(errMsg)
				logTTEvent(ctx, client, instance, "TaskFailed", errMsg, false)

				switch status.Type {
				case "RepDuplicate":
					for _, err := range status.AgentDuplicateReply.DupErr {
						logTTEvent(ctx, client, instance, "Error", err, true)
					}

				case "CreateCg":
					for _, err := range *status.AgentCreateCgReply.CgErr {
						logTTEvent(ctx, client, instance, "Error", err, true)
					}

				default:
				}

				return &status, err
			}

			if status.Running == true {
				// still processing, do nothing

				if status.Started != nil {
					asyncTaskElapsed := time.Now().Unix() - *status.Started
					asyncTaskTimeRemaining := int64(instance.GetAgentAsyncTimeout()) - asyncTaskElapsed
					reqLogger.V(1).Info(fmt.Sprintf("%s: %v has been running task %s for %v secs, timeout in %v secs",
						us, requestId, taskType, asyncTaskElapsed, asyncTaskTimeRemaining))

					_, ok := os.LookupEnv("TT_DEBUG")
					if ok {
						logMsg := fmt.Sprintf("%s: Async polling for %s, timeout in %v secs", us, taskType, asyncTaskTimeRemaining)
						logTTEvent(ctx, client, instance, "Info", logMsg, false)
					}

				} else {
					reqLogger.V(1).Info(fmt.Sprintf("%s: %v is running %s on %v", us, requestId, taskType, ourDNSName))
				}

			}

			if status.Complete == true {
				//reqLogger.Info(fmt.Sprintf("%s: %s task complete on %s", us, taskType, ourDNSName))

				if status.Started != nil && status.Ended != nil {
					asyncTaskTime := *status.Ended - *status.Started
					reqLogger.Info(fmt.Sprintf("%s: %v on %v completed task %s in %v secs",
						us, requestId, ourDNSName, taskType, asyncTaskTime))
					_, ok := os.LookupEnv("TT_DEBUG")
					if ok {
						logTTEvent(ctx, client, instance, "Info", fmt.Sprintf("%s completed in %v secs",
							taskType, asyncTaskTime), false)
					}

				} else {
					reqLogger.Info(fmt.Sprintf("%s: %v on %v has completed", us, requestId, ourDNSName))
				}

				return &status, nil
			}

		}
	}
	//return nil, errors.New("error: unable to get status of async request")
}

// calls GET on agent to retrieve async task status
func getAsyncStatus(ctx context.Context, instance timestenv2.TimesTenObject, ourDNSName string, tts *TTSecretInfo, requestId string) (AsyncTask, error) {
	us := "getAsyncStatus"
	reqLogger := log.FromContext(ctx)

	var asyncTaskOut AsyncTask

	statusUrl := "https://" + ourDNSName + ":8443/agent/status"

	req, err := http.NewRequest("GET", statusUrl, nil)
	req.Header.Set("X-Request-Id", requestId)
	req.SetBasicAuth(tts.HttpUID, tts.HttpPWD)
	req.Close = false

	// get our saved http client object (has persistent connection to pod)
	klient := getHttpClient(ctx, instance, ourDNSName, "GET", tts, nil)
	resp, err := klient.Do(req)
	if err != nil {
		reqLogger.Info(us + " : client request failed, returning error")
		return asyncTaskOut, err
	}

	// close response body on exit; otherwise connection will not persist
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &asyncTaskOut)

	if err != nil {
		reqLogger.Info(us + ": failed to read json data")
		return asyncTaskOut, err
	}

	//reqLogger.V(2).Info(fmt.Sprintf("%s: %s response was : %v", us, ourDNSName,string(body)))
	//reqLogger.V(2).Info(fmt.Sprintf( us + ": asyncTaskOut : %v", asyncTaskOut))

	if asyncTaskOut.Id == "" {

		reqLogger.V(2).Info(fmt.Sprintf("%v: unknown requestId %v on %v", us, requestId, ourDNSName))
		return asyncTaskOut, nil

	} else {

		if requestId == asyncTaskOut.Id {

			////reqLogger.V(2).Info(fmt.Sprintf("%v: requestId %v matched on %v ", us, requestId, ourDNSName))
			//if asyncTaskOut.Running == true {
			//    // we don't update the updated field yet; in the future, this could provide task progress
			//    //if asyncTaskOut.Updated != nil {
			//    //    asyncTaskLastUpdated := time.Now().Unix() - *asyncTaskOut.Updated
			//    //    reqLogger.V(2).Info(fmt.Sprintf(us + ": async task was last updated %v secs ago", asyncTaskLastUpdated))
			//    //}
			//}

		} else {

			errMsg := fmt.Sprintf("requestId %v unknown on %v", requestId, ourDNSName)
			err := errors.New(us + ": " + errMsg)
			reqLogger.Error(err, us+": "+errMsg)
			asyncTaskOut.Errno = newInt(99)

			return asyncTaskOut, err
		}

	}

	return asyncTaskOut, nil

}

// Fetch the TimesTen status from the agent URL
func getTTAgentOut(ctx context.Context,
	instance timestenv2.TimesTenObject,
	ttcuid string, podDNSName string, podIP string,
	client client.Client, tts *TTSecretInfo, reqParams map[string]string) (*TTAgentOut, error, string) {
	us := "getTTAgentOut"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " called with podDNSName=" + podDNSName + ", ttcuid=" + ttcuid)
	defer reqLogger.V(2).Info(us + " ends")

	v := url.Values{}
	v.Set("ourDNSName", podDNSName)
	v.Set("ttObjectName", instance.ObjectName())
	if instance.GetAgentTestInfo() != nil {
		v.Set("zzTestInfo", *instance.GetAgentTestInfo())
	}

	for param, val := range reqParams {
		v.Set(param, val)
		reqLogger.V(1).Info(fmt.Sprintf("%s: set passed param %s=%s", us, param, val))
	}

	req, err := http.NewRequest("GET", "https://"+podDNSName+":8443/agent?"+v.Encode(), nil)
	req.SetBasicAuth(tts.HttpUID, tts.HttpPWD)
	req.Close = false

	// get our saved http client object (has persistent connection to pod)
	klient := getHttpClient(ctx, instance, podDNSName, "GET", tts, nil)
	resp, err := klient.Do(req)
	if err != nil {
		return nil, err, ""
	}

	// close response body on exit; otherwise connection will not persist
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	var ttAgentOut TTAgentOut
	err = json.Unmarshal(body, &ttAgentOut)

	reqLogger.V(1).Info(fmt.Sprintf("%s: response from %s was %s", us, podDNSName, string(body)))

	if err != nil {
		return nil, err, string(body)
	} else {
		return &ttAgentOut, nil, string(body)
	}
}

func getPodStatus(ctx context.Context, instance timestenv2.TimesTenObject, ourDNSName string, isP *timestenv2.TimesTenPodStatus, client client.Client, tts *TTSecretInfo, reqParams map[string]string) {
	us := "getPodStatus"
	reqLogger := log.FromContext(ctx)
	switch tt := instance.(type) {
	case *timestenv2.TimesTenClassic:
		getClassicPodStatus(ctx, *tt, ourDNSName, isP, client, tts, reqParams)
	default:
		err := errors.New(us + ": Unknown type")
		reqLogger.Error(err, "Unknown type "+fmt.Sprintf("%T", instance)+" in "+us)
	}
}

func getClassicPodStatus(ctx context.Context, instance timestenv2.TimesTenClassic, ourDNSName string, isP *timestenv2.TimesTenPodStatus, client client.Client, tts *TTSecretInfo, reqParams map[string]string) {
	us := "getClassicPodStatus"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered for pod " + ourDNSName)
	defer reqLogger.V(2).Info(us + " returns")

	ttcuid := string(instance.ObjectMeta.UID)

	isP.DbStatus.DbId = 0 // Will overwrite with actual data below if we got any

	prevPodStatus := *isP // Save the 'before' state of the pod
	defer reportChangesInPodState(ctx, isP.Name, &prevPodStatus, isP, instance.GetHighLevelState(), client, instance)

	reqLogger.V(1).Info(us + ": calling getTTAgentOut() for ourDNSName=" + ourDNSName)

	ttAgentOut, err, rawOut := getTTAgentOut(ctx, &instance, ttcuid, ourDNSName, isP.PodStatus.PodIP, client, tts, reqParams)

	isP.ScaleoutStatus.InstanceType = "classic"

	// If we couldn't get a response at all, then we don't know what's going on

	gotValidReply := true

	if err != nil {
		gotValidReply = false
		reqLogger.V(2).Info(fmt.Sprintf(us+": getTTAgentOut returned : err=%v", err))
		reqLogger.Error(err, "Could not fetch ttAgentOut: "+err.Error())
	}

	if ttAgentOut == nil {
		gotValidReply = false
	} else {
		printStderr := false
		switch ttAgentOut.Errno {
		case 999: // No valid reply though we may have received something
			gotValidReply = false
			printStderr = true
		case 833: // TimesTen native error code - no database found
			gotValidReply = true
		default:
		}

		if printStderr && len(ttAgentOut.Stderr) > 0 {
			for _, l := range ttAgentOut.Stderr {
				if len(l) > 0 {
					logTTEvent(ctx, client, instance, "Warning", fmt.Sprintf("Error from %s: %s", ourDNSName, l), true)
				}
			}
		}
	}

	if gotValidReply == false {
		reqLogger.V(1).Info(fmt.Sprintf("got no valid reply; got %s", rawOut))
		isP.PodStatus.Agent = "Unknown"
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
		return
	}

	isP.PrevCGroupInfo = isP.CGroupInfo
	isP.CGroupInfo = ttAgentOut.CGroupInfo

	isP.PodStatus.Agent = "Up"

	isP.Quiescing = ttAgentOut.Quiescing
	isP.NonRepUpgradeFailed = ttAgentOut.NonRepUpgradeFailed

	if ttAgentOut.InstallRelease != "" {
		isP.InstallRelease = ttAgentOut.InstallRelease
	}

	if ttAgentOut.ImageRelease != "" {
		isP.ImageRelease = ttAgentOut.ImageRelease
	}

	if ttAgentOut.ClockSync != nil {
		if *ttAgentOut.ClockSync == false {
			reqLogger.Info(us + ": clock sync error detected for pod " + ourDNSName)
			// maybe we don't want to be overrun with clock sync events
			val, ok := os.LookupEnv("TT_DISABLE_CLOCK_EVENTS")
			if !ok {
				if val != "1" {
					logTTEvent(ctx, client, instance, "Info", "Clock sync error detected for pod "+ourDNSName, true)
				}
			}
		}
	} else {
		reqLogger.Info(us + ": agent did not return clock sync status for pod " + ourDNSName)
	}

	if ttAgentOut.InstanceExists != nil {
		if *ttAgentOut.InstanceExists == true {
			isP.TimesTenStatus.Instance = "Exists"
		} else {
			isP.TimesTenStatus.Instance = "Missing"
		}
	} else {
		isP.TimesTenStatus.Instance = "Unknown"
	}

	isP.TimesTenStatus.Release = ttAgentOut.InstallRelease

	isP.AdminUserFile = ttAgentOut.AdminUserFile
	isP.SchemaFile = ttAgentOut.SchemaFile
	isP.CacheGroupsFile = ttAgentOut.CacheGroupsFile
	isP.CacheUserFile = ttAgentOut.CacheUserFile

	if ttAgentOut.DaemonUp {
		isP.TimesTenStatus.Daemon = "Up"

		// Could we connect to the database? If we got a transient connection error
		// then the database status may be 'Unknown'
		// TODO: There may be other TimesTen error codes that should be here.

		if ttAgentOut.Errno == 9996 {
			// Couldn't connect due to process recovery in process (TT9996)
			reqLogger.V(2).Info(fmt.Sprintf(us+": Agent unable to connect to database due to transient error TT%d", ttAgentOut.Errno))
			_, ok := os.LookupEnv("TT_DEBUG")
			if ok {
				logTTEvent(ctx, client, instance, "Info", fmt.Sprintf(us+": Agent unable to connect to database due to transient error TT%d", ttAgentOut.Errno), true)
			}
			isP.DbStatus.Db = "Unknown"
			isP.DbStatus.DbUpdatable = "Unknown"
			isP.ReplicationStatus.RepAgent = "Unknown"
			isP.ReplicationStatus.RepState = "Unknown"
			isP.ReplicationStatus.RepScheme = "Unknown"
			isP.ReplicationStatus.RepPeerPState = "Unknown"
			isP.ReplicationStatus.RepPeerPStateFetchErr = "Unknown"
			isP.CacheStatus.CacheAgent = "Unknown"
			isP.CacheStatus.AwtBehindMb = nil
			return
		}

		// Presumably we DID connect to the database, so the returned status is meaningful

		if ttAgentOut.Configuration != nil {
			isP.DbStatus.DbConfiguration = *ttAgentOut.Configuration
		}

		if ttAgentOut.Monitor != nil {
			isP.DbStatus.Monitor = *ttAgentOut.Monitor
		}

		if ttAgentOut.SystemStats != nil {
			isP.DbStatus.SystemStats = *ttAgentOut.SystemStats
		}

		// This is an error exception captured by 'get1.sql' during its execution
		// We should probably do more with it?
		if ttAgentOut.DbError != "" {
			reqLogger.Info(fmt.Sprintf(us+": ttAgent reports dbError=%v", ttAgentOut.DbError))
		}

		if ttAgentOut.UpdateErr != "" {
			// TT16265: This database is currently the STANDBY.  Change to TTAGENT.S not permitted
			// this error is expected, the agent intentionally tries to update the database on a GET,
			// so it can report back to the operator whether the database is writable or not.  You’ll get
			// that on the standby as it discovers that it is not.
			TT16265 := regexp.MustCompile("TT16265")
			if TT16265.FindString(ttAgentOut.UpdateErr) != "" {
				reqLogger.V(2).Info(fmt.Sprintf(us+": ttAgent reports UpdateErr=%v", ttAgentOut.UpdateErr))
			}
		}

		if len(ttAgentOut.TTStatus.DbInfo) == 0 {
			isP.DbStatus.Db = "None"
		} else {
			if ttAgentOut.TTStatus.DbInfo[0].Loading != nil {
				isP.DbStatus.Db = "Loading"
			} else if ttAgentOut.TTStatus.DbInfo[0].Unloading != nil {
				isP.DbStatus.Db = "Unloading"
			} else {
				if ttAgentOut.TTStatus.DbInfo[0].RamPolicyNote != nil &&
					*ttAgentOut.TTStatus.DbInfo[0].RamPolicyNote == "Data store should be manually loaded but inactive due to failures" {
					isP.DbStatus.Db = "Unloaded"
				} else {
					if ((ttAgentOut.TTStatus.DbInfo[0].ObsoleteInfo == nil ||
						len(*ttAgentOut.TTStatus.DbInfo[0].ObsoleteInfo) == 0) &&
						ttAgentOut.TTStatus.DbInfo[0].NConnections == 0) ||
						ttAgentOut.Errno == 707 {
						isP.DbStatus.Db = "Unloaded"
					} else if ttAgentOut.TTStatus.DbInfo[0].ObsoleteInfo != nil {
						isP.DbStatus.Db = "Transitioning"
					} else {
						if ttAgentOut.TTStatus.DbInfo[0].NConnections > 0 {
							isP.DbStatus.Db = "Loaded"

							isP.DbStatus.DbId = ttAgentOut.NewId

							isP.RepStats = ttAgentOut.RepStats

							if ttAgentOut.TTStatus.DbInfo[0].Open == "yes" {
								isP.DbStatus.DbOpen = true
							} else {
								isP.DbStatus.DbOpen = false
							}

							// Is the database updatable?

							if ttAgentOut.Updatable {
								isP.DbStatus.DbUpdatable = "Yes"
							} else {
								isP.DbStatus.DbUpdatable = "No"
							}

							if ttAgentOut.UsingTwosafe == nil {
								isP.UsingTwosafe = nil
							} else {
								isP.UsingTwosafe = newBool(*ttAgentOut.UsingTwosafe)
							}

							if ttAgentOut.DisableReturn == nil {
								isP.DisableReturn = nil
							} else {
								isP.DisableReturn = newBool(*ttAgentOut.DisableReturn)
							}

							if ttAgentOut.LocalCommit == nil {
								isP.LocalCommit = nil
							} else {
								isP.LocalCommit = newBool(*ttAgentOut.LocalCommit)
							}

							if ttAgentOut.AwtBehindMb == nil {
								isP.CacheStatus.AwtBehindMb = nil
							} else {
								// NOTE that awtBehindMb is defined as an INT, and we can't really change (fix) it
								// due to CRD Versioning. But get1.sql returns this as a FLOAT. So we have to
								// ROUND the returned value.
								isP.CacheStatus.AwtBehindMb = newInt(int(*ttAgentOut.AwtBehindMb + 0.5)) // ROUNDED UP
							}

							// If it's loaded is the repagent running?
							// How about the cache agent?

							repAgentConnected := false
							cacheAgentConnected := false

							reqLogger.V(2).Info(fmt.Sprintf(us+": ttAgentOut.TTStatus.DbInfo[0].Conn=%v", ttAgentOut.TTStatus.DbInfo[0].Conn))
							for _, con := range ttAgentOut.TTStatus.DbInfo[0].Conn {
								if con.ProcType == "Replication" {
									repAgentConnected = true
								} else if con.ProcType == "Cache Agent" {
									cacheAgentConnected = true
								}
							}

							if repAgentConnected {
								isP.ReplicationStatus.RepAgent = "Running"
							} else {
								isP.ReplicationStatus.RepAgent = "Not Running"
							}
							reqLogger.V(2).Info(fmt.Sprintf(us+": setting ReplicationStatus.RepAgent to %v", isP.ReplicationStatus.RepAgent))

							if cacheAgentConnected {
								isP.CacheStatus.CacheAgent = "Running"
							} else {
								isP.CacheStatus.CacheAgent = "Not Running"
							}
							reqLogger.V(2).Info(fmt.Sprintf(us+": setting CacheStatus.CacheAgent to %v", isP.CacheStatus.CacheAgent))

							// What's the replication state?

							isP.ReplicationStatus.RepState = ttAgentOut.RepState
							reqLogger.V(2).Info(fmt.Sprintf(us+": ReplicationStatus.RepState=%v", isP.ReplicationStatus.RepState))

							// Is there a replication scheme defined?

							if ttAgentOut.NRepSchemes > 0 {
								isP.ReplicationStatus.RepScheme = "Exists"
							} else {
								isP.ReplicationStatus.RepScheme = "None"
							}
							reqLogger.V(2).Info(fmt.Sprintf(us+": ReplicationStatus.RepScheme=%v", isP.ReplicationStatus.RepScheme))

							isP.ReplicationStatus.RepPeerPState = ttAgentOut.RepPeerPState
							isP.ReplicationStatus.RepPeerPStateFetchErr = ttAgentOut.RepPeerPStateFetchErr
							isP.CacheStatus.NCacheGroups = ttAgentOut.NCacheGroups
							isP.CacheStatus.CacheUidPwdSet = ttAgentOut.CacheUidPwdSet
							isP.CacheStatus.Cachegroups = ttAgentOut.Cachegroups

						} else {
							isP.DbStatus.Db = "Unloaded"
							isP.ReplicationStatus.RepAgent = "Not Running"
							isP.ReplicationStatus.RepState = "Unknown"
							isP.CacheStatus.CacheAgent = "Not Running"
							// Do NOT log this

						}
					}
				}
			}
		}
	} else {
		isP.TimesTenStatus.Daemon = "Down"
	}
}

// verifies that a row inserted into TTAGENT.PING on the active is replicated to the standby
func verifyASReplication(ctx context.Context, instance *timestenv2.TimesTenClassic, activePodNo int, standbyPodNo int, client client.Client, tts *TTSecretInfo) error {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("verifyReplication entered")
	defer reqLogger.V(2).Info("verifyReplication ends")

	replVerified := false

	// dbId is a counter we're using to verify replication; get1.sql inserts a row into TTAGENT.PING and
	// we'll make sure that row makes it to the STANDBY

	activePodInfo := &instance.Status.PodStatus[activePodNo]
	activePodDNSName := activePodInfo.Name + "." + instance.Name + "." + instance.Namespace + ".svc.cluster.local"

	standbyPodInfo := &instance.Status.PodStatus[standbyPodNo]
	standbyPodDNSName := standbyPodInfo.Name + "." + instance.Name + "." + instance.Namespace + ".svc.cluster.local"

	getPodStatus(ctx, instance, activePodDNSName, activePodInfo, client, tts, nil)
	reqLogger.V(1).Info(fmt.Sprintf("verifyReplication: ACTIVE DbId=%v", activePodInfo.DbStatus.DbId))

	n := 1
	maxAttempts := 30
	for n < maxAttempts {
		reqLogger.V(1).Info(fmt.Sprintf("verifyReplication: calling getPodStatus (attempt %v/%v) for STANDBY POD %v standbyPodDNSName=%v",
			n, maxAttempts, standbyPodNo, standbyPodDNSName))
		getPodStatus(ctx, instance, standbyPodDNSName, standbyPodInfo, client, tts, nil)

		reqLogger.V(1).Info(fmt.Sprintf("verifyReplication: active had DbId=%v, standby has DbId=%v", activePodInfo.DbStatus.DbId, standbyPodInfo.DbStatus.DbId))

		if standbyPodInfo.DbStatus.DbId == activePodInfo.DbStatus.DbId {
			reqLogger.V(1).Info(fmt.Sprintf("verifyReplication: standby matched DbId %v, replication verified", activePodInfo.DbStatus.DbId))
			replVerified = true
			break
		}
		n++
		reqLogger.V(1).Info("verifyReplication: did not match dbId on the STANDBY, sleep 10")
		time.Sleep(10 * time.Second)
	}

	if replVerified == false {
		reqLogger.V(1).Info("verifyReplication: replication cannot be verified, setting ActiveStatus=failed")
		instance.Status.ClassicUpgradeStatus.ActiveStatus = "failed"
		err := errors.New("Replication cannot be verified; error upgrading standby")
		return err
	}

	return nil
}

// calls ttAdmin -open <dsn> on the active
func openDb(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, tts *TTSecretInfo) error {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("openDb entered")
	defer reqLogger.V(2).Info("openDb ends")

	err, pairStates := getCurrActiveStandby(ctx, instance)
	if err != nil {
		reqLogger.Info("openDb: getCurrActiveStandby failed, cannot determine AS pod states")
		return err
	}
	reqLogger.V(2).Info(fmt.Sprintf("openDb: pairStates=%v", pairStates))

	reqParams := make(map[string]string)
	reqParams["dbName"] = instance.Name

	podNo := pairStates["activePodNo"]

	reqLogger.Info("openDb: call openDb on the active, pod " + strconv.Itoa(podNo))
	reqLogger.V(2).Info(fmt.Sprintf("openDb: passing reqParams=%v to RunAction", reqParams))

	err = RunAction(ctx, instance, podNo, "openDb", reqParams, client, tts, nil)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Error calling openDb on active pod %d", pairStates["activePodNo"]))
		logTTEvent(ctx, client, instance, "Info", "Unable to open the database", true)
		unknownVerb := regexp.MustCompile("Error 255")
		if unknownVerb.FindString(err.Error()) != "" {
			reqLogger.Info("openDb: agent does not support this command")
			return nil
		}
		return err
	}

	reqLogger.V(1).Info("openDb: successfully opened db " + instance.Name)

	return nil

}

// calls ttAdmin -close <dsn> on the active
func closeDb(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, tts *TTSecretInfo) error {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("closeDb entered")
	defer reqLogger.V(2).Info("closeDb ends")

	err, pairStates := getCurrActiveStandby(ctx, instance)
	if err != nil {
		reqLogger.Info("closeDb: getCurrActiveStandby failed, cannot determine AS pod states")
		return err
	}
	reqLogger.V(2).Info(fmt.Sprintf("closeDb: pairStates=%v", pairStates))

	reqParams := make(map[string]string)
	reqParams["dbName"] = instance.Name
	podNo := pairStates["activePodNo"]
	reqParams["podName"] = instance.Status.PodStatus[podNo].Name

	reqLogger.Info("closeDb: call closeDb on the active, pod " + strconv.Itoa(podNo))
	reqLogger.V(2).Info(fmt.Sprintf("closeDb: passing reqParams=%v to RunAction", reqParams))

	err = RunAction(ctx, instance, podNo, "closeDb", reqParams, client, tts, nil)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Error calling closeDb on active pod %d", pairStates["activePodNo"]))
		unknownVerb := regexp.MustCompile("Error 255")
		if unknownVerb.FindString(err.Error()) != "" {
			reqLogger.Info("closeDb: agent does not support this command")
			return nil
		}
		logTTEvent(ctx, client, instance, "Info", "Unable to close the database", true)
		return err
	}

	reqLogger.Info("closeDb: successfully closed db " + instance.Name)

	return nil

}

// calls ttAdmin -disconnect <urgency> <dsn> on the active
func forceDisconnect(ctx context.Context, urgency string, instance *timestenv2.TimesTenClassic, client client.Client, tts *TTSecretInfo) error {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("forceDisconnect entered")
	defer reqLogger.V(2).Info("forceDisconnect ends")

	err, pairStates := getCurrActiveStandby(ctx, instance)
	if err != nil {
		reqLogger.Info("forceDisconnect: getCurrActiveStandby failed, cannot determine AS pod states")
		return err
	}
	reqLogger.V(2).Info(fmt.Sprintf("forceDisconnect: pairStates=%v", pairStates))

	podNo := pairStates["activePodNo"]

	reqParams := make(map[string]string)
	reqParams["dbName"] = instance.Name
	reqParams["podName"] = instance.Status.PodStatus[pairStates["activePodNo"]].Name
	reqParams["standbyHost"] = fmt.Sprintf("%s.%s.%s.svc.cluster.local", instance.Status.PodStatus[pairStates["standbyPodNo"]].Name, instance.Name, instance.Namespace)
	reqParams["urgency"] = urgency

	reqLogger.Info(fmt.Sprintf("forceDisconnect: calling doForceDisconnect with urgency=%v on the active pod %d", reqParams["urgency"], pairStates["activePodNo"]))
	reqLogger.V(2).Info(fmt.Sprintf("forceDisconnect: passing reqParams=%v to RunAction", reqParams))

	err = RunAction(ctx, instance, podNo, "doForceDisconnect", reqParams, client, tts, nil)
	if err != nil {
		unknownVerb := regexp.MustCompile("Error 255")
		if unknownVerb.FindString(err.Error()) != "" {
			reqLogger.Info("forceDisconnect: agent does not support this command")
			return nil
		} else {
			reqLogger.Error(err, fmt.Sprintf("Error calling doForceDisconnect on active pod %d", pairStates["activePodNo"]))
			logTTEvent(ctx, client, instance, "Info", "Force disconnect failed", true)
		}
		return err
	}

	reqLogger.V(1).Info(fmt.Sprintf("forceDisconnect: doForceDisconnect with urgency=%v successful on active pod %d", reqParams["urgency"], pairStates["activePodNo"]))

	return nil

}

// call ttRepAdmin -wait on the active to determine whether the standby has caught up
func repAdminWait(ctx context.Context, timeout int, instance *timestenv2.TimesTenClassic, client client.Client, tts *TTSecretInfo) error {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info("repAdminWait entered")
	defer reqLogger.V(2).Info("repAdminWait ends")

	err, pairStates := getCurrActiveStandby(ctx, instance)
	if err != nil {
		reqLogger.Info("repAdminWait: getCurrActiveStandby failed, cannot determine AS pod states")
		return err
	}
	reqLogger.V(2).Info(fmt.Sprintf("repAdminWait: pairStates=%v", pairStates))

	repIsCurrent := false
	n := 1
	for n < 11 {
		reqParams := make(map[string]string)
		reqParams["dbName"] = instance.Name
		podNo := pairStates["activePodNo"]
		reqParams["podName"] = instance.Status.PodStatus[podNo].Name
		reqParams["standbyHost"] = fmt.Sprintf("%s.%s.%s.svc.cluster.local", instance.Status.PodStatus[pairStates["standbyPodNo"]].Name, instance.Name, instance.Namespace)
		reqParams["timeout"] = strconv.Itoa(timeout)

		reqLogger.V(1).Info(fmt.Sprintf("repAdminWait: iter %v/10, call doRepAdminWait on active before pod deletion", n))
		reqLogger.V(2).Info(fmt.Sprintf("repAdminWait: passing reqParams=%v to doRepAdminWait", reqParams))

		err = RunAction(ctx, instance, podNo, "doRepAdminWait", reqParams, client, tts, nil)
		if err != nil {
			reqLogger.Info(fmt.Sprintf("repAdminWait: doRepAdminWait failed on the active, err=%v", err))
			unknownVerb := regexp.MustCompile("Error 255")
			if unknownVerb.FindString(err.Error()) != "" {
				reqLogger.Info("doRepAdminWait: agent does not support this command")
				// set repIsCurrent to true even though we were unable to execute RepAdminWait
				// this allows us to proceed with operations (ie upgrades) against older agents
				repIsCurrent = true
				break
			}
			n++
			continue
		} else {
			reqLogger.Info("repAdminWait: doRepAdminWait successful on the active")
			repIsCurrent = true
			break
		}
	}

	if repIsCurrent == false {
		logTTEvent(ctx, client, instance, "Info", "repAdminWait timed out, standby behind active", true)
		return errors.New("standby behind active")
	}

	return nil

}

// kill the agent
func killAgent(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject, dnsName string, podName string, tts *TTSecretInfo, timeoutOverride *int) (success bool, err error) {
	us := "killAgent"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " ends")

	// get our saved http client object (has persistent connection to pod)
	httpClient := getHttpClient(ctx, instance, dnsName, "POST", tts, timeoutOverride)

	agentDieUrl := "https://" + dnsName + ":8443/agent"
	reqLogger.Info(fmt.Sprintf("%s: terminating agent running on %s", us, dnsName))
	vDie := url.Values{}
	vDie.Set("verb", "die")
	dReq, dErr := http.NewRequest("POST", agentDieUrl, strings.NewReader(vDie.Encode()))
	if dErr != nil {
		reqLogger.Error(dErr, us+": die action on agent returned error: "+dErr.Error())
		return false, dErr
	}
	dReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")
	dReq.SetBasicAuth(tts.HttpUID, tts.HttpPWD)
	dReq.Close = false
	dResp, dErr := httpClient.Do(dReq)
	if dErr != nil {
		reqLogger.Error(err, us+": calling die on agent returned error: "+dErr.Error())
		return false, dErr
	}

	logTTEvent(ctx, client, instance, "Info", fmt.Sprintf("Pod %s: Terminating 'tt' container by killing agent", podName), true)

	reqLogger.V(2).Info(fmt.Sprintf("%s: calling die on agent returned http %v", us, dResp.StatusCode))

	return true, nil

}

// determine whether a slice contains a given string
func sliceContains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

// get substring after a string
func stringAfter(value string, a string) string {
	pos := strings.LastIndex(value, a)
	if pos == -1 {
		return ""
	}
	adjustedPos := pos + len(a)
	if adjustedPos >= len(value) {
		return ""
	}
	return value[adjustedPos:len(value)]
}

// Is shared memory configured properly to create a database/element?
func VerifyDbShmRequirement(ctx context.Context, requiredMemory int64, cg timestenv2.CGroupMemoryInfo) (error, string) {
	us := "VerifyDbShmRequirement"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(fmt.Sprintf("%s: entered (required: %d", us, requiredMemory))
	reqLogger.V(2).Info(fmt.Sprintf("%s: huge2MB limit: %d, memory limit: %d", us, cg.Huge2MBLimitInBytes, cg.MemoryLimitInBytes))
	defer reqLogger.V(2).Info(us + " ends")

	var Mi int64 = 1024 * 1024
	var MemoryNoLimits int64 = 0x7FFFFFFFFFFFF000

	requiredHugePages := requiredMemory / 2 * 1024 * 1024

	if cg.NodeHugePagesFree > requiredHugePages {

		if cg.Huge2MBLimitInBytes >= requiredMemory {
			return nil, fmt.Sprintf("Sufficient huge pages are available (%d Mi) to create the database (%d Mi)",
				cg.Huge2MBLimitInBytes/Mi, requiredMemory/Mi)
		}

		if cg.Huge2MBLimitInBytes > 0 {
			return errors.New(fmt.Sprintf("Some huge pages are available (%d Mi) but not enough to create the database (%d Mi). Provide either sufficient huge pages (recommended) or none",
				cg.Huge2MBLimitInBytes/Mi, requiredMemory/Mi)), ""
		}
	}

	if cg.MemoryLimitInBytes == MemoryNoLimits {
		return nil, fmt.Sprintf("Memory limit (unlimited) is sufficient to create the database (%d Mi), but is not recommended", requiredMemory)
	}

	if cg.MemoryLimitInBytes <= requiredMemory {
		return errors.New(fmt.Sprintf("Memory limit (%d Mi) is not sufficient to create the database (%d Mi)",
			cg.MemoryLimitInBytes/Mi, requiredMemory/Mi)), ""
	}
	return nil, fmt.Sprintf("Memory limit (%d Mi) is sufficient to create the database (%d Mi)",
		cg.MemoryLimitInBytes/Mi, requiredMemory/Mi)

}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
