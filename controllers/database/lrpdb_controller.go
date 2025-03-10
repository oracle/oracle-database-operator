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
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"

	//"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	lrcommons "github.com/oracle/oracle-database-operator/commons/multitenant/lrest"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Bitmask functions
const (
	MPAPPL = 0x00000001 /* The map config has been applyed */
	MPSYNC = 0x00000002 /* The map config is in sync with v$parameters where is default=flase */
	MPEMPT = 0x00000004 /* The map is empty - not specify */
	MPWARN = 0x00000008 /* Map applied with warnings */
	MPINIT = 0x00000010 /* Config map init */
	SPARE3 = 0x00000020
)

func bis(bitmask int, bitval int) int {
	bitmask = ((bitmask) | (bitval))
	return bitmask
}

func bit(bitmask int, bitval int) bool {
	if bitmask&bitval != 0 {
		return true
	} else {
		return false
	}
}

func bid(bitmask int, bitval int) int {
	bitmask ^= ((bitval) & (bitmask))
	return bitmask
}

func bitmaskprint(bitmask int) string {
	BitRead := "|"
	if bit(bitmask, MPAPPL) {
		BitRead = strings.Join([]string{BitRead, "MPAPPL|"}, "")
	}
	if bit(bitmask, MPSYNC) {
		BitRead = strings.Join([]string{BitRead, "MPSYNC|"}, "")
	}
	if bit(bitmask, MPEMPT) {
		BitRead = strings.Join([]string{BitRead, "MPEMPT|"}, "")
	}
	if bit(bitmask, MPWARN) {
		BitRead = strings.Join([]string{BitRead, "MPWARN|"}, "")
	}
	if bit(bitmask, MPINIT) {
		BitRead = strings.Join([]string{BitRead, "MPINIT|"}, "")
	}
	if bit(bitmask, SPARE3) {
		BitRead = strings.Join([]string{BitRead, "SPARE3|"}, "")
	}

	return BitRead
}

// LRPDBReconciler reconciles a LRPDB object
type LRPDBReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Interval time.Duration
	Recorder record.EventRecorder
}

type restSQLCollection struct {
	Env struct {
		DefaultTimeZone string `json:"defaultTimeZone,omitempty"`
	} `json:"env"`
	Items []SQL_Item `json:"items"`
}

type SQL_Item struct {
	StatementId  int      `json:"statementId,omitempty"`
	Response     []string `json:"response"`
	ErrorCode    int      `json:"errorCode,omitempty"`
	ErrorLine    int      `json:"errorLine,omitempty"`
	ErrorColumn  int      `json:"errorColumn,omitempty"`
	ErrorDetails string   `json:"errorDetails,omitempty"`
	Result       int      `json:"result,omitempty"`
}

type LRESTError struct {
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Type     string `json:"type,omitempty"`
	Instance string `json:"instance,omitempty"`
}

var (
	lrpdbPhaseCreate    = "Creating"
	lrpdbPhasePlug      = "Plugging"
	lrpdbPhaseUnplug    = "Unplugging"
	lrpdbPhaseClone     = "Cloning"
	lrpdbPhaseFinish    = "Finishing"
	lrpdbPhaseReady     = "Ready"
	lrpdbPhaseDelete    = "Deleting"
	lrpdbPhaseModify    = "Modifying"
	lrpdbPhaseMap       = "Mapping"
	lrpdbPhaseStatus    = "CheckingState"
	lrpdbPhaseFail      = "Failed"
	lrpdbPhaseAlterPlug = "AlterPlugDb"
	lrpdbPhaseSpare     = "NoAction"
)

const LRPDBFinalizer = "database.oracle.com/LRPDBfinalizer"

var tde_Password string
var tde_Secret string
var flood_control bool = false
var assertiveLpdbDeletion bool = false /* Global variable for assertive pdb deletion */
/*
	        We need to record the config map name after pdb creation
		in order to use it during open and clone op if config map
		name is not set the open and clone yaml file
*/
var globalconfigmap string
var globalsqlcode int

/* mind  https://github.com/kubernetes-sigs/kubebuilder/issues/549 */
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrpdbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrpdbs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrpdbs/finalizers,verbs=get;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LRPDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *LRPDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("multitenantoperator", req.NamespacedName)
	log.Info("Reconcile requested")

	reconcilePeriod := r.Interval * time.Second
	requeueY := ctrl.Result{Requeue: true, RequeueAfter: reconcilePeriod}
	requeueN := ctrl.Result{}

	var err error
	lrpdb := &dbapi.LRPDB{}

	// Execute for every reconcile
	defer func() {
		//log.Info("DEFER LRPDB", "Name", lrpdb.Name, "Phase", lrpdb.Status.Phase, "Status", strconv.FormatBool(lrpdb.Status.Status))
		if !lrpdb.Status.Status {
			if lrpdb.Status.Phase == lrpdbPhaseReady {
				lrpdb.Status.Status = true
			}
			if err := r.Status().Update(ctx, lrpdb); err != nil {
				log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
			}
		}
	}()

	err = r.Client.Get(context.TODO(), req.NamespacedName, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("LRPDB Resource Not found", "Name", lrpdb.Name)
			// Request object not found, could have been deleted after reconcile req.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			lrpdb.Status.Status = true
			return requeueN, nil
		}
		// Error reading the object - requeue the req.
		return requeueY, err
	}

	// Finalizer section
	err = r.manageLRPDBDeletion2(ctx, req, lrpdb)
	if err != nil {
		log.Info("Reconcile queued")
		return requeueY, nil
	}

	// Check for Duplicate LRPDB
	if !lrpdb.Status.Status {
		err = r.checkDuplicateLRPDB(ctx, req, lrpdb)
		if err != nil {
			return requeueN, nil
		}
	}

	action := strings.ToUpper(lrpdb.Spec.Action)
	/*
		       	Bug 36714702 - LREST OPERATOR - POST ALTER PDB OPTION LRPDB STATUS INTERMITTENTLY
			  	               SHOWS "WAITING FOR LRPDB PARAMETER TO BE MODIFIED"
		        introducing additional check to avoid alter system repetition during
			reconciliation loop
	*/
	if lrpdb.Status.Phase == lrpdbPhaseReady {
		if (lrpdb.Status.Action != "" || action != "NOACTION") && (action == "ALTER" || action == "MODIFY" || action == "STATUS" || lrpdb.Status.Action != action) {
			lrpdb.Status.Status = false
		} else {
			err = r.getLRPDBState(ctx, req, lrpdb)
			if err != nil {
				lrpdb.Status.Phase = lrpdbPhaseFail
			} else {
				lrpdb.Status.Phase = lrpdbPhaseReady
				lrpdb.Status.Msg = "Success"
			}
			r.Status().Update(ctx, lrpdb)
		}
	}

	if !lrpdb.Status.Status {
		r.validatePhase(ctx, req, lrpdb)
		phase := lrpdb.Status.Phase
		log.Info("LRPDB:", "Name", lrpdb.Name, "Phase", phase, "Status", strconv.FormatBool(lrpdb.Status.Status))

		switch phase {
		case lrpdbPhaseCreate:
			err = r.createLRPDB(ctx, req, lrpdb)
		case lrpdbPhaseClone:
			err = r.cloneLRPDB(ctx, req, lrpdb)
		case lrpdbPhasePlug:
			err = r.plugLRPDB(ctx, req, lrpdb)
		case lrpdbPhaseUnplug:
			err = r.unplugLRPDB(ctx, req, lrpdb)
		case lrpdbPhaseModify:
			err = r.modifyLRPDB(ctx, req, lrpdb)
		case lrpdbPhaseDelete:
			err = r.deleteLRPDB(ctx, req, lrpdb)
		case lrpdbPhaseStatus:
			err = r.getLRPDBState(ctx, req, lrpdb)
		case lrpdbPhaseMap:
			err = r.mapLRPDB(ctx, req, lrpdb)
		case lrpdbPhaseFail:
			err = r.mapLRPDB(ctx, req, lrpdb)
		case lrpdbPhaseAlterPlug:
			err = r.alterSystemLRPDB(ctx, req, lrpdb)
		default:
			log.Info("DEFAULT:", "Name", lrpdb.Name, "Phase", phase, "Status", strconv.FormatBool(lrpdb.Status.Status))
			return requeueN, nil
		}
		lrpdb.Status.Action = strings.ToUpper(lrpdb.Spec.Action)
		if err != nil {
			lrpdb.Status.Phase = lrpdbPhaseFail
			lrpdb.Status.SqlCode = globalsqlcode
		} else {
			lrpdb.Status.Phase = lrpdbPhaseReady
			lrpdb.Status.Msg = "Success"
		}
	}

	r.ManageConfigMapForCloningAndPlugin(ctx, req, lrpdb)
	lrpdb.Status.BitStatStr = bitmaskprint(lrpdb.Status.Bitstat)

	log.Info("Reconcile completed")
	return requeueY, nil
}

/*
************************************************
  - Validate the LRPDB Spec
    /***********************************************
*/
func (r *LRPDBReconciler) validatePhase(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) {

	log := r.Log.WithValues("validatePhase", req.NamespacedName)

	action := strings.ToUpper(lrpdb.Spec.Action)

	log.Info("Validating LRPDB phase for: "+lrpdb.Name, "Action", action)

	switch action {
	case "CREATE":
		lrpdb.Status.Phase = lrpdbPhaseCreate
	case "CLONE":
		lrpdb.Status.Phase = lrpdbPhaseClone
	case "PLUG":
		lrpdb.Status.Phase = lrpdbPhasePlug
	case "UNPLUG":
		lrpdb.Status.Phase = lrpdbPhaseUnplug
	case "MODIFY":
		lrpdb.Status.Phase = lrpdbPhaseModify
	case "DELETE":
		lrpdb.Status.Phase = lrpdbPhaseDelete
	case "STATUS":
		lrpdb.Status.Phase = lrpdbPhaseStatus
	case "MAP":
		lrpdb.Status.Phase = lrpdbPhaseMap
	case "ALTER":
		lrpdb.Status.Phase = lrpdbPhaseAlterPlug
	case "NOACTION":
		lrpdb.Status.Phase = lrpdbPhaseStatus

	}

	log.Info("Validation complete")
}

/*
   This function scans the list of crd
   pdb to verify the existence of the
   pdb (crd) that we want to clone.
   Bug 36752925 - LREST OPERATOR - CLONE NON-EXISTENT
                  PDB CREATES A LRPDB WITH STATUS FAILED

   return 1 - CRD found
   return 0 - CRD not found / Stop clone process

   Bug 36753107 - LREST OPERATOR - CLONE
                  CLOSED PDB SUCCESSFULLY CLONES

*/

func (r *LRPDBReconciler) checkPDBforCloninig(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, targetPdbName string) (int, error) {
	log := r.Log.WithValues("checkDuplicateLRPDB", req.NamespacedName)
	var pdbCounter int
	pdbCounter = 0

	lrpdbList := &dbapi.LRPDBList{}
	listOpts := []client.ListOption{client.InNamespace(req.Namespace), client.MatchingFields{"spec.pdbName": targetPdbName}}
	err := r.List(ctx, lrpdbList, listOpts...)
	if err != nil {
		log.Info("Failed to list lrpdbs", "Namespace", req.Namespace, "Error", err)
		return 0, err
	}
	if len(lrpdbList.Items) == 0 {
		log.Info("No pdbs  available")
		return pdbCounter, err
	}

	for _, p := range lrpdbList.Items {
		fmt.Printf("DEBUGCLONE %s %s %i\n", p.Spec.LRPDBName, targetPdbName, pdbCounter)
		if p.Spec.LRPDBName == targetPdbName {
			log.Info("Found " + targetPdbName + " in the crd list")
			if p.Status.OpenMode == "MOUNTED" {
				log.Info("Cannot clone a mounted pdb")
				return pdbCounter, err
			}
			pdbCounter++
			fmt.Printf("DEBUGCLONE %s %s %i\n", p.Spec.LRPDBName, targetPdbName, pdbCounter)
			return pdbCounter, err
		}

	}
	return pdbCounter, err
}

/*
***************************************************************
  - Check for Duplicate LRPDB. Same LRPDB name on the same LREST resource.

/**************************************************************
*/
func (r *LRPDBReconciler) checkDuplicateLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("checkDuplicateLRPDB", req.NamespacedName)

	// Name of the LREST CR that holds the LREST container
	lrestResName := lrpdb.Spec.CDBResName
	//lrestame := lrpdb.Spec.LRESTName

	// Name of the LRPDB resource
	lrpdbResName := lrpdb.Spec.LRPDBName

	lrpdbList := &dbapi.LRPDBList{}

	listOpts := []client.ListOption{client.InNamespace(req.Namespace), client.MatchingFields{"spec.pdbName": lrpdbResName}}

	// List retrieves list of objects for a given namespace and list options.
	err := r.List(ctx, lrpdbList, listOpts...)
	if err != nil {
		log.Info("Failed to list lrpdbs", "Namespace", req.Namespace, "Error", err)
		return err
	}

	if len(lrpdbList.Items) == 0 {
		log.Info("No lrpdbs found for LRPDBName: "+lrpdbResName, "CDBResName", lrestResName)
		return nil
	}

	for _, p := range lrpdbList.Items {
		log.Info("Found LRPDB: " + p.Name)
		if (p.Name != lrpdb.Name) && (p.Spec.CDBResName == lrestResName) {
			log.Info("Duplicate LRPDB found")
			lrpdb.Status.Msg = "LRPDB Resource already exists"
			lrpdb.Status.Status = false
			lrpdb.Status.Phase = lrpdbPhaseFail
			return errors.New("Duplicate LRPDB found")
		}
	}
	return nil
}

/*
***************************************************************
  - Get the Custom Resource for the LREST mentioned in the LRPDB Spec
    /**************************************************************
*/
func (r *LRPDBReconciler) getLRESTResource(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) (dbapi.LREST, error) {

	log := r.Log.WithValues("getLRESTResource", req.NamespacedName)

	var lrest dbapi.LREST // LREST CR corresponding to the LREST name specified in the LRPDB spec

	// Name of the LREST CR that holds the LREST container
	lrestResName := lrpdb.Spec.CDBResName
	lrestNamespace := lrpdb.Spec.CDBNamespace

	log.Info("lrestResName...........:" + lrestResName)
	log.Info("lrestNamespace.........:" + lrestNamespace)

	// Get LREST CR corresponding to the LREST name specified in the LRPDB spec
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: lrestNamespace,
		Name:      lrestResName,
	}, &lrest)

	if err != nil {
		log.Info("Failed to get CRD for LREST", "Name", lrestResName, "Namespace", lrestNamespace, "Error", err.Error())
		lrpdb.Status.Msg = "Unable to get CRD for LREST : " + lrestResName
		r.Status().Update(ctx, lrpdb)
		return lrest, err
	}

	log.Info("Found CR for LREST", "Name", lrestResName, "CR Name", lrest.Name)
	return lrest, nil
}

/*
***************************************************************
  - Get the LREST Pod for the LREST mentioned in the LRPDB Spec
    /**************************************************************
*/
func (r *LRPDBReconciler) getLRESTPod(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) (corev1.Pod, error) {

	log := r.Log.WithValues("getLRESTPod", req.NamespacedName)

	var lrestPod corev1.Pod // LREST Pod container with connection to the concerned LREST

	// Name of the LREST CR that holds the LREST container
	lrestResName := lrpdb.Spec.CDBResName

	// Get LREST Pod associated with the LREST Name specified in the LRPDB Spec
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: req.Namespace,
		Name:      lrestResName + "-lrest",
	}, &lrestPod)

	if err != nil {
		log.Info("Failed to get Pod for LREST", "Name", lrestResName, "Namespace", req.Namespace, "Error", err.Error())
		lrpdb.Status.Msg = "Unable to get LREST Pod for LREST : " + lrestResName
		return lrestPod, err
	}

	log.Info("Found LREST Pod for LREST", "Name", lrestResName, "Pod Name", lrestPod.Name, "LREST Container hostname", lrestPod.Spec.Hostname)
	return lrestPod, nil
}

/*
************************************************
  - Get Secret Key for a Secret Name
    /***********************************************
*/
func (r *LRPDBReconciler) getSecret(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, secretName string, keyName string) (string, error) {

	log := r.Log.WithValues("getSecret", req.NamespacedName)

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: lrpdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + secretName)
			lrpdb.Status.Msg = "Secret not found:" + secretName
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	return string(secret.Data[keyName]), nil
}

/*
************************************************
  - Issue a REST API Call to the LREST container
    /***********************************************
*/
func (r *LRPDBReconciler) callAPI(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, url string, payload map[string]string, action string) (string, error) {
	log := r.Log.WithValues("callAPI", req.NamespacedName)

	var err error

	secret := &corev1.Secret{}

	err = r.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.LRPDBTlsKey.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.LRPDBTlsKey.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	rsaKeyPEM := secret.Data[lrpdb.Spec.LRPDBTlsKey.Secret.Key]

	err = r.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.LRPDBTlsCrt.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.LRPDBTlsCrt.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	rsaCertPEM := secret.Data[lrpdb.Spec.LRPDBTlsCrt.Secret.Key]

	err = r.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.LRPDBTlsCat.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.LRPDBTlsCat.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	caCert := secret.Data[lrpdb.Spec.LRPDBTlsCat.Secret.Key]
	/*
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTINFO", string(rsaKeyPEM))
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTINFO", string(rsaCertPEM))
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTINFO", string(caCert))
	*/

	certificate, err := tls.X509KeyPair([]byte(rsaCertPEM), []byte(rsaKeyPEM))
	if err != nil {
		lrpdb.Status.Msg = "Error tls.X509KeyPair"
		return "", err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	/*
			tlsConf := &tls.Config{Certificates: []tls.Certificate{certificate},
		                               RootCAs: caCertPool}
	*/
	tlsConf := &tls.Config{Certificates: []tls.Certificate{certificate},
		RootCAs: caCertPool,
		//MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}

	tr := &http.Transport{TLSClientConfig: tlsConf}

	httpclient := &http.Client{Transport: tr}

	log.Info("Issuing REST call", "URL", url, "Action", action)

	webUser, err := r.getEncriptedSecret(ctx, req, lrpdb, lrpdb.Spec.WebLrpdbServerUser.Secret.SecretName, lrpdb.Spec.WebLrpdbServerUser.Secret.Key, lrpdb.Spec.LRPDBPriKey.Secret.SecretName, lrpdb.Spec.LRPDBPriKey.Secret.Key)
	if err != nil {
		log.Error(err, "Unable to get webuser account name   ")
		return "", err
	}

	webUserPwd, err := r.getEncriptedSecret(ctx, req, lrpdb, lrpdb.Spec.WebLrpdbServerPwd.Secret.SecretName, lrpdb.Spec.WebLrpdbServerPwd.Secret.Key, lrpdb.Spec.LRPDBPriKey.Secret.SecretName, lrpdb.Spec.LRPDBPriKey.Secret.Key)
	if err != nil {
		log.Error(err, "Unable to get webuser account password  ")
		return "", err
	}

	var httpreq *http.Request
	if action == "GET" {
		httpreq, err = http.NewRequest(action, url, nil)
	} else {
		jsonValue, _ := json.Marshal(payload)
		httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
	}

	if err != nil {
		log.Info("Unable to create HTTP Request for LRPDB : "+lrpdb.Name, "err", err.Error())
		return "", err
	}

	httpreq.Header.Add("Accept", "application/json")
	httpreq.Header.Add("Content-Type", "application/json")
	httpreq.SetBasicAuth(webUser, webUserPwd)

	resp, err := httpclient.Do(httpreq)
	if err != nil {
		errmsg := err.Error()
		log.Error(err, "Failed - Could not connect to LREST Pod", "err", err.Error())
		lrpdb.Status.Msg = "Error: Could not connect to LREST Pod"
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTError", errmsg)
		return "", err
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "Done", lrpdb.Spec.CDBResName)
	if resp.StatusCode != http.StatusOK {
		bb, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode == 404 {
			lrpdb.Status.ConnString = ""
			lrpdb.Status.Msg = lrpdb.Spec.LRPDBName + " not found"

		} else {
			if flood_control == false {
				lrpdb.Status.Msg = "LREST Error - HTTP Status Code:" + strconv.Itoa(resp.StatusCode)
			}
		}

		if flood_control == false {
			log.Info("LREST Error - HTTP Status Code :"+strconv.Itoa(resp.StatusCode), "Err", string(bb))
		}

		var apiErr LRESTError
		json.Unmarshal([]byte(bb), &apiErr)
		if flood_control == false {
			r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTError", "Failed: %s", apiErr.Message)
		}
		fmt.Printf("\n================== APIERR ======================\n")
		fmt.Printf("%+v \n", apiErr)
		fmt.Printf(string(bb))
		fmt.Printf("URL=%s\n", url)
		fmt.Printf("resp.StatusCode=%s\n", strconv.Itoa(resp.StatusCode))
		fmt.Printf("\n================== APIERR ======================\n")
		flood_control = true
		return "", errors.New("LREST Error")
	}
	flood_control = false

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Print(err.Error())
	}
	respData := string(bodyBytes)
	fmt.Print("CALL API return msg.....:")
	fmt.Println(string(bodyBytes))

	var apiResponse restSQLCollection
	json.Unmarshal([]byte(bodyBytes), &apiResponse)
	fmt.Printf("===> %#v\n", apiResponse)
	fmt.Printf("===> %+v\n", apiResponse)

	errFound := false
	for _, sqlItem := range apiResponse.Items {
		if sqlItem.ErrorDetails != "" {
			log.Info("LREST Error - Oracle Error Code :" + strconv.Itoa(sqlItem.ErrorCode))
			if !errFound {
				lrpdb.Status.Msg = sqlItem.ErrorDetails
			}
			r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "OraError", "%s", sqlItem.ErrorDetails)
			errFound = true
		}
	}

	if errFound {
		return "", errors.New("Oracle Error")
	}

	return respData, nil
}

/*
************************************************
  - Create a LRPDB

***********************************************
*/
func (r *LRPDBReconciler) createLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("createLRPDB", req.NamespacedName)

	var err error
	var tde_Password string
	var tde_Secret string

	log.Info("call  getLRESTResource \n")
	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	lrpdbAdminName, err := r.getEncriptedSecret(ctx, req, lrpdb, lrpdb.Spec.AdminpdbUser.Secret.SecretName, lrpdb.Spec.AdminpdbUser.Secret.Key, lrpdb.Spec.LRPDBPriKey.Secret.SecretName, lrpdb.Spec.LRPDBPriKey.Secret.Key)
	if err != nil {
		log.Error(err, "Unable to find pdb admin user ")
		return err
	}

	lrpdbAdminPwd, err := r.getEncriptedSecret(ctx, req, lrpdb, lrpdb.Spec.AdminpdbPass.Secret.SecretName, lrpdb.Spec.AdminpdbPass.Secret.Key, lrpdb.Spec.LRPDBPriKey.Secret.SecretName, lrpdb.Spec.LRPDBPriKey.Secret.Key)

	if err != nil {
		log.Error(err, "Unable to find pdb admin password ")
		return err
	}

	err = r.getLRPDBState(ctx, req, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Check LRPDB not existence completed", "LRPDB Name", lrpdb.Spec.LRPDBName)
		}

	} else {

		lrpdb.Status.Phase = lrpdbPhaseFail
		lrpdb.Status.Msg = "PDB " + lrpdb.Spec.LRPDBName + " already exists "
		if err := r.Status().Update(ctx, lrpdb); err != nil {
			log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
		}
		log.Info("Database already exists ", "LRPDB Name", lrpdb.Spec.LRPDBName)
		err := fmt.Errorf("%v", 65012)
		return err
	}

	values := map[string]string{
		"method":              "CREATE",
		"pdb_name":            lrpdb.Spec.LRPDBName,
		"adminName":           lrpdbAdminName,
		"adminPwd":            lrpdbAdminPwd,
		"fileNameConversions": lrpdb.Spec.FileNameConversions,
		"reuseTempFile":       strconv.FormatBool(*(lrpdb.Spec.ReuseTempFile)),
		"unlimitedStorage":    strconv.FormatBool(*(lrpdb.Spec.UnlimitedStorage)),
		"totalSize":           lrpdb.Spec.TotalSize,
		"tempSize":            lrpdb.Spec.TempSize,
		"getScript":           strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	fmt.Printf("===== PAYLOAD ===\n")
	fmt.Print(" method ", values["method"], "\n")
	fmt.Print(" pdb_name ", values["pdb_name"], "\n")
	fmt.Print(" adminName ", values["adminName"], "\n")
	fmt.Print(" adminPwd --------------\n")
	fmt.Print(" fileNameConversions ", values["fileNameConversions"], "\n")
	fmt.Print(" unlimitedStorage ", values["unlimitedStorage"], "\n")
	fmt.Print(" reuseTempFile ", values["reuseTempFile"], "\n")
	fmt.Print(" tempSize ", values["tempSize"], "\n")
	fmt.Print(" totalSize ", values["totalSize"], "\n")
	fmt.Print(" getScript ", values["getScript"], "\n")

	if *(lrpdb.Spec.LTDEImport) {
		tde_Password, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDEPassword.Secret.SecretName, lrpdb.Spec.LTDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tde_Secret, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDESecret.Secret.SecretName, lrpdb.Spec.LTDESecret.Secret.Key)
		if err != nil {
			return err
		}

		tde_Secret = tde_Secret[:len(tde_Secret)-1]
		tde_Password = tde_Secret[:len(tde_Password)-1]
		values["tde_Password"] = tde_Password
		values["tdeKeystorePath"] = lrpdb.Spec.LTDEKeystorePath
		values["tde_Secret"] = tde_Secret
	}

	//url := "https://" + lrpdb.Spec.CDBResName + "-lrest:" + strconv.Itoa(lrest.Spec.LRESTPort) + "/database/pdbs/"
	url := r.BaseUrl(ctx, req, lrpdb, lrest)
	fmt.Print("============================================================\n")
	fmt.Print(url)
	fmt.Print("\n============================================================\n")
	lrpdb.Status.TotalSize = lrpdb.Spec.TotalSize
	lrpdb.Status.Phase = lrpdbPhaseCreate
	lrpdb.Status.Msg = "Waiting for LRPDB to be created"

	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode
	if lrpdb.Status.SqlCode != 0 {
		err := fmt.Errorf("%v", lrpdb.Status.SqlCode)
		return err
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal,
		"Created", "LRPDB '%s' created successfully", lrpdb.Spec.LRPDBName)

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString =
			lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		log.Info("Parsing connectstring")
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))
	}

	assertiveLpdbDeletion = lrpdb.Spec.AssertiveLrpdbDeletion
	if lrpdb.Spec.AssertiveLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "PDB '%s' assertive pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	r.getLRPDBState(ctx, req, lrpdb)
	log.Info("Created LRPDB Resource", "LRPDB Name", lrpdb.Spec.LRPDBName)

	if bit(lrpdb.Status.Bitstat, MPINIT) == false {
		r.InitConfigMap(ctx, req, lrpdb)
		Cardinality, _ := r.ApplyConfigMap(ctx, req, lrpdb)
		log.Info("Config Map Cardinality " + strconv.Itoa(int(Cardinality)))
	}

	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	return nil
}

/*
************************************************
  - Clone a LRPDB
    /***********************************************
*/
func (r *LRPDBReconciler) cloneLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	if lrpdb.Spec.LRPDBName == lrpdb.Spec.SrcLRPDBName {
		return nil
	}

	log := r.Log.WithValues("cloneLRPDB", req.NamespacedName)

	globalsqlcode = 0
	var err error

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	/* Prevent cloning an existing lrpdb */
	err = r.getLRPDBState(ctx, req, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Check LRPDB not existence completed", "LRPDB Name", lrpdb.Spec.LRPDBName)
		}

	} else {
		log.Info("Database already exists ", "LRPDB Name", lrpdb.Spec.LRPDBName)
		return nil
	}

	values := map[string]string{
		"method":           "CLONE",
		"pdb_name":         lrpdb.Spec.LRPDBName,
		"srcPdbName":       lrpdb.Spec.SrcLRPDBName,
		"reuseTempFile":    strconv.FormatBool(*(lrpdb.Spec.ReuseTempFile)),
		"unlimitedStorage": strconv.FormatBool(*(lrpdb.Spec.UnlimitedStorage)),
		"getScript":        strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	//* check the existence of lrpdb.Spec.SrcLRPDBName //
	var allErrs field.ErrorList
	pdbCounter, _ := r.checkPDBforCloninig(ctx, req, lrpdb, lrpdb.Spec.SrcLRPDBName)
	if pdbCounter == 0 {
		log.Info("target pdb " + lrpdb.Spec.SrcLRPDBName + " does not exists or is not open")
		allErrs = append(allErrs, field.NotFound(field.NewPath("Spec").Child("LRPDBName"), " "+lrpdb.Spec.LRPDBName+" does not exist :  failure"))
		r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
		return nil
	}

	if lrpdb.Spec.SparseClonePath != "" {
		values["sparseClonePath"] = lrpdb.Spec.SparseClonePath
	}
	if lrpdb.Spec.FileNameConversions != "" {
		values["fileNameConversions"] = lrpdb.Spec.FileNameConversions
	}
	if lrpdb.Spec.TotalSize != "" {
		values["totalSize"] = lrpdb.Spec.TotalSize
	}
	if lrpdb.Spec.TempSize != "" {
		values["tempSize"] = lrpdb.Spec.TempSize
	}

	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdb.Spec.LRPDBName + "/"

	lrpdb.Status.Phase = lrpdbPhaseClone
	lrpdb.Status.Msg = "Waiting for LRPDB to be cloned"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}
	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode != 0 {
		errclone := errors.New("Cannot clone database: ora-" + strconv.Itoa(lrpdb.Status.SqlCode))
		log.Info("Cannot clone database ora-" + strconv.Itoa(lrpdb.Status.SqlCode))
		lrpdb.Status.Msg = lrpdb.Spec.SrcLRPDBName + " is open in mount cannot clone "
		if err := r.Status().Update(ctx, lrpdb); err != nil {
			log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
		}
		return errclone
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "LRPDB '%s' cloned successfully", lrpdb.Spec.LRPDBName)

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))

	}
	assertiveLpdbDeletion = lrpdb.Spec.AssertiveLrpdbDeletion
	if lrpdb.Spec.AssertiveLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Clone", "PDB '%s' assertive pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	log.Info("Cloned LRPDB successfully", "Source LRPDB Name", lrpdb.Spec.SrcLRPDBName, "Clone LRPDB Name", lrpdb.Spec.LRPDBName)
	r.getLRPDBState(ctx, req, lrpdb)
	return nil
}

/*
************************************************
  - Plug a LRPDB

***********************************************
*/
func (r *LRPDBReconciler) plugLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("plugLRPDB", req.NamespacedName)
	globalsqlcode = 0

	var err error
	var tde_Password string
	var tde_Secret string

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"method":                    "PLUG",
		"xmlFileName":               lrpdb.Spec.XMLFileName,
		"pdb_name":                  lrpdb.Spec.LRPDBName,
		"sourceFileNameConversions": lrpdb.Spec.SourceFileNameConversions,
		"copyAction":                lrpdb.Spec.CopyAction,
		"fileNameConversions":       lrpdb.Spec.FileNameConversions,
		"unlimitedStorage":          strconv.FormatBool(*(lrpdb.Spec.UnlimitedStorage)),
		"reuseTempFile":             strconv.FormatBool(*(lrpdb.Spec.ReuseTempFile)),
		"totalSize":                 lrpdb.Spec.TotalSize,
		"tempSize":                  lrpdb.Spec.TempSize,
		"getScript":                 strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if *(lrpdb.Spec.LTDEImport) {
		tde_Password, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDEPassword.Secret.SecretName, lrpdb.Spec.LTDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tde_Secret, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDESecret.Secret.SecretName, lrpdb.Spec.LTDESecret.Secret.Key)
		if err != nil {
			return err
		}

		tde_Secret = tde_Secret[:len(tde_Secret)-1]
		tde_Password = tde_Secret[:len(tde_Password)-1]
		values["tde_Password"] = tde_Password
		values["tdeKeystorePath"] = lrpdb.Spec.LTDEKeystorePath
		values["tde_Secret"] = tde_Secret
		values["tdeImport"] = strconv.FormatBool(*(lrpdb.Spec.LTDEImport))
	}
	if *(lrpdb.Spec.AsClone) {
		values["asClone"] = strconv.FormatBool(*(lrpdb.Spec.AsClone))
	}

	url := r.BaseUrl(ctx, req, lrpdb, lrest)

	lrpdb.Status.TotalSize = lrpdb.Spec.TotalSize
	lrpdb.Status.Phase = lrpdbPhasePlug
	lrpdb.Status.Msg = "Waiting for LRPDB to be plugged"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode != 0 {
		log.Info("Plug database failure........:" + strconv.Itoa(lrpdb.Status.SqlCode))
		err = fmt.Errorf("%v", lrpdb.Status.SqlCode)
		return err
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "LRPDB '%s' plugged successfully", lrpdb.Spec.LRPDBName)

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		log.Info("Parsing connectstring")
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))
	}

	assertiveLpdbDeletion = lrpdb.Spec.AssertiveLrpdbDeletion
	if lrpdb.Spec.AssertiveLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Plug", "PDB '%s' assertive pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	log.Info("Successfully plugged LRPDB", "LRPDB Name", lrpdb.Spec.LRPDBName)
	r.getLRPDBState(ctx, req, lrpdb)
	return nil
}

/*
************************************************
  - Unplug a LRPDB

***********************************************
*/
func (r *LRPDBReconciler) unplugLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("unplugLRPDB", req.NamespacedName)
	globalsqlcode = 0

	var err error
	var tde_Password string
	var tde_Secret string

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"method":      "UNPLUG",
		"xmlFileName": lrpdb.Spec.XMLFileName,
		"getScript":   strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if *(lrpdb.Spec.LTDEExport) {
		// Get the TDE Password
		tde_Password, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDEPassword.Secret.SecretName, lrpdb.Spec.LTDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tde_Secret, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDESecret.Secret.SecretName, lrpdb.Spec.LTDESecret.Secret.Key)
		if err != nil {
			return err
		}

		tde_Secret = tde_Secret[:len(tde_Secret)-1]
		tde_Password = tde_Secret[:len(tde_Password)-1]
		values["tde_Password"] = tde_Password
		values["tdeKeystorePath"] = lrpdb.Spec.LTDEKeystorePath
		values["tde_Secret"] = tde_Secret
		values["tdeExport"] = strconv.FormatBool(*(lrpdb.Spec.LTDEExport))
	}

	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdb.Spec.LRPDBName + "/"

	log.Info("CallAPI(url)", "url", url)
	lrpdb.Status.Phase = lrpdbPhaseUnplug
	lrpdb.Status.Msg = "Waiting for LRPDB to be unplugged"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}
	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))

	if lrpdb.Status.SqlCode != 0 {
		globalsqlcode = lrpdb.Status.SqlCode

		lrpdb.Status.Msg = lrpdb.Spec.LRPDBName + " database cannot be unplugged  "
		log.Info(lrpdb.Spec.LRPDBName + " database cannot be unplugged  ")
		if lrpdb.Status.SqlCode == 65170 {
			log.Info(lrpdb.Spec.XMLFileName + " xml file already exists  ")
		}

		/*
			err := r.Update(ctx, lrpdb)
			if err != nil {
				log.Info("Fail to update crd", "err", err.Error())
				return err
			}

			if err := r.Status().Update(ctx, lrpdb); err != nil {
				log.Error(err, "Failed to update status"+lrpdb.Name, "err", err.Error())
				return err
			}
		*/

		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Unplugged", " ORA-%s ", strconv.Itoa(lrpdb.Status.SqlCode))
		err = fmt.Errorf("%v", lrpdb.Status.SqlCode)
		return err
	}

	if controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
		log.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
		err = r.Update(ctx, lrpdb)
		if err != nil {
			log.Info("Could not remove finalizer", "err", err.Error())
			return err
		}
		lrpdb.Status.Status = true
		err = r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
		if err != nil {
			log.Info("Could not delete LRPDB resource", "err", err.Error())
			return err
		}
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Unplugged", "LRPDB '%s' unplugged successfully", lrpdb.Spec.LRPDBName)
	globalsqlcode = 0
	log.Info("Successfully unplugged LRPDB resource")
	return nil
}

/**************************************************
Alter system LRPDB
**************************************************/

/**just push the trasnsaction **/
func (r *LRPDBReconciler) alterSystemLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("alterSystemLRPDB", req.NamespacedName)
	globalsqlcode = 0

	var err error
	err = r.getLRPDBState(ctx, req, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Warning LRPDB does not exist", "LRPDB Name", lrpdb.Spec.LRPDBName)
			return nil
		}
		return err
	}

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		log.Info("Cannot find LREST server")
		return err
	}

	/* alter system payload */

	values := map[string]string{
		"state":                "ALTER",
		"alterSystemParameter": lrpdb.Spec.AlterSystemParameter,
		"alterSystemValue":     lrpdb.Spec.AlterSystemValue,
		"parameterScope":       lrpdb.Spec.ParameterScope,
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName
	log.Info("alter system payload...:", "lrpdb.Spec.AlterSystemValue=", lrpdb.Spec.AlterSystemValue)
	log.Info("alter system payload...:", "lrpdb.Spec.AlterSystemParameter=", lrpdb.Spec.AlterSystemParameter)
	log.Info("alter system payload...:", "lrpdb.Spec.ParameterScope=", lrpdb.Spec.ParameterScope)
	log.Info("alter system path.......:", "url=", url)

	lrpdb.Status.Phase = lrpdbPhaseAlterPlug
	lrpdb.Status.ModifyOption = lrpdb.Spec.AlterSystem + " " + lrpdb.Spec.ParameterScope
	lrpdb.Status.Msg = "Waiting for LRPDB parameter to be modified"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update lrpdb parameter  :"+lrpdb.Name, "err", err.Error())
		return err
	}

	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode == 0 {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Altered", "LRPDB(name,cmd,sqlcode) '%s %s %d' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.AlterSystem, lrpdb.Status.SqlCode)
		lrpdb.Status.Phase = lrpdbPhaseReady
		lrpdb.Spec.Action = "Noaction"
		lrpdb.Status.Action = "Noaction"
		lrpdb.Status.Status = true

		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Cannot rest lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
			return err
		}

		if err := r.Status().Update(ctx, lrpdb); err != nil {
			log.Error(err, "Failed to update lrpdb parameter  :"+lrpdb.Name, "err", err.Error())
			return err
		}
		return nil

	}

	if lrpdb.Status.SqlCode != 0 {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "alter system failure", "LRPDB(name,cmd,sqlcode) '%s %s %d' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.AlterSystem, lrpdb.Status.SqlCode)
		erralter := errors.New("Error: cannot modify parameter")

		lrpdb.Status.ModifyOption = lrpdb.Spec.AlterSystem + " " + lrpdb.Spec.ParameterScope
		lrpdb.Status.Msg = "Failed: cannot modify system parameter"
		lrpdb.Status.Phase = lrpdbPhaseStatus
		lrpdb.Spec.AlterSystem = ""
		lrpdb.Spec.ParameterScope = ""
		lrpdb.Spec.Action = "Noaction"
		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Cannot rest lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
			return err
		}

		if err := r.Status().Update(ctx, lrpdb); err != nil {
			log.Error(err, "Failed to update lrpdb parameter  :"+lrpdb.Name, "err", err.Error())
			return err
		}
		return erralter
	}

	lrpdb.Status.Status = false

	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update lrpdb parameter  :"+lrpdb.Name, "err", err.Error())
		return err
	}
	return nil
}

/*************************************************
 * Modify a LRPDB state
 ***********************************************/
func (r *LRPDBReconciler) modifyLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("modifyLRPDB", req.NamespacedName)
	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Modify", "Info:'%s %s %s' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState, lrpdb.Status.ModifyOption)

	var err error
	err = r.getLRPDBState(ctx, req, lrpdb)
	if err != nil {
		if lrpdb.Status.SqlCode == 1403 {
			// BUG 36752465
			// We have to handle to verify a non existings results using both
			log.Info("Database does not exists ")
			r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
			return nil
		}
		if apierrors.IsNotFound(err) {
			log.Info("Warning LRPDB does not exist", "LRPDB Name", lrpdb.Spec.LRPDBName)
			r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
			return nil
		}
		return err
	}

	/* This scenario is managed by webhook acceptance test ... leave it here anyway */
	if lrpdb.Status.OpenMode == "READ WRITE" && lrpdb.Spec.LRPDBState == "OPEN" && lrpdb.Spec.ModifyOption == "READ WRITE" {
		/* Database is already open no action required */
		return nil
	}

	if lrpdb.Status.OpenMode == "MOUNTED" && lrpdb.Spec.LRPDBState == "CLOSE" && lrpdb.Spec.ModifyOption == "IMMEDIATE" {
		/* Database is already close no action required */
		return nil
	}

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	values := map[string]string{}
	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		values = map[string]string{
			"state":        lrpdb.Spec.LRPDBState,
			"modifyOption": lrpdb.Spec.ModifyOption,
			"getScript":    strconv.FormatBool(*(lrpdb.Spec.GetScript))}
		if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
			log.Info("MODIFY LRPDB", "lrpdb.Spec.LRPDBState=", lrpdb.Spec.LRPDBState, "lrpdb.Spec.ModifyOption=", lrpdb.Spec.ModifyOption)
			log.Info("LRPDB STATUS OPENMODE", "lrpdb.Status.OpenMode=", lrpdb.Status.OpenMode)
		}
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName + "/status/"

	lrpdb.Status.Phase = lrpdbPhaseModify
	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		lrpdb.Status.ModifyOption = lrpdb.Spec.LRPDBState + "-" + lrpdb.Spec.ModifyOption
	}

	lrpdb.Status.Msg = "Waiting for LRPDB to be modified"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Modified", " '%s' modified successfully '%s'", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState)
	}

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))

	}

	lrpdb.Status.Msg = "alter lrpdb completed"
	lrpdb.Status.Status = false
	lrpdb.Status.Phase = lrpdbPhaseReady

	log.Info("Successfully modified LRPDB state", "LRPDB Name", lrpdb.Spec.LRPDBName)

	/* After database openining we reapply the config map if warning is present */
	if lrpdb.Spec.LRPDBState == "OPEN" {
		if bit(lrpdb.Status.Bitstat, MPWARN|MPINIT) {
			log.Info("re-apply config map")
			r.ApplyConfigMap(ctx, req, lrpdb)

		}
	}
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	//r.getLRPDBState(ctx, req, lrpdb)
	return nil
}

/*
************************************************
  - Get LRPDB State
    /***********************************************
*/
func (r *LRPDBReconciler) getLRPDBState(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("getLRPDBState", req.NamespacedName)

	var err error

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName + "/status/"

	lrpdb.Status.Msg = "Getting LRPDB state"
	fmt.Print("============================\n")
	fmt.Println(lrpdb.Status)
	fmt.Print("============================\n")
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, nil, "GET")
	if err != nil {
		log.Info("Begin respData")
		log.Info(respData)
		log.Info("End respData")
		lrpdb.Status.Msg = "getLRPDBState failure : check lrpdb status"
		lrpdb.Status.Status = false
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode == 1403 {
		lrpdb.Status.OpenMode = "unknown"
		lrpdb.Status.Msg = "check lrpdb status"
		lrpdb.Status.Status = false
		return errors.New("NO_DATA_FOUND")
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
		log.Error(err, "Failed to get state of LRPDB :"+lrpdbName, "err", err.Error())
	}
	lrpdb.Status.OpenMode = objmap["open_mode"].(string)

	/*	if lrpdb.Status.Phase == lrpdbPhaseCreate && sqlcode == 1403 {

			if lrpdb.Status.OpenMode == "READ WRITE" {
				err := r.mapLRPDB(ctx, req, lrpdb)
				if err != nil {
					log.Info("Fail to Map resource getting LRPDB state")
				}
			}

			if lrpdb.Status.OpenMode == "MOUNTED" {
				err := r.mapLRPDB(ctx, req, lrpdb)
				if err != nil {
					log.Info("Fail to Map resource getting LRPDB state")
				}
			}
	       }*/

	lrpdb.Status.Msg = "check lrpdb ok"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	log.Info("Successfully obtained LRPDB state", "LRPDB Name", lrpdb.Spec.LRPDBName, "State", objmap["open_mode"].(string))
	return nil
}

/*
************************************************
  - Map Database LRPDB to Kubernetes LRPDB CR

/***********************************************
*/
func (r *LRPDBReconciler) mapLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("mapLRPDB", req.NamespacedName)

	var err error

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	log.Info("callapi get to map lrpdb")

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName
	log.Info("DEBUG NEW URL " + url)

	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, nil, "GET")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
		log.Error(err, "Failed json.Unmarshal :"+lrpdbName, "err", err.Error())
	}

	//fmt.Printf("%+v\n", objmap)
	totSizeInBytes := objmap["total_size"].(float64)
	totSizeInGB := totSizeInBytes / 1024 / 1024 / 1024

	lrpdb.Status.OpenMode = objmap["open_mode"].(string)
	lrpdb.Status.TotalSize = fmt.Sprintf("%4.2f", totSizeInGB) + "G"
	assertiveLpdbDeletion = lrpdb.Spec.AssertiveLrpdbDeletion
	if lrpdb.Spec.AssertiveLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Map", "PDB '%s' assertive pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))
	}

	lrpdb.Status.Phase = lrpdbPhaseReady

	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	log.Info("Successfully mapped LRPDB to Kubernetes resource", "LRPDB Name", lrpdb.Spec.LRPDBName)
	lrpdb.Status.Status = true
	return nil
}

/*
************************************************
  - Delete a LRPDB
    /***********************************************
*/
func (r *LRPDBReconciler) deleteLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("deleteLRPDB", req.NamespacedName)

	errstate := r.getLRPDBState(ctx, req, lrpdb)
	if errstate != nil {
		if lrpdb.Status.SqlCode == 1403 {
			// BUG 36752336:
			log.Info("Database does not exists ")
			r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
			return nil
		}
		if apierrors.IsNotFound(errstate) {
			log.Info("Warning LRPDB does not exist", "LRPDB Name", lrpdb.Spec.LRPDBName)
			r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
			return nil
		}
		log.Error(errstate, "Failed to update status for :"+lrpdb.Name, "err", errstate.Error())
		return errstate
		//* if the pdb does not exists delete the crd *//

	}

	if lrpdb.Status.OpenMode == "READ WRITE" {

		errdel := errors.New("pdb is open cannot delete it")
		log.Info("LRPDB is open in read write cannot drop ")
		lrpdb.Status.Msg = "LRPDB is open in read write cannot drop "
		if err := r.Status().Update(ctx, lrpdb); err != nil {
			log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
		}

		return errdel
	}

	err := r.deleteLRPDBInstance(req, ctx, lrpdb)
	if err != nil {
		log.Info("Could not delete LRPDB", "LRPDB Name", lrpdb.Spec.LRPDBName, "err", err.Error())
		return err
	}

	if controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
		log.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
		err := r.Update(ctx, lrpdb)
		if err != nil {
			log.Info("Could not remove finalizer", "err", err.Error())
			return err
		}
		lrpdb.Status.Status = true
		err = r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
		if err != nil {
			log.Info("Could not delete LRPDB resource", "err", err.Error())
			return err
		}
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Deleted", "LRPDB '%s' dropped successfully", lrpdb.Spec.LRPDBName)

	log.Info("Successfully deleted LRPDB resource")
	return nil
}

/*
************************************************
  - Check LRPDB deletion
    /***********************************************
*/
func (r *LRPDBReconciler) manageLRPDBDeletion(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("manageLRPDBDeletion", req.NamespacedName)

	// Check if the LRPDB instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isLRPDBMarkedToBeDeleted := lrpdb.GetDeletionTimestamp() != nil
	if isLRPDBMarkedToBeDeleted {
		log.Info("Marked to be deleted")
		lrpdb.Status.Phase = lrpdbPhaseDelete
		lrpdb.Status.Status = true
		r.Status().Update(ctx, lrpdb)

		if controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
			// Remove LRPDBFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			log.Info("Removing finalizer")
			controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
			err := r.Update(ctx, lrpdb)
			if err != nil {
				log.Info("Could not remove finalizer", "err", err.Error())
				return err
			}
			log.Info("Successfully removed LRPDB resource")
			return nil
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(lrpdb, LRPDBFinalizer)
		err := r.Update(ctx, lrpdb)
		if err != nil {
			log.Info("Could not add finalizer", "err", err.Error())
			return err
		}
		lrpdb.Status.Status = false
	}
	return nil
}

/*
************************************************
  - Finalization logic for LRPDBFinalizer

***********************************************
*/
func (r *LRPDBReconciler) deleteLRPDBInstance(req ctrl.Request, ctx context.Context, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("deleteLRPDBInstance", req.NamespacedName)

	var err error

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"action":    "KEEP",
		"getScript": strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if lrpdb.Spec.DropAction != "" {
		values["action"] = lrpdb.Spec.DropAction
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName + "/"

	lrpdb.Status.Phase = lrpdbPhaseDelete
	lrpdb.Status.Msg = "Waiting for LRPDB to be deleted"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	respData, err := NewCallLAPI(r, ctx, req, lrpdb, url, values, "DELETE")
	if err != nil {
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	log.Info("Successfully dropped LRPDB", "LRPDB Name", lrpdbName)
	return nil
}

/*
***********************************************************
  - SetupWithManager sets up the controller with the Manager

************************************************************
*/
func (r *LRPDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.LRPDB{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Ignore updates to CR status in which case metadata.Generation does not change
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				//return !e.DeleteStateUnknown
				return false
			},
		}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		Complete(r)
}

/*************************************************************
Enh 35357707 - PROVIDE THE LRPDB TNSALIAS INFORMATION
**************************************************************/

func parseTnsAlias(tns *string, lrpdbsrv *string) {
	fmt.Printf("Analyzing string [%s]\n", *tns)
	fmt.Printf("Relacing  srv [%s]\n", *lrpdbsrv)
	var swaptns string

	if strings.Contains(strings.ToUpper(*tns), "SERVICE_NAME") == false {
		fmt.Print("Cannot generate tns alias for lrpdb")
		return
	}

	if strings.Contains(strings.ToUpper(*tns), "ORACLE_SID") == true {
		fmt.Print("Cannot generate tns alias for lrpdb")
		return
	}

	swaptns = fmt.Sprintf("SERVICE_NAME=%s", *lrpdbsrv)
	tnsreg := regexp.MustCompile(`SERVICE_NAME=\w+`)
	*tns = tnsreg.ReplaceAllString(*tns, swaptns)

	fmt.Printf("Newstring [%s]\n", *tns)

}

// Compose url
func (r *LRPDBReconciler) BaseUrl(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, lrest dbapi.LREST) string {
	log := r.Log.WithValues("BaseUrl", req.NamespacedName)
	baseurl := "https://" + lrpdb.Spec.CDBResName + "-lrest." + lrpdb.Spec.CDBNamespace + ":" + strconv.Itoa(lrest.Spec.LRESTPort) + "/database/pdbs/"
	log.Info("Baseurl:" + baseurl)
	return baseurl
}

func (r *LRPDBReconciler) DecryptWithPrivKey(Key string, Buffer string, req ctrl.Request) (string, error) {
	log := r.Log.WithValues("DecryptWithPrivKey", req.NamespacedName)
	Debug := 0
	block, _ := pem.Decode([]byte(Key))
	pkcs8PrivateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		log.Error(err, "Failed to parse private key - "+err.Error())
		return "", err
	}
	if Debug == 1 {
		fmt.Printf("======================================\n")
		fmt.Printf("%s\n", Key)
		fmt.Printf("======================================\n")
	}

	encString64, err := base64.StdEncoding.DecodeString(string(Buffer))
	if err != nil {
		log.Error(err, "Failed to decode encrypted string to base64 - "+err.Error())
		return "", err
	}

	decryptedB, err := rsa.DecryptPKCS1v15(nil, pkcs8PrivateKey.(*rsa.PrivateKey), encString64)
	if err != nil {
		log.Error(err, "Failed to decrypt string - "+err.Error())
		return "", err
	}
	if Debug == 1 {
		fmt.Printf("[%s]\n", string(decryptedB))
	}
	return strings.TrimSpace(string(decryptedB)), err

}

// New function to decrypt credential using private key
func (r *LRPDBReconciler) getEncriptedSecret(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, secretName string, keyName string, secretNamePk string, keyNamePk string) (string, error) {

	log := r.Log.WithValues("getEncriptedSecret", req.NamespacedName)

	log.Info("getEncriptedSecret :" + secretName)
	secret1 := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: lrpdb.Namespace}, secret1)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + secretName)
			lrpdb.Status.Msg = "Secret not found:" + secretName
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	secret2 := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secretNamePk, Namespace: lrpdb.Namespace}, secret2)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + secretNamePk)
			lrpdb.Status.Msg = "Secret not found:" + secretNamePk
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	Encval := string(secret1.Data[keyName])
	Encval = strings.TrimSpace(Encval)

	privKey := string(secret2.Data[keyNamePk])
	privKey = strings.TrimSpace(privKey)

	/* Debuug info for dev phase
	fmt.Printf("DEBUG Secretename:secretName :%s\n", secretName)
	fmt.Printf("DEBUG privKey                :%s\n", privKey)
	fmt.Printf("DEBUG Encval                 :%s\n", Encval)
	*/

	DecVal, err := r.DecryptWithPrivKey(privKey, Encval, req)
	if err != nil {
		log.Error(err, "Fail to decrypt secret:"+secretName)
		lrpdb.Status.Msg = " Fail to decrypt secret:" + secretName
		return "", err
	}
	return DecVal, nil
}

func (r *LRPDBReconciler) manageLRPDBDeletion2(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("manageLRPDBDeletion", req.NamespacedName)
	if lrpdb.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
			controllerutil.AddFinalizer(lrpdb, LRPDBFinalizer)
			if err := r.Update(ctx, lrpdb); err != nil {
				return err
			}
		}
	} else {
		log.Info("Pdb marked to be delted")
		if controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
			if assertiveLpdbDeletion == true {
				log.Info("Deleting lrpdb CRD: Assertive approach is turned on ")
				lrest, err := r.getLRESTResource(ctx, req, lrpdb)
				if err != nil {
					log.Error(err, "Cannont find cdb resource ", "err", err.Error())
					return err
				}

				lrpdbName := lrpdb.Spec.LRPDBName
				if lrpdb.Status.OpenMode == "READ WRITE" {
					valuesclose := map[string]string{
						"state":        "CLOSE",
						"modifyOption": "IMMEDIATE",
						"getScript":    "FALSE"}
					lrpdbName := lrpdb.Spec.LRPDBName
					url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName + "/status/"
					_, errclose := r.callAPI(ctx, req, lrpdb, url, valuesclose, "POST")
					if errclose != nil {
						log.Info("Warning error closing lrpdb continue anyway")
					}
				}

				valuesdrop := map[string]string{
					"action":    "INCLUDING",
					"getScript": "FALSE"}
				url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName + "/"

				log.Info("Call Delete()")
				_, errdelete := r.callAPI(ctx, req, lrpdb, url, valuesdrop, "DELETE")
				if errdelete != nil {
					log.Error(errdelete, "Fail to delete lrpdb :"+lrpdb.Name, "err", err.Error())
					return errdelete
				}
			} /* END OF ASSERTIVE SECTION */

			log.Info("Marked to be deleted")
			lrpdb.Status.Phase = lrpdbPhaseDelete
			lrpdb.Status.Status = true
			r.Status().Update(ctx, lrpdb)

			controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
			if err := r.Update(ctx, lrpdb); err != nil {
				log.Info("Cannot remove finalizer")
				return err
			}

		}

		return nil
	}

	return nil
}

func (r *LRPDBReconciler) InitConfigMap(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) *corev1.ConfigMap {
	log := r.Log.WithValues("InitConfigMap", req.NamespacedName)
	log.Info("ConfigMap..............:" + "ConfigMap" + lrpdb.Name)
	log.Info("ConfigMap nmsp.........:" + lrpdb.Namespace)
	/*
	 *  PDB SYSTEM PARAMETER
	 *  record [name,value=[paramete_val|reset],level=[session|system]]
	 */

	if lrpdb.Spec.PDBConfigMap == "" {
		/* if users does not specify a config map
		we generate an empty new one for possible
		future pdb parameter  modification */

		var SystemParameters map[string]string

		log.Info("Generating an empty configmap")
		globalconfigmap = "configmap-" + lrpdb.Spec.LRPDBName + "-default"
		DbParameters := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "configmap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      globalconfigmap,
				Namespace: lrpdb.Namespace,
			},
			Data: SystemParameters,
		}

		if err := ctrl.SetControllerReference(lrpdb, DbParameters, r.Scheme); err != nil {
			log.Error(err, "Fail to set SetControllerReference", "err", err.Error())
			return nil
		}

		/* Update Spec.PDBConfigMap */
		lrpdb.Spec.PDBConfigMap = "configmap" + lrpdb.Spec.LRPDBName + "default"
		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Failure updating Spec.PDBConfigMap ", "err", err.Error())
			return nil
		}
		lrpdb.Status.Bitstat = bis(lrpdb.Status.Bitstat, MPEMPT)
		return DbParameters

	} else {

		lrpdb.Status.Bitstat = bis(lrpdb.Status.Bitstat, MPINIT)
		globalconfigmap = lrpdb.Spec.PDBConfigMap
		DbParameters, err := r.GetConfigMap(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, "Fail to fetch configmap ", "err", err.Error())
			return nil
		}

		//ParseConfigMapData(DbParameters)

		return DbParameters
	}

	return nil
}

func (r *LRPDBReconciler) GetConfigMap(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) (*corev1.ConfigMap, error) {
	log := r.Log.WithValues("GetConfigMap", req.NamespacedName)
	log.Info("ConfigMapGlobal.............:" + globalconfigmap)
	DbParameters, err := k8s.FetchConfigMap(r.Client, lrpdb.Namespace, globalconfigmap)
	if err != nil {
		log.Error(err, "Fail to fetch configmap", "err", err.Error())
		return nil, err
	}

	return DbParameters, nil
}

func (r *LRPDBReconciler) ApplyConfigMap(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) (int32, error) {
	log := r.Log.WithValues("ApplyConfigMap", req.NamespacedName)
	/* We read the config map and apply the setting to the  pdb */

	log.Info("Starting Apply Config Map Process")
	configmap, err := r.GetConfigMap(ctx, req, lrpdb)
	if err != nil {
		log.Info("Cannot get config map in the open yaml file")
		return 0, nil
	}
	Cardinality := int32(len(configmap.Data))
	if Cardinality == 0 {
		log.Info("Empty config map... nothing to do ")
		return 0, nil
	}
	log.Info("GetConfigMap completed")

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		log.Info("Cannot find lrest server")
		return 0, nil
	}
	tokens := lrcommons.ParseConfigMapData(configmap)
	for cnt := range tokens {
		if len(tokens[cnt]) != 0 {
			/*  avoid null token and check malformed value  */
			fmt.Printf("token=[%s]\n", tokens[cnt])
			Parameter := strings.Split(tokens[cnt], " ")
			if len(Parameter) != 3 {
				log.Info("WARNING  malformed value in the configmap")
			} else {
				fmt.Printf("alter system set %s=%s scope=%s instances=all\n", Parameter[0], Parameter[1], Parameter[2])
				/* Preparing PayLoad
				   -----------------
				   WARNING: event setting is not yet supported. It will be implemented in future release
				*/
				AlterSystemPayload := map[string]string{
					"state":                "ALTER",
					"alterSystemParameter": Parameter[0],
					"alterSystemValue":     Parameter[1],
					"parameterScope":       Parameter[2],
				}
				url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdb.Spec.LRPDBName
				respData, err := r.callAPI(ctx, req, lrpdb, url, AlterSystemPayload, "POST")
				if err != nil {
					log.Error(err, "callAPI failure durring Apply Config Map", "err", err.Error())
					return 0, err
				}
				/* check sql code execution */
				var retJson map[string]interface{}
				if err := json.Unmarshal([]byte(respData), &retJson); err != nil {
					log.Error(err, "failed to get Data from callAPI", "err", err.Error())
					return 0, err
				}
				/* We do not the execution if something goes wrong for a single parameter
				   just report the error in the event queue */
				SqlCode := strconv.Itoa(int(retJson["sqlcode"].(float64)))
				AlterMsg := fmt.Sprintf("pdb=%s:%s:%s:%s:%s", lrpdb.Spec.LRPDBName, Parameter[0], Parameter[1], Parameter[2], SqlCode)
				log.Info("Config Map Apply:......." + AlterMsg)

				if SqlCode != "0" {
					r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTINFO", AlterMsg)
					lrpdb.Status.Bitstat = bis(lrpdb.Status.Bitstat, MPWARN)
				}

			}
		}

	}

	lrpdb.Status.Bitstat = bis(lrpdb.Status.Bitstat, MPAPPL)

	return Cardinality, nil
}

func (r *LRPDBReconciler) ManageConfigMapForCloningAndPlugin(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("ManageConfigMapForCloningAndPlugin", req.NamespacedName)
	log.Info("Frame:")
	/*
	   If configmap parameter is set and init flag is not set
	   then we need to iniialized the init mask. This is the case for
	   pdb generated by clone and plug action
	*/
	if lrpdb.Spec.Action != "CREATE" && lrpdb.Spec.PDBConfigMap != "" && bit(lrpdb.Status.Bitstat, MPINIT) == false {
		if r.InitConfigMap(ctx, req, lrpdb) == nil {
			log.Info("Cannot initialize config map for pdb.........:" + lrpdb.Spec.LRPDBName)
			return nil
		}
		log.Info("Call...........:ApplyConfigMap(ctx, req, lrpdb)")
		Cardinality, _ := r.ApplyConfigMap(ctx, req, lrpdb)
		log.Info("Cardnality:....:" + strconv.Itoa(int(Cardinality)))
		if Cardinality == 0 {
			return nil
		}

	}
	return nil
}

func NewCallLAPI(intr interface{}, ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, url string, payload map[string]string, action string) (string, error) {
	var c client.Client
	var r logr.Logger
	var e record.EventRecorder
	var err error

	recpdb, ok1 := intr.(*LRPDBReconciler)
	if ok1 {
		fmt.Printf("func NewCallLApi ((*PDBReconciler),......)\n")
		c = recpdb.Client
		e = recpdb.Recorder
		r = recpdb.Log
	}

	reccdb, ok2 := intr.(*LRESTReconciler)
	if ok2 {
		fmt.Printf("func NewCallLApi ((*CDBReconciler),......)\n")
		c = reccdb.Client
		e = reccdb.Recorder
		r = reccdb.Log
	}

	log := r.WithValues("NewCallLAPI", req.NamespacedName)

	secret := &corev1.Secret{}

	err = c.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.LRPDBTlsKey.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.LRPDBTlsKey.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	rsaKeyPEM := secret.Data[lrpdb.Spec.LRPDBTlsKey.Secret.Key]

	err = c.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.LRPDBTlsCrt.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.LRPDBTlsCrt.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	rsaCertPEM := secret.Data[lrpdb.Spec.LRPDBTlsCrt.Secret.Key]

	err = c.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.LRPDBTlsCat.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.LRPDBTlsCat.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	caCert := secret.Data[lrpdb.Spec.LRPDBTlsCat.Secret.Key]
	/*
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTINFO", string(rsaKeyPEM))
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTINFO", string(rsaCertPEM))
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTINFO", string(caCert))
	*/

	certificate, err := tls.X509KeyPair([]byte(rsaCertPEM), []byte(rsaKeyPEM))
	if err != nil {
		lrpdb.Status.Msg = "Error tls.X509KeyPair"
		return "", err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	/*
			tlsConf := &tls.Config{Certificates: []tls.Certificate{certificate},
		                               RootCAs: caCertPool}
	*/
	tlsConf := &tls.Config{Certificates: []tls.Certificate{certificate},
		RootCAs: caCertPool,
		//MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}

	tr := &http.Transport{TLSClientConfig: tlsConf}

	httpclient := &http.Client{Transport: tr}

	log.Info("Issuing REST call", "URL", url, "Action", action)

	// Get Web Server User
	//secret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.WebLrpdbServerUser.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.WebLrpdbServerUser.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	webUserEnc := string(secret.Data[lrpdb.Spec.WebLrpdbServerUser.Secret.Key])
	webUserEnc = strings.TrimSpace(webUserEnc)

	err = c.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.LRPDBPriKey.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.LRPDBPriKey.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	privKey := string(secret.Data[lrpdb.Spec.LRPDBPriKey.Secret.Key])
	webUser, err := lrcommons.CommonDecryptWithPrivKey(privKey, webUserEnc, req)

	// Get Web Server User Password
	secret = &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: lrpdb.Spec.WebLrpdbServerPwd.Secret.SecretName, Namespace: lrpdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + lrpdb.Spec.WebLrpdbServerPwd.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	webUserPwdEnc := string(secret.Data[lrpdb.Spec.WebLrpdbServerPwd.Secret.Key])
	webUserPwdEnc = strings.TrimSpace(webUserPwdEnc)
	webUserPwd, err := lrcommons.CommonDecryptWithPrivKey(privKey, webUserPwdEnc, req)

	var httpreq *http.Request
	if action == "GET" {
		httpreq, err = http.NewRequest(action, url, nil)
	} else {
		jsonValue, _ := json.Marshal(payload)
		httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
	}

	if err != nil {
		log.Info("Unable to create HTTP Request for LRPDB : "+lrpdb.Name, "err", err.Error())
		return "", err
	}

	httpreq.Header.Add("Accept", "application/json")
	httpreq.Header.Add("Content-Type", "application/json")
	httpreq.SetBasicAuth(webUser, webUserPwd)

	resp, err := httpclient.Do(httpreq)
	if err != nil {
		errmsg := err.Error()
		log.Error(err, "Failed - Could not connect to LREST Pod", "err", err.Error())
		lrpdb.Status.Msg = "Error: Could not connect to LREST Pod"
		e.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTError", errmsg)
		return "", err
	}

	e.Eventf(lrpdb, corev1.EventTypeWarning, "Done", lrpdb.Spec.CDBResName)
	if resp.StatusCode != http.StatusOK {
		bb, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode == 404 {
			lrpdb.Status.ConnString = ""
			lrpdb.Status.Msg = lrpdb.Spec.LRPDBName + " not found"

		} else {
			if flood_control == false {
				lrpdb.Status.Msg = "LREST Error - HTTP Status Code:" + strconv.Itoa(resp.StatusCode)
			}
		}

		if flood_control == false {
			log.Info("LREST Error - HTTP Status Code :"+strconv.Itoa(resp.StatusCode), "Err", string(bb))
		}

		var apiErr LRESTError
		json.Unmarshal([]byte(bb), &apiErr)
		if flood_control == false {
			e.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTError", "Failed: %s", apiErr.Message)
		}
		fmt.Printf("\n================== APIERR ======================\n")
		fmt.Printf("%+v \n", apiErr)
		fmt.Printf(string(bb))
		fmt.Printf("URL=%s\n", url)
		fmt.Printf("resp.StatusCode=%s\n", strconv.Itoa(resp.StatusCode))
		fmt.Printf("\n================== APIERR ======================\n")
		flood_control = true
		return "", errors.New("LREST Error")
	}
	flood_control = false

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Print(err.Error())
	}
	respData := string(bodyBytes)
	fmt.Print("CALL API return msg.....:")
	fmt.Println(string(bodyBytes))

	var apiResponse restSQLCollection
	json.Unmarshal([]byte(bodyBytes), &apiResponse)
	fmt.Printf("===> %#v\n", apiResponse)
	fmt.Printf("===> %+v\n", apiResponse)

	errFound := false
	for _, sqlItem := range apiResponse.Items {
		if sqlItem.ErrorDetails != "" {
			log.Info("LREST Error - Oracle Error Code :" + strconv.Itoa(sqlItem.ErrorCode))
			if !errFound {
				lrpdb.Status.Msg = sqlItem.ErrorDetails
			}
			e.Eventf(lrpdb, corev1.EventTypeWarning, "OraError", "%s", sqlItem.ErrorDetails)
			errFound = true
		}
	}

	if errFound {
		return "", errors.New("Oracle Error")
	}

	return respData, nil
}

func (r *LRPDBReconciler) GetSqlCode(rsp string, sqlcode *int) error {
	log := r.Log.WithValues("GetSqlCode", "callAPI(...)")

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		return err
	}

	*sqlcode = int(objmap["sqlcode"].(float64))
	log.Info("sqlcode.......:ora-" + strconv.Itoa(*sqlcode))
	if *sqlcode != 0 {
		switch strconv.Itoa(*sqlcode) {
		case "65019": /* already open */
			return nil
		case "65020": /* already closed */
			return nil
		}
		err := fmt.Errorf("%v", sqlcode)
		return err
	}
	return nil
}
