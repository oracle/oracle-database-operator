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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"

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
	lrcommons "github.com/oracle/oracle-database-operator/commons/multitenant/lrest"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

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

// PDBReconciler reconciles a PDB object
type PDBReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Interval time.Duration
	Recorder record.EventRecorder
}

type controllers struct {
	Pdbc PDBReconciler
	Cdbc CDBReconciler
}

type RESTSQLCollection struct {
	Env struct {
		DefaultTimeZone string `json:"defaultTimeZone,omitempty"`
	} `json:"env"`
	Items []SQLItem `json:"items"`
}

type SQLItem struct {
	StatementId  int      `json:"statementId,omitempty"`
	Response     []string `json:"response"`
	ErrorCode    int      `json:"errorCode,omitempty"`
	ErrorLine    int      `json:"errorLine,omitempty"`
	ErrorColumn  int      `json:"errorColumn,omitempty"`
	ErrorDetails string   `json:"errorDetails,omitempty"`
	Result       int      `json:"result,omitempty"`
}

type ORDSError struct {
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Type     string `json:"type,omitempty"`
	Instance string `json:"instance,omitempty"`
}

var (
	pdbPhaseCreate = "Creating"
	pdbPhasePlug   = "Plugging"
	pdbPhaseUnplug = "Unplugging"
	pdbPhaseClone  = "Cloning"
	pdbPhaseFinish = "Finishing"
	pdbPhaseReady  = "Ready"
	pdbPhaseDelete = "Deleting"
	pdbPhaseModify = "Modifying"
	pdbPhaseMap    = "Mapping"
	pdbPhaseStatus = "CheckingState"
	pdbPhaseFail   = "Failed"
)

const PDBFinalizer = "database.oracle.com/PDBfinalizer"
const ONE = 1
const ZERO = 0

var tdePassword string
var tdeSecret string
var floodcontrol bool = false
var assertivePdbDeletion bool = false /* Global variable for assertive pdb deletion */

//+kubebuilder:rbac:groups=database.oracle.com,resources=pdbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=pdbs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=pdbs/finalizers,verbs=get;create;update;patch;delete

// +kubebuilder:rbac:groups=core,resources=pods;pods/log;pods/exec;secrets;containers;services;events;configmaps;namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups='',resources=statefulsets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile

func (r *PDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("multitenantoperator", req.NamespacedName)
	log.Info("Reconcile requested")

	reconcilePeriod := r.Interval * time.Second
	requeueY := ctrl.Result{Requeue: true, RequeueAfter: reconcilePeriod}
	requeueN := ctrl.Result{}

	var err error
	pdb := &dbapi.PDB{}

	// Execute for every reconcile
	defer func() {
		//log.Info("DEFER PDB", "Name", pdb.Name, "Phase", pdb.Status.Phase, "Status", strconv.FormatBool(pdb.Status.Status))
		if !pdb.Status.Status {
			if pdb.Status.Phase == pdbPhaseReady {
				pdb.Status.Status = true
			}
			if err := r.Status().Update(ctx, pdb); err != nil {
				log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
			}
		}
	}()

	err = r.Client.Get(context.TODO(), req.NamespacedName, pdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PDB Resource Not found", "Name", pdb.Name)
			// Request object not found, could have been deleted after reconcile req.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			pdb.Status.Status = true
			return requeueN, nil
		}
		// Error reading the object - requeue the req.
		return requeueY, err
	}

	// Finalizer section
	err = r.managePDBDeletion2(ctx, req, pdb)
	if err != nil {
		log.Info("managePDBDeletion2 Error Deleting resource ")
		return requeueY, nil
	}

	// Check for Duplicate PDB
	if !pdb.Status.Status {
		err = r.checkDuplicatePDB(ctx, req, pdb)
		if err != nil {
			return requeueN, nil
		}
	}

	action := strings.ToUpper(pdb.Spec.Action)

	if pdb.Status.Phase == pdbPhaseReady {
		//log.Info("PDB:", "Name", pdb.Name, "Phase", pdb.Status.Phase, "Status", strconv.FormatBool(pdb.Status.Status))
		if (pdb.Status.Action != "") && (action == "MODIFY" || action == "STATUS" || pdb.Status.Action != action) {
			pdb.Status.Status = false
		} else {
			err = r.getPDBState(ctx, req, pdb)
			if err != nil {
				pdb.Status.Phase = pdbPhaseFail
			} else {
				pdb.Status.Phase = pdbPhaseReady
				pdb.Status.Msg = "Success"
			}
			r.Status().Update(ctx, pdb)
		}
	}

	if !pdb.Status.Status {
		r.validatePhase(ctx, req, pdb)
		phase := pdb.Status.Phase
		log.Info("PDB:", "Name", pdb.Name, "Phase", phase, "Status", strconv.FormatBool(pdb.Status.Status))

		switch phase {
		case pdbPhaseCreate:
			err = r.createPDB(ctx, req, pdb)
		case pdbPhaseClone:
			err = r.clonePDB(ctx, req, pdb)
		case pdbPhasePlug:
			err = r.plugPDB(ctx, req, pdb)
		case pdbPhaseUnplug:
			err = r.unplugPDB(ctx, req, pdb)
		case pdbPhaseModify:
			err = r.modifyPDB(ctx, req, pdb)
		case pdbPhaseDelete:
			err = r.deletePDB(ctx, req, pdb)
		case pdbPhaseStatus:
			err = r.getPDBState(ctx, req, pdb)
		case pdbPhaseMap:
			err = r.mapPDB(ctx, req, pdb)
		case pdbPhaseFail:
			err = r.mapPDB(ctx, req, pdb)
		default:
			log.Info("DEFAULT:", "Name", pdb.Name, "Phase", phase, "Status", strconv.FormatBool(pdb.Status.Status))
			return requeueN, nil
		}
		pdb.Status.Action = strings.ToUpper(pdb.Spec.Action)
		if err != nil {
			pdb.Status.Phase = pdbPhaseFail
		} else {
			pdb.Status.Phase = pdbPhaseReady
			pdb.Status.Msg = "Success"
		}
	}

	log.Info("Reconcile completed")
	return requeueY, nil
}

/*
************************************************
  - Validate the PDB Spec
    /***********************************************
*/
func (r *PDBReconciler) validatePhase(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) {

	log := r.Log.WithValues("validatePhase", req.NamespacedName)

	action := strings.ToUpper(pdb.Spec.Action)

	log.Info("Validating PDB phase for: "+pdb.Name, "Action", action)

	switch action {
	case "CREATE":
		pdb.Status.Phase = pdbPhaseCreate
	case "CLONE":
		pdb.Status.Phase = pdbPhaseClone
	case "PLUG":
		pdb.Status.Phase = pdbPhasePlug
	case "UNPLUG":
		pdb.Status.Phase = pdbPhaseUnplug
	case "MODIFY":
		pdb.Status.Phase = pdbPhaseModify
	case "DELETE":
		pdb.Status.Phase = pdbPhaseDelete
	case "STATUS":
		pdb.Status.Phase = pdbPhaseStatus
	case "MAP":
		pdb.Status.Phase = pdbPhaseMap
	}

	log.Info("Validation complete")
}

/*
***************************************************************
  - Check for Duplicate PDB. Same PDB name on the same CDB resource.
    /**************************************************************
*/
func (r *PDBReconciler) checkDuplicatePDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	// Name of the CDB CR that holds the ORDS container
	cdbResName := pdb.Spec.CDBResName

	log := r.Log.WithValues("checkDuplicatePDB", pdb.Spec.CDBNamespace)
	pdbResName := pdb.Spec.PDBName

	pdbList := &dbapi.PDBList{}

	listOpts := []client.ListOption{client.InNamespace(pdb.Spec.CDBNamespace), client.MatchingFields{"spec.pdbName": pdbResName}}

	// List retrieves list of objects for a given namespace and list options.
	err := r.List(ctx, pdbList, listOpts...)
	if err != nil {
		log.Info("Failed to list pdbs", "Namespace", pdb.Spec.CDBNamespace, "Error", err)
		return err
	}

	if len(pdbList.Items) == 0 {
		log.Info("No pdbs found for PDBName: "+pdbResName, "CDBResName", cdbResName)
		return nil
	}

	for _, p := range pdbList.Items {
		log.Info("Found PDB: " + p.Name)
		if (p.Name != pdb.Name) && (p.Spec.CDBResName == cdbResName) {
			log.Info("Duplicate PDB found")
			pdb.Status.Msg = "PDB Resource already exists"
			pdb.Status.Status = false
			pdb.Status.Phase = pdbPhaseFail
			return errors.New("Duplicate PDB found")
		}
	}
	return nil
}

/*
***************************************************************
  - Get the Custom Resource for the CDB mentioned in the PDB Spec
    /**************************************************************
*/
func (r *PDBReconciler) getCDBResource(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) (dbapi.CDB, error) {

	log := r.Log.WithValues("getCDBResource", req.NamespacedName)

	var cdb dbapi.CDB // CDB CR corresponding to the CDB name specified in the PDB spec

	// Name of the CDB CR that holds the ORDS container
	cdbResName := pdb.Spec.CDBResName
	cdbNamespace := pdb.Spec.CDBNamespace

	// Get CDB CR corresponding to the CDB name specified in the PDB spec
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: cdbNamespace,
		Name:      cdbResName,
	}, &cdb)

	if err != nil {
		log.Info("Failed to get CRD for CDB", "Name", cdbResName, "Namespace", pdb.Spec.CDBNamespace, "Error", err.Error())
		pdb.Status.Msg = "Unable to get CRD for CDB : " + cdbResName
		r.Status().Update(ctx, pdb)
		return cdb, err
	}

	log.Info("Found CR for CDB", "Name", cdbResName, "CR Name", cdb.Name)
	return cdb, nil
}

/*
***************************************************************
  - Get the ORDS Pod for the CDB mentioned in the PDB Spec
    /**************************************************************
*/
func (r *PDBReconciler) getORDSPod(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) (corev1.Pod, error) {

	log := r.Log.WithValues("getORDSPod", req.NamespacedName)

	var cdbPod corev1.Pod // ORDS Pod container with connection to the concerned CDB

	// Name of the CDB CR that holds the ORDS container
	cdbResName := pdb.Spec.CDBResName

	// Get ORDS Pod associated with the CDB Name specified in the PDB Spec
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: pdb.Spec.CDBNamespace,
		Name:      cdbResName + "-ords",
	}, &cdbPod)

	if err != nil {
		log.Info("Failed to get Pod for CDB", "Name", cdbResName, "Namespace", pdb.Spec.CDBNamespace, "Error", err.Error())
		pdb.Status.Msg = "Unable to get ORDS Pod for CDB : " + cdbResName
		return cdbPod, err
	}

	log.Info("Found ORDS Pod for CDB", "Name", cdbResName, "Pod Name", cdbPod.Name, "ORDS Container hostname", cdbPod.Spec.Hostname)
	return cdbPod, nil
}

/*
************************************************
  - Get Secret Key for a Secret Name
    /***********************************************
*/
func (r *PDBReconciler) getSecret(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB, secretName string, keyName string) (string, error) {

	log := r.Log.WithValues("getSecret", req.NamespacedName)

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + secretName)
			pdb.Status.Msg = "Secret not found:" + secretName
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	return string(secret.Data[keyName]), nil
}

/*
************************************************
  - Issue a REST API Call to the ORDS container

***********************************************
*/
func (r *PDBReconciler) callAPI(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB, url string, payload map[string]string, action string) (string, error) {
	log := r.Log.WithValues("callAPI", req.NamespacedName)

	var err error

	secret := &corev1.Secret{}

	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBTlsKey.Secret.SecretName, Namespace: pdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBTlsKey.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	rsaKeyPEM := secret.Data[pdb.Spec.PDBTlsKey.Secret.Key]

	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBTlsCrt.Secret.SecretName, Namespace: pdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBTlsCrt.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	rsaCertPEM := secret.Data[pdb.Spec.PDBTlsCrt.Secret.Key]

	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBTlsCat.Secret.SecretName, Namespace: pdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBTlsCat.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	caCert := secret.Data[pdb.Spec.PDBTlsCat.Secret.Key]

	certificate, err := tls.X509KeyPair([]byte(rsaCertPEM), []byte(rsaKeyPEM))
	if err != nil {
		pdb.Status.Msg = "Error tls.X509KeyPair"
		return "", err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConf := &tls.Config{Certificates: []tls.Certificate{certificate}, RootCAs: caCertPool}

	tr := &http.Transport{TLSClientConfig: tlsConf}

	httpclient := &http.Client{Transport: tr}

	log.Info("Issuing REST call", "URL", url, "Action", action)

	/*
		cdb, err := r.getCDBResource(ctx, req, pdb)
		if err != nil {
			return "", err
		}
	*/

	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.WebServerUsr.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.WebServerUsr.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	webUser := string(secret.Data[pdb.Spec.WebServerUsr.Secret.Key])
	webUser = strings.TrimSpace(webUser)

	secret = &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.WebServerPwd.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.WebServerPwd.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	webUserPwd := string(secret.Data[pdb.Spec.WebServerPwd.Secret.Key])
	webUserPwd = strings.TrimSpace(webUserPwd)

	var httpreq *http.Request
	if action == "GET" {
		httpreq, err = http.NewRequest(action, url, nil)
	} else {
		jsonValue, _ := json.Marshal(payload)
		httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
	}

	if err != nil {
		log.Info("Unable to create HTTP Request for PDB : "+pdb.Name, "err", err.Error())
		return "", err
	}

	httpreq.Header.Add("Accept", "application/json")
	httpreq.Header.Add("Content-Type", "application/json")
	httpreq.SetBasicAuth(webUser, webUserPwd)

	resp, err := httpclient.Do(httpreq)
	if err != nil {
		errmsg := err.Error()
		log.Error(err, "Failed - Could not connect to ORDS Pod", "err", err.Error())
		pdb.Status.Msg = "Error: Could not connect to ORDS Pod"
		r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "ORDSError", errmsg)
		return "", err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "Done", pdb.Spec.CDBResName)
	if resp.StatusCode != http.StatusOK {
		bb, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode == 404 {
			pdb.Status.ConnString = ""
			pdb.Status.Msg = pdb.Spec.PDBName + " not found"

		} else {
			if floodcontrol == false {
				pdb.Status.Msg = "ORDS Error - HTTP Status Code:" + strconv.Itoa(resp.StatusCode)
			}
		}

		if floodcontrol == false {
			log.Info("ORDS Error - HTTP Status Code :"+strconv.Itoa(resp.StatusCode), "Err", string(bb))
		}

		var apiErr ORDSError
		json.Unmarshal([]byte(bb), &apiErr)
		if floodcontrol == false {
			r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "ORDSError", "Failed: %s", apiErr.Message)
		}
		//fmt.Printf("%+v", apiErr)
		//fmt.Println(string(bb))
		floodcontrol = true
		return "", errors.New("ORDS Error")
	}
	floodcontrol = false

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Print(err.Error())
	}
	respData := string(bodyBytes)
	//fmt.Println(string(bodyBytes))

	var apiResponse RESTSQLCollection
	json.Unmarshal([]byte(bodyBytes), &apiResponse)
	//fmt.Printf("%#v", apiResponse)
	//fmt.Printf("%+v", apiResponse)

	errFound := false
	for _, sqlItem := range apiResponse.Items {
		if sqlItem.ErrorDetails != "" {
			log.Info("ORDS Error - Oracle Error Code :" + strconv.Itoa(sqlItem.ErrorCode))
			if !errFound {
				pdb.Status.Msg = sqlItem.ErrorDetails
			}
			r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "OraError", "%s", sqlItem.ErrorDetails)
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
  - Create a PDB

***********************************************
*/
func (r *PDBReconciler) createPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("createPDB", req.NamespacedName)

	var err error
	var tdePassword string
	var tdeSecret string

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	/*** BEGIN GET ENCPASS ***/
	secret := &corev1.Secret{}

	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.AdminName.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.AdminName.Secret.SecretName)
			return err
		}
		log.Error(err, "Unable to get the secret.")
		return err
	}
	pdbAdminNameEnc := string(secret.Data[pdb.Spec.AdminName.Secret.Key])
	pdbAdminNameEnc = strings.TrimSpace(pdbAdminNameEnc)

	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBPriKey.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBPriKey.Secret.SecretName)
			return err
		}
		log.Error(err, "Unable to get the secret.")
		return err
	}
	privKey := string(secret.Data[pdb.Spec.PDBPriKey.Secret.Key])
	pdbAdminName, err := lrcommons.CommonDecryptWithPrivKey(privKey, pdbAdminNameEnc, req)

	// Get Web Server User Password
	secret = &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: pdb.Spec.AdminPwd.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.AdminPwd.Secret.SecretName)
			return err
		}
		log.Error(err, "Unable to get the secret.")
		return err
	}
	pdbAdminPwdEnc := string(secret.Data[pdb.Spec.AdminPwd.Secret.Key])
	pdbAdminPwdEnc = strings.TrimSpace(pdbAdminPwdEnc)
	pdbAdminPwd, err := lrcommons.CommonDecryptWithPrivKey(privKey, pdbAdminPwdEnc, req)
	pdbAdminName = strings.TrimSuffix(pdbAdminName, "\n")
	pdbAdminPwd = strings.TrimSuffix(pdbAdminPwd, "\n")
	/*** END GET ENCPASS ***/

	log.Info("====================> " + pdbAdminName + ":" + pdbAdminPwd)
	/* Prevent creating an existing pdb */
	err = r.getPDBState(ctx, req, pdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Check PDB not existence completed", "PDB Name", pdb.Spec.PDBName)
		}

	} else {
		log.Info("Database already exists ", "PDB Name", pdb.Spec.PDBName)
		return nil
	}

	values := map[string]string{
		"method":              "CREATE",
		"pdb_name":            pdb.Spec.PDBName,
		"adminName":           pdbAdminName,
		"adminPwd":            pdbAdminPwd,
		"fileNameConversions": pdb.Spec.FileNameConversions,
		"reuseTempFile":       strconv.FormatBool(*(pdb.Spec.ReuseTempFile)),
		"unlimitedStorage":    strconv.FormatBool(*(pdb.Spec.UnlimitedStorage)),
		"totalSize":           pdb.Spec.TotalSize,
		"tempSize":            pdb.Spec.TempSize,
		"getScript":           strconv.FormatBool(*(pdb.Spec.GetScript))}

	if *(pdb.Spec.TDEImport) {
		tdePassword, err = r.getSecret(ctx, req, pdb, pdb.Spec.TDEPassword.Secret.SecretName, pdb.Spec.TDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tdeSecret, err = r.getSecret(ctx, req, pdb, pdb.Spec.TDESecret.Secret.SecretName, pdb.Spec.TDESecret.Secret.Key)
		if err != nil {
			return err
		}

		tdeSecret = tdeSecret[:len(tdeSecret)-1]
		tdePassword = tdeSecret[:len(tdePassword)-1]
		values["tdePassword"] = tdePassword
		values["tdeKeystorePath"] = pdb.Spec.TDEKeystorePath
		values["tdeSecret"] = tdeSecret
	}

	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/"

	pdb.Status.TotalSize = pdb.Spec.TotalSize
	pdb.Status.Phase = pdbPhaseCreate
	pdb.Status.Msg = "Waiting for PDB to be created"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = NewCallApi(r, ctx, req, pdb, url, values, "POST")
	if err != nil {
		log.Error(err, "callAPI error", "err", err.Error())
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Created", "PDB '%s' created successfully", pdb.Spec.PDBName)

	if cdb.Spec.DBServer != "" {
		pdb.Status.ConnString = cdb.Spec.DBServer + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	} else {
		pdb.Status.ConnString = cdb.Spec.DBTnsurl
		ParseTnsAlias(&(pdb.Status.ConnString), &(pdb.Spec.PDBName))
	}

	assertivePdbDeletion = pdb.Spec.AssertivePdbDeletion
	if pdb.Spec.AssertivePdbDeletion == true {
		r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Created", "PDB '%s' assertive pdb deletion turned on", pdb.Spec.PDBName)
	}
	log.Info("New connect strinng", "tnsurl", cdb.Spec.DBTnsurl)
	log.Info("Created PDB Resource", "PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*
************************************************
  - Clone a PDB

***********************************************
*/
func (r *PDBReconciler) clonePDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	if pdb.Spec.PDBName == pdb.Spec.SrcPDBName {
		return nil
	}

	log := r.Log.WithValues("clonePDB", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	/* Prevent cloning an existing pdb */
	err = r.getPDBState(ctx, req, pdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Check PDB not existence completed", "PDB Name", pdb.Spec.PDBName)
		}

	} else {
		log.Info("Database already exists ", "PDB Name", pdb.Spec.PDBName)
		return nil
	}

	values := map[string]string{
		"method":           "CLONE",
		"clonePDBName":     pdb.Spec.PDBName,
		"reuseTempFile":    strconv.FormatBool(*(pdb.Spec.ReuseTempFile)),
		"unlimitedStorage": strconv.FormatBool(*(pdb.Spec.UnlimitedStorage)),
		"getScript":        strconv.FormatBool(*(pdb.Spec.GetScript))}

	if pdb.Spec.SparseClonePath != "" {
		values["sparseClonePath"] = pdb.Spec.SparseClonePath
	}
	if pdb.Spec.FileNameConversions != "" {
		values["fileNameConversions"] = pdb.Spec.FileNameConversions
	}
	if pdb.Spec.TotalSize != "" {
		values["totalSize"] = pdb.Spec.TotalSize
	}
	if pdb.Spec.TempSize != "" {
		values["tempSize"] = pdb.Spec.TempSize
	}

	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdb.Spec.SrcPDBName + "/"

	pdb.Status.Phase = pdbPhaseClone
	pdb.Status.Msg = "Waiting for PDB to be cloned"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = NewCallApi(r, ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Created", "PDB '%s' cloned successfully", pdb.Spec.PDBName)

	if cdb.Spec.DBServer != "" {
		pdb.Status.ConnString = cdb.Spec.DBServer + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	} else {
		pdb.Status.ConnString = cdb.Spec.DBTnsurl
		ParseTnsAlias(&(pdb.Status.ConnString), &(pdb.Spec.PDBName))
	}

	assertivePdbDeletion = pdb.Spec.AssertivePdbDeletion
	if pdb.Spec.AssertivePdbDeletion == true {
		r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Clone", "PDB '%s' assertive pdb deletion turned on", pdb.Spec.PDBName)
	}

	log.Info("Cloned PDB successfully", "Source PDB Name", pdb.Spec.SrcPDBName, "Clone PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*
************************************************
  - Plug a PDB

***********************************************
*/
func (r *PDBReconciler) plugPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("plugPDB", req.NamespacedName)

	var err error
	var tdePassword string
	var tdeSecret string

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"method":      "PLUG",
		"xmlFileName": pdb.Spec.XMLFileName,
		"pdb_name":    pdb.Spec.PDBName,
		//"adminName":                 pdbAdminName,
		//"adminPwd":                  pdbAdminPwd,
		"sourceFileNameConversions": pdb.Spec.SourceFileNameConversions,
		"copyAction":                pdb.Spec.CopyAction,
		"fileNameConversions":       pdb.Spec.FileNameConversions,
		"unlimitedStorage":          strconv.FormatBool(*(pdb.Spec.UnlimitedStorage)),
		"reuseTempFile":             strconv.FormatBool(*(pdb.Spec.ReuseTempFile)),
		"totalSize":                 pdb.Spec.TotalSize,
		"tempSize":                  pdb.Spec.TempSize,
		"getScript":                 strconv.FormatBool(*(pdb.Spec.GetScript))}

	if *(pdb.Spec.TDEImport) {
		tdePassword, err = r.getSecret(ctx, req, pdb, pdb.Spec.TDEPassword.Secret.SecretName, pdb.Spec.TDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tdeSecret, err = r.getSecret(ctx, req, pdb, pdb.Spec.TDESecret.Secret.SecretName, pdb.Spec.TDESecret.Secret.Key)
		if err != nil {
			return err
		}

		tdeSecret = tdeSecret[:len(tdeSecret)-1]
		tdePassword = tdeSecret[:len(tdePassword)-1]
		values["tdePassword"] = tdePassword
		values["tdeKeystorePath"] = pdb.Spec.TDEKeystorePath
		values["tdeSecret"] = tdeSecret
		values["tdeImport"] = strconv.FormatBool(*(pdb.Spec.TDEImport))
	}
	if *(pdb.Spec.AsClone) {
		values["asClone"] = strconv.FormatBool(*(pdb.Spec.AsClone))
	}

	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/"

	pdb.Status.TotalSize = pdb.Spec.TotalSize
	pdb.Status.Phase = pdbPhasePlug
	pdb.Status.Msg = "Waiting for PDB to be plugged"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = NewCallApi(r, ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Created", "PDB '%s' plugged successfully", pdb.Spec.PDBName)

	if cdb.Spec.DBServer != "" {
		pdb.Status.ConnString = cdb.Spec.DBServer + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	} else {
		pdb.Status.ConnString = cdb.Spec.DBTnsurl
		ParseTnsAlias(&(pdb.Status.ConnString), &(pdb.Spec.PDBName))
	}

	assertivePdbDeletion = pdb.Spec.AssertivePdbDeletion
	if pdb.Spec.AssertivePdbDeletion == true {
		r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Plugged", "PDB '%s' assertive pdb deletion turned on", pdb.Spec.PDBName)
	}

	log.Info("Successfully plugged PDB", "PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*
************************************************
  - Unplug a PDB

***********************************************
*/
func (r *PDBReconciler) unplugPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("unplugPDB", req.NamespacedName)

	var err error
	var tdePassword string
	var tdeSecret string

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"method":      "UNPLUG",
		"xmlFileName": pdb.Spec.XMLFileName,
		"getScript":   strconv.FormatBool(*(pdb.Spec.GetScript))}

	if *(pdb.Spec.TDEExport) {
		// Get the TDE Password
		tdePassword, err = r.getSecret(ctx, req, pdb, pdb.Spec.TDEPassword.Secret.SecretName, pdb.Spec.TDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tdeSecret, err = r.getSecret(ctx, req, pdb, pdb.Spec.TDESecret.Secret.SecretName, pdb.Spec.TDESecret.Secret.Key)
		if err != nil {
			return err
		}

		tdeSecret = tdeSecret[:len(tdeSecret)-1]
		tdePassword = tdeSecret[:len(tdePassword)-1]
		values["tdePassword"] = tdePassword
		values["tdeKeystorePath"] = pdb.Spec.TDEKeystorePath
		values["tdeSecret"] = tdeSecret
		values["tdeExport"] = strconv.FormatBool(*(pdb.Spec.TDEExport))
	}

	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdb.Spec.PDBName + "/"

	log.Info("CallAPI(url)", "url", url)

	pdb.Status.Phase = pdbPhaseUnplug
	pdb.Status.Msg = "Waiting for PDB to be unplugged"

	if cdb.Spec.DBServer != "" {
		pdb.Status.ConnString = cdb.Spec.DBServer + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	} else {
		pdb.Status.ConnString = cdb.Spec.DBTnsurl
		ParseTnsAlias(&(pdb.Status.ConnString), &(pdb.Spec.PDBName))
	}

	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = NewCallApi(r, ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	if controllerutil.ContainsFinalizer(pdb, PDBFinalizer) {
		log.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(pdb, PDBFinalizer)
		err := r.Update(ctx, pdb)
		if err != nil {
			log.Info("Could not remove finalizer", "err", err.Error())
			return err
		}
		pdb.Status.Status = true
		err = r.Delete(context.Background(), pdb, client.GracePeriodSeconds(1))
		if err != nil {
			log.Info("Could not delete PDB resource", "err", err.Error())
			return err
		}
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Unplugged", "PDB '%s' unplugged successfully", pdb.Spec.PDBName)

	log.Info("Successfully unplugged PDB resource")
	return nil
}

/*
************************************************
  - Modify a PDB state
    /***********************************************
*/
func (r *PDBReconciler) modifyPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("modifyPDB", req.NamespacedName)

	var err error

	err = r.getPDBState(ctx, req, pdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Warning PDB does not exist", "PDB Name", pdb.Spec.PDBName)
			return nil
		}
		return err
	}

	if pdb.Status.OpenMode == "READ WRITE" && pdb.Spec.PDBState == "OPEN" && pdb.Spec.ModifyOption == "READ WRITE" {
		/* Database is already open no action required */
		return nil
	}

	if pdb.Status.OpenMode == "MOUNTED" && pdb.Spec.PDBState == "CLOSE" && pdb.Spec.ModifyOption == "IMMEDIATE" {
		/* Database is already close no action required */
		return nil
	}

	// To prevent Reconcile from Modifying again whenever the Operator gets re-started
	/*
		modOption := pdb.Spec.PDBState + "-" + pdb.Spec.ModifyOption
		if pdb.Status.ModifyOption == modOption {
			return nil
		}
	*/

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"state":        pdb.Spec.PDBState,
		"modifyOption": pdb.Spec.ModifyOption,
		"getScript":    strconv.FormatBool(*(pdb.Spec.GetScript))}
	log.Info("MODIFY PDB", "pdb.Spec.PDBState=", pdb.Spec.PDBState, "pdb.Spec.ModifyOption=", pdb.Spec.ModifyOption)
	log.Info("PDB STATUS OPENMODE", "pdb.Status.OpenMode=", pdb.Status.OpenMode)

	pdbName := pdb.Spec.PDBName
	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/status"

	pdb.Status.Phase = pdbPhaseModify
	pdb.Status.ModifyOption = pdb.Spec.PDBState + "-" + pdb.Spec.ModifyOption
	pdb.Status.Msg = "Waiting for PDB to be modified"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = NewCallApi(r, ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Modified", "PDB '%s' modified successfully", pdb.Spec.PDBName)

	if cdb.Spec.DBServer != "" {
		pdb.Status.ConnString = cdb.Spec.DBServer + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	} else {
		pdb.Status.ConnString = cdb.Spec.DBTnsurl
	}

	log.Info("Successfully modified PDB state", "PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*
************************************************
  - Get PDB State
    /***********************************************
*/
func (r *PDBReconciler) getPDBState(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("getPDBState", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	pdbName := pdb.Spec.PDBName
	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/status"

	pdb.Status.Msg = "Getting PDB state"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}

	respData, err := NewCallApi(r, ctx, req, pdb, url, nil, "GET")

	if err != nil {
		pdb.Status.OpenMode = "UNKNOWN"
		pdb.Status.Msg = "CHECK PDB STATUS"
		pdb.Status.Status = false
		return err
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
		log.Error(err, "Failed to get state of PDB :"+pdbName, "err", err.Error())
	}

	pdb.Status.OpenMode = objmap["open_mode"].(string)

	if pdb.Status.OpenMode == "READ WRITE" {
		err := r.mapPDB(ctx, req, pdb)
		if err != nil {
			log.Info("Fail to Map resource getting PDB state")
		}
	}

	log.Info("Successfully obtained PDB state", "PDB Name", pdb.Spec.PDBName, "State", objmap["open_mode"].(string))
	return nil
}

/*
************************************************
  - Map Database PDB to Kubernetes PDB CR
    /***********************************************
*/
func (r *PDBReconciler) mapPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("mapPDB", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	pdbName := pdb.Spec.PDBName
	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/"

	pdb.Status.Msg = "Mapping PDB"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}

	respData, err := NewCallApi(r, ctx, req, pdb, url, nil, "GET")

	if err != nil {
		pdb.Status.OpenMode = "UNKNOWN"
		return err
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
		log.Error(err, "Failed to get state of PDB :"+pdbName, "err", err.Error())
	}

	totSizeInBytes := objmap["total_size"].(float64)
	totSizeInGB := totSizeInBytes / 1024 / 1024 / 1024

	pdb.Status.OpenMode = objmap["open_mode"].(string)
	pdb.Status.TotalSize = fmt.Sprintf("%.2f", totSizeInGB) + "G"
	assertivePdbDeletion = pdb.Spec.AssertivePdbDeletion
	if pdb.Spec.AssertivePdbDeletion == true {
		r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Mapped", "PDB '%s' assertive pdb deletion turned on", pdb.Spec.PDBName)
	}

	if cdb.Spec.DBServer != "" {
		pdb.Status.ConnString = cdb.Spec.DBServer + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	} else {
		pdb.Status.ConnString = cdb.Spec.DBTnsurl
		ParseTnsAlias(&(pdb.Status.ConnString), &(pdb.Spec.PDBName))
	}

	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}

	log.Info("Successfully mapped PDB to Kubernetes resource", "PDB Name", pdb.Spec.PDBName)
	return nil
}

/*
************************************************
  - Delete a PDB
    /***********************************************
*/
func (r *PDBReconciler) deletePDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("deletePDB", req.NamespacedName)

	err := r.deletePDBInstance(req, ctx, pdb)
	if err != nil {
		log.Info("Could not delete PDB", "PDB Name", pdb.Spec.PDBName, "err", err.Error())
		return err
	}

	if controllerutil.ContainsFinalizer(pdb, PDBFinalizer) {
		log.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(pdb, PDBFinalizer)
		err := r.Update(ctx, pdb)
		if err != nil {
			log.Info("Could not remove finalizer", "err", err.Error())
			return err
		}
		pdb.Status.Status = true
		err = r.Delete(context.Background(), pdb, client.GracePeriodSeconds(1))
		if err != nil {
			log.Info("Could not delete PDB resource", "err", err.Error())
			return err
		}
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Deleted", "PDB '%s' dropped successfully", pdb.Spec.PDBName)

	log.Info("Successfully deleted PDB resource")
	return nil
}

/*************************************************
  - Check PDB deletion
**************************************************/

func (r *PDBReconciler) managePDBDeletion2(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {
	log := r.Log.WithValues("managePDBDeletion", req.NamespacedName)
	if pdb.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(pdb, PDBFinalizer) {
			controllerutil.AddFinalizer(pdb, PDBFinalizer)
			if err := r.Update(ctx, pdb); err != nil {
				return err
			}
		}
	} else {
		log.Info("Pdb marked to be delted")
		if controllerutil.ContainsFinalizer(pdb, PDBFinalizer) {
			if assertivePdbDeletion == true {
				log.Info("Deleting pdb CRD: Assertive approach is turned on ")
				cdb, err := r.getCDBResource(ctx, req, pdb)
				if err != nil {
					log.Error(err, "Cannont find cdb resource ", "err", err.Error())
					return err
				}

				var errclose error
				pdbName := pdb.Spec.PDBName
				if pdb.Status.OpenMode == "READ WRITE" {
					valuesclose := map[string]string{
						"state":        "CLOSE",
						"modifyOption": "IMMEDIATE",
						"getScript":    "FALSE"}
					url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/status"
					_, errclose = NewCallApi(r, ctx, req, pdb, url, valuesclose, "POST")
					if errclose != nil {
						log.Info("Warning error closing pdb continue anyway")
					}
				}

				if errclose == nil {
					valuesdrop := map[string]string{
						"action":    "INCLUDING",
						"getScript": "FALSE"}
					url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/"

					log.Info("Call Delete()")
					_, errdelete := NewCallApi(r, ctx, req, pdb, url, valuesdrop, "DELETE")
					if errdelete != nil {
						log.Error(errdelete, "Fail to delete pdb :"+pdb.Name, "err", errdelete.Error())
						return errdelete
					}
				}

			} /* END OF ASSERTIVE SECTION */

			log.Info("Marked to be deleted")
			pdb.Status.Phase = pdbPhaseDelete
			pdb.Status.Status = true
			r.Status().Update(ctx, pdb)

			controllerutil.RemoveFinalizer(pdb, PDBFinalizer)
			if err := r.Update(ctx, pdb); err != nil {
				log.Info("Cannot remove finalizer")
				return err
			}

		}

		return nil
	}

	return nil
}

/*
************************************************
  - Finalization logic for PDBFinalizer
    /***********************************************
*/
func (r *PDBReconciler) deletePDBInstance(req ctrl.Request, ctx context.Context, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("deletePDBInstance", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"action":    "KEEP",
		"getScript": strconv.FormatBool(*(pdb.Spec.GetScript))}

	if pdb.Spec.DropAction != "" {
		values["action"] = pdb.Spec.DropAction
	}

	pdbName := pdb.Spec.PDBName
	url := "https://" + pdb.Spec.CDBResName + "-ords." + pdb.Spec.CDBNamespace + ":" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/"

	pdb.Status.Phase = pdbPhaseDelete
	pdb.Status.Msg = "Waiting for PDB to be deleted"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = NewCallApi(r, ctx, req, pdb, url, values, "DELETE")
	if err != nil {
		pdb.Status.ConnString = ""
		return err
	}

	log.Info("Successfully dropped PDB", "PDB Name", pdbName)
	return nil
}

/*
*************************************************************
  - SetupWithManager sets up the controller with the Manager.
    /************************************************************
*/
func (r *PDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.PDB{}).
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
Enh 35357707 - PROVIDE THE PDB TNSALIAS INFORMATION
**************************************************************/

func ParseTnsAlias(tns *string, pdbsrv *string) {
	var swaptns string
	fmt.Printf("Analyzing string [%s]\n", *tns)
	fmt.Printf("Relacing  srv [%s]\n", *pdbsrv)

	if strings.Contains(strings.ToUpper(*tns), "SERVICE_NAME") == false {
		fmt.Print("Cannot generate tns alias for pdb")
		return
	}

	if strings.Contains(strings.ToUpper(*tns), "ORACLE_SID") == true {
		fmt.Print("Cannot generate tns alias for pdb")
		return
	}

	swaptns = fmt.Sprintf("SERVICE_NAME=%s", *pdbsrv)
	tnsreg := regexp.MustCompile(`SERVICE_NAME=\w+`)
	*tns = tnsreg.ReplaceAllString(*tns, swaptns)

	fmt.Printf("Newstring [%s]\n", *tns)

}

func NewCallApi(intr interface{}, ctx context.Context, req ctrl.Request, pdb *dbapi.PDB, url string, payload map[string]string, action string) (string, error) {

	var c client.Client
	var r logr.Logger
	var e record.EventRecorder
	var err error

	recpdb, ok1 := intr.(*PDBReconciler)
	if ok1 {
		fmt.Printf("func NewCallApi ((*PDBReconciler),......)\n")
		c = recpdb.Client
		e = recpdb.Recorder
		r = recpdb.Log
	}

	reccdb, ok2 := intr.(*CDBReconciler)
	if ok2 {
		fmt.Printf("func NewCallApi ((*CDBReconciler),......)\n")
		c = reccdb.Client
		e = reccdb.Recorder
		r = reccdb.Log
	}

	secret := &corev1.Secret{}

	log := r.WithValues("NewCallApi", req.NamespacedName)
	log.Info("Call c.Get")
	err = c.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBTlsKey.Secret.SecretName, Namespace: pdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBTlsKey.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	rsaKeyPEM := secret.Data[pdb.Spec.PDBTlsKey.Secret.Key]

	err = c.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBTlsCrt.Secret.SecretName, Namespace: pdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBTlsCrt.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	rsaCertPEM := secret.Data[pdb.Spec.PDBTlsCrt.Secret.Key]

	err = c.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBTlsCat.Secret.SecretName, Namespace: pdb.Namespace}, secret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBTlsCat.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}

	caCert := secret.Data[pdb.Spec.PDBTlsCat.Secret.Key]
	/*
		r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "ORDSINFO", string(rsaKeyPEM))
		r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "ORDSINFO", string(rsaCertPEM))
		r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "ORDSINFO", string(caCert))
	*/

	certificate, err := tls.X509KeyPair([]byte(rsaCertPEM), []byte(rsaKeyPEM))
	if err != nil {
		pdb.Status.Msg = "Error tls.X509KeyPair"
		return "", err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConf := &tls.Config{Certificates: []tls.Certificate{certificate}, RootCAs: caCertPool}

	tr := &http.Transport{TLSClientConfig: tlsConf}

	httpclient := &http.Client{Transport: tr}

	log.Info("Issuing REST call", "URL", url, "Action", action)

	/*
		cdb, err := r.getCDBResource(ctx, req, pdb)
		if err != nil {
			return "", err
		}
	*/

	err = c.Get(ctx, types.NamespacedName{Name: pdb.Spec.WebServerUsr.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.WebServerUsr.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	webUserEnc := string(secret.Data[pdb.Spec.WebServerUsr.Secret.Key])
	webUserEnc = strings.TrimSpace(webUserEnc)

	err = c.Get(ctx, types.NamespacedName{Name: pdb.Spec.PDBPriKey.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.PDBPriKey.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	privKey := string(secret.Data[pdb.Spec.PDBPriKey.Secret.Key])
	webUser, err := lrcommons.CommonDecryptWithPrivKey(privKey, webUserEnc, req)

	// Get Web Server User Password
	secret = &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: pdb.Spec.WebServerPwd.Secret.SecretName, Namespace: pdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + pdb.Spec.WebServerPwd.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	webUserPwdEnc := string(secret.Data[pdb.Spec.WebServerPwd.Secret.Key])
	webUserPwdEnc = strings.TrimSpace(webUserPwdEnc)
	webUserPwd, err := lrcommons.CommonDecryptWithPrivKey(privKey, webUserPwdEnc, req)
	///////////////////////////////////////////////////////////////////////////////////

	var httpreq *http.Request
	if action == "GET" {
		httpreq, err = http.NewRequest(action, url, nil)
	} else {
		jsonValue, _ := json.Marshal(payload)
		httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
	}

	if err != nil {
		log.Info("Unable to create HTTP Request for PDB : "+pdb.Name, "err", err.Error())
		return "", err
	}

	httpreq.Header.Add("Accept", "application/json")
	httpreq.Header.Add("Content-Type", "application/json")
	httpreq.SetBasicAuth(webUser, webUserPwd)

	resp, err := httpclient.Do(httpreq)
	if err != nil {
		errmsg := err.Error()
		log.Error(err, "Failed - Could not connect to ORDS Pod", "err", err.Error())
		pdb.Status.Msg = "Error: Could not connect to ORDS Pod"
		e.Eventf(pdb, corev1.EventTypeWarning, "ORDSError", errmsg)
		return "", err
	}

	e.Eventf(pdb, corev1.EventTypeWarning, "Done", pdb.Spec.CDBResName)
	if resp.StatusCode != http.StatusOK {
		bb, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode == 404 {
			pdb.Status.ConnString = ""
			pdb.Status.Msg = pdb.Spec.PDBName + " not found"

		} else {
			if floodcontrol == false {
				pdb.Status.Msg = "ORDS Error - HTTP Status Code:" + strconv.Itoa(resp.StatusCode)
			}
		}

		if floodcontrol == false {
			log.Info("ORDS Error - HTTP Status Code :"+strconv.Itoa(resp.StatusCode), "Err", string(bb))
		}

		var apiErr ORDSError
		json.Unmarshal([]byte(bb), &apiErr)
		if floodcontrol == false {
			e.Eventf(pdb, corev1.EventTypeWarning, "ORDSError", "Failed: %s", apiErr.Message)
		}
		//fmt.Printf("%+v", apiErr)
		//fmt.Println(string(bb))
		floodcontrol = true
		return "", errors.New("ORDS Error")
	}
	floodcontrol = false

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Print(err.Error())
	}
	respData := string(bodyBytes)
	//fmt.Println(string(bodyBytes))

	var apiResponse RESTSQLCollection
	json.Unmarshal([]byte(bodyBytes), &apiResponse)
	//fmt.Printf("%#v", apiResponse)
	//fmt.Printf("%+v", apiResponse)

	errFound := false
	for _, sqlItem := range apiResponse.Items {
		if sqlItem.ErrorDetails != "" {
			log.Info("ORDS Error - Oracle Error Code :" + strconv.Itoa(sqlItem.ErrorCode))
			if !errFound {
				pdb.Status.Msg = sqlItem.ErrorDetails
			}
			e.Eventf(pdb, corev1.EventTypeWarning, "OraError", "%s", sqlItem.ErrorDetails)
			errFound = true
		}
	}

	if errFound {
		return "", errors.New("Oracle Error")
	}

	return respData, nil
}
