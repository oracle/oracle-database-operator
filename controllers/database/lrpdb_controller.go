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
	"reflect"
	"sort"

	//"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	. "github.com/oracle/oracle-database-operator/commons/multitenant/lrest"

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

/*

   BITMASK STATUS RECAP.
   ~~~~~~~~~~~~~~~~~~~~
   PDBCRT = 0x00000001 -- Create pdb
   PDBOPN = 0x00000002 -- Open pdb read write
   PDBCLS = 0x00000004 -- Close pdb
   PDBDIC = 0x00000008 -- Drop pdb include datafiles
   OCIHDL = 0x00000010 -- OCI handle allocation
   OCICON = 0x00000020 -- Rdbms connection
   FNALAZ = 0x00000040 -- Finalizer configured
   PDBUPL = 0x00000080 -- Unplug pdb
   PDBPLG = 0x00000100 -- plug pdb
   -- Error section --
   PDBCRE = 0x00001000 -- PDB creation error
   PDBOPE = 0x00002000 -- PDB open error
   PDBCLE = 0x00004000 -- PDB close error
   OCIHDE = 0x00008000 -- Allocation Handle Error
   OCICOE = 0x00010000 -- CDD connection Error
   FNALAE = 0x00020000 -- Finalizer error
   PDBUPE = 0x00040000 -- Unplug Error
   PDBPLE = 0x00080000 -- Plug Error
   PDBPLW = 0x00100000 -- Plug Warining
   -- Autodiscover
   PDBAUT = 0x01000000 -- Autodisover


   BITMASK CONFIGMAP PARAMETER RECAP.
   ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
   MPAPPL = 0x00000001 -- The map config has been applyed
   MPSYNC = 0x00000002 -- The map config is in sync with v$parameters where is default=flase
   MPEMPT = 0x00000004 -- The map is empty - not specify
   MPWARN = 0x00000008 -- Map applied with warnings
   MPINIT = 0x00000010 -- Config map init
   SPARE3 = 0x00000020 --


*/

// Bitmask functions

const (
	DBGAPI = 0x00000001 /* Debug NewcallApi */
	DBGCRT = 0x00000002 /* Debug pdb creation */
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

type PLSQLPayLoad struct {
	Values    map[string]string
	Sqltokens []string
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
	lrpdbPhaseApplySql  = "ApplySqlCode"
)

const LRPDBFinalizer = "database.oracle.com/LRPDBfinalizer"

var tde_Password string
var tde_Secret string
var flood_control bool = false
var imperativeLpdbDeletion bool = false /* Global variable for imperative pdb deletion */
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

/**** RECONCILIATION LOOP ****/
func (r *LRPDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("multitenantoperator", req.NamespacedName)
	log.Info("Reconcile requested")

	reconcilePeriod := r.Interval * time.Second
	requeueY := ctrl.Result{Requeue: true, RequeueAfter: reconcilePeriod}
	//requeueN := ctrl.Result{}

	var err error
	lrpdb := &dbapi.LRPDB{}

	/**** GET CLIENT ****/
	err = r.Client.Get(context.TODO(), req.NamespacedName, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PDB resource not found", "Pdb", lrpdb.Spec.LRPDBName)
			return requeueN, nil
		}
		log.Info("Client.Get Error")
		return requeueN, err
	}

	/****  CREATE ****/
	if Bit(lrpdb.Status.PDBBitMask, PDBCRT|PDBCRE) == false && lrpdb.Spec.SrcLRPDBName == "" && lrpdb.Spec.XMLFileName == "" {
		log.Info("REC. LOOP: create pdb")
		err = r.CreateLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/*** INIT CONFIG MAP ***/
	if Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && Bit(lrpdb.Status.CmBitstat, MPINIT) == false {
		log.Info("REC. LOOP: init config map")
		r.InitConfigMap(ctx, req, lrpdb)
	}

	/*** FINALYZER ***/
	if Bit(lrpdb.Status.PDBBitMask, FNALAZ) == false && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true {
		if lrpdb.ObjectMeta.DeletionTimestamp.IsZero() {
			if !controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
				log.Info("REC. LOOP: add finalizer")
				controllerutil.AddFinalizer(lrpdb, LRPDBFinalizer)
				if err := r.Update(ctx, lrpdb); err != nil {
					log.Info("Cannot add finalizer")
					return requeueN, err

				}
				lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, FNALAZ)
				lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
				r.UpdateStatus(ctx, req, lrpdb)
			}
		}
	}

	/**** OPEN ****/
	if lrpdb.Spec.LRPDBState == "OPEN" && Bit(lrpdb.Status.PDBBitMask, PDBOPN|PDBOPE) == false {
		log.Info("REC. LOOP: open pdb")
		err = r.OpenLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}
	}

	/**** CLOSE ****/
	if lrpdb.Spec.LRPDBState == "CLOSE" && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true {
		log.Info("REC. LOOP: open pdb")
		err = r.CloseLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}
	}

	/**** DELETE (imperative approach) ****/
	if !lrpdb.ObjectMeta.DeletionTimestamp.IsZero() && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && Bit(lrpdb.Status.PDBBitMask, FNALAZ) == true && Bit(lrpdb.Status.PDBBitMask, PDBDIC) == false {
		log.Info("REC. LOOP: delete  pdb - imperative approach")
		log.Info(" ObjectMeta.DeletionTimestamp.IsZero is not null")
		err = r.DeleteLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/**** DELETE (declarative approach) ****/
	if lrpdb.Spec.LRPDBState == "DELETE" && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && Bit(lrpdb.Status.PDBBitMask, FNALAZ) == true && Bit(lrpdb.Status.PDBBitMask, PDBDIC) == false {
		log.Info("REC. LOOP: delete  pdb - imperative approach")
		err = r.DeleteLRPDBDeclarative(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/**** CLONE *****/
	if lrpdb.Spec.SrcLRPDBName != "" && Bit(lrpdb.Status.PDBBitMask, PDBCRT|FNALAZ|PDBCRE) == false {
		log.Info("REC. LOOP: clone  pdb ")
		err = r.CloneLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/**** UNPLUG AND PLUG SECTION ****/
	if lrpdb.Spec.LRPDBState == "UNPLUG" && lrpdb.Spec.XMLFileName != "" && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && Bit(lrpdb.Status.PDBBitMask, FNALAZ) == true && Bit(lrpdb.Status.PDBBitMask, PDBUPE) == false {
		log.Info("REC. LOOP: unplug  pdb ")
		err = r.UnplugLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	if lrpdb.Spec.LRPDBState == "PLUG" && lrpdb.Spec.XMLFileName != "" && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == false && Bit(lrpdb.Status.PDBBitMask, PDBPLE) == false {
		log.Info("REC. LOOP: plug  pdb ")
		err = r.PlugLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/**** APPLY CONFIG MAP PARAMETER ****/
	if lrpdb.Spec.PDBConfigMap != "" && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && Bit(lrpdb.Status.CmBitstat, MPAPPL) == false && lrpdb.Spec.LRPDBState != "UNPLUG" {
		log.Info("REC. LOOP: plug  pdb ")
		log.Info("Apply configmap:" + lrpdb.Spec.PDBConfigMap)
		Cardinality, err := r.ApplyConfigMap(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}
		log.Info("Config. Map Cardinality:" + strconv.FormatInt(int64(Cardinality), 10))

	}

	/**** APPLY PLSQL/SQL SCRIPT *****/
	if lrpdb.Spec.PLSQLBlock != "" && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && lrpdb.Spec.LRPDBState != "UNPLUG" && Bit(lrpdb.Status.CmBitstat, MPINIT) == true && Bit(lrpdb.Status.PDBBitMask, FNALAZ) == true {
		log.Info("REC. LOOP: apply plsql/sql")
		err = r.execPLSQL(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/**** ALTER SYSTEM ****/
	if lrpdb.Spec.AlterSystemValue != "" && lrpdb.Spec.AlterSystemParameter != "" && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && lrpdb.Spec.LRPDBState != "UNPLUG" && Bit(lrpdb.Status.CmBitstat, MPINIT) == true && Bit(lrpdb.Status.PDBBitMask, FNALAZ) == true && lrpdb.Spec.PLSQLBlock == "" {
		log.Info("REC. LOOP: Alter system ")
		err = r.alterSystemLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/****  MONITOR PDB *****/
	if Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && Bit(lrpdb.Status.PDBBitMask, FNALAZ) == true && lrpdb.Spec.PLSQLBlock == "" && lrpdb.Spec.AlterSystemValue == "" && lrpdb.Spec.XMLFileName == "" && Bit(lrpdb.Status.CmBitstat, MPINIT) == true {
		log.Info("REC. LOOP: Monitor PDB")
		err = r.MonitorLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}

	}

	/* REST STAT */
	if lrpdb.Spec.PDBBitMask != 0 && lrpdb.Spec.LRPDBState == "RESET" {
		log.Info("REC. LOOP: reset state")
		lrpdb.Status.PDBBitMask = lrpdb.Spec.PDBBitMask
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.UpdateStatus(ctx, req, lrpdb)
		lrpdb.Spec.PDBBitMask = 0
		lrpdb.Spec.LRPDBState = "NONE"

		err = r.Update(ctx, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}
	}

	return requeueY, nil
}

/*
*********************************************************************
  - MONITOR PDB

*********************************************************************
*/

func (r *LRPDBReconciler) MonitorLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("MonitorLRPDB ", req.NamespacedName)
	r.getLRPDBState(ctx, req, lrpdb)

	/* Check open mode consistency */
	if Bit(lrpdb.Status.PDBBitMask, PDBCLS) == true && lrpdb.Status.OpenMode == "READ WRITE" {
		log.Info("Open mode inconsistency.......:target:close - status read write")
		log.Info("Fix inconsistency.............:call(r.CloseLRPDB(ctx, req, lrpdb) )")
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "open mode inconsistency", "Target:[PDBCLS] Status:['%s']", lrpdb.Status.OpenMode)
		err := r.CloseLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}

		return nil
	}

	if Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true && lrpdb.Status.OpenMode == "MOUNTED" {
		log.Info("Open mode inconsistency.......:target:read write - status mounted")
		log.Info("Fix inconsistency.............:call(r.OpenLRPDB(ctx, req, lrpdb) )")
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "open mode inconsistency", "Target:[PDBOPN] Status:['%s']", lrpdb.Status.OpenMode)
		err := r.OpenLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		return nil
	}

	return nil
}

/*
*********************************************************************
  - PLUG PDB

*********************************************************************
*/

func (r *LRPDBReconciler) PlugLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("PlugLRPDB", req.NamespacedName)
	globalsqlcode = 0

	var err error
	// var tde_Password string
	// var tde_Secret string

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

	/*
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
	*/

	lrpdb.Status.Msg = "plug:[op. in progress]"
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBPLG)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	r.UpdateStatus(ctx, req, lrpdb)

	url := r.BaseUrl(ctx, req, lrpdb, lrest)

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode != 0 {
		globalsqlcode = lrpdb.Status.SqlCode
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBPLE) /* Upplug error */
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBPLG) /* Remove unplug flag */
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode) /* Print the oracle error */
		lrpdb.Status.Msg = "close:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return errors.New(oer)

	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "LRPDB '%s' plugged successfully", lrpdb.Spec.LRPDBName)

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		log.Info("Parsing connectstring")
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))
	}

	imperativeLpdbDeletion = lrpdb.Spec.ImperativeLrpdbDeletion
	if lrpdb.Spec.ImperativeLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Plug", "PDB '%s' imperative pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	r.getLRPDBState(ctx, req, lrpdb)

	lrpdb.Status.Msg = "plug:[op. completed]"
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT) /* Set the creation flag */
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBOPN) /* Set the creation flag */
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	r.UpdateStatus(ctx, req, lrpdb)

	log.Info("Successfully plugged LRPDB", "LRPDB Name", lrpdb.Spec.LRPDBName)
	return nil
}

/*
*********************************************************************
  - UNPLUG PDB

*********************************************************************
*/

func (r *LRPDBReconciler) UnplugLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("unplugLRPDB", req.NamespacedName)
	globalsqlcode = 0

	var err error
	//var tde_Password string
	//var tde_Secret string

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	values := map[string]string{
		"method":      "UNPLUG",
		"xmlFileName": lrpdb.Spec.XMLFileName,
		"getScript":   strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	/*
		if *(lrpdb.Spec.LTDEExport) {
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
	*/

	lrpdb.Status.Msg = "unplug:[op. in progress]"
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBUPL)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)

	if Bit(lrpdb.Status.PDBBitMask, PDBPLG) { /*database already plugged in the past */
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBPLG)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	}

	r.UpdateStatus(ctx, req, lrpdb)
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdb.Spec.LRPDBName

	log.Info("CallAPI(url)", "url", url)
	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	r.UpdateStatus(ctx, req, lrpdb)

	if lrpdb.Status.SqlCode != 0 {
		globalsqlcode = lrpdb.Status.SqlCode
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBUPE) /* Upplug error */
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBUPL) /* Remove unplug flag */
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode) /* Print the oracle error */
		lrpdb.Status.Msg = "close:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return errors.New(oer)

	}

	/*... CRD is going to be delete... loging message in the logfile */
	lrpdb.Status.Msg = "unplug:[op. completed]"
	r.UpdateStatus(ctx, req, lrpdb)
	log.Info("unplug:[op. completed]")

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

/*
*********************************************************************
  - OPEN PDB

*********************************************************************
*/
func (r *LRPDBReconciler) OpenLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("OpenLRPDB", req.NamespacedName)
	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Modify", "Info:'%s %s %s' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState, lrpdb.Status.ModifyOption)

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		log.Info("Failure: Cannot get lrest info")
		return err
	}

	values := map[string]string{}
	values = map[string]string{
		"state":         lrpdb.Spec.LRPDBState,
		"modifyOption":  lrpdb.Spec.ModifyOption,
		"modifyOption2": lrpdb.Spec.ModifyOption2,
		"getScript":     strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		log.Info("MODIFY LRPDB", "lrpdb.Spec.LRPDBState=", lrpdb.Spec.LRPDBState, "lrpdb.Spec.ModifyOption=", lrpdb.Spec.ModifyOption)
		log.Info("LRPDB STATUS OPENMODE", "lrpdb.Status.OpenMode=", lrpdb.Status.OpenMode)
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName

	lrpdb.Status.Msg = "open:[op in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	/* if sqlcode is zero then unset the closebit */
	if lrpdb.Status.SqlCode == 0 {
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBCLS)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	}

	r.UpdateStatus(ctx, req, lrpdb)

	/* Return Error if sqlcode != */
	if lrpdb.Status.SqlCode != 0 {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBOPE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "open:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return errors.New(oer)
	}

	globalsqlcode = lrpdb.Status.SqlCode
	r.getLRPDBState(ctx, req, lrpdb)

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Modified", " '%s' modified successfully '%s'", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState)
	}

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))

	}

	lrpdb.Status.Msg = "open:[op. completed]"

	log.Info("Successfully modified LRPDB state", "LRPDB Name", lrpdb.Spec.LRPDBName)

	/* After database openining we reapply the config map if warning is present */
	if lrpdb.Spec.LRPDBState == "OPEN" {
		if bit(lrpdb.Status.CmBitstat, MPWARN|MPINIT) {
			log.Info("re-apply config map")
			r.ApplyConfigMap(ctx, req, lrpdb)

		}
	}
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBOPN)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	r.UpdateStatus(ctx, req, lrpdb)

	return nil
}

/*
*********************************************************************
  - CLOSE PDB

*********************************************************************
*/
func (r *LRPDBReconciler) CloseLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("CloseLRPDB", req.NamespacedName)
	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Close", "Info:'%s %s %s' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState, lrpdb.Status.ModifyOption)

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		log.Info("Failure: Cannot get lrest info")
		return err
	}

	values := map[string]string{}
	values = map[string]string{
		"state":         lrpdb.Spec.LRPDBState,
		"modifyOption":  lrpdb.Spec.ModifyOption,
		"modifyOption2": lrpdb.Spec.ModifyOption2,
		"getScript":     strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		log.Info("MODIFY LRPDB", "lrpdb.Spec.LRPDBState=", lrpdb.Spec.LRPDBState, "lrpdb.Spec.ModifyOption=", lrpdb.Spec.ModifyOption)
		log.Info("LRPDB STATUS OPENMODE", "lrpdb.Status.OpenMode=", lrpdb.Status.OpenMode)
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName

	lrpdb.Status.Msg = "close:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	/* if sqlcode is zero then unset the openbit */
	if lrpdb.Status.SqlCode == 0 {
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBOPN)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	}

	r.UpdateStatus(ctx, req, lrpdb)

	/* Return Error if sqlcode != */
	if lrpdb.Status.SqlCode != 0 {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCLE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "close:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return errors.New(oer)
	}

	globalsqlcode = lrpdb.Status.SqlCode
	r.getLRPDBState(ctx, req, lrpdb)

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Modified", " '%s' modified successfully '%s'", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState)
	}

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))

	}

	lrpdb.Status.Msg = "close:[op. completed]"
	log.Info("Successfully modified LRPDB state", "LRPDB Name", lrpdb.Spec.LRPDBName)
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCLS)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	r.UpdateStatus(ctx, req, lrpdb)

	return nil
}

/*
*********************************************************************
  - DELETE PDB - IMPERATIVE APPROAC

*********************************************************************
*/
func (r *LRPDBReconciler) DeleteLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("deleteLRPDB", req.NamespacedName)

	var err error

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		log.Info("Failure: Cannot get lrest info")
		return err
	}

	/* Close the pdb if it's open */
	if Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true {
		valuesclose := map[string]string{
			"state":        "CLOSE",
			"modifyOption": "IMMEDIATE",
			"getScript":    "FALSE"}
		lrpdbName := lrpdb.Spec.LRPDBName
		url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName
		respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, valuesclose, "POST")
		r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
		if lrpdb.Status.SqlCode != 0 {
			oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
			lrpdb.Status.Msg = "close:[" + oer + "]"
			r.UpdateStatus(ctx, req, lrpdb)
		}
		if err != nil {
			log.Info("Warning error closing lrpdb continue anyway")

		}
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBOPN)
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCLS)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.UpdateStatus(ctx, req, lrpdb)

	}

	values := map[string]string{
		"action":    "INCLUDING",
		"getScript": strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if lrpdb.Spec.DropAction != "" {
		values["action"] = lrpdb.Spec.DropAction
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "DELETE")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode
	if lrpdb.Status.SqlCode != 0 {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, FNALAE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "delete:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return err
	} else {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBDIC)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.UpdateStatus(ctx, req, lrpdb)
	}

	log.Info("Successfully dropped LRPDB", "LRPDB Name", lrpdbName)

	controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
	if err := r.Update(ctx, lrpdb); err != nil {
		log.Info("Cannot remove finalizer")
		return err
	}

	return nil
}

/*
*********************************************************************
  - DELETE PDB - DECLARATIVE APPROACH

*********************************************************************
*/

func (r *LRPDBReconciler) DeleteLRPDBDeclarative(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("deleteLRPDBDeclaratve", req.NamespacedName)

	var err error

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		log.Info("Failure: Cannot get lrest info")
		return err
	}

	/* Close the pdb if it's open */
	if Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true {
		valuesclose := map[string]string{
			"state":        "CLOSE",
			"modifyOption": "IMMEDIATE",
			"getScript":    "FALSE"}
		lrpdbName := lrpdb.Spec.LRPDBName
		url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName
		respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, valuesclose, "POST")
		r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
		if lrpdb.Status.SqlCode != 0 {
			oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
			lrpdb.Status.Msg = "close:[" + oer + "]"
			r.UpdateStatus(ctx, req, lrpdb)
		}
		if err != nil {
			log.Info("Warning error closing lrpdb continue anyway")

		}
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBOPN)
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCLS)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.UpdateStatus(ctx, req, lrpdb)

	}

	values := map[string]string{
		"action":    "INCLUDING",
		"getScript": strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if lrpdb.Spec.DropAction != "" {
		values["action"] = lrpdb.Spec.DropAction
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "DELETE")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode
	if lrpdb.Status.SqlCode != 0 {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, FNALAE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "delete:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return err
	} else {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBDIC)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.UpdateStatus(ctx, req, lrpdb)
	}

	log.Info("Successfully dropped LRPDB", "LRPDB Name", lrpdbName)

	if controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
		log.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
		err := r.Update(ctx, lrpdb)
		if err != nil {
			log.Info("Could not remove finalizer", "err", err.Error())
			return err
		}
	}

	err = r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
	if err != nil {
		log.Info("Could not delete LRPDB resource", "err", err.Error())
		return err
	}

	return nil
}

/*
*********************************************************************
  - CHECK BEFORE CLONING

*********************************************************************
*/
func (r *LRPDBReconciler) checkPDBforCloninig(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, targetPdbName string) (int, error) {
	log := r.Log.WithValues("checkPDBforCloninig", req.NamespacedName)
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
		fmt.Printf("DEBUGCLONE %s %s %d\n", p.Spec.LRPDBName, targetPdbName, pdbCounter)
		if p.Spec.LRPDBName == targetPdbName {
			log.Info("Found " + targetPdbName + " in the crd list")
			if p.Status.OpenMode == "MOUNTED" {
				log.Info("Cannot clone a mounted pdb")
				return pdbCounter, err
			}
			pdbCounter++
			fmt.Printf("DEBUGCLONE %s %s %d\n", p.Spec.LRPDBName, targetPdbName, pdbCounter)
			return pdbCounter, err
		}

	}
	return pdbCounter, err
}

/*
*********************************************************************
  - CLONE PDB

*********************************************************************
*/
func (r *LRPDBReconciler) CloneLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("CloneLRPDB", req.NamespacedName)
	if lrpdb.Spec.LRPDBName == lrpdb.Spec.SrcLRPDBName {
		log.Info("Invalid Name")
		return nil
	}

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

	lrpdb.Status.Msg = "clone:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode
	r.UpdateStatus(ctx, req, lrpdb)

	if lrpdb.Status.SqlCode != 0 {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "open:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return errors.New(oer)

	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "LRPDB '%s' cloned successfully", lrpdb.Spec.LRPDBName)
	lrpdb.Status.TotalSize = r.GetPdbSize(ctx, req, lrpdb, lrpdb.Spec.SrcLRPDBName)

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		lrpdb.Status.ConnString = strings.TrimSpace(lrest.Spec.DBTnsurl)
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))

	}
	imperativeLpdbDeletion = lrpdb.Spec.ImperativeLrpdbDeletion
	if lrpdb.Spec.ImperativeLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Clone", "PDB '%s' imperative pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	log.Info("Cloned LRPDB successfully", "Source LRPDB Name", lrpdb.Spec.SrcLRPDBName, "Clone LRPDB Name", lrpdb.Spec.LRPDBName)
	r.getLRPDBState(ctx, req, lrpdb)

	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	lrpdb.Status.Msg = "clone:[op. completed]"
	r.UpdateStatus(ctx, req, lrpdb)

	/* If we clone we don't have to re-exec sql/plsql */
	lrpdb.Spec.PLSQLBlock = ""
	if err := r.Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failred to update lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
		return err
	}
	log.Info("plsql block reset :[" + lrpdb.Spec.PLSQLBlock + "]")

	return nil
}

/*
**************************************************************
  - Check for Duplicate LRPDB. Same LRPDB name on the same LREST resource.

**************************************************************
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
			return errors.New("Duplicate LRPDB found")
		}
	}
	return nil
}

/*
*********************************************************************
  - GET THE CUSTOM RESOURCE FOR THE LREST MENTIONED IN THE LRPDB SPEC

*********************************************************************
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

	return lrest, nil
}

/*
*********************************************************************
  - GET THE LREST POD FOR THE LREST MENTIONED IN THE LRPDB SPEC

*********************************************************************
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
*********************************************************************
  - GET SECRET KEY FOR A SECRET NAME

*********************************************************************
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
*********************************************************************
  - CREATE PDB

*********************************************************************
*/
func (r *LRPDBReconciler) CreateLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("CreateLRPDB", req.NamespacedName)
	log.Info("call  getLRESTResource \n")
	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}
	/* If it's not created by lrest autodiscover */
	if Bit(lrpdb.Status.PDBBitMask, PDBAUT) == false {

		var err error
		var tde_Password string
		var tde_Secret string

		AutoDiscover := lrest.Spec.PdbAutoDiscover
		err = r.AutoDiscoverActivation(ctx, req, lrpdb, false)

		/*** reset sqlcode***/
		lrpdb.Status.SqlCode = 0

		lrpdbAdminName, err := r.getEncriptedSecret(ctx, req, lrpdb, lrpdb.Spec.AdminpdbUser.Secret.SecretName, lrpdb.Spec.AdminpdbUser.Secret.Key, lrpdb.Spec.LRPDBPriKey.Secret.SecretName, lrpdb.Spec.LRPDBPriKey.Secret.Key)
		if err != nil {
			log.Error(err, "Unable to find pdb admin user ")
			_ = r.AutoDiscoverActivation(ctx, req, lrpdb, AutoDiscover)
			return err
		}

		lrpdbAdminPwd, err := r.getEncriptedSecret(ctx, req, lrpdb, lrpdb.Spec.AdminpdbPass.Secret.SecretName, lrpdb.Spec.AdminpdbPass.Secret.Key, lrpdb.Spec.LRPDBPriKey.Secret.SecretName, lrpdb.Spec.LRPDBPriKey.Secret.Key)

		if err != nil {
			log.Error(err, "Unable to find pdb admin password ")
			_ = r.AutoDiscoverActivation(ctx, req, lrpdb, AutoDiscover)
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
				_ = r.AutoDiscoverActivation(ctx, req, lrpdb, AutoDiscover)
				return err
			}
			tde_Secret, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDESecret.Secret.SecretName, lrpdb.Spec.LTDESecret.Secret.Key)
			if err != nil {
				_ = r.AutoDiscoverActivation(ctx, req, lrpdb, AutoDiscover)
				return err
			}

			tde_Secret = tde_Secret[:len(tde_Secret)-1]
			tde_Password = tde_Secret[:len(tde_Password)-1]
			values["tde_Password"] = tde_Password
			values["tdeKeystorePath"] = lrpdb.Spec.LTDEKeystorePath
			values["tde_Secret"] = tde_Secret
		}

		url := r.BaseUrl(ctx, req, lrpdb, lrest)
		fmt.Print("============================================================\n")
		fmt.Print(url)
		fmt.Print("\n============================================================\n")
		lrpdb.Status.Msg = "create:[op in progress]"

		r.UpdateStatus(ctx, req, lrpdb)

		respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "POST")
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			_ = r.AutoDiscoverActivation(ctx, req, lrpdb, AutoDiscover)
			return err
		}

		r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
		globalsqlcode = lrpdb.Status.SqlCode
		if lrpdb.Status.SqlCode != 0 {
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRE)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
			oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
			lrpdb.Status.Msg = "create:[" + oer + "]"
			r.UpdateStatus(ctx, req, lrpdb)
			return errors.New(oer)
		} else {
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
			r.UpdateStatus(ctx, req, lrpdb)
		}

		r.getLRPDBState(ctx, req, lrpdb)
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal,
			"Created", "LRPDB '%s' created successfully", lrpdb.Spec.LRPDBName)

		log.Info("Parsing connectstring")
		lrpdb.Status.ConnString = strings.TrimSpace(lrest.Spec.DBTnsurl)
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))
		r.UpdateStatus(ctx, req, lrpdb)

		imperativeLpdbDeletion = lrpdb.Spec.ImperativeLrpdbDeletion
		if lrpdb.Spec.ImperativeLrpdbDeletion == true {
			r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "PDB '%s' imperative pdb deletion turned on", lrpdb.Spec.LRPDBName)
		}

		_ = r.AutoDiscoverActivation(ctx, req, lrpdb, AutoDiscover)

		lrpdb.Status.Msg = "create:[op completed]"
		r.UpdateStatus(ctx, req, lrpdb)
	} else {
		log.Info("CRD created by autodiscover")
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		lrpdb.Status.ConnString = strings.TrimSpace(lrest.Spec.DBTnsurl)
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))
		lrpdb.Status.Msg = "autodiscover:[op completed]"
		r.UpdateStatus(ctx, req, lrpdb)
	}
	return nil
}

/**************************************************
ALTER SYSTEM lRPDB
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

	lrpdb.Status.ModifyOption = lrpdb.Spec.AlterSystem + " " + lrpdb.Spec.ParameterScope
	lrpdb.Status.Msg = "alter system:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode == 0 {

		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Altered", "LRPDB(name,cmd,sqlcode) '%s %s %d' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.AlterSystem, lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "alter system:[op. completed]"
		r.UpdateStatus(ctx, req, lrpdb)

		/* Reset parameters */
		lrpdb.Spec.AlterSystemValue = ""
		lrpdb.Spec.AlterSystemParameter = ""
		lrpdb.Spec.ParameterScope = ""

		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Cannot rest lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
			return err
		}

		return nil

	}

	if lrpdb.Status.SqlCode != 0 {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "alter system failure", "LRPDB(name,cmd,sqlcode) '%s %s %d' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.AlterSystem, lrpdb.Status.SqlCode)
		erralter := errors.New("Error: cannot modify parameter")

		lrpdb.Status.Msg = "alter system:[op. failure]"
		r.UpdateStatus(ctx, req, lrpdb)

		lrpdb.Spec.AlterSystem = ""
		lrpdb.Spec.ParameterScope = ""
		lrpdb.Spec.ParameterScope = ""

		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Cannot rest lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
			return err
		}

		return erralter
	}

	return nil
}

func (r *LRPDBReconciler) execPLSQL(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("execPLSQL", req.NamespacedName)

	// TO BE DONE Ad control for the pdb existence
	lrpdbName := lrpdb.Spec.LRPDBName

	log.Info("Reading code config Map ")
	configmap, err := r.GetConfigMapCode(ctx, req, lrpdb)
	if err != nil {
		log.Error(err, "Fail to fetch code configmap", "err", err.Error())
		return err
	}

	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		return err
	}

	lrpdb.Status.Msg = "plsql/sql apply[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	var tokens []string
	var CodeSize int
	/** Sort keys **/
	keys := reflect.ValueOf(configmap.Data).MapKeys()
	keysOrder := func(i, j int) bool { return keys[i].Interface().(string) < keys[j].Interface().(string) }
	sort.Slice(keys, keysOrder)
	/** End of sort section **/

	for _, key := range keys {
		Value := configmap.Data[key.Interface().(string)]
		fmt.Printf("Code Block Name (SQL/PLSQL):%s\n", key)
		tokens = strings.Split(Value, "\n")
		/* Debug Section */
		for cnt := range tokens {
			fmt.Printf("line[%d]:%s\n", cnt, tokens[cnt])
			CodeSize += len(tokens[cnt])
		}

		//* removing laste null emlements
		if len(tokens) > 0 {
			tokens = tokens[:len(tokens)-1]
		}

		fmt.Printf("call to restsertver (%s,%d)\n", key, CodeSize)

		jsonpayload := &PLSQLPayLoad{Values: map[string]string{"method": "APPLYSQL"}, Sqltokens: tokens}
		//* Debug section **//

		encjson, _ := json.Marshal(jsonpayload)
		fmt.Println(string(encjson))

		url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName

		respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, jsonpayload, "POST")
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			return err
		}
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			return err
		}

		r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))

		EvLevel := corev1.EventTypeNormal
		skey := fmt.Sprintf("[%s]", key)
		if lrpdb.Status.SqlCode != 0 {
			oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
			lrpdb.Status.Msg = skey + ":[" + oer + "]"
			r.UpdateStatus(ctx, req, lrpdb)
			EvLevel = corev1.EventTypeWarning
		}
		/*
		   Add the timestamp to the event
		*/
		t := time.Now()
		formatted := fmt.Sprintf("APPLYSQL-%02d%02d%02d", t.Hour(), t.Minute(), t.Second())
		r.Recorder.Eventf(lrpdb, EvLevel, formatted, " CODE:SQLCODE '%s':'%d'", skey, lrpdb.Status.SqlCode)

		/* sql execution complete successfully than report the name of the tag */
		if lrpdb.Status.SqlCode == 0 && err == nil {
			lrpdb.Status.LastPLSQL = skey
			r.UpdateStatus(ctx, req, lrpdb)
			/* reset code buffer */
		}
		tokens = nil
		CodeSize = 0
	}

	lrpdb.Spec.PLSQLBlock = "" /* rest block */
	if err := r.Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failred to update lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
		return err
	}
	lrpdb.Status.Msg = "plsql/sql apply[op. completed]"
	r.UpdateStatus(ctx, req, lrpdb)
	log.Info("plsql block reset :[" + lrpdb.Spec.PLSQLBlock + "]")
	return nil
}

/*
************************************************
  - Get LRPDB State

************************************************
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

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, nil, "GET")
	if err != nil {
		lrpdb.Status.Msg = "getLRPDBState failure : check lrpdb status"
		log.Error(err, "Failure NewCallLAPI( "+url+")", "err", err.Error())
		r.UpdateStatus(ctx, req, lrpdb)
		return err
	}

	r.GetSqlCode(respData, &(lrpdb.Status.SqlCode))
	globalsqlcode = lrpdb.Status.SqlCode
	if lrpdb.Status.SqlCode == 1403 {
		lrpdb.Status.OpenMode = "N/A"
		lrpdb.Status.Msg = "N/A ORA-1403"
		return errors.New("NO_DATA_FOUND")
	}

	r.GetOpenMode(respData, &(lrpdb.Status.OpenMode))
	r.GetRestricted(respData, &(lrpdb.Status.Restricted))
	r.GetPdbSize2(respData, &(lrpdb.Status.TotalSize))

	r.UpdateStatus(ctx, req, lrpdb)

	/* lrpdb.Status.Msg = "check lrpdb ok"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}
	*/

	log.Info("Successfully obtained LRPDB state", "LRPDB Name", lrpdb.Spec.LRPDBName, "State", lrpdb.Status.OpenMode)
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
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName + "/status/"
	log.Info("DEBUG NEW URL " + url)

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, nil, "GET")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
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
	imperativeLpdbDeletion = lrpdb.Spec.ImperativeLrpdbDeletion
	if lrpdb.Spec.ImperativeLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Map", "PDB '%s' imperative pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	if lrest.Spec.DBServer != "" {
		lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
	} else {
		lrpdb.Status.ConnString = lrest.Spec.DBTnsurl
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName))
	}

	lrpdb.Status.Status = true

	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	log.Info("Successfully mapped LRPDB to Kubernetes resource", "LRPDB Name", lrpdb.Spec.LRPDBName)
	return nil
}

/*
************************************************
  - Delete a LRPDB
    /***********************************************
*/
func (r *LRPDBReconciler) deleteLRPDB2(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("deleteLRPDB2", req.NamespacedName)

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
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName

	lrpdb.Status.Msg = "Waiting for LRPDB to be deleted"
	if err := r.Status().Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failed to update status for :"+lrpdb.Name, "err", err.Error())
	}

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, values, "DELETE")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
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

	*tns = strings.ReplaceAll(*tns, " ", "")

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
			if imperativeLpdbDeletion == true {
				log.Info("Deleting lrpdb CRD: Imperative approach is turned on ")
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
				url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName

				log.Info("Call Delete()")
				_, errdelete := r.callAPI(ctx, req, lrpdb, url, valuesdrop, "DELETE")
				if errdelete != nil {
					log.Error(errdelete, "Fail to delete lrpdb :"+lrpdb.Name, "err", err.Error())
					return errdelete
				}
			} /* END OF ASSERTIVE SECTION */

			log.Info("Marked to be deleted")
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
		lrpdb.Status.CmBitstat = bis(lrpdb.Status.CmBitstat, MPEMPT)
		lrpdb.Status.CmBitStatStr = CMBitmaskprint(lrpdb.Status.CmBitstat)
		r.UpdateStatus(ctx, req, lrpdb)
		return DbParameters

	} else {

		lrpdb.Status.CmBitstat = bis(lrpdb.Status.CmBitstat, MPINIT)
		lrpdb.Status.CmBitStatStr = CMBitmaskprint(lrpdb.Status.CmBitstat)
		globalconfigmap = lrpdb.Spec.PDBConfigMap
		DbParameters, err := r.GetConfigMap(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, "Fail to fetch configmap ", "err", err.Error())
			return nil
		}

		//ParseConfigMapData(DbParameters)

		r.UpdateStatus(ctx, req, lrpdb)
		return DbParameters
	}

	// return nil
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

func (r *LRPDBReconciler) GetConfigMapCode(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) (*corev1.ConfigMap, error) {
	log := r.Log.WithValues("GetConfigMapCode", req.NamespacedName)
	log.Info("CodeMapGlobal.............:" + lrpdb.Spec.PLSQLBlock)
	CodeBlock, err := k8s.FetchConfigMap(r.Client, lrpdb.Namespace, lrpdb.Spec.PLSQLBlock)
	if err != nil {
		log.Error(err, "Fail to fetch configmap", "err", err.Error())
		return nil, err
	}

	return CodeBlock, nil
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
	tokens := ParseConfigMapData(configmap)
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
					r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "lrpdbinfo", AlterMsg)
					lrpdb.Status.CmBitstat = bis(lrpdb.Status.CmBitstat, MPWARN)
				}

			}
		}

	}

	lrpdb.Status.CmBitstat = bis(lrpdb.Status.CmBitstat, MPAPPL)
	lrpdb.Status.CmBitStatStr = CMBitmaskprint(lrpdb.Status.CmBitstat)

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
	if lrpdb.Spec.Action != "CREATE" && lrpdb.Spec.Action != "APPLYSQL" && lrpdb.Spec.PDBConfigMap != "" && bit(lrpdb.Status.CmBitstat, MPINIT) == false {
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

	log := r.WithValues("NewCallAPISQL", req.NamespacedName)

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
	webUser, err := CommonDecryptWithPrivKey2(privKey, webUserEnc, req)

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
	webUserPwd, err := CommonDecryptWithPrivKey2(privKey, webUserPwdEnc, req)

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

func (r *LRPDBReconciler) GetRestricted(rsp string, restrictmode *string) error {
	log := r.Log.WithValues("GetRestriced", "callAPI(...)")

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		return err
	}

	*restrictmode = string(objmap["restricted"].(string))

	return nil
}

func (r *LRPDBReconciler) GetPdbSize2(rsp string, pdbsize *string) error {
	log := r.Log.WithValues("GetPdbSize2", "callAPI(...)")
	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		return err
	}
	*pdbsize = fmt.Sprintf("%4.2f", ((objmap["total_size"].(float64))/1024/1024/1024)) + "G"
	return nil
}

func (r *LRPDBReconciler) GetOpenMode(rsp string, openmode *string) error {
	log := r.Log.WithValues("GetRestriced", "callAPI(...)")

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		return err
	}

	*openmode = string(objmap["open_mode"].(string))

	return nil
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

func NewCallAPISQL(intr interface{}, ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, url string, payload interface{}, action string) (string, error) {
	var c client.Client
	var r logr.Logger
	var e record.EventRecorder
	var TestBuffer string
	var jsonMap map[string]interface{}
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

	log := r.WithValues("NewCallAPISQL", req.NamespacedName)

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
		RootCAs:                  caCertPool,
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
	webUser, err := CommonDecryptWithPrivKey2(privKey, webUserEnc, req)

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
	webUserPwd, err := CommonDecryptWithPrivKey2(privKey, webUserPwdEnc, req)

	var Httpreq *http.Request

	if action == "GET" {
		Httpreq, err = http.NewRequest(action, url, nil)
	} else {
		/* Section to execute sql and plsql code */
		payloadsql, ok4 := payload.(*PLSQLPayLoad)
		if ok4 {
			TestBuffer = ParseSQLPayload(payloadsql)
			json.Unmarshal([]byte(TestBuffer), &jsonMap)
			jsonValue, _ := json.Marshal(jsonMap)
			Httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
			if bit(lrpdb.Spec.Debug, DBGAPI) {
				fmt.Println("=========================PLSQLDEBUG==============================")
				fmt.Println(string(jsonValue))
				fmt.Println("=========================PLSQLDEBUG==============================")
			}
			if err != nil {
				log.Info("Unable to create HTTP Request for LRPDB : "+lrpdb.Name, "err", err.Error())
				return "", err
			}
		}
		/* Section to execute standard pdb operation */
		payloadpdb, ok3 := payload.(map[string]string)
		if ok3 {
			jsonValue, _ := json.Marshal(payloadpdb)
			Httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
			if bit(lrpdb.Spec.Debug, DBGAPI) {
				fmt.Println("=========================PLSQLDEBUG==============================")
				fmt.Println(string(jsonValue))
				fmt.Println("=========================PLSQLDEBUG==============================")
			}
			if err != nil {
				log.Info("Unable to create HTTP Request for LRPDB : "+lrpdb.Name, "err", err.Error())
				return "", err
			}
		}
	}

	Httpreq.Header.Add("Accept", "application/json")
	Httpreq.Header.Add("Content-Type", "application/json")
	Httpreq.SetBasicAuth(webUser, webUserPwd)

	resp, err := httpclient.Do(Httpreq)
	if err != nil {
		errmsg := err.Error()
		log.Error(err, "Failed - Could not connect to LREST Pod", "err", err.Error())
		lrpdb.Status.Msg = "Error: Could not connect to LREST Pod"
		e.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTError", errmsg)
		return "", err
	}

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
	fmt.Printf("%s\n", respData)

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

func ParseSQLPayload(payload *PLSQLPayLoad) string {
	var Buffer string

	cnt := 0
	Buffer = "{"
	for key, value := range payload.Values {
		Buffer += "\"" + key + "\" : \"" + value + "\","
	}

	Nelem := len(payload.Sqltokens)
	fmt.Printf("ParseSQLPayload :: Num tokens %d\n", Nelem)
	Buffer += "\"Sqltokens\":["
	for _, value := range payload.Sqltokens {
		Buffer += "\"" + value + "\""
		if cnt < (Nelem - 1) {
			Buffer += ","
		}
		cnt++
	}

	Buffer += "]}"
	fmt.Printf("ParseSQLPayload :: %s\n", Buffer)
	return Buffer
}

func (r *LRPDBReconciler) AutoDiscoverActivation(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, active bool) error {

	log := r.Log.WithValues("AutoDiscoverActivation", req.NamespacedName)
	if active == false {
		log.Info("Disable autodiscover")
	} else {
		log.Info("Enable autodiscover")
	}

	var lrest dbapi.LREST
	lrestResName := lrpdb.Spec.CDBResName
	lrestNamespace := lrpdb.Spec.CDBNamespace
	err := r.Get(context.Background(), client.ObjectKey{
		Namespace: lrestNamespace,
		Name:      lrestResName,
	}, &lrest)
	lrest.Spec.PdbAutoDiscover = active
	err = r.Update(context.TODO(), &lrest)
	if err != nil {
		return err
	}

	return nil
}

func (r *LRPDBReconciler) GetPdbSize(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, pdbaname string) string {
	log := r.Log.WithValues("GetPdbSize", req.NamespacedName)
	var PdbSize string
	// if we cannot get the pdbsize ,whatever reason, we return "undefined" size
	lrest, err := r.getLRESTResource(ctx, req, lrpdb)
	if err != nil {
		log.Info("Cannot get lrest server")
		return "undefined"
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseUrl(ctx, req, lrpdb, lrest) + lrpdbName + "/status/"

	respData, err := NewCallAPISQL(r, ctx, req, lrpdb, url, nil, "GET")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return "undefined"
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(respData), &objmap); err != nil {
		log.Error(err, "Failed json.Unmarshal :"+lrpdbName, "err", err.Error())
		return "undefined"
	}

	PdbSize = fmt.Sprintf("%4.2f", ((objmap["total_size"].(float64))/1024/1024/1024)) + "G"
	return PdbSize
}

func (r *LRPDBReconciler) UpdateStatus(ctx context.Context, req ctrl.Request, lrpdb *databasev4.LRPDB) {
	log := r.Log.WithValues("UpdateStatus", req.NamespacedName)
	err := r.Status().Update(ctx, lrpdb)
	if err != nil {
		fmt.Printf("[1]Error updating status\n")
		log.Error(err, err.Error())
	}
}
