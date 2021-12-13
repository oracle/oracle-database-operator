/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"

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
	pdbPhaseFail   = "Failed"
)

const PDBFinalizer = "database.oracle.com/PDBfinalizer"

//+kubebuilder:rbac:groups=database.oracle.com,resources=pdbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=pdbs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=pdbs/finalizers,verbs=get;create;update;patch;delete

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
	log := r.Log.WithValues("onpremdboperator", req.NamespacedName)
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
	err = r.managePDBDeletion(ctx, req, pdb)
	if err != nil {
		log.Info("Reconcile queued")
		return requeueY, nil
	}

	action := strings.ToUpper(pdb.Spec.Action)

	if (pdb.Status.Phase == pdbPhaseReady) && (pdb.Status.Action != "") && (action == "MODIFY" || pdb.Status.Action != action) {
		if action == "MODIFY" {
			modOption := pdb.Spec.PDBState + "-" + pdb.Spec.ModifyOption
			// To prevent Reconcile from Modifying again whenever the Operator gets re-started
			if pdb.Status.ModifyOption != modOption {
				pdb.Status.Status = false
			}
		} else {
			pdb.Status.Status = false
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
	return requeueN, nil
}

/*************************************************
 * Validate the PDB Spec
 /************************************************/
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
	}

	log.Info("Validation complete")
}

/****************************************************************
 * Get the Custom Resource for the CDB mentioned in the PDB Spec
 /***************************************************************/
func (r *PDBReconciler) getCDBResource(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) (dbapi.CDB, error) {

	log := r.Log.WithValues("getCDBResource", req.NamespacedName)

	var cdb dbapi.CDB // CDB CR corresponding to the CDB name specified in the PDB spec

	// Name of the CDB CR that holds the ORDS container
	cdbResName := pdb.Spec.CDBResName

	// Get CDB CR corresponding to the CDB name specified in the PDB spec
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: req.Namespace,
		Name:      cdbResName,
	}, &cdb)

	if err != nil {
		log.Info("Failed to get CRD for CDB", "Name", cdbResName, "Namespace", req.Namespace, "Error", err.Error())
		pdb.Status.Msg = "Unable to get CRD for CDB : " + cdbResName
		r.Status().Update(ctx, pdb)
		return cdb, err
	}

	log.Info("Found CR for CDB", "Name", cdbResName, "CR Name", cdb.Name)
	return cdb, nil
}

/****************************************************************
 * Get the ORDS Pod for the CDB mentioned in the PDB Spec
 /***************************************************************/
func (r *PDBReconciler) getORDSPod(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) (corev1.Pod, error) {

	log := r.Log.WithValues("getORDSPod", req.NamespacedName)

	var cdbPod corev1.Pod // ORDS Pod container with connection to the concerned CDB

	// Name of the CDB CR that holds the ORDS container
	cdbResName := pdb.Spec.CDBResName

	// Get ORDS Pod associated with the CDB Name specified in the PDB Spec
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: req.Namespace,
		Name:      cdbResName + "-ords",
	}, &cdbPod)

	if err != nil {
		log.Info("Failed to get Pod for CDB", "Name", cdbResName, "Namespace", req.Namespace, "Error", err.Error())
		pdb.Status.Msg = "Unable to get ORDS Pod for CDB : " + cdbResName
		return cdbPod, err
	}

	log.Info("Found ORDS Pod for CDB", "Name", cdbResName, "Pod Name", cdbPod.Name, "ORDS Container hostname", cdbPod.Spec.Hostname)
	return cdbPod, nil
}

/*************************************************
 * Get Secret Key for a Secret Name
 /************************************************/
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

/*************************************************
 * Issue a REST API Call to the ORDS container
 /************************************************/
func (r *PDBReconciler) callAPI(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB, url string, payload map[string]string, action string) (string, error) {
	log := r.Log.WithValues("callAPI", req.NamespacedName)

	var err error
	httpclient := &http.Client{}

	log.Info("Issuing REST call", "URL", url, "Action", action)

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return "", err
	}

	// Get Web Server User
	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: cdb.Spec.WebServerUser.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + cdb.Spec.WebServerUser.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	webUser := string(secret.Data[cdb.Spec.WebServerUser.Secret.Key])

	// Get Web Server User Password
	secret = &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: cdb.Spec.WebServerPwd.Secret.SecretName, Namespace: cdb.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret not found:" + cdb.Spec.WebServerPwd.Secret.SecretName)
			return "", err
		}
		log.Error(err, "Unable to get the secret.")
		return "", err
	}
	webUserPwd := string(secret.Data[cdb.Spec.WebServerPwd.Secret.Key])

	var httpreq *http.Request
	if action == "GET" {
		httpreq, err = http.NewRequest(action, url, nil)
	} else {
		//fmt.Println("payload:", payload)
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
		log.Error(err, "Failed - Could not connect to ORDS Pod", "err", err.Error())
		pdb.Status.Msg = "Could not connect to ORDS Pod"
		r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "ORDSError", "Failed: Could not connect to ORDS Pod")
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		bb, _ := ioutil.ReadAll(resp.Body)
		pdb.Status.Msg = "ORDS Error - HTTP Status Code:" + strconv.Itoa(resp.StatusCode)
		log.Info("ORDS Error - HTTP Status Code :"+strconv.Itoa(resp.StatusCode), "Err", string(bb))

		var apiErr ORDSError
		json.Unmarshal([]byte(bb), &apiErr)
		r.Recorder.Eventf(pdb, corev1.EventTypeWarning, "ORDSError", "Failed: %s", apiErr.Message)
		//fmt.Printf("%+v", apiErr)
		//fmt.Println(string(bb))
		return "", errors.New("ORDS Error")
	}

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

/*************************************************
 * Create a PDB
 /************************************************/
func (r *PDBReconciler) createPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("createPDB", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	pdbAdminName, err := r.getSecret(ctx, req, pdb, pdb.Spec.AdminName.Secret.SecretName, pdb.Spec.AdminName.Secret.Key)
	if err != nil {
		return err
	}
	pdbAdminPwd, err := r.getSecret(ctx, req, pdb, pdb.Spec.AdminPwd.Secret.SecretName, pdb.Spec.AdminPwd.Secret.Key)
	if err != nil {
		return err
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
		tdePassword, err := r.getSecret(ctx, req, pdb, pdb.Spec.TDEPassword.Secret.SecretName, pdb.Spec.TDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tdeSecret, err := r.getSecret(ctx, req, pdb, pdb.Spec.TDESecret.Secret.SecretName, pdb.Spec.TDESecret.Secret.Key)
		if err != nil {
			return err
		}
		values["tdePassword"] = tdePassword
		values["tdeKeystorePath"] = pdb.Spec.TDEKeystorePath
		values["tdeSecret"] = tdeSecret
	}

	url := "http://" + pdb.Spec.CDBResName + "-ords:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/"

	pdb.Status.Phase = pdbPhaseCreate
	pdb.Status.Msg = "Waiting for PDB to be created"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = r.callAPI(ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Created", "PDB '%s' created successfully", pdb.Spec.PDBName)

	pdb.Status.ConnString = cdb.Spec.SCANName + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	log.Info("Created PDB Resource", "PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*************************************************
 * Clone a PDB
 /************************************************/
func (r *PDBReconciler) clonePDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("clonePDB", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
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

	url := "http://" + pdb.Spec.CDBResName + "-ords:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdb.Spec.SrcPDBName + "/"

	pdb.Status.Phase = pdbPhaseClone
	pdb.Status.Msg = "Waiting for PDB to be cloned"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = r.callAPI(ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Created", "PDB '%s' cloned successfully", pdb.Spec.PDBName)

	pdb.Status.ConnString = cdb.Spec.SCANName + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	log.Info("Cloned PDB successfully", "Source PDB Name", pdb.Spec.SrcPDBName, "Clone PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*************************************************
 * Plug a PDB
 /************************************************/
func (r *PDBReconciler) plugPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("plugPDB", req.NamespacedName)

	var err error

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
		tdePassword, err := r.getSecret(ctx, req, pdb, pdb.Spec.TDEPassword.Secret.SecretName, pdb.Spec.TDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tdeSecret, err := r.getSecret(ctx, req, pdb, pdb.Spec.TDESecret.Secret.SecretName, pdb.Spec.TDESecret.Secret.Key)
		if err != nil {
			return err
		}
		values["tdePassword"] = tdePassword
		values["tdeKeystorePath"] = pdb.Spec.TDEKeystorePath
		values["tdeSecret"] = tdeSecret
		values["tdeImport"] = strconv.FormatBool(*(pdb.Spec.TDEImport))
	}
	if *(pdb.Spec.AsClone) {
		values["asClone"] = strconv.FormatBool(*(pdb.Spec.AsClone))
	}

	url := "http://" + pdb.Spec.CDBResName + "-ords:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/"

	pdb.Status.Phase = pdbPhasePlug
	pdb.Status.Msg = "Waiting for PDB to be plugged"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = r.callAPI(ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Created", "PDB '%s' plugged successfully", pdb.Spec.PDBName)

	pdb.Status.ConnString = cdb.Spec.SCANName + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName
	log.Info("Successfully plugged PDB", "PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*************************************************
 * Unplug a PDB
 /************************************************/
func (r *PDBReconciler) unplugPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("unplugPDB", req.NamespacedName)

	var err error

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
		tdePassword, err := r.getSecret(ctx, req, pdb, pdb.Spec.TDEPassword.Secret.SecretName, pdb.Spec.TDEPassword.Secret.Key)
		if err != nil {
			return err
		}
		tdeSecret, err := r.getSecret(ctx, req, pdb, pdb.Spec.TDESecret.Secret.SecretName, pdb.Spec.TDESecret.Secret.Key)
		if err != nil {
			return err
		}
		values["tdePassword"] = tdePassword
		values["tdeKeystorePath"] = pdb.Spec.TDEKeystorePath
		values["tdeSecret"] = tdeSecret
		values["tdeExport"] = strconv.FormatBool(*(pdb.Spec.TDEExport))
	}

	url := "http://" + pdb.Spec.CDBResName + "-ords:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdb.Spec.PDBName + "/"

	pdb.Status.Phase = pdbPhaseUnplug
	pdb.Status.Msg = "Waiting for PDB to be unplugged"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = r.callAPI(ctx, req, pdb, url, values, "POST")
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

/*************************************************
 * Modify a PDB state
 /************************************************/
func (r *PDBReconciler) modifyPDB(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("modifyPDB", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"state":        pdb.Spec.PDBState,
		"modifyOption": pdb.Spec.ModifyOption,
		"getScript":    strconv.FormatBool(*(pdb.Spec.GetScript))}

	pdbName := pdb.Spec.PDBName
	url := "http://" + pdb.Spec.CDBResName + "-ords:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/status"

	pdb.Status.Phase = pdbPhaseModify
	pdb.Status.ModifyOption = pdb.Spec.PDBState + "-" + pdb.Spec.ModifyOption
	pdb.Status.Msg = "Waiting for PDB to be modified"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = r.callAPI(ctx, req, pdb, url, values, "POST")
	if err != nil {
		return err
	}

	r.Recorder.Eventf(pdb, corev1.EventTypeNormal, "Modified", "PDB '%s' modified successfully", pdb.Spec.PDBName)
	pdb.Status.ConnString = cdb.Spec.SCANName + ":" + strconv.Itoa(cdb.Spec.DBPort) + "/" + pdb.Spec.PDBName

	log.Info("Successfully modified PDB state", "PDB Name", pdb.Spec.PDBName)
	r.getPDBState(ctx, req, pdb)
	return nil
}

/*************************************************
 * Get PDB State
 /************************************************/
func (r *PDBReconciler) getPDBState(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("getPDBState", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	pdbName := pdb.Spec.PDBName
	url := "http://" + pdb.Spec.CDBResName + "-ords:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/status"

	pdb.Status.Msg = "Getting PDB state"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}

	respData, err := r.callAPI(ctx, req, pdb, url, nil, "GET")

	if err != nil {
		pdb.Status.OpenMode = "UNKNOWN"
		return err
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
		log.Error(err, "Failed to get state of PDB :"+pdbName, "err", err.Error())
	}

	pdb.Status.OpenMode = objmap["open_mode"].(string)

	log.Info("Successfully obtained PDB state", "PDB Name", pdb.Spec.PDBName)
	return nil
}

/*************************************************
 * Delete a PDB
 /************************************************/
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
 *   Check PDB deletion
 /************************************************/
func (r *PDBReconciler) managePDBDeletion(ctx context.Context, req ctrl.Request, pdb *dbapi.PDB) error {
	log := r.Log.WithValues("managePDBDeletion", req.NamespacedName)

	// Check if the PDB instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isPDBMarkedToBeDeleted := pdb.GetDeletionTimestamp() != nil
	if isPDBMarkedToBeDeleted {
		log.Info("Marked to be deleted")
		if controllerutil.ContainsFinalizer(pdb, PDBFinalizer) {
			// Remove PDBFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			log.Info("Removing finalizer")
			controllerutil.RemoveFinalizer(pdb, PDBFinalizer)
			err := r.Update(ctx, pdb)
			if err != nil {
				log.Info("Could not remove finalizer", "err", err.Error())
				return err
			}
			pdb.Status.Status = true
			log.Info("Successfully removed PDB resource")
			return nil
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(pdb, PDBFinalizer) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(pdb, PDBFinalizer)
		err := r.Update(ctx, pdb)
		if err != nil {
			log.Info("Could not add finalizer", "err", err.Error())
			return err
		}
		pdb.Status.Status = false
	}
	return nil
}

/*************************************************
 *   Finalization logic for PDBFinalizer
 /************************************************/
func (r *PDBReconciler) deletePDBInstance(req ctrl.Request, ctx context.Context, pdb *dbapi.PDB) error {

	log := r.Log.WithValues("deletePDBInstance", req.NamespacedName)

	var err error

	cdb, err := r.getCDBResource(ctx, req, pdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"method":    "DELETE",
		"getScript": strconv.FormatBool(*(pdb.Spec.GetScript))}

	if pdb.Spec.DropAction != "" {
		values["action"] = pdb.Spec.DropAction
	}

	pdbName := pdb.Spec.PDBName
	url := "http://" + pdb.Spec.CDBResName + "-ords:" + strconv.Itoa(cdb.Spec.ORDSPort) + "/ords/_/db-api/latest/database/pdbs/" + pdbName + "/"

	pdb.Status.Phase = pdbPhaseDelete
	pdb.Status.Msg = "Waiting for PDB to be deleted"
	if err := r.Status().Update(ctx, pdb); err != nil {
		log.Error(err, "Failed to update status for :"+pdb.Name, "err", err.Error())
	}
	_, err = r.callAPI(ctx, req, pdb, url, values, "DELETE")
	if err != nil {
		return err
	}

	log.Info("Successfully dropped PDB", "PDB Name", pdbName)
	return nil
}

/**************************************************************
 * SetupWithManager sets up the controller with the Manager.
 /*************************************************************/
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
