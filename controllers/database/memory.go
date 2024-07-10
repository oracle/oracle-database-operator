// Copyright (c) 2019-2023, Oracle and/or its affiliates. All rights reserved.
//
// Functions related to memory management. Functions here apply to
// both TimesTenClassic and TimesTenScaleout

package controllers

import (
	"context"
	"os"

	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	//appsv1 "k8s.io/api/apps/v1"
	//k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	resource "k8s.io/apimachinery/pkg/api/resource"
	//intstr "k8s.io/apimachinery/pkg/util/intstr"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	//"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//"encoding/json"
	"strconv"
	//"time"
	"strings"
	//"os"
	"errors"
	//"io"
	//"io/ioutil"
	//"bytes"
	//"os/exec"
	"fmt"
	//"encoding/base64"
	//"os/user"
	//"bufio"
	"regexp"
)

//----------------------------------------------------------------------
// Constants
//----------------------------------------------------------------------

var Mi int64 = 1024 * 1024
var MemoryNoLimits int64 = 0x7FFFFFFFFFFFF000

//----------------------------------------------------------------------

// See if we've run out of memory or anything else terrible
func checkCGroupInfo(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject, pod *corev1.Pod, isP *timestenv2.TimesTenPodStatus) error {
	us := "checkCGroupInfo"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var warnPct int64 = int64(instance.GetMemoryWarningPercent())

	old := isP.PrevCGroupInfo
	new := isP.CGroupInfo

	if new == nil {
		if old == nil {
			// Don't have old OR new info
		} else {
			// We have the old info but no new info
			msg := fmt.Sprintf("Pod %s cgroup info not returned by agent", pod.Name)
			reqLogger.V(1).Info(msg)
			logTTEvent(ctx, client, instance, "Warning", msg, true)
		}
	} else {
		var warnSize int64
		if new.MemoryLimitInBytes == MemoryNoLimits {
			warnSize = -1
		} else {
			warnSize = new.MemoryLimitInBytes * warnPct / 100
		}

		reqLogger.V(2).Info(fmt.Sprintf("warnSize %d; new.MemoryLimitInBytes %d", warnSize, new.MemoryLimitInBytes))

		if old != nil {
			if new.MemoryFailCnt > old.MemoryFailCnt {
				msg := fmt.Sprintf("Pod %s memory fail count is %d, was %d", pod.Name, new.MemoryFailCnt, old.MemoryFailCnt)
				reqLogger.V(1).Info(msg)
				logTTEvent(ctx, client, instance, "Warning", msg, true)
			}
			if new.Huge2MBFailCnt > old.Huge2MBFailCnt {
				msg := fmt.Sprintf("Pod %s huge page (2MB) memory fail count was %d now %d", pod.Name, old.Huge2MBFailCnt, new.Huge2MBFailCnt)
				reqLogger.V(1).Info(msg)
				logTTEvent(ctx, client, instance, "Warning", msg, true)
			}
			if new.MemoryLimitInBytes == MemoryNoLimits {
			} else {
				if old.MemoryUsageInBytes < warnSize &&
					new.MemoryUsageInBytes > warnSize {
					msg := fmt.Sprintf("Pod %s container 'tt' memory usage is %d percent of limit. Limit: %d Mi; Current: %d Mi; Previous: %d Mi", pod.Name, new.MemoryUsageInBytes*100/new.MemoryLimitInBytes, new.MemoryLimitInBytes/Mi, new.MemoryUsageInBytes/Mi, old.MemoryUsageInBytes/Mi)
					reqLogger.V(1).Info(msg)
					logTTEvent(ctx, client, instance, "Warning", msg, true)
				} else {
					// Finally, a GOOD case! Memory seems fine.
				}
			}
		} else {
			if new.MemoryFailCnt > 0 {
				msg := fmt.Sprintf("Pod %s memory fail count is %d", pod.Name, new.MemoryFailCnt)
				reqLogger.V(1).Info(msg)
				logTTEvent(ctx, client, instance, "Warning", msg, true)
			}
			if new.Huge2MBFailCnt > 0 {
				msg := fmt.Sprintf("Pod %s huge page (2MB) memory fail count is %d", pod.Name, new.Huge2MBFailCnt)
				reqLogger.V(1).Info(msg)
				logTTEvent(ctx, client, instance, "Warning", msg, true)
			}
			if new.MemoryLimitInBytes == MemoryNoLimits {
				msg := fmt.Sprintf("Warning: Pod %s No memory limit set", pod.Name)
				reqLogger.V(1).Info(msg)
				logTTEvent(ctx, client, instance, "Warning", msg, true)
			} else {
				if new.MemoryUsageInBytes > warnSize {
					msg := fmt.Sprintf("Pod %s memory usage is %d percent of limit. Limit: %d Mi; Current: %d Mi", pod.Name, new.MemoryUsageInBytes*100/new.MemoryLimitInBytes, new.MemoryLimitInBytes/Mi, new.MemoryUsageInBytes/Mi)
					reqLogger.V(1).Info(msg)
					logTTEvent(ctx, client, instance, "Warning", msg, true)
				} else {
					// Finally, a GOOD case! Memory seems fine.
				}
			}
		}
	}

	return nil
}

// Check the status of the containers in a pod
// err, podReplaced = checkStatusOfContainersInPod(ctx, client, instance, pod, isP)
func checkStatusOfContainersInPod(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject, pod *corev1.Pod, isP *timestenv2.TimesTenPodStatus) (error, bool) {
	us := "checkStatusOfContainersInPod"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + "(" + instance.ObjectName() + ") entered")
	defer reqLogger.V(2).Info(us + " returns")

	if isP == nil {
		return errors.New(us + " was called with isP nil"), false
	}

	var restarted []string  // List of containers that have restarted
	var terminated []string // List of containers that have terminated
	var sigkilled []string  // List of containers that were SIG killed

	// Is the pod we're looking at even associated with us? It might be left
	// over from some other TimesTen* object or something unrelated

	if a, ok := pod.Annotations["TTC"]; ok {
		if a == string(instance.ObjectUID()) {
			// It's ours, continue
		} else {
			msg := fmt.Sprintf("Pod belongs to another TT object. Instance: %s. Pod: %s", instance.ObjectUID(), a)
			reqLogger.V(2).Info(msg)
			return errors.New(msg), false
		}
	} else {
		if b, ok := pod.Annotations["TTS"]; ok {
			if b == string(instance.ObjectUID()) {
				// It's ours (Scaleout), continue
			} else {
				msg := fmt.Sprintf("Pod belongs to another TT object. Instance: %s. Pod: %s", instance.ObjectUID(), a)
				reqLogger.V(2).Info(msg)
				return errors.New(msg), false
			}
		} else {
			// Positively not ours, ignore it
			msg := "No TTC annotation; not our pod"
			reqLogger.V(2).Info(msg)
			return errors.New(msg), false
		}
	}

	// Are we looking at a completely different pod than the last time?

	if isP.PodStatus.PrevUID == nil {
		msg := fmt.Sprintf("Pod %s is new. UID %s", pod.Name, pod.ObjectMeta.UID)
		reqLogger.V(1).Info(msg)
		isP.PodStatus.PrevUID = newUID(pod.ObjectMeta.UID)
		isP.PodStatus.PrevContainerStatuses = nil
		return nil, false
	} else {
		if *isP.PodStatus.PrevUID != pod.ObjectMeta.UID {
			msg := fmt.Sprintf("Pod %s was replaced", pod.Name)
			reqLogger.V(1).Info(msg)
			isP.PodStatus.PrevUID = newUID(pod.ObjectMeta.UID)
			isP.PodStatus.PrevContainerStatuses = nil
			isP.PodStatus.LastTimeReachable = 0
			return errors.New(msg), true
		}
	}
	isP.PodStatus.PrevUID = newUID(pod.ObjectMeta.UID)

	// See if any containers that were running are now terminated or restarted

	isTTWaiting := false
	waitingReason := ""

	for _, prevc := range isP.PodStatus.PrevContainerStatuses {
		for _, c := range pod.Status.ContainerStatuses {
			if prevc.Name != c.Name {
				continue
			}
			if prevc.RestartCount != c.RestartCount {
				// Container restarted!
				restarted = append(restarted, c.Name)
			}
			if prevc.State.Waiting == nil {
				if c.State.Waiting != nil {
					isTTWaiting = true
					switch c.State.Waiting.Reason {
					case "CrashLoopBackOff":
						if strings.HasPrefix(c.State.Waiting.Message, "back-off") {
							words := strings.Fields(c.State.Waiting.Message)
							waitingReason = fmt.Sprintf("Container %s waiting in CrashLoopBackOff: %s %s", c.Name, words[0], words[1])
						} else {
							waitingReason = fmt.Sprintf("Container %s waiting in CrashLoopBackOff: %s", c.Name, c.State.Waiting.Message)
						}
					case "PodInitializing": // Startup, nothing is broken
						isTTWaiting = false
						waitingReason = ""
					default:
						waitingReason = fmt.Sprintf("in %s", c.State.Waiting.Reason)
					}
				} else if c.State.Running != nil {
					// Running
				} else {
					// Can't happen
				}
			} else {
				// It WAS running
				if c.State.Running != nil {
					// ...and it's still running
				} else if c.State.Waiting != nil {
					// ...and oddly now its just starting up
				} else if c.State.Terminated != nil {
					// ...and now it's dead
					exitCode := c.State.Terminated.ExitCode
					if exitCode == 137 {
						sigkilled = append(sigkilled, c.Name)
					} else {
						terminated = append(terminated, c.Name)
					}
				}
			}
		}
	}

	didTTFail := false
	ttFailedHow := ""
	didSidecarFail := false

	for _, c := range sigkilled {
		msg := fmt.Sprintf("Pod %s container '%s' killed by SIGKILL signal", pod.Name, c)
		reqLogger.V(1).Info(msg)
		switch c {
		case "tt":
			didTTFail = true
			ttFailedHow = "killed by SIGKILL signal"
		case "daemonlog", "exporter", "zk":
			didSidecarFail = true
		default:
		}
	}
	for _, c := range restarted {
		msg := fmt.Sprintf("Pod %s container '%s' restarted", pod.Name, c)
		reqLogger.V(1).Info(msg)
		switch c {
		case "tt":
			didTTFail = true
			ttFailedHow = "container restarted"
		case "daemonlog", "exporter", "zk":
			didSidecarFail = true
		default:
		}
	}
	for _, c := range terminated {
		msg := fmt.Sprintf("Pod %s container '%s' terminated", pod.Name, c)
		reqLogger.V(1).Info(msg)
		switch c {
		case "tt":
			didTTFail = true
			ttFailedHow = "container terminated"
		case "daemonlog", "exporter", "zk":
			didSidecarFail = true
		default:
		}
	}

	isP.PodStatus.PrevContainerStatuses = pod.Status.ContainerStatuses

	if didTTFail {
		isP.PodStatus.LastTimeReachable = 0
		return errors.New(fmt.Sprintf("Pod %s TimesTen %s", pod.Name, ttFailedHow)), true
	}

	if didSidecarFail {
		return errors.New(fmt.Sprintf("Pod %s One or more TimesTen sidecar containers terminated or restarted", pod.Name)), false // for now
	}

	if isTTWaiting {
		isP.PodStatus.LastTimeReachable = 0
		return errors.New(fmt.Sprintf("Pod %s TimesTen %s", pod.Name, waitingReason)), true // ?
	}

	return nil, false
}

var ourTTRelease int

func GetTTMajorRelease(ctx context.Context) (int, error) {
	us := "GetTTMajorRelease"
	reqLogger := log.FromContext(ctx)

	if ourTTRelease > 0 {
		return ourTTRelease, nil
	}

	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var out string
	timestenHome, _ := os.LookupEnv("TIMESTEN_HOME")

	rc, stdout, stderr := runShellCommand(ctx, []string{timestenHome + "/bin/ttenv", "ttVersion", "major1"})
	if rc != 0 {
		var errmsg string
		if len(stderr) > 0 {
			errmsg = fmt.Sprintf("Could not determine TimesTen release: ttVersion returns %d: %s", rc, stderr[0])
		} else {
			errmsg = fmt.Sprintf("Could not determine TimesTen release: ttVersion returns %d: no stderr", rc)
		}
		return -1, errors.New(errmsg)
	}

	if len(stdout) < 1 {
		return -1, errors.New("Could not determine TimesTen release: ttVersion returned no output")
	}

	out = stdout[0]

	rel, err := strconv.Atoi(out)
	if err != nil {
		return -1, err
	}

	reqLogger.V(1).Info(fmt.Sprintf("%s: this is TimesTen release %d", us, rel))
	return rel, nil
}

func getDbSizeViaTTShmSize(ctx context.Context, dbi string, isScaleout bool) (resource.Quantity, error) {
	us := "getDbSizeViaTTShmSize"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(fmt.Sprintf("%s (%s) entered", us, dbi))
	defer reqLogger.V(2).Info(us + " returns")

	var out resource.Quantity

	// TimesTen Scaleout quirk / bug(?): if Connections isn't specified
	// then it automatically inserts "connections=100". Just in Scaleout.
	// We need to take that into account.

	sawConnections := false

	items := strings.Fields(dbi)
	connstr := "driver=x;"
	for _, c := range items {
		nv := strings.Split(c, "=")
		if len(nv) == 2 {
			connstr = connstr + c + ";"
			if strings.EqualFold(nv[0], "connections") {
				sawConnections = true
			}
		}
	}

	if isScaleout && sawConnections == false {
		connstr = connstr + "connections=100;"
	}
	timestenHome, _ := os.LookupEnv("TIMESTEN_HOME")
	reqLogger.V(2).Info(fmt.Sprintf("connstr: %s", connstr))
	rc, stdout, stderr := runShellCommand(ctx, []string{timestenHome + "/bin/ttenv", "ttShmSize", "-connstr", connstr})
	if rc != 0 {
		var errmsg string
		if len(stderr) > 0 {
			errmsg = fmt.Sprintf("Could not determine db size: ttShmSize returns %d: %s", rc, stderr[0])
		} else {
			errmsg = fmt.Sprintf("Could not determine db size: ttShmSize returns %d: no stderr", rc)
		}
		return out, errors.New(errmsg)
	}

	if len(stdout) < 1 {
		return out, errors.New("Could not determine db size: ttShmSize returned no output")
	}

	re := regexp.MustCompile(`^The required shared memory size is (.*) bytes`)
	x := re.FindStringSubmatch(stdout[0])
	if x == nil {
		return out, errors.New(fmt.Sprintf("Could not determine db size: ttShmSize output not recognizable: '%s'", stdout[0]))
	}
	if len(x) < 2 {
		return out, errors.New(fmt.Sprintf("Could not determine db size: ttShmSize output not parseable: '%s'", stdout[0]))
	}

	requiredMemory, err := strconv.ParseInt(x[1], 10, 64)
	if err != nil {
		return out, errors.New(fmt.Sprintf("Could not determine db size: ttShmSize output size not parseable: '%s' : %s", stdout[0], err.Error()))
	}

	reqLogger.V(2).Info(fmt.Sprintf("Returning %d", requiredMemory))

	out = *resource.NewQuantity(requiredMemory, "")

	return out, nil
}

// Find the db.ini provided by the user, if possible
// (If they are using init containers it might not be)
func getDbSizeFromDbIni(ctx context.Context, client client.Client, instance timestenv2.TimesTenObject) (resource.Quantity, bool, error) {
	us := "getDbSizeFromDbIni"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var out resource.Quantity

	isScaleout := false
	switch instance.ObjectType() {
	case "TimesTenClassic":
		isScaleout = false
	case "TimesTenScaleout":
		isScaleout = true
	}

	// Fetch some data from the TimesTenClassic / TimesTenScaleout
	// object that requires some casting to get to

	var dbConfigMap *[]string
	var dbSecret *[]string

	switch v := instance.(type) {
	case *timestenv2.TimesTenClassic:
		dbConfigMap = v.Spec.TTSpec.DbConfigMap
		dbSecret = v.Spec.TTSpec.DbSecret

	default:
		return out, false, errors.New(fmt.Sprintf("%s: passed unknown type %T", us, v))
	}

	if dbConfigMap != nil {
		for _, c := range *dbConfigMap {
			cm := &corev1.ConfigMap{}
			err := client.Get(ctx, types.NamespacedName{Namespace: instance.ObjectNamespace(), Name: c}, cm)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "Error",
					fmt.Sprintf("Could not fetch ConfigMap %s: %s", c, errorMsg), true)
				return out, false, errors.New(fmt.Sprintf("Could not fetch ConfigMap %s: %s", c, errorMsg))
			}

			if dbi, ok := cm.Data["db.ini"]; ok {
				out, err = getDbSizeViaTTShmSize(ctx, dbi, isScaleout)
				if err == nil {
					return out, true, nil
				}
				return out, false, err
			}

			if dbib, ok := cm.BinaryData["db.ini"]; ok {
				dbi := string(dbib)
				out, err = getDbSizeViaTTShmSize(ctx, dbi, isScaleout)
				if err == nil {
					return out, true, nil
				}
				return out, false, err
			}
		}
	}

	if dbSecret != nil {
		for _, c := range *dbSecret {
			s := &corev1.Secret{}
			err := client.Get(ctx, types.NamespacedName{Namespace: instance.ObjectNamespace(), Name: c}, s)
			if err != nil {
				//Checks if the error was because of lack of permission, if not, return the original message
				var errorMsg, _ = verifyUnauthorizedError(err.Error())
				logTTEvent(ctx, client, instance, "Error",
					fmt.Sprintf("Could not fetch Secret %s: %s", c, errorMsg), true)
				return out, false, errors.New(fmt.Sprintf("Could not fetch Secret %s: %s", c, errorMsg))
			}
			if dbib, ok := s.Data["db.ini"]; ok {
				dbi := string(dbib)
				out, err = getDbSizeViaTTShmSize(ctx, dbi, isScaleout)
				if err == nil {
					return out, true, nil
				}
				return out, false, err
			}

		}
	}

	return out, false, nil // User didn't specify it

}

// Determine the resource requests/limits for a "tt" container
func setTTResources(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client, in corev1.ResourceRequirements) (error, corev1.ResourceRequirements) {
	us := "setTTResources"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var err error
	var out corev1.ResourceRequirements
	out.Limits = make(corev1.ResourceList)
	out.Requests = make(corev1.ResourceList)

	reqLogger.V(2).Info(fmt.Sprintf("in: %+v", in))

	// Fetch some data from the TimesTenClassic / TimesTenScaleout
	// object that requires some casting to get to

	var addtlSz string
	var additionalSize resource.Quantity
	var useHugePages bool
	var dbSz *string
	var databaseCPURequest *string

	switch v := instance.(type) {
	case *timestenv2.TimesTenClassic:
		addtlSz = v.Spec.TTSpec.AdditionalMemoryRequest
		useHugePages = v.Spec.TTSpec.UseHugePages
		dbSz = v.Spec.TTSpec.DatabaseMemorySize
		databaseCPURequest = v.Spec.TTSpec.DatabaseCPURequest

	default:
		return errors.New(fmt.Sprintf("%s: passed unexpected type %T", us, v)), out
	}

	additionalSize, err = resource.ParseQuantity(addtlSz)
	if err != nil {
		err = errors.New(fmt.Sprintf("Error: User specified 'additionalMemoryRequest: \"%s\", cannot parse: %s", addtlSz, err.Error()))
		return err, out
	}

	// Did the user specify Limits and/or Requests? If so just do what they said.

	nUserSpecifiedMemory := 0
	userSpecifiedCPU := false
	for k, _ := range in.Limits {
		switch k {
		case "hugepages-1Gi", "hugepages-2Mi", "memory":
			nUserSpecifiedMemory++
		case "cpu":
			userSpecifiedCPU = true
		default:
		}
	}
	for k, _ := range in.Requests {
		switch k {
		case "hugepages-1Gi", "hugepages-2Mi", "memory":
			nUserSpecifiedMemory++
		case "cpu":
			userSpecifiedCPU = true
		default:
		}
	}

	// If the user specified CPU requests or limits, do what they said

	if userSpecifiedCPU {
		if l, ok := in.Limits["cpu"]; ok {
			out.Limits["cpu"] = l
		}
		if r, ok := in.Requests["cpu"]; ok {
			out.Requests["cpu"] = r
		}
	} else {
		// User didn't specify CPU budget in their template.
		// Did they specify it in the TimesTen object?

		if databaseCPURequest != nil {
			cpur, err := resource.ParseQuantity(*databaseCPURequest)
			if err != nil {
				err = errors.New(fmt.Sprintf("Error: User specified 'databaseCPURequest: \"%s\", cannot parse: %s", *databaseCPURequest, err.Error()))
				return err, out
			}
			out.Limits["cpu"] = cpur
			out.Requests["cpu"] = cpur
		} else {
			// If they didn't specify CPU their either then oh well
			reqLogger.V(1).Info(fmt.Sprintf("User did not specify database CPU request / limit"))
			logTTEvent(ctx, client, instance, "Warning",
				"Database CPU limit/request not specified",
				true)

		}
	}

	if nUserSpecifiedMemory > 0 {
		reqLogger.V(2).Info(fmt.Sprintf("User specified %d memory-related items, using user provided resource specification", nUserSpecifiedMemory))
		return nil, in
	}

	reqLogger.V(2).Info("User did not specify any memory-related resources")

	// If not...then figure out the right thing to do using their
	// other settings (if any) or our own best idea (if not)

	// Copy over everything the user specified EXCEPT for the ones we
	// might need to adjust / set. We don't care what CPU budget the user
	// wants, for example.

	for k, v := range in.Limits {
		switch k {
		case "hugepages-1Gi", "hugepages-2Mi", "memory":
			// Don't copy it
		default:
			out.Limits[k] = v
		}
	}

	for k, v := range in.Requests {
		switch k {
		case "hugepages-1Gi", "hugepages-2Mi", "memory":
			// Don't copy it
		default:
			out.Requests[k] = v
		}
	}

	zeroSz := resource.MustParse("0")

	// Make use quantity strings specified by user are legal quantities
	// This is what ttShmSize returns for PermSize=200, our default
	// Rounded up to a multiple of the 2Mi huge page size, just in case
	defaultDbSize := resource.MustParse("580911104")

	dbSizeSpecified := false
	var dbSize resource.Quantity

	if dbSz == nil {
		// The user didn't specify databaseMemorySize. Can we figure it out?
		dbSize, dbSizeSpecified, err = getDbSizeFromDbIni(ctx, client, instance)
		if err != nil {
			return err, out
		}
	} else {
		// User specified databaseMemorySize, use that
		dbSize, err = resource.ParseQuantity(*dbSz)
		if err != nil {
			err = errors.New(fmt.Sprintf("Error: User specified 'databaseMemorySize: \"%s\", cannot parse: %s", *dbSz, err.Error()))
			return err, out
		}
		dbSizeSpecified = true
	}

	// OK, now set up the limits

	if useHugePages {
		if dbSizeSpecified == false {
			out.Limits["hugepages-2Mi"] = defaultDbSize
			out.Requests["hugepages-2Mi"] = defaultDbSize
		} else {
			// We have to round dbSize up to an integral number of huge pages

			if dbSize.Value()%(2*1024*1024) != 0 {
				var wholePages int64
				wholePages = dbSize.Value() / (2 * 1024 * 1024)
				dbSize = *resource.NewQuantity((wholePages+1)*(2*1024*1024), resource.BinarySI)
			}

			out.Limits["hugepages-2Mi"] = dbSize
			out.Requests["hugepages-2Mi"] = dbSize
		}

		if mem, ok := in.Limits["memory"]; ok {
			// If user specified a memory limit, just use it
			out.Limits["memory"] = mem
			out.Requests["memory"] = mem
		} else {
			// If user didn't specify a memory limit, use their "additional" request
			out.Limits["memory"] = additionalSize
			out.Requests["memory"] = additionalSize

		}
	} else {
		out.Limits["hugepages-1Gi"] = zeroSz
		out.Limits["hugepages-2Mi"] = zeroSz
		out.Requests["hugepages-1Gi"] = zeroSz
		out.Requests["hugepages-2Mi"] = zeroSz

		sz := defaultDbSize
		if dbSizeSpecified == true {
			sz = dbSize
		}

		sz.Add(additionalSize)

		out.Limits["memory"] = sz
		out.Requests["memory"] = sz

	}

	//m := out.Limits["memory"]
	//h := out.Limits["hugepages-2Mi"]
	//logTTEvent(ctx, client, instance, "Debug",
	//    fmt.Sprintf("%s: Requests: memory %s hugepages %s", us, m.String(), h.String()), false)

	return nil, out
}

// Determine the resource requests/limits for a "daemonlog" container
func setDaemonLogResources(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client, in corev1.ResourceRequirements) (error, corev1.ResourceRequirements) {
	us := "setDaemonLogResources"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")
	containerName := "daemonlog"

	var err error
	var out corev1.ResourceRequirements
	out.Limits = make(corev1.ResourceList)
	out.Requests = make(corev1.ResourceList)

	reqLogger.V(2).Info(fmt.Sprintf("in: %+v", in))

	// Fetch some data from the TimesTenClassic / TimesTenScaleout
	// object that requires some casting to get to

	var daemonLogMemoryRequest string
	var daemonLogCPURequest string
	switch v := instance.(type) {
	case *timestenv2.TimesTenClassic:
		daemonLogMemoryRequest = v.Spec.TTSpec.DaemonLogMemoryRequest
		daemonLogCPURequest = v.Spec.TTSpec.DaemonLogCPURequest

	default:
		return errors.New(fmt.Sprintf("%s: passed unexpected type %T", us, v)), out
	}

	zeroSz := resource.MustParse("0")

	if lim, ok := in.Limits["hugepages-1Gi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-1Gi"] = zeroSz
	if req, ok := in.Requests["hugepages-1Gi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-1Gi"] = zeroSz
	if lim, ok := in.Limits["hugepages-2Mi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-2Mi"] = zeroSz
	if req, ok := in.Requests["hugepages-2Mi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-2Mi"] = zeroSz

	userMemoryRequest, err := resource.ParseQuantity(daemonLogMemoryRequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse daemonLogMemoryRequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["memory"]; ok {
		out.Limits["memory"] = in.Limits["memory"]
	} else {
		out.Limits["memory"] = userMemoryRequest
	}
	if _, ok := in.Requests["memory"]; ok {
		out.Requests["memory"] = in.Requests["memory"]
	} else {
		out.Requests["memory"] = userMemoryRequest
	}

	userCPURequest, err := resource.ParseQuantity(daemonLogCPURequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse daemonLogCPURequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["cpu"]; ok {
		out.Limits["cpu"] = in.Limits["cpu"]
	} else {
		out.Limits["cpu"] = userCPURequest
	}
	if _, ok := in.Requests["cpu"]; ok {
		out.Requests["cpu"] = in.Requests["cpu"]
	} else {
		out.Requests["cpu"] = userCPURequest
	}

	return err, out
}

// Determine the resource requests/limits for a "mgmt" container
// This is the "tt" container in a management instance
func setMgmtResources(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client, in corev1.ResourceRequirements) (error, corev1.ResourceRequirements) {
	us := "setMgmtResources"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")
	containerName := "tt"

	var err error
	var out corev1.ResourceRequirements
	out.Limits = make(corev1.ResourceList)
	out.Requests = make(corev1.ResourceList)

	reqLogger.V(2).Info(fmt.Sprintf("in: %+v", in))

	// Fetch some data from the TimesTenClassic / TimesTenScaleout
	// object that requires some casting to get to

	var mgmtMemoryRequest string
	var mgmtCPURequest string

	zeroSz := resource.MustParse("0")

	if lim, ok := in.Limits["hugepages-1Gi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-1Gi"] = zeroSz
	if req, ok := in.Requests["hugepages-1Gi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-1Gi"] = zeroSz
	if lim, ok := in.Limits["hugepages-2Mi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-2Mi"] = zeroSz
	if req, ok := in.Requests["hugepages-2Mi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-2Mi"] = zeroSz

	userMemoryRequest, err := resource.ParseQuantity(mgmtMemoryRequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse mgmtMemoryRequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["memory"]; ok {
		out.Limits["memory"] = in.Limits["memory"]
	} else {
		out.Limits["memory"] = userMemoryRequest
	}
	if _, ok := in.Requests["memory"]; ok {
		out.Requests["memory"] = in.Requests["memory"]
	} else {
		out.Requests["memory"] = userMemoryRequest
	}

	userCPURequest, err := resource.ParseQuantity(mgmtCPURequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse mgmtCPURequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["cpu"]; ok {
		out.Limits["cpu"] = in.Limits["cpu"]
	} else {
		out.Limits["cpu"] = userCPURequest
	}
	if _, ok := in.Requests["cpu"]; ok {
		out.Requests["cpu"] = in.Requests["cpu"]
	} else {
		out.Requests["cpu"] = userCPURequest
	}

	return err, out
}

// Determine the resource requests/limits for an "exporter" container
func setExporterResources(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client, in corev1.ResourceRequirements) (error, corev1.ResourceRequirements) {
	us := "setExporterResources"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")
	containerName := "exporter"

	var err error
	var out corev1.ResourceRequirements
	out.Limits = make(corev1.ResourceList)
	out.Requests = make(corev1.ResourceList)

	reqLogger.V(2).Info(fmt.Sprintf("in: %+v", in))

	// Fetch some data from the TimesTenClassic / TimesTenScaleout
	// object that requires some casting to get to

	var exporterMemoryRequest string
	var exporterCPURequest string

	switch v := instance.(type) {
	case *timestenv2.TimesTenClassic:
		exporterMemoryRequest = v.Spec.TTSpec.ExporterMemoryRequest
		exporterCPURequest = v.Spec.TTSpec.ExporterCPURequest

	default:
		return errors.New(fmt.Sprintf("%s: passed unexpected type %T", us, v)), out
	}

	zeroSz := resource.MustParse("0")

	if lim, ok := in.Limits["hugepages-1Gi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-1Gi"] = zeroSz
	if req, ok := in.Requests["hugepages-1Gi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-1Gi"] = zeroSz
	if lim, ok := in.Limits["hugepages-2Mi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-2Mi"] = zeroSz
	if req, ok := in.Requests["hugepages-2Mi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-2Mi"] = zeroSz

	userMemoryRequest, err := resource.ParseQuantity(exporterMemoryRequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse exporterMemoryRequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["memory"]; ok {
		out.Limits["memory"] = in.Limits["memory"]
	} else {
		out.Limits["memory"] = userMemoryRequest
	}
	if _, ok := in.Requests["memory"]; ok {
		out.Requests["memory"] = in.Requests["memory"]
	} else {
		out.Requests["memory"] = userMemoryRequest
	}

	userCPURequest, err := resource.ParseQuantity(exporterCPURequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse exporterCPURequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["cpu"]; ok {
		out.Limits["cpu"] = in.Limits["cpu"]
	} else {
		out.Limits["cpu"] = userCPURequest
	}
	if _, ok := in.Requests["cpu"]; ok {
		out.Requests["cpu"] = in.Requests["cpu"]
	} else {
		out.Requests["cpu"] = userCPURequest
	}

	return err, out
}

// Determine the resource requests/limits for an "exporter" container
func setZookeeperResources(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client, in corev1.ResourceRequirements) (error, corev1.ResourceRequirements) {
	us := "setZookeeperResources"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")
	containerName := "zookeeper"

	var err error
	var out corev1.ResourceRequirements
	out.Limits = make(corev1.ResourceList)
	out.Requests = make(corev1.ResourceList)

	reqLogger.V(2).Info(fmt.Sprintf("in: %+v", in))

	// Fetch some data from the TimesTenClassic / TimesTenScaleout
	// object that requires some casting to get to

	var zookeeperMemoryRequest string
	var zookeeperCPURequest string

	zeroSz := resource.MustParse("0")

	if lim, ok := in.Limits["hugepages-1Gi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-1Gi"] = zeroSz
	if req, ok := in.Requests["hugepages-1Gi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-1Gi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-1Gi"] = zeroSz
	if lim, ok := in.Limits["hugepages-2Mi"]; ok {
		if lim.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi limit for '%s' container", containerName), true)
		}
	}
	out.Limits["hugepages-2Mi"] = zeroSz
	if req, ok := in.Requests["hugepages-2Mi"]; ok {
		if req.IsZero() == false {
			logTTEvent(ctx, client, instance, "Create",
				fmt.Sprintf("Warning: ignoring user specified hugepages-2Mi request for '%s' container", containerName), true)
		}
	}
	out.Requests["hugepages-2Mi"] = zeroSz

	userMemoryRequest, err := resource.ParseQuantity(zookeeperMemoryRequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse zookeeperMemoryRequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["memory"]; ok {
		out.Limits["memory"] = in.Limits["memory"]
	} else {
		out.Limits["memory"] = userMemoryRequest
	}
	if _, ok := in.Requests["memory"]; ok {
		out.Requests["memory"] = in.Requests["memory"]
	} else {
		out.Requests["memory"] = userMemoryRequest
	}

	userCPURequest, err := resource.ParseQuantity(zookeeperCPURequest)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not parse zookeeperCPURequest: %s", err.Error())), out
	}

	if _, ok := in.Limits["cpu"]; ok {
		out.Limits["cpu"] = in.Limits["cpu"]
	} else {
		out.Limits["cpu"] = userCPURequest
	}
	if _, ok := in.Requests["cpu"]; ok {
		out.Requests["cpu"] = in.Requests["cpu"]
	} else {
		out.Requests["cpu"] = userCPURequest
	}

	return err, out
}

// Determine the resource requests/limits for a container
func setContainerResources(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client, containerName string, in corev1.ResourceRequirements) (error, corev1.ResourceRequirements) {
	us := "setContainerResources"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	var out corev1.ResourceRequirements
	out.Limits = make(corev1.ResourceList)
	out.Requests = make(corev1.ResourceList)

	reqLogger.V(2).Info(fmt.Sprintf("in: %+v", in))

	// Fetch some data from the TimesTenClassic / TimesTenScaleout
	// object that requires some casting to get to

	var automaticMemoryRequests bool

	switch v := instance.(type) {
	case *timestenv2.TimesTenClassic:
		automaticMemoryRequests = v.Spec.TTSpec.AutomaticMemoryRequests
	default:
		return errors.New(fmt.Sprintf("%s: passed unexpected type %T", us, v)), out
	}

	// If the user said "just do what I say" ... then just do what she said

	if automaticMemoryRequests == false {
		reqLogger.V(2).Info("automaticMemoryRequests=false, using user provided resources specification")
		return nil, in
	}

	switch containerName {
	case "tt":
		return setTTResources(ctx, instance, client, in)
	case "mgmt":
		return setMgmtResources(ctx, instance, client, in)
	case "daemonlog":
		return setDaemonLogResources(ctx, instance, client, in)
	case "exporter":
		return setExporterResources(ctx, instance, client, in)
	case "zookeeper":
		return setZookeeperResources(ctx, instance, client, in)
	default:
		return errors.New(fmt.Sprintf("%s: passed unexpected container name %s", us, containerName)), out
	}
}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
