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

// Package controllers - Oracle Database Operator
package controllers

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"reflect"
	"sort"

	//"encoding/pem"
	"errors"
	"fmt"

	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	//databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"

	. "github.com/oracle/oracle-database-operator/commons/multitenant/lrest"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"

	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
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
   APPUSR = 0x00000200 -- Application user created
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
   PDBCNE = 0x00200000 -- Call Error
   APPERR = 0x00400000 -- Application user erro
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

// LRPDBReconciler reconciles a LRPDB object
type LRPDBReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Interval time.Duration
	Recorder record.EventRecorder
	Authz    kubernetes.Interface
}

type restSQLCollection struct {
	Env struct {
		DefaultTimeZone string `json:"defaultTimeZone,omitempty"`
	} `json:"env"`
	Items []SQLItem `json:"items"`
}

// SQLItem Oracle handle error output
type SQLItem struct {
	StatementID  int      `json:"statementId,omitempty"`
	Response     []string `json:"response"`
	ErrorCode    int      `json:"errorCode,omitempty"`
	ErrorLine    int      `json:"errorLine,omitempty"`
	ErrorColumn  int      `json:"errorColumn,omitempty"`
	ErrorDetails string   `json:"errorDetails,omitempty"`
	Result       int      `json:"result,omitempty"`
}

// LRESTError json errorcode
type LRESTError struct {
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Type     string `json:"type,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// PLSQLPayLoad plsql/sql code tokes
type PLSQLPayLoad struct {
	Values    map[string]string
	Sqltokens []string
}

// LRPDBFinalizer finalyzer name
const LRPDBFinalizer = "database.oracle.com/LRPDBfinalizer"

// var tdePassword string
// var tdeSecret string
var floodcontrol = false

// var imperativeLpdbDeletion = false --> Unsed

//cdxhint: get rid of global variable defaultconfigmap
//var defaultconfigmap string
//var globalsqlcode int

/* mind  https://github.com/kubernetes-sigs/kubebuilder/issues/549 */

//+kubebuilder:rbac:groups=database.oracle.com,resources=lrpdbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrpdbs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrpdbs/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=lrpdbs/configmaps,verbs=get;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LRPDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile

// Reconcile reconciliation loop
func (r *LRPDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("multitenantoperator", req.NamespacedName)
	log.Info("Reconcile requested")

	reconcilePeriod := r.Interval * time.Second
	requeueY := ctrl.Result{Requeue: true, RequeueAfter: reconcilePeriod}
	//requeueN := ctrl.Result{}

	var err error
	lrpdb := &dbapi.LRPDB{}

	// lint QF1008 r.Client.Get(..)
	err = r.Get(context.TODO(), req.NamespacedName, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PDB resource not found", "Pdb", lrpdb.Spec.LRPDBName)
			return requeueN, nil
		}
		log.Info("Client.Get Error")
		return requeueN, err
	}

	/*
		lrest, err := r.getLRESTResource(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueY, err
		}
	*/

	/****  CREATE ****/
	if Bit(lrpdb.Status.PDBBitMask, PDBCRT) == false && Bit(lrpdb.Status.PDBBitMask, PDBCRE) == false && lrpdb.Spec.SrcLRPDBName == "" && lrpdb.Spec.XMLFileName == "" {
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

	/*** MONITOR CONFIG MAP ***/
	if lrpdb.Spec.MonitorInitParameter == true &&
		(Bit(lrpdb.Status.CmBitstat, MPAPPL) == true || Bit(lrpdb.Status.CmBitstat, MPWARN) == true) &&
		Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true &&
		Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true {
		log.Info("REC. LOOP: monitor config map")
		//cdxhint: MonitorCofingMap check return code
		if err := r.MonitorConfigMap(ctx, req, lrpdb); err != nil {
			log.Error(err, "MonitorConfigMapFailed")
			return requeueY, err
		}
	}

	/*** FINALYZER ***/
	if Bit(lrpdb.Status.PDBBitMask, FNALAZ) == false && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true {
		// fix lint QF1008 if lrpdb.ObjectMeta.DeletionTimestamp.IsZero() {
		if lrpdb.DeletionTimestamp.IsZero() {
			if !controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
				log.Info("add finalizer:" + lrpdb.Spec.LRPDBName)
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
	if lrpdb.Spec.LRPDBState == "OPEN" && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == false && Bit(lrpdb.Status.PDBBitMask, PDBOPE) == false {
		log.Info("REC. LOOP: open pdb")
		err = r.OpenLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}
	}

	/**** CLOSE ****/
	if lrpdb.Spec.LRPDBState == "CLOSE" && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true {
		log.Info("REC. LOOP: close pdb")
		err = r.CloseLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return requeueN, err
		}
	}

	/**** DELETE (imperative approach) ****/
	//  lint QF1008
	//	if !lrpdb.ObjectMeta.DeletionTimestamp.IsZero() &&
	if !lrpdb.DeletionTimestamp.IsZero() &&
		Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true &&
		Bit(lrpdb.Status.PDBBitMask, FNALAZ) == true &&
		Bit(lrpdb.Status.PDBBitMask, PDBDIC) == false {
		log.Info("REC. LOOP: delete  pdb - imperative approach")
		log.Info("ObjectMeta.DeletionTimestamp.IsZero is not null")
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
		if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
			fmt.Printf("TRCCFM: Config. Map Cardinality:%s", strconv.FormatInt(int64(Cardinality), 10))
		}
	}

	/**** APPLY USERS CREATION ****/
	//if lrpdb.Spec.Pdbappuser != "" && Bit(lrpdb.Status.PDBBitMask, APPUSR) == false && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && lrpdb.Spec.LRPDBState != "UNPLUG" {
	if lrpdb.Spec.Pdbappuser != "" && Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true && Bit(lrpdb.Status.PDBBitMask, PDBCRT) == true && lrpdb.Spec.LRPDBState != "UNPLUG" {
		log.Info("REC. LOOP: apply user")
		log.Info("Secret will be deleted after user creation")

		err = r.CreateApplicanUsers(ctx, req, lrpdb, lrpdb.Spec.Pdbappuser)
		if err != nil {
			return requeueN, err
		}
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
	if (lrpdb.Spec.PDBBitMask != 0 || lrpdb.Spec.PDBBitMaskStr != "") && lrpdb.Spec.LRPDBState == "RESET" {
		log.Info("REC. LOOP: reset state")
		if lrpdb.Spec.PDBBitMask != 0 {
			lrpdb.Status.PDBBitMask = lrpdb.Spec.PDBBitMask
			log.Info("lrpdb.Status.PDBBitMask:" + strconv.Itoa(lrpdb.Status.PDBBitMask))
			log.Info("lrpdb.Spec.PDBBitMask:" + strconv.Itoa(lrpdb.Spec.PDBBitMask))
			if Bit(lrpdb.Spec.PDBBitMask, PDBAUT) == true {
				log.Info("reset state PDBAUT")
				if controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
					lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, FNALAZ)
				}

				lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT)
			}
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

		if lrpdb.Spec.PDBBitMaskStr != "" {
			log.Info("Rest bitmask status using string values")
			/* reset bitmask status using string */
			newBitMaskValue := ParseStrStatus(lrpdb.Spec.PDBBitMaskStr)
			if newBitMaskValue != 0 {
				lrpdb.Status.PDBBitMask = newBitMaskValue
				lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
				r.UpdateStatus(ctx, req, lrpdb)
				lrpdb.Spec.PDBBitMask = 0
				lrpdb.Spec.LRPDBState = "NONE"

				err = r.Update(ctx, lrpdb)
				if err != nil {
					log.Error(err, err.Error())
					return requeueN, err
				}

			} else {
				log.Info("Invalid reset value")
			}
		}
	}

	return requeueY, nil
}

// MonitorLRPDB : check open and close status
func (r *LRPDBReconciler) MonitorLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("MonitorLRPDB ", req.NamespacedName)
	err := r.getLRPDBState(ctx, req, lrpdb)
	if err != nil {
		log.Info("getLRPDBState cannot find the resource ")
		return err
	}

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

	// Check config map existencs
	if lrpdb.Spec.PDBConfigMap == "" && Bit(lrpdb.Status.CmBitstat, MPINIT) == true {
		log.Info("Init config map no longer specified in attribute of " + lrpdb.Name)
		lrpdb.Status.CmBitstat = Bis(lrpdb.Status.CmBitstat, MPRMVD)
		lrpdb.Status.CmBitStatStr = CMBitmaskprint(lrpdb.Status.CmBitstat)
		r.UpdateStatus(ctx, req, lrpdb)
	}

	return nil
}

// PlugLRPDB : plug pdb
func (r *LRPDBReconciler) PlugLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("PlugLRPDB", req.NamespacedName)
	log.Info("Begin call")
	//globalsqlcode = 0

	var err error
	// var tde_Password string
	// var tde_Secret string

	/*
		lrest, err := r.getLRESTResource(ctx, req, lrpdb)
		if err != nil {
			return err
		}
	*/
	if Bit(lrpdb.Spec.Trclvl, TRCPLG) == true {
		fmt.Printf("TRCPLG: PDB:[%s] XMLFILE:[%s]\n", lrpdb.Spec.LRPDBName, lrpdb.Spec.XMLFileName)
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

	url := r.BaseURL2(req, lrpdb)

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}
	//globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode != 0 {
		//globalsqlcode = lrpdb.Status.SqlCode
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBPLE) /* Upplug error */
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBPLG) /* Remove unplug flag */
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode) /* Print the oracle error */
		lrpdb.Status.Msg = "close:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return errors.New(oer)

	}

	if Bit(lrpdb.Spec.Trclvl, TRCPLG) == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "TRCPLG: '%s' plugged successfully", lrpdb.Spec.LRPDBName)
	}

	r.RetriveTnsAlias(ctx, req, lrpdb)

	//imperativeLpdbDeletion = lrpdb.Spec.ImperativeLrpdbDeletion
	if lrpdb.Spec.ImperativeLrpdbDeletion == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Plug", "PDB '%s' imperative pdb deletion turned on", lrpdb.Spec.LRPDBName)
	}

	//r.getLRPDBState(ctx, req, lrpdb, lrest)

	if err = r.getLRPDBState(ctx, req, lrpdb); err != nil {
		log.Error(err, "getLRPDBState")
		return err
	}

	lrpdb.Status.Msg = "plug:[op. completed]"
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT) /* Set the creation flag */
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBOPN) /* Set the creation flag */
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	r.UpdateStatus(ctx, req, lrpdb)

	if Bit(lrpdb.Spec.Trclvl, TRCPLG) == true {
		fmt.Printf("TRCPLG: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
		fmt.Printf("TRCPLG: Successfully plugged LRPDB Name [%s]", lrpdb.Spec.LRPDBName)
	}
	return nil
}

// UnplugLRPDB : unplug pdb
func (r *LRPDBReconciler) UnplugLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("unplugLRPDB", req.NamespacedName)
	//globalsqlcode = 0

	log.Info("Begin call")
	var err error
	//var tde_Password string
	//var tde_Secret string

	/*
		lrest, err := r.getLRESTResource(ctx, req, lrpdb)
		if err != nil {
			return err
		}
	*/

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
		if Bit(lrpdb.Spec.Trclvl, TRCUPL) == true {
			fmt.Printf("TRCUPL: pdb already plugged in the past\n")
			fmt.Printf("TRCUPL: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
		}
	}

	r.UpdateStatus(ctx, req, lrpdb)
	url := r.BaseURL2(req, lrpdb) + lrpdb.Spec.LRPDBName

	if Bit(lrpdb.Spec.Trclvl, TRCUPL) == true {
		fmt.Printf("TRCUPL: Starting unplugging process\n")
	}

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}

	r.UpdateStatus(ctx, req, lrpdb)

	if lrpdb.Status.SqlCode != 0 {
		//globalsqlcode = lrpdb.Status.SqlCode
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
	//log.Info("unplug:[op. completed]")
	if Bit(lrpdb.Spec.Trclvl, TRCUPL) == true {
		fmt.Printf("TRCUPL: Unplug process completed\n")
	}
	if controllerutil.ContainsFinalizer(lrpdb, LRPDBFinalizer) {
		//log.Info("Removing finalizer")

		if Bit(lrpdb.Spec.Trclvl, TRCUPL) == true {
			fmt.Printf("TRCUPL: Removing finalizer\n")
		}

		controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
		err = r.Update(ctx, lrpdb)
		if err != nil {
			log.Info("Could not remove finalizer", "err", err.Error())
			return err
		}
		lrpdb.Status.Status = true
		if Bit(lrpdb.Spec.Trclvl, TRCUPL) == true {
			fmt.Printf("TRCUPL: Delete crd\n")
		}
		err = r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1))
		if err != nil {
			log.Info("Could not delete LRPDB resource", "err", err.Error())
			return err
		}
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Unplugged", "LRPDB '%s' unplugged successfully", lrpdb.Spec.LRPDBName)
	//globalsqlcode = 0

	if Bit(lrpdb.Spec.Trclvl, TRCUPL) == true {
		fmt.Printf("TRCUPL: Successfully unplugged LRPDB resource")
	}
	return nil
}

// OpenLRPDB : open pdb
func (r *LRPDBReconciler) OpenLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("OpenLRPDB", req.NamespacedName)
	log.Info("Begin call")

	/*
		lrest, err := r.getLRESTResource(ctx, req, lrpdb)
		if err != nil {
			log.Info("TRCOPN: Failure cannot get lrest resource")
			return err
		}
	*/

	/* If open is called directly by the create pdb function */
	ModOption := lrpdb.Spec.ModifyOption
	PdbState := lrpdb.Spec.LRPDBState

	if lrpdb.Spec.ModifyOption == "" || lrpdb.Spec.LRPDBState == "" {
		ModOption = "READ WRITE"
		PdbState = "OPEN"
	}

	//values := map[string]string{}
	values := map[string]string{
		"state":         PdbState,
		"modifyOption":  ModOption,
		"modifyOption2": lrpdb.Spec.ModifyOption2,
		"getScript":     strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		if Bit(lrpdb.Spec.Trclvl, TRCOPN) == true {
			fmt.Print("TRCOPN: MODIFY LRPDB lrpdb.Spec.LRPDBState=", lrpdb.Spec.LRPDBState, "lrpdb.Spec.ModifyOption=", lrpdb.Spec.ModifyOption, "\n")
			fmt.Print("TRCOPN: LRPDB STATUS OPENMODE lrpdb.Status.OpenMode=", lrpdb.Status.OpenMode, "\n")

			//log.Info("TRCOPN: MODIFY LRPDB", "lrpdb.Spec.LRPDBState=", lrpdb.Spec.LRPDBState, "lrpdb.Spec.ModifyOption=", lrpdb.Spec.ModifyOption)
			//log.Info("TRCOPN:LRPDB STATUS OPENMODE", "lrpdb.Status.OpenMode=", lrpdb.Status.OpenMode)
		}
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseURL2(req, lrpdb) + lrpdbName

	lrpdb.Status.Msg = "open:[op in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}

	/* if sqlcode is zero then unset the closebit */
	if lrpdb.Status.SqlCode == 0 {
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBCLS)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	}
	if Bit(lrpdb.Spec.Trclvl, TRCOPN) == true {
		fmt.Print("TRCOPN: PDBBitMask[", lrpdb.Status.PDBBitMask, "] PDBBitMaskStr [", lrpdb.Status.PDBBitMaskStr, "]\n")
	}

	r.UpdateStatus(ctx, req, lrpdb)

	/* Return Error if sqlcode != */
	if lrpdb.Status.SqlCode != 0 {
		if DiscardableError(lrpdb.Status.SqlCode) == false {
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBOPE)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		}
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "open:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		if DiscardableError(lrpdb.Status.SqlCode) == false {
			return errors.New(oer)
		}
	}

	if err = r.getLRPDBState(ctx, req, lrpdb); err != nil {
		log.Error(err, "getLRPDBState")
		return err
	}

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Modified", " '%s' modified successfully '%s'", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState)
	}

	r.RetriveTnsAlias(ctx, req, lrpdb)

	log.Info("Successfully modified LRPDB state", "LRPDB Name", lrpdb.Spec.LRPDBName)

	/* After database openining we reapply the config map if warning is present */
	if lrpdb.Spec.LRPDBState == "OPEN" {
		if Bit(lrpdb.Status.CmBitstat, MPWARN|MPINIT) {
			if Bit(lrpdb.Spec.Trclvl, TRCOPN) == true {
				fmt.Printf("TRCOPN: re-apply config map\n")
			}
			crdnlty, err := r.ApplyConfigMap(ctx, req, lrpdb)
			if err != nil {
				log.Info("ApplyConfigMap Failure")
				fmt.Printf("ConfigMap Num Element:%d", crdnlty)
			}

		}
	}
	lrpdb.Status.Msg = "open:[op. completed]"
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBOPN)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	if Bit(lrpdb.Spec.Trclvl, TRCOPN) == true {
		fmt.Printf("TRCOPN: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
	}
	r.UpdateStatus(ctx, req, lrpdb)
	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "OPEN", "LRPDB:'%s' open completed successfully", lrpdb.Spec.LRPDBName)

	return nil
}

// CloseLRPDB : close plb
func (r *LRPDBReconciler) CloseLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("CloseLRPDB", req.NamespacedName)
	log.Info("Begin call")
	if Bit(lrpdb.Spec.Trclvl, TRCCLS) == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Close", "Info:'%s %s %s' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState, lrpdb.Status.ModifyOption)
	}

	var modoption string

	if lrpdb.Spec.ModifyOption == "" {
		modoption = "IMMEDIATE"
	} else {
		modoption = lrpdb.Spec.ModifyOption
	}

	values := map[string]string{
		"state":         lrpdb.Spec.LRPDBState,
		"modifyOption":  modoption,
		"modifyOption2": lrpdb.Spec.ModifyOption2,
		"getScript":     strconv.FormatBool(*(lrpdb.Spec.GetScript))}

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		if Bit(lrpdb.Spec.Trclvl, TRCCLS) == true {
			fmt.Printf("TRCCLS: MODIFY LRPDB lrpdb.Spec.LRPDBName=%s lrpdb.Spec.LRPDBState=%s lrpdb.Spec.ModifyOption=%s\n", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState, lrpdb.Spec.ModifyOption)
			fmt.Printf("TRCCLS: LRPDB STATUS OPENMODE lrpdb.Status.OpenMode=%s\n", lrpdb.Status.OpenMode)
		}
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseURL2(req, lrpdb) + lrpdbName

	lrpdb.Status.Msg = "close:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	//	r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}

	/* if sqlcode is zero then unset the openbit */
	if lrpdb.Status.SqlCode == 0 {
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBOPN)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		if Bit(lrpdb.Spec.Trclvl, TRCCLS) == true {
			fmt.Printf("TRCCLS: lrpdb.Status.SqlCode=%d\n", lrpdb.Status.SqlCode)
		}
	}

	r.UpdateStatus(ctx, req, lrpdb)

	/* Return Error if sqlcode != */
	if lrpdb.Status.SqlCode != 0 {
		if DiscardableError(lrpdb.Status.SqlCode) == false {
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCLE)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		}
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "close:[" + oer + "]"
		if Bit(lrpdb.Spec.Trclvl, TRCCLS) == true {
			fmt.Printf("TRCCLS: lrpdb.Status.SqlCode=%d\n", lrpdb.Status.SqlCode)
		}
		r.UpdateStatus(ctx, req, lrpdb)
		if DiscardableError(lrpdb.Status.SqlCode) == false {
			return errors.New(oer)
		}
	}

	//globalsqlcode = lrpdb.Status.SqlCode
	//r.getLRPDBState(ctx, req, lrpdb, lrest)
	if err = r.getLRPDBState(ctx, req, lrpdb); err != nil {
		log.Error(err, "getLRPDBState")
		return err
	}

	if lrpdb.Spec.LRPDBState == "OPEN" || lrpdb.Spec.LRPDBState == "CLOSE" {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Modified", " '%s' modified successfully '%s'", lrpdb.Spec.LRPDBName, lrpdb.Spec.LRPDBState)
	}

	r.RetriveTnsAlias(ctx, req, lrpdb)
	lrpdb.Status.Msg = "close:[op. completed]"
	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCLS)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	r.UpdateStatus(ctx, req, lrpdb)
	if Bit(lrpdb.Spec.Trclvl, TRCCLS) == true {
		fmt.Printf("TRCCLS: pdb close operation completed\n")
		fmt.Printf("TRCCLS: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
		fmt.Printf("TRCCLS: Successfully modified LRPDB state(close) - LRPDB Name:%s", lrpdb.Spec.LRPDBName)
	}

	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "CLOSE", "LRPDB:'%s' close completed successfully", lrpdb.Spec.LRPDBName)
	return nil
}

// DeleteLRPDB : delete pdb
func (r *LRPDBReconciler) DeleteLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("deleteLRPDB", req.NamespacedName)

	/*
		lrest, err := r.getLRESTResource(ctx, req, lrpdb)
		if err != nil {
			log.Info("Failure: Cannot get lrest info")
			return err
		}
	*/

	if lrpdb.Spec.ImperativeLrpdbDeletion == true {
		/* Close the pdb if it's open */
		if Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true {
			valuesclose := map[string]string{
				"state":        "CLOSE",
				"modifyOption": "IMMEDIATE",
				"getScript":    "FALSE"}
			lrpdbName := lrpdb.Spec.LRPDBName
			url := r.BaseURL2(req, lrpdb) + lrpdbName
			respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, valuesclose, "POST")
			//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
			if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
				log.Error(err, "GetSQLCodeFailure")
				return err
			}

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
		url := r.BaseURL2(req, lrpdb) + lrpdbName

		respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "DELETE")
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			return err
		}

		//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
		if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
			log.Error(err, "GetSQLCodeFailure")
			return err
		}

		//globalsqlcode = lrpdb.Status.SqlCode
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

	}

	log.Info("Successfully dropped LRPDB", "LRPDB Name", lrpdb.Spec.LRPDBName)
	r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "DROP", "LRPDB:'%s' drop completed successfully", lrpdb.Spec.LRPDBName)

	controllerutil.RemoveFinalizer(lrpdb, LRPDBFinalizer)
	if err := r.Update(ctx, lrpdb); err != nil {
		log.Info("Cannot remove finalizer")
		return err
	}

	return nil
}

// DeleteLRPDBDeclarative : remove pdb crd and drop pdb
func (r *LRPDBReconciler) DeleteLRPDBDeclarative(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("deleteLRPDBDeclaratve", req.NamespacedName)

	var err error

	/*
		lrest, err := r.getLRESTResource(ctx, req, lrpdb)
		if err != nil {
			log.Info("Failure: Cannot get lrest info")
			return err
		}
	*/

	if lrpdb.Spec.ImperativeLrpdbDeletion == true {
		/* Close the pdb if it's open */
		if Bit(lrpdb.Status.PDBBitMask, PDBOPN) == true {
			valuesclose := map[string]string{
				"state":        "CLOSE",
				"modifyOption": "IMMEDIATE",
				"getScript":    "FALSE"}
			lrpdbName := lrpdb.Spec.LRPDBName
			url := r.BaseURL2(req, lrpdb) + lrpdbName
			respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, valuesclose, "POST")
			//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
			if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
				log.Error(err, "GetSQLCodeFailure")
				return err
			}

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
		url := r.BaseURL2(req, lrpdb) + lrpdbName

		respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "DELETE")
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			return err
		}

		//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
		if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
			log.Error(err, "GetSQLCodeFailure")
			return err
		}

		//globalsqlcode = lrpdb.Status.SqlCode
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
	}
	log.Info("Successfully dropped LRPDB", "LRPDB Name", lrpdb.Spec.LRPDBName)

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
func (r *LRPDBReconciler) checkPDBforCloninig(ctx context.Context, req ctrl.Request, targetPdbName string) (int, error) {
	log := r.Log.WithValues("checkPDBforCloninig", req.NamespacedName)
	log.Info("Begin call")
	// lint S1021 pdbCounter = 0
	// lit drop =0 hint var pdbCounter int = 0
	var pdbCounter int

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
		if Bit(p.Spec.Trclvl, TRCCLN) == true {
			fmt.Printf("TRCCLN (check): %s %s %d\n", p.Spec.LRPDBName, targetPdbName, pdbCounter)
		}

		if p.Spec.LRPDBName == targetPdbName {
			log.Info("Found " + targetPdbName + " in the crd list")
			if p.Status.OpenMode == "MOUNTED" {
				log.Info("Cannot clone a mounted pdb")
				return pdbCounter, err
			}
			pdbCounter++
			if Bit(p.Spec.Trclvl, TRCCLN) == true {
				fmt.Printf("TRCCLN (check): %s %s %d\n", p.Spec.LRPDBName, targetPdbName, pdbCounter)
			}
			return pdbCounter, err
		}

	}
	return pdbCounter, err
}

// CloneLRPDB Clone pdb
func (r *LRPDBReconciler) CloneLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("CloneLRPDB", req.NamespacedName)
	log.Info("Begin call")
	if lrpdb.Spec.LRPDBName == lrpdb.Spec.SrcLRPDBName {
		log.Info("Invalid Name")
		return nil
	}

	//globalsqlcode = 0
	var err error

	/*
		lrest, err := r.getLRESTResource(ctx, req, lrpdb)
		if err != nil {
			return err
		}
	*/

	/* Prevent cloning an existing lrpdb */
	err = r.getLRPDBState(ctx, req, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if Bit(lrpdb.Spec.Trclvl, TRCCLN) == true {
				fmt.Printf("TRCCLN: Check LRPDB not existence completed %s\n", lrpdb.Spec.LRPDBName)
			}
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
	pdbCounter, _ := r.checkPDBforCloninig(ctx, req, lrpdb.Spec.SrcLRPDBName)
	if pdbCounter == 0 {
		log.Info("target pdb " + lrpdb.Spec.SrcLRPDBName + " does not exists or is not open")
		allErrs = append(allErrs, field.NotFound(field.NewPath("Spec").Child("LRPDBName"), " "+lrpdb.Spec.LRPDBName+" does not exist :  failure"))
		LenErrorList := len(allErrs)
		fmt.Printf("checkPDBforCloninig len(allErrs)=%d\n", LenErrorList)

		if r.Delete(context.Background(), lrpdb, client.GracePeriodSeconds(1)) != nil {
			log.Info("r.Delete(context.Background(), lrpdb... failure")
		}
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

	url := r.BaseURL2(req, lrpdb) + lrpdb.Spec.LRPDBName + "/"

	lrpdb.Status.Msg = "clone:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}

	//globalsqlcode = lrpdb.Status.SqlCode
	r.UpdateStatus(ctx, req, lrpdb)

	if lrpdb.Status.SqlCode != 0 {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		if Bit(lrpdb.Spec.Trclvl, TRCCLN) == true {
			fmt.Printf("TRCCLN: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
		}
		oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "open:[" + oer + "]"
		r.UpdateStatus(ctx, req, lrpdb)
		return errors.New(oer)

	}

	if Bit(lrpdb.Spec.Trclvl, TRCCLN) == true {
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "LRPDB '%s' cloned successfully", lrpdb.Spec.LRPDBName)
		fmt.Printf("TRCCLN: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
	}
	lrpdb.Status.TotalSize = r.GetPdbSize(ctx, req, lrpdb)

	/*
		if lrest.Spec.DBServer != "" {
			lrpdb.Status.ConnString = lrest.Spec.DBServer + ":" + strconv.Itoa(lrest.Spec.DBPort) + "/" + lrpdb.Spec.LRPDBName
		} else {
			lrpdb.Status.ConnString = strings.TrimSpace(lrest.Spec.DBTnsurl)
			parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName), lrpdb.Spec.Trclvl)

		}
	*/

	r.RetriveTnsAlias(ctx, req, lrpdb)
	if Bit(lrpdb.Spec.Trclvl, TRCCLN) == true {
		fmt.Printf("TRCCLN: tnsalias=%s\n", lrpdb.Status.ConnString)
	}

	//imperativeLpdbDeletion = lrpdb.Spec.ImperativeLrpdbDeletion
	if lrpdb.Spec.ImperativeLrpdbDeletion == true {

		if Bit(lrpdb.Spec.Trclvl, TRCCLN) == true {
			fmt.Printf("TRCCLN: imperative deletion  true\n")
			r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Clone", "PDB '%s' imperative pdb deletion turned on", lrpdb.Spec.LRPDBName)
		}
	}

	if Bit(lrpdb.Spec.Trclvl, TRCCLN) == true {
		fmt.Printf("TRCCLN: Clone completed successfully Source[%s]->Clone[%s]\n", lrpdb.Spec.SrcLRPDBName, lrpdb.Spec.LRPDBName)
	}
	//r.getLRPDBState(ctx, req, lrpdb, lrest)
	if err = r.getLRPDBState(ctx, req, lrpdb); err != nil {
		log.Error(err, "getLRPDBState")
		return err
	}

	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	lrpdb.Status.Msg = "clone:[op. completed]"
	r.UpdateStatus(ctx, req, lrpdb)
	if Bit(lrpdb.Spec.Trclvl, TRCCLN) == true {
		fmt.Printf("TRCCLN: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
		if lrpdb.Spec.PLSQLBlock != "" {
			fmt.Printf("TRCCLN: plsql block reset :[%s]\n", lrpdb.Spec.PLSQLBlock)
		}
	}

	/* If we clone we don't have to re-exec sql/plsql */
	lrpdb.Spec.PLSQLBlock = ""
	if err := r.Update(ctx, lrpdb); err != nil {
		log.Error(err, "Failred to update lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
		return err
	}

	return nil
}

// CreateLRPDB create pluggable database
func (r *LRPDBReconciler) CreateLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("CreateLRPDB", req.NamespacedName)
	if Bit(lrpdb.Spec.Trclvl, TRCCRT) == true {
		fmt.Print("TRCCRT: call  getLRESTResource \n")
	}

	log.Info("lrpdb.Spec.LRPDBName:" + lrpdb.Spec.LRPDBName)
	/* If it's not created by lrest autodiscover */
	if Bit(lrpdb.Status.PDBBitMask, PDBAUT) == false && lrpdb.Spec.PDBBitMask == 0 {

		var err error
		var tdePassword string
		var tdeSecret string

		/*
			AutoDiscover := lrest.Spec.PdbAutoDiscover
			err = r.AutoDiscoverActivation(req, lrpdb, false)
			if err != nil {
				log.Error(err, "Autodiscovery deactivstion")
				return err
			}
		*/

		/*** reset sqlcode***/
		lrpdb.Status.SqlCode = 0

		lrpdbAdminName, err := getGenericSecret3(ctx, r, req, lrpdb, lrpdb.Spec.AdminpdbUser.Secret.SecretName,
			lrpdb.Spec.AdminpdbUser.Secret.Key,
			lrpdb.Spec.LRPDBPriKey.Secret.SecretName,
			lrpdb.Spec.LRPDBPriKey.Secret.Key,
			NULL, NULL, true)
		if err != nil {
			log.Error(err, "Unable to find pdb admin user ")
			//_ = r.AutoDiscoverActivation(req, lrpdb, AutoDiscover)
			return err
		}

		lrpdbAdminPwd, err := getGenericSecret3(ctx, r, req, lrpdb, lrpdb.Spec.AdminpdbPass.Secret.SecretName,
			lrpdb.Spec.AdminpdbPass.Secret.Key,
			lrpdb.Spec.LRPDBPriKey.Secret.SecretName,
			lrpdb.Spec.LRPDBPriKey.Secret.Key,
			NULL, NULL, true)
		if err != nil {
			log.Error(err, "Unable to find pdb admin password ")
			//_ = r.AutoDiscoverActivation(req, lrpdb, AutoDiscover)
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

		if Bit(lrpdb.Spec.Trclvl, TRCCRT) == true {
			fmt.Print("TRCCRT: BEGIN PAYLOAD\n")
			fmt.Print("TRCCRT: method ", values["method"], "\n")
			fmt.Print("TRCCRT: pdb_name ", values["pdb_name"], "\n")
			fmt.Print("TRCCRT: adminName ", values["adminName"], "\n")
			fmt.Print("TRCCRT: adminPwd --------------\n")
			fmt.Print("TRCCRT: fileNameConversions ", values["fileNameConversions"], "\n")
			fmt.Print("TRCCRT: unlimitedStorage ", values["unlimitedStorage"], "\n")
			fmt.Print("TRCCRT: reuseTempFile ", values["reuseTempFile"], "\n")
			fmt.Print("TRCCRT: tempSize ", values["tempSize"], "\n")
			fmt.Print("TRCCRT: totalSize ", values["totalSize"], "\n")
			fmt.Print("TRCCRT: getScript ", values["getScript"], "\n")
			fmt.Print("TRCCRT: END PAYLOAD\n")
		}

		if *(lrpdb.Spec.LTDEImport) {
			//tde_Password, err = r.getSecret(ctx, req, lrpdb, lrpdb.Spec.LTDEPassword.Secret.SecretName, lrpdb.Spec.LTDEPassword.Secret.Key)
			tdePassword, err = getGenericSecret3(ctx, r, req, lrpdb, lrpdb.Spec.LTDEPassword.Secret.SecretName,
				lrpdb.Spec.LTDEPassword.Secret.Key, NULL, NULL, NULL, NULL, true)
			if err != nil {
				//_ = r.AutoDiscoverActivation(req, lrpdb, AutoDiscover)
				return err
			}
			tdeSecret, err = getGenericSecret3(ctx, r, req, lrpdb, lrpdb.Spec.LTDESecret.Secret.SecretName,
				lrpdb.Spec.LTDESecret.Secret.Key, NULL, NULL, NULL, NULL, true)
			if err != nil {
				//_ = r.AutoDiscoverActivation(req, lrpdb, AutoDiscover)
				return err
			}

			tdeSecret = tdeSecret[:len(tdeSecret)-1]
			tdePassword = tdeSecret[:len(tdePassword)-1]
			values["tde_Password"] = tdePassword
			values["tdeKeystorePath"] = lrpdb.Spec.LTDEKeystorePath
			values["tde_Secret"] = tdeSecret
		}

		url := r.BaseURL2(req, lrpdb)
		if Bit(lrpdb.Spec.Trclvl, TRCCRT) == true {
			fmt.Print("TRCCRT:==== URL ===\n")
			fmt.Print("TRCCRT:" + url)
			fmt.Print("\nTRCCRT:===========\n")
		}
		lrpdb.Status.Msg = "create:[op in progress]"

		r.UpdateStatus(ctx, req, lrpdb)

		respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			//_ = r.AutoDiscoverActivation(req, lrpdb, AutoDiscover)
			return err
		}

		//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
		if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
			log.Error(err, "GetSQLCodeFailure")
			//return err
		}

		//globalsqlcode = lrpdb.Status.SqlCode
		if lrpdb.Status.SqlCode != 0 {
			if DiscardableError(lrpdb.Status.SqlCode) == false {
				lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRE)
				lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
			}
			oer := fmt.Sprintf("ORA-%d", lrpdb.Status.SqlCode)
			lrpdb.Status.Msg = "create:[" + oer + "]"
			r.UpdateStatus(ctx, req, lrpdb)
			if DiscardableError(lrpdb.Status.SqlCode) == false {
				return errors.New(oer)
			}
		} else {
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
			r.UpdateStatus(ctx, req, lrpdb)
		}

		if err = r.getLRPDBState(ctx, req, lrpdb); err != nil {
			log.Error(err, "getLRPDBState")
			return err
		}

		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal,
			"Created", "LRPDB '%s' created successfully", lrpdb.Spec.LRPDBName)

		if Bit(lrpdb.Spec.Trclvl, TRCCRT) == true {
			fmt.Printf("TRCCRT: Parsing connectstring")
		}
		/*
			lrpdb.Status.ConnString = strings.TrimSpace(lrest.Spec.DBTnsurl)
			parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName), lrpdb.Spec.Trclvl)
		*/
		r.RetriveTnsAlias(ctx, req, lrpdb)
		r.UpdateStatus(ctx, req, lrpdb)

		//imperativeLpdbDeletion = lrpdb.Spec.ImperativeLrpdbDeletion
		if lrpdb.Spec.ImperativeLrpdbDeletion == true {
			r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Created", "PDB '%s' imperative pdb deletion turned on", lrpdb.Spec.LRPDBName)
		}

		//_ = r.AutoDiscoverActivation(req, lrpdb, AutoDiscover)

		lrpdb.Status.Msg = "create:[op completed]"
		r.UpdateStatus(ctx, req, lrpdb)

		/* Open pdb after creation */
		if Bit(lrpdb.Spec.Trclvl, TRCCRT) == true {
			fmt.Print("TRCCRT: opening pdb\n")
		}
		err = r.OpenLRPDB(ctx, req, lrpdb)
		if err != nil {
			log.Error(err, err.Error())
			return err
		}
		if Bit(lrpdb.Spec.Trclvl, TRCCRT) == true {
			fmt.Printf("TRCCRT: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
		}

	} else {
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCRT)
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBAUT)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.RetriveTnsAlias(ctx, req, lrpdb)
		r.UpdateStatus(ctx, req, lrpdb)
		parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName), lrpdb.Spec.Trclvl)
		lrpdb.Status.Msg = "autodiscover:[op completed]"
		if Bit(lrpdb.Spec.Trclvl, TRCCRT) == true {
			fmt.Printf("TRCCRT: PDBBitMask[%d] PDBBitMaskStr[%s]\n", lrpdb.Status.PDBBitMask, lrpdb.Status.PDBBitMaskStr)
			fmt.Printf("TRCCRT: CRT created by autodiscovery\n")
		}
		r.UpdateStatus(ctx, req, lrpdb)
	}
	return nil
}

/**************************************************
ALTER SYSTEM lRPDB
**************************************************/

func (r *LRPDBReconciler) alterSystemLRPDB2(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, parameter string, value string, scope string, infocaller string) error {

	log := r.Log.WithValues("alterSystemLRPDB2", req.NamespacedName)
	//globalsqlcode = 0

	payload := map[string]string{
		"state":                "ALTER",
		"alterSystemParameter": parameter,
		"alterSystemValue":     value,
		"parameterScope":       scope,
	}

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseURL2(req, lrpdb) + lrpdbName

	lrpdb.Status.ModifyOption = infocaller + ":[" + parameter + "][" + value + "]"
	lrpdb.Status.Msg = "alter system2:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, payload, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}

	//globalsqlcode = lrpdb.Status.SqlCode

	if lrpdb.Status.SqlCode == 0 {

		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "Altered", "resync param(name,cmd,sqlcode) '%s %s %d' ",
			parameter, value, lrpdb.Status.SqlCode)
		lrpdb.Status.Msg = "alter system2:[op. completed]"
		r.UpdateStatus(ctx, req, lrpdb)
		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Cannot rest lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
			return err
		}

		return nil

	}

	if lrpdb.Status.SqlCode != 0 {

		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "alter system failure", "LRPDB(name,cmd,sqlcode) '%s %s %d' ", lrpdb.Spec.LRPDBName, lrpdb.Spec.AlterSystem, lrpdb.Status.SqlCode)
		r.Recorder.Eventf(lrpdb, corev1.EventTypeNormal, "failure", "resync param(name,cmd,sqlcode) '%s %s %d' ",
			parameter, value, lrpdb.Status.SqlCode)
		erralter := errors.New("Error: cannot modify parameter")

		lrpdb.Status.Msg = "alter system2:[op. failure]"
		r.UpdateStatus(ctx, req, lrpdb)

		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Cannot rest lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
			return err
		}

		return erralter
	}

	return nil
}

/**just push the trasnsaction **/
func (r *LRPDBReconciler) alterSystemLRPDB(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {

	log := r.Log.WithValues("alterSystemLRPDB", req.NamespacedName)
	//globalsqlcode = 0

	var err error
	err = r.getLRPDBState(ctx, req, lrpdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Warning LRPDB does not exist", "LRPDB Name", lrpdb.Spec.LRPDBName)
			return nil
		}
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
	url := r.BaseURL2(req, lrpdb) + lrpdbName
	log.Info("alter system payload...:", "lrpdb.Spec.AlterSystemValue=", lrpdb.Spec.AlterSystemValue)
	log.Info("alter system payload...:", "lrpdb.Spec.AlterSystemParameter=", lrpdb.Spec.AlterSystemParameter)
	log.Info("alter system payload...:", "lrpdb.Spec.ParameterScope=", lrpdb.Spec.ParameterScope)
	log.Info("alter system path.......:", "url=", url)

	lrpdb.Status.ModifyOption = lrpdb.Spec.AlterSystem + " " + lrpdb.Spec.ParameterScope
	lrpdb.Status.Msg = "alter system:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
	if err != nil {
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		return err
	}

	//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}

	//globalsqlcode = lrpdb.Status.SqlCode

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
	log.Info("Begin call")

	// TO BE DONE Ad control for the pdb existence
	lrpdbName := lrpdb.Spec.LRPDBName

	if Bit(lrpdb.Spec.Trclvl, TRCPSQ) == true {
		fmt.Printf("TRCPSQ: Reafing configmap %s\n", lrpdb.Spec.PLSQLBlock)
	}
	configmap, err := r.GetConfigMapCode(req, lrpdb)
	if err != nil {
		log.Error(err, "Fail to fetch code configmap", "err", err.Error())
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
		if Bit(lrpdb.Spec.Trclvl, TRCPSQ) == true {
			fmt.Printf("TRCPSQ: Code Block Name (SQL/PLSQL):%s\n", key)
		}
		tokens = strings.Split(Value, "\n")
		/* Trclvl Section */
		for cnt := range tokens {
			if Bit(lrpdb.Spec.Trclvl, TRCPSQ) == true {
				fmt.Printf("TRCPSQ: line[%d]:%s\n", cnt, tokens[cnt])
			}
			CodeSize += len(tokens[cnt])
		}

		//* removing laste null emlements
		if len(tokens) > 0 {
			tokens = tokens[:len(tokens)-1]
		}

		fmt.Printf("call to restsertver (%s,%d)\n", key, CodeSize)

		jsonpayload := &PLSQLPayLoad{Values: map[string]string{"method": "APPLYSQL"}, Sqltokens: tokens}
		//* Trclvl section **//

		encjson, _ := json.Marshal(jsonpayload)
		if Bit(lrpdb.Spec.Trclvl, TRCPSQ) == true {
			fmt.Printf("TRCPSQ: %s\n", string(encjson))
		}

		url := r.BaseURL2(req, lrpdb) + lrpdbName

		respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, jsonpayload, "POST")
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			return err
		}

		//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
		if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
			// In case of error we log the information in the event
			log.Info("Waring error during plsqlexecution:see event for details")
		}

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

		if Bit(lrpdb.Spec.Trclvl, TRCPSQ) == true {
			fmt.Printf("TRCPSQ: [CODE:SQLCODE:KEY] [%s:%d:%s] \n", formatted, lrpdb.Status.SqlCode, skey)
		}

		/* sql execution complete successfully than report the name of the tag */
		if lrpdb.Status.SqlCode == 0 {
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
	if Bit(lrpdb.Spec.Trclvl, TRCPSQ) == true {
		fmt.Printf("TRCPSQ: plsql block reset :[%s]\n", lrpdb.Spec.PLSQLBlock)
	}
	return nil
}

// getLRPDBState : retrieves the resource status
func (r *LRPDBReconciler) getLRPDBState(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("getLRPDBState", req.NamespacedName)
	log.Info("Begin call")

	var err error

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseURL2(req, lrpdb) + lrpdbName + "/status/"

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, nil, "GET")
	/* Connection failure */
	if err != nil {
		lrpdb.Status.Msg = "getLRPDBState failure : callAPI connection failure "
		log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, PDBCNE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.UpdateStatus(ctx, req, lrpdb)
		return err
	}
	/* Connection restored */
	if err == nil && Bit(lrpdb.Status.PDBBitMask, PDBCNE) == true {
		lrpdb.Status.PDBBitMask = Bid(lrpdb.Status.PDBBitMask, PDBCNE)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		lrpdb.Status.Msg = "CallAPISQL OK!"
		log.Info("LREST<-->LRPDB OK Connection restored")
		r.UpdateStatus(ctx, req, lrpdb)
	}

	//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
	if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
		log.Error(err, "GetSQLCodeFailure")
		return err
	}

	//globalsqlcode = lrpdb.Status.SqlCode
	if lrpdb.Status.SqlCode == 1403 {
		lrpdb.Status.OpenMode = "N/A"
		lrpdb.Status.Msg = "N/A ORA-1403"
		if Bit(lrpdb.Spec.Trclvl, TRCSTA) == true {
			fmt.Printf("TRCSTA: SqlCode[NO_DATA_FOUND]:[%s]\n", lrpdb.Status.OpenMode)
		}
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
	if Bit(lrpdb.Spec.Trclvl, TRCSTA) == true {
		fmt.Printf("TRCSTA: LRPDB state Name OK.........:[%s]\n", lrpdb.Spec.LRPDBName)
		fmt.Printf("TRCSTA: lrpdb.Status.Restricted.....:[%s]\n", lrpdb.Status.Restricted)
		fmt.Printf("TRCSTA: lrpdb.Status.TotalSize......:[%s]\n", lrpdb.Status.TotalSize)
		fmt.Printf("TRCSTA: lrpdb.Status.openmode.......:[%s]\n", lrpdb.Status.OpenMode)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager
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

func parseTnsAlias(tns *string, lrpdbsrv *string, tracelevel int) {
	if Bit(tracelevel, TRCTNS) == true {
		fmt.Printf("TRCTNS: Analyzing string......:[%s]\n", *tns)
		fmt.Printf("TRCTNS: Replacing srv.........:[%s]\n", *lrpdbsrv)
		fmt.Printf("TRCTNS: Newstring.............:[%s]\n", *tns)
	}

	var swaptns string
	var swapcid string

	if strings.Contains(strings.ToUpper(*tns), "SERVICE_NAME") == false && (strings.ContainsRune(*tns, ':') == false || strings.ContainsRune(*tns, '/') == false) {
		if Bit(tracelevel, TRCTNS) == true {
			fmt.Printf("TRCTNS: Cannot generate tns alias for lrpdb")
		}
		return
	}

	if strings.Contains(strings.ToUpper(*tns), "ORACLE_SID") == true {
		if Bit(tracelevel, TRCTNS) == true {
			fmt.Print("TRCTNS: generate tns alias for lrpdb")
		}
		return
	}

	*tns = strings.ReplaceAll(*tns, " ", "")

	if strings.ContainsRune(*tns, ':') && strings.ContainsRune(*tns, '/') {
		fmt.Printf("TRCTNS: ......................:[%s]\n", "Three tokens format")
		swaptns = fmt.Sprintf("/%s", *lrpdbsrv)
		tnsreg := regexp.MustCompile(`/\w+`)
		*tns = tnsreg.ReplaceAllString(*tns, swaptns)
	} else {
		fmt.Printf("TRCTNS: ......................:[%s]\n", "Long string format")
		swaptns = fmt.Sprintf("SERVICE_NAME=%s", *lrpdbsrv)
		swapcid = ""
		tnsreg := regexp.MustCompile(`SERVICE_NAME=\w+`)
		// tnsreg2 := regexp.MustCompile(`CID=\(PROGRAM=\w+\)`)
		// remove cid information
		// tnsreg2 := regexp.MustCompile(`\(CID=\(PROGRAM=\w+\)(HOST=\w+\)\(USER=\w+\)\)`)
		tnsreg2 := regexp.MustCompile(`\(CID=\(PROGRAM=[^)]*\)\(HOST=[^)]*\)\(USER=[^)]*\)\)`)

		*tns = tnsreg.ReplaceAllString(*tns, swaptns)
		*tns = tnsreg2.ReplaceAllString(*tns, swapcid)
	}

	if Bit(tracelevel, TRCTNS) == true {
		fmt.Printf("TRCTNS: Newstring.............:[%s]\n", *tns)
	}

}

// BaseURL composes  url path for the rest calls
func (r *LRPDBReconciler) BaseURL2(req ctrl.Request, lrpdb *dbapi.LRPDB) string {
	log := r.Log.WithValues("BaseUrl", req.NamespacedName)
	baseurl := "https://" + lrpdb.Spec.CDBResName + "-lrest." + lrpdb.Spec.CDBNamespace + ":" + strconv.Itoa(8888) + "/database/pdbs/"
	if Bit(lrpdb.Spec.Trclvl, TRCSQL) == true {
		log.Info("Baseurl:" + baseurl)
	}
	return baseurl
}

// MonitorConfigMap : method to monitor pdb init settings. It compares config map contents with the v$parameter check divergnences
// an take an action
func (r *LRPDBReconciler) MonitorConfigMap(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("InitConfigMap", req.NamespacedName)
	log.Info("Monitoring configmap")
	lrpdbName := lrpdb.Spec.LRPDBName

	url := r.BaseURL2(req, lrpdb) + lrpdbName + "/modparameters/"

	// get data from v$parameter
	output, err := NewCallAPISQL(ctx, r, req, lrpdb, url, nil, "GET")
	if err != nil {
		log.Info("NewCallAPISQL Error")
	}

	// The logic is for each row in the v$parameter check the
	// the entry in configmap

	data := []byte(` {"INIT":` + output + `}`)
	var idata interface{}
	err = json.Unmarshal(data, &idata)
	if err != nil {
		log.Info("error json.Unmarshal")
		return err
	}

	mdata, ok := idata.(map[string]interface{})
	if !ok {
		return errors.New("Fail to cast idata")
	}

	ndata, ok := mdata["INIT"].([]interface{})
	if !ok {
		return errors.New("Fail to cast mdata")
	}

	fmt.Printf(":%s:\n", ndata)

	configmap, err := r.GetConfigMap(req, lrpdb)
	if err != nil {
		log.Error(err, "Fail to fetch code configmap", "err", err.Error())
		return err
	}
	tokens := ParseConfigMapData(configmap, lrpdb.Spec.Trclvl)

	// Pushing configmap values into a bidimensional hash array
	// 0->key [0][value] [0][scope]
	mapMatrix := make(map[string]map[string]string)

	for cnt := range tokens {
		Parameter := strings.Split(tokens[cnt], " ")
		if len(Parameter) == 3 {
			mapMatrix[Parameter[0]] = make(map[string]string)
			mapMatrix[Parameter[0]]["value"] = Parameter[1]
			mapMatrix[Parameter[0]]["scope"] = Parameter[2]
		}
	}
	// select json_arrayagg( json_object( 'name' is a.name, 'value' is a.value,'issys_modifiable' is a.issys_modifiable)
	// returning clob pretty ) from v$parameter  a where a.ismodified = 'SYSTEM_MOD'

	for idx := range ndata {
		name, ok := ndata[idx].(map[string]interface{})["name"].(string)
		if !ok {
			return errors.New("fail to cast v$parameter.name")
		}
		value, ok := ndata[idx].(map[string]interface{})["value"].(string)
		if !ok {
			return errors.New("fail to cast v$parameter.value")
		}
		ismod, ok := ndata[idx].(map[string]interface{})["issys_modifiable"].(string)
		if !ok {
			return errors.New("fail to cast v$parameter.issys_modifiable")
		}
		if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
			fmt.Printf("TRCCFM: [v$parameter].name=%s value=%s issys_modifiable=%s\n", name, value, ismod)

			// check matrix
			fmt.Printf("TRCCFM: mapMatrix[%s][value]=%s\n", name, mapMatrix[name]["value"])
			fmt.Printf("TRCCFM: mapMatrix[%s][scope]=%s\n", name, mapMatrix[name]["scope"])
		}

		// We monitor the config not the altersystem via kubectl this is the case mapMatrix is nil
		if value != mapMatrix[name]["value"] && mapMatrix[name]["value"] != "" {
			log.Info("config map and v$parameter out of sync")
			if r.alterSystemLRPDB2(ctx, req, lrpdb, name, mapMatrix[name]["value"], mapMatrix[name]["scope"], "resyncparameter") != nil {
				//log.Error(err, "alterSystemLRPDB2 failure ", "err", err.Error())
				log.Info("alterSystemLRPDB2 failure")
			}

			if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
				fmt.Printf("TRCCFM: MonitorConfigMap................:%s\n", lrpdb.Name)
				fmt.Printf("TRCCFM: MonitorConfigMpp nmsp...........:%s\n", lrpdb.Namespace)
				fmt.Printf("TRCCFM: Parameter out sync..............:%s\n", name)
				fmt.Printf("TRCCFM: CM value          ..............:%s\n", mapMatrix[name]["value"])
				fmt.Printf("TRCCFM: v$parameter value ..............:%s\n", value)
			}

		} else {
			log.Info("config map and v$parameter sync")
		}
	}

	return nil
}

// InitConfigMap  Initialize the parameter config map; if user does not specificate a cm
// then it creates an empty one as a place holder
func (r *LRPDBReconciler) InitConfigMap(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) *corev1.ConfigMap {
	log := r.Log.WithValues("InitConfigMap", req.NamespacedName)
	log.Info("Begin call")

	if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
		fmt.Printf("TRCCFM: (init)ConfigMap................:ConfigMaps%s\n", lrpdb.Name)
		fmt.Printf("TRCCFM: (init)ConfigMap nmsp...........:%s\n", lrpdb.Namespace)
	}
	/*
	 *  PDB SYSTEM PARAMETER
	 *  record [name,value=[paramete_val|reset],level=[session|system]]
	 */

	if lrpdb.Spec.PDBConfigMap == "" {
		/* if users does not specify a config map
		we generate an empty new one for possible
		future pdb parameter  modification */

		var SystemParameters map[string]string
		var cfgmp string

		if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
			fmt.Printf("TRCCFM: (init)Generating an empty configmap")
		}
		cfgmp = "configmap-" + lrpdb.Spec.LRPDBName + "-default"
		// RFC 1123
		cfgmp = strings.ToLower(cfgmp)
		cfgmp = strings.ReplaceAll(cfgmp, "_", "-")

		DBParameters := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "configmap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfgmp,
				Namespace: lrpdb.Namespace,
			},
			Data: SystemParameters,
		}

		if err := ctrl.SetControllerReference(lrpdb, DBParameters, r.Scheme); err != nil {
			log.Error(err, "Fail to set SetControllerReference", "err", err.Error())
			return nil
		}

		if err := r.Create(ctx, DBParameters); err != nil {
			if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
				fmt.Printf("TRCCFM: (init) Configmap creatio")
			}
			if apierrors.IsAlreadyExists(err) == true {
				log.Info("Default configmap already exists")
				if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
					fmt.Printf("TRCCFM: (init) Configmap already exists\n")
					fmt.Printf("TRCCFM: (init) Configmap Status[%s][%d][%s]\n", lrpdb.Spec.LRPDBName, lrpdb.Status.CmBitstat, lrpdb.Status.CmBitStatStr)
					fmt.Printf("TRCCFM: (init) defaultconfigmap=%s\n", cfgmp)
				}
				return nil
			}

			if err != nil {
				log.Info("Cannot create config map")
				r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "LRPDBError", "Failed configmap: %v", err)
				return nil
			}

		}

		lrpdb.Spec.PDBConfigMap = cfgmp
		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Failure updating Spec.PDBConfigMap ", "err", err.Error())
			return nil
		}
		lrpdb.Status.CmBitstat = Bis(lrpdb.Status.CmBitstat, MPEMPT)
		lrpdb.Status.CmBitStatStr = CMBitmaskprint(lrpdb.Status.CmBitstat)
		if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
			fmt.Printf("TRCCFM: (init) Configmap Status[%s][%d][%s]\n", lrpdb.Spec.LRPDBName, lrpdb.Status.CmBitstat, lrpdb.Status.CmBitStatStr)
		}
		r.UpdateStatus(ctx, req, lrpdb)
		return DBParameters

	} else {

		lrpdb.Status.CmBitstat = Bis(lrpdb.Status.CmBitstat, MPINIT)
		lrpdb.Status.CmBitStatStr = CMBitmaskprint(lrpdb.Status.CmBitstat)
		//defaultconfigmap = lrpdb.Spec.PDBConfigMap
		if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
			fmt.Printf("TRCCFM: (init) Configmap Status[%s][%d][%s]\n", lrpdb.Spec.LRPDBName, lrpdb.Status.CmBitstat, lrpdb.Status.CmBitStatStr)
		}
		DBParameters, err := r.GetConfigMap(req, lrpdb)
		if err != nil {
			log.Error(err, "Fail to fetch configmap ", "err", err.Error())
			return nil
		}

		r.UpdateStatus(ctx, req, lrpdb)
		return DBParameters
	}

	//return nil
}

// GetConfigMap : wrappinh method - k8s.FetchConfigMap
func (r *LRPDBReconciler) GetConfigMap(req ctrl.Request, lrpdb *dbapi.LRPDB) (*corev1.ConfigMap, error) {
	log := r.Log.WithValues("GetConfigMap", req.NamespacedName)
	var cfgmp string

	if lrpdb.Spec.PDBConfigMap == "" {
		cfgmp = "configmap-" + lrpdb.Spec.LRPDBName + "-default"
		cfgmp = strings.ToLower(cfgmp)
		cfgmp = strings.ReplaceAll(cfgmp, "_", "-")
	} else {
		cfgmp = lrpdb.Spec.PDBConfigMap
	}
	log.Info("GetapGlobal.............:" + cfgmp)

	DBParameters, err := r.fetchCfgm2(r.Client, lrpdb.Namespace, cfgmp)
	if err != nil {
		log.Error(err, "Fail to fetch configmap", "err", err.Error())
		return nil, err
	}

	return DBParameters, nil
}

// GetConfigMapCode : reads plsql sql code from code config map
func (r *LRPDBReconciler) GetConfigMapCode(req ctrl.Request, lrpdb *dbapi.LRPDB) (*corev1.ConfigMap, error) {
	log := r.Log.WithValues("GetConfigMapCode", req.NamespacedName)
	log.Info("CodeMapGlobal.............:" + lrpdb.Spec.PLSQLBlock)
	CodeBlock, err := r.fetchCfgm2(r.Client, lrpdb.Namespace, lrpdb.Spec.PLSQLBlock)
	if err != nil {
		log.Error(err, "Fail to fetch configmap", "err", err.Error())
		return nil, err
	}

	return CodeBlock, nil
}

// ApplyConfigMap : Apply init config map to the pdb
func (r *LRPDBReconciler) ApplyConfigMap(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) (int32, error) {
	log := r.Log.WithValues("ApplyConfigMap", req.NamespacedName)
	/* We read the config map and apply the setting to the  pdb */

	log.Info("Begin call")
	configmap, err := r.GetConfigMap(req, lrpdb)
	if err != nil {
		log.Info("Cannot get config map in the open yaml file")
		return 0, nil
	}
	Cardinality := int32(len(configmap.Data))
	if Cardinality == 0 {
		if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
			fmt.Printf("TRCCFM: (apply) Empty config map... nothing to do")
		}
		return 0, nil
	}

	if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
		fmt.Printf("TRCCFM: (apply) GetConfigMap completed")
		fmt.Printf("TRCCFM: (apply) ConfigMap cardinality %d\n", Cardinality)
	}

	tokens := ParseConfigMapData(configmap, lrpdb.Spec.Trclvl)
	for cnt := range tokens {
		if len(tokens[cnt]) != 0 {
			/*  avoid null token and check malformed value  */
			if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
				fmt.Printf("TRCCFM: token=[%s]\n", tokens[cnt])
			}

			Parameter := strings.Split(tokens[cnt], " ")
			if len(Parameter) != 3 {
				log.Info("WARNING  malformed value in the configmap")
			} else {
				if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
					fmt.Printf("TRCCFM: (apply) alter system set %s=%s scope=%s instances=all\n", Parameter[0], Parameter[1], Parameter[2])
				}
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
				url := r.BaseURL2(req, lrpdb) + lrpdb.Spec.LRPDBName
				respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, AlterSystemPayload, "POST")
				if err != nil {
					log.Error(err, "callAPI failure durring Apply Config Map", "err", err.Error())
					return 0, err
				}
				/* check sql code execution */
				var retJSON map[string]interface{}
				if err := json.Unmarshal([]byte(respData), &retJSON); err != nil {
					log.Error(err, "failed to get Data from callAPI", "err", err.Error())
					return 0, err
				}
				/* We do not the execution if something goes wrong for a single parameter
				   just report the error in the event queue */
				SQLCode := strconv.Itoa(int(retJSON["sqlcode"].(float64)))
				AlterMsg := fmt.Sprintf("pdb=%s:%s:%s:%s:%s", lrpdb.Spec.LRPDBName, Parameter[0], Parameter[1], Parameter[2], SQLCode)
				if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
					fmt.Printf("TRCCFM: (apply) Config Map Applied......%s\n", AlterMsg)
				}

				if SQLCode != "0" {
					r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "lrpdbinfo", AlterMsg)
					lrpdb.Status.CmBitstat = Bis(lrpdb.Status.CmBitstat, MPWARN)
				}

			}
		}
	}

	/*
		if err := r.Update(ctx, lrpdb); err != nil {
			log.Error(err, "Cannot rest lrpdb Spec  :"+lrpdb.Name, "err", err.Error())
		}
	*/

	lrpdb.Status.CmBitstat = Bis(lrpdb.Status.CmBitstat, MPAPPL)
	lrpdb.Status.CmBitStatStr = CMBitmaskprint(lrpdb.Status.CmBitstat)
	r.UpdateStatus(ctx, req, lrpdb)
	if Bit(lrpdb.Spec.Trclvl, TRCCFM) == true {
		fmt.Printf("TRCCFM: (apply) Configmap Status[%s][%d][%s]\n", lrpdb.Spec.LRPDBName, lrpdb.Status.CmBitstat, lrpdb.Status.CmBitStatStr)
	}

	return Cardinality, nil
}

// ManageConfigMapForCloningAndPlugin : Manage config maps during clone process
func (r *LRPDBReconciler) ManageConfigMapForCloningAndPlugin(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("ManageConfigMapForCloningAndPlugin", req.NamespacedName)
	log.Info("Begin Call")
	/*
	   If configmap parameter is set and init flag is not set
	   then we need to iniialized the init mask. This is the case for
	   pdb generated by clone and plug action
	*/
	if lrpdb.Spec.Action != "CREATE" &&
		lrpdb.Spec.Action != "APPLYSQL" &&
		lrpdb.Spec.PDBConfigMap != "" &&
		Bit(lrpdb.Status.CmBitstat, MPINIT) == false {
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

// GetSQLCode reads sqlcode from resource status and copy into sqlcode pointer
func (r *LRPDBReconciler) GetSQLCode(rsp string, sqlcode *int, tracelevel int) error {
	log := r.Log.WithValues("GetSQLCode", "callAPI(...)")
	log.Info("Begin call")

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		return err
	}

	*sqlcode = int(objmap["sqlcode"].(float64))
	if Bit(tracelevel, TRCSQL) == true {
		fmt.Printf("TRCSQL :sqlcode.......:ora-%s\n", strconv.Itoa(*sqlcode))
	}
	if *sqlcode != 0 {
		// Check the white list
		if DiscardableError(*sqlcode) == true {
			log.Info("Discardable error: ora-" + strconv.Itoa(*sqlcode))
			return nil
		}
		err := fmt.Errorf("%v", sqlcode)
		return err
	}
	return nil
}

// GetRestricted checks whether or not the pdb is in restricted mode
func (r *LRPDBReconciler) GetRestricted(rsp string, restrictmode *string) {
	log := r.Log.WithValues("GetRestriced", "callAPI(...)")

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		*restrictmode = "GetRestricted failure"
	}

	*restrictmode = string(objmap["restricted"].(string))

}

// GetPdbSize2 Return the size of the pluggable database
func (r *LRPDBReconciler) GetPdbSize2(rsp string, pdbsize *string) {
	log := r.Log.WithValues("GetPdbSize2", "callAPI(...)")
	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		*pdbsize = "GetPdbSize2 Failure"
	}
	*pdbsize = fmt.Sprintf("%4.2f", ((objmap["total_size"].(float64))/1024/1024/1024)) + "G"
}

// GetOpenMode returns the openmode status
func (r *LRPDBReconciler) GetOpenMode(rsp string, openmode *string) {
	log := r.Log.WithValues("GetRestriced", "callAPI(...)")

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		log.Error(err, "failed to get respData from callAPI", "err", err.Error())
		*openmode = "GetOpenMode failure"
	}

	*openmode = string(objmap["open_mode"].(string))

}

// ParseSQLPayload read plsqlcode from config map
func ParseSQLPayload(payload *PLSQLPayLoad, Trclvl int) string {
	var Buffer string

	cnt := 0
	Buffer = "{"
	for key, value := range payload.Values {
		Buffer += "\"" + key + "\" : \"" + value + "\","
	}

	Nelem := len(payload.Sqltokens)
	if Bit(Trclvl, TRCPSQ) == true {
		fmt.Printf("TRCPSQ: ParseSQLPayload :: Num tokens %d\n", Nelem)
	}
	Buffer += "\"Sqltokens\":["
	for _, value := range payload.Sqltokens {
		Buffer += "\"" + value + "\""
		if cnt < (Nelem - 1) {
			Buffer += ","
		}
		cnt++
	}

	Buffer += "]}"
	if Bit(Trclvl, TRCPSQ) == true {
		fmt.Printf("TRCPSQ: ParseSQLPayload :: %s\n", Buffer)
	}
	return Buffer
}

// GetPdbSize : returns the size of pdb
func (r *LRPDBReconciler) GetPdbSize(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) string {
	log := r.Log.WithValues("GetPdbSize", req.NamespacedName)
	var PdbSize string

	lrpdbName := lrpdb.Spec.LRPDBName
	url := r.BaseURL2(req, lrpdb) + lrpdbName + "/status/"

	respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, nil, "GET")
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

// UpdateStatus : method to update the resource status and check the op return spoke
func (r *LRPDBReconciler) UpdateStatus(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) {
	log := r.Log.WithValues("UpdateStatus", req.NamespacedName)
	err := r.Status().Update(ctx, lrpdb)
	if err != nil {
		fmt.Printf("[1]Error updating status\n")
		log.Error(err, err.Error())
		if Bit(lrpdb.Spec.Trclvl, TRCSTK) == true {
			Backtrace()
		}
	}
}

// NewCallAPISQL : - DO NOT SWITCH TO LOCAL - lrest controller invokes this function for autodiscover
// and pdb deletion
func NewCallAPISQL(ctx context.Context, intr interface{}, req ctrl.Request, lrcrd interface{}, url string, payload interface{}, action string) (string, error) {
	var c client.Client
	var r logr.Logger
	var e record.EventRecorder
	var TestBuffer string
	var jsonMap map[string]interface{}
	var webUser string
	var webUserPwd string
	/*
		var rsaKeyPEM string
		var rsaCertPEM string
	*/
	//var caCert string
	var err error
	var Trclvl int
	//var NmTLSKey = [2]string{"", ""}
	//var NmTLSCrt = [2]string{"", ""}
	//var NmTLSCat = [2]string{"", ""}
	//var NmPriKey = [2]string{"", ""}
	//var NmWebUse = [2]string{"", ""}
	//var NmWebPwd = [2]string{"", ""}
	var respData string

	recpdb, ok1 := intr.(*LRPDBReconciler)
	if ok1 {
		// fmt.Printf("func NewCallApiSQL ((*PDBReconciler),......)\n")
		c = recpdb.Client
		e = recpdb.Recorder
		r = recpdb.Log
	}

	reccdb, ok2 := intr.(*LRESTReconciler)
	if ok2 {
		// fmt.Printf("func NewCallApiSQL ((*CDBReconciler),......)\n")
		c = reccdb.Client
		e = reccdb.Recorder
		r = reccdb.Log
	}
	lrpdb, ok3 := lrcrd.(*dbapi.LRPDB)
	lrest, ok4 := lrcrd.(*dbapi.LREST)

	log := r.WithValues("NewCallAPISQL", req.NamespacedName)

	if ok3 {

		//NmTLSKey[0] = lrpdb.Spec.LRPDBTlsKey.Secret.SecretName
		//NmTLSKey[1] = lrpdb.Spec.LRPDBTlsKey.Secret.Key

		//NmTLSCrt[0] = lrpdb.Spec.LRPDBTlsCrt.Secret.SecretName
		//NmTLSCrt[1] = lrpdb.Spec.LRPDBTlsCrt.Secret.Key

		//NmTLSCat[0] = lrpdb.Spec.LRPDBTlsCat.Secret.SecretName
		//NmTLSCat[1] = lrpdb.Spec.LRPDBTlsCat.Secret.Key

		Trclvl = lrpdb.Spec.Trclvl
		/*
			NmWebUse[0] = lrpdb.Spec.WebLrpdbServerUser.Secret.SecretName
			NmWebUse[1] = lrpdb.Spec.WebLrpdbServerUser.Secret.Key

			NmWebPwd[0] = lrpdb.Spec.WebLrpdbServerPwd.Secret.SecretName
			NmWebPwd[1] = lrpdb.Spec.WebLrpdbServerPwd.Secret.Key
		*/
		//NmPriKey[0] = lrpdb.Spec.LRPDBPriKey.Secret.SecretName
		//NmPriKey[1] = lrpdb.Spec.LRPDBPriKey.Secret.Key
	}

	if ok4 {
		//NmTLSKey[0] = lrest.Spec.LRESTTlsKey.Secret.SecretName
		//NmTLSKey[1] = lrest.Spec.LRESTTlsKey.Secret.Key

		//NmTLSCrt[0] = lrest.Spec.LRESTTlsCrt.Secret.SecretName
		//NmTLSCrt[1] = lrest.Spec.LRESTTlsCrt.Secret.Key

		//NmTLSCat[0] = lrest.Spec.LRESTTlsCat.Secret.SecretName
		//NmTLSCat[1] = lrest.Spec.LRESTTlsCat.Secret.Key

		Trclvl = lrest.Spec.Trclvl

		//NmWebUse[0] = lrest.Spec.WebLrestServerUser.Secret.SecretName
		//NmWebUse[1] = lrest.Spec.WebLrestServerUser.Secret.Key

		//NmWebPwd[0] = lrest.Spec.WebLrestServerPwd.Secret.SecretName
		//NmWebPwd[1] = lrest.Spec.WebLrestServerPwd.Secret.Key

		//NmPriKey[0] = lrest.Spec.LRESTPriKey.Secret.SecretName
		//NmPriKey[1] = lrest.Spec.LRESTPriKey.Secret.Key

	}

	/* FIX for pin CA */
	var lrestName, lrestNamespace string

	if ok3 {
		lrestName = lrpdb.Spec.CDBResName
		lrestNamespace = lrpdb.Spec.CDBNamespace
	}

	if ok4 {
		lrestName = lrest.Name
		lrestNamespace = lrest.Namespace
	}

	secret := &corev1.Secret{}

	err = c.Get(ctx, types.NamespacedName{Name: lrestCAPinSecretName(lrestName), Namespace: lrestNamespace}, secret)
	if err != nil {
		return "", err
	}

	caPEM, ok := secret.Data[lrestCAPinSecretKey]
	if !ok || len(caPEM) == 0 {
		return "", fmt.Errorf("missing %s in %s/%s", lrestCAPinSecretKey, lrestNamespace, secret.Name)
	}

	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caPEM); !ok {
		return "", fmt.Errorf("invalid pinned CA in %s/%s", lrestNamespace, secret.Name)
	}

	if Bit(Trclvl, TRCAPI) == true {
		if ok1 {
			fmt.Printf("TRCAPI: NewCallApiSQL ((*LRPDBReconciler),......)\n")
		}
		if ok2 {
			fmt.Printf("TRCAPI: NewCallApiSQL ((*LRSETReconciler),......)\n")
		}
		if Bit(Trclvl, TRCSTK) == true {
			Backtrace()
		}
	}

	/* DROP TEST
	rsaKeyPEM, err = getGenericSecret3(ctx, intr, req, lrcrd,
		NULL, NULL, NULL, NULL, NmTLSKey[0], NmTLSKey[1], true)
	if CheckErr(ctx, err, intr, req, lrcrd, nil) == true {
		return "", err
	}

	rsaCertPEM, err = getGenericSecret3(ctx, intr, req, lrcrd,
		NULL, NULL, NULL, NULL, NmTLSCrt[0], NmTLSCrt[1], true)
	if CheckErr(ctx, err, intr, req, lrcrd, nil) {
		return "", err
	}
	*/

	/*
		caCert, err = getGenericSecret3(ctx, intr, req, lrcrd,
			NULL, NULL, NULL, NULL, NmTLSCat[0], NmTLSCat[1], true)
		if CheckErr(ctx, err, intr, req, lrcrd, nil) == true {
			return "", err
		}
	*/

	/*
		certificate, err := tls.X509KeyPair([]byte(rsaCertPEM), []byte(rsaKeyPEM))
		if err != nil {
			log.Info("Error tls.X509KeyPair")
			return "", err
		}
	*/

	// DROP TEST
	//	tlsConf := &tls.Config{Certificates: []tls.Certificate{certificate},
	tlsConf := &tls.Config{RootCAs: caCertPool,
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

	if Bit(Trclvl, TRCAPI) == true {
		fmt.Printf("TRCAPI: Restcall [URL]:[%s] [ACTION]:[%s]\n", url, action)
	}

	/**
	webUser, err = getGenericSecret3(intr, ctx, req, lrcrd,
		NmWebUse[0], NmWebUse[1],
		NmPriKey[0], NmPriKey[1],
		NULL, NULL, true)
	if CheckErr(err, intr, ctx, req, lrcrd, nil) == true {
		return "", err
	}

	webUserPwd, err = getGenericSecret3(intr, ctx, req, lrcrd,
		NmWebPwd[0], NmWebPwd[1],
		NmPriKey[0], NmPriKey[1],
		NULL, NULL, true)
	if CheckErr(err, intr, ctx, req, lrcrd, nil) == true {
		return "", err
	}
	**/

	/*
		webUser = GenHash(GetBaseName(url), "USR")
		webUserPwd = GenHash(GetBaseName(url), "PWD")
	*/
	webUser = string(secret.Data[consR3])
	webUserPwd = string(secret.Data[consR4])

	var Httpreq *http.Request

	if url == "" {
		return "", errors.New("Null url passed")
	}

	if action == "GET" {
		Httpreq, err = http.NewRequest(action, url, nil)
		if err != nil {
			log.Info("http request error hhtp.NewRequest")
			if Bit(Trclvl, TRCAPI) == true {
				log.Error(err, err.Error())
			}
		}
	} else {
		/* Section to execute sql and plsql code */
		if payload != nil {
			payloadsql, oksql := payload.(*PLSQLPayLoad)
			if oksql {
				TestBuffer = ParseSQLPayload(payloadsql, Trclvl)
				//json.Unmarshal([]byte(TestBuffer), &jsonMap)
				if err = json.Unmarshal([]byte(TestBuffer), &jsonMap); err != nil {
					log.Info("Unmarshall Warning")
					//return "", err
				}
				jsonValue, _ := json.Marshal(jsonMap)
				Httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
				if Bit(Trclvl, TRCAPI) == true {
					fmt.Printf("TRCAPI:BEGIN PLSQLPAYLOAD\n")
					fmt.Printf("TRCAPI:%s\n", string(jsonValue))
					fmt.Printf("TRCAPI:END PLSQLPAYLOAD\n")
				}
				if err != nil {
					log.Info("Unable to create HTTP Request (PLSQLPAYLOAD)", "err", err.Error())
					return "", err
				}
			}
			/* Section to execute standard pdb operation */
			payloadpdb, okpdb := payload.(map[string]string)
			if okpdb {
				jsonValue, _ := json.Marshal(payloadpdb)
				Httpreq, err = http.NewRequest(action, url, bytes.NewBuffer(jsonValue))
				if Bit(Trclvl, TRCAPI) == true {
					fmt.Printf("TRCAPI: BEGIN PDBPAYLOAD\n")
					fmt.Printf("TRCAPI:%s\n", string(jsonValue))
					fmt.Printf("TRCAPI: END PDBPAYLOAD\n")
				}
				if err != nil {
					log.Info("Unable to create HTTP Request for PDBPAYLOAD ", "err", err.Error())
					return "", err
				}
			}
		}
	}

	Httpreq.Header.Add("Accept", "application/json")
	Httpreq.Header.Add("Content-Type", "application/json")
	Httpreq.SetBasicAuth(webUser, webUserPwd)

	resp, err := httpclient.Do(Httpreq)
	/* CALL FROM LRPDB CONTROLLER */
	if ok3 {
		if err != nil {
			errmsg := err.Error()
			log.Error(err, "Failed - Could not connect to LREST Pod", "err", err.Error())
			lrpdb.Status.Msg = "Error: Could not connect to LREST Pod"
			e.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTError", errmsg)
			return "", err
		}

		if resp.StatusCode != http.StatusOK {
			// lint SA1019 bb, _ := ioutil.ReadAll(resp.Body)
			bb, _ := io.ReadAll(resp.Body)

			if resp.StatusCode == 404 {
				lrpdb.Status.ConnString = ""
				lrpdb.Status.Msg = lrpdb.Spec.LRPDBName + " not found"

			} else {
				if floodcontrol == false {
					lrpdb.Status.Msg = "LREST Error - HTTP Status Code:" + strconv.Itoa(resp.StatusCode)
				}
			}

			if floodcontrol == false {
				log.Info("LREST Error - HTTP Status Code :"+strconv.Itoa(resp.StatusCode), "Err", string(bb))
			}

			var apiErr LRESTError
			//json.Unmarshal([]byte(bb), &apiErr)
			if err = json.Unmarshal([]byte(bb), &apiErr); err != nil {
				log.Info("Unmarshal warning")
				//return "", err
			}

			if floodcontrol == false {
				e.Eventf(lrpdb, corev1.EventTypeWarning, "LRESTError", "Failed: %s", apiErr.Message)
			}
			if Bit(lrpdb.Spec.Trclvl, TRCAPI) == true {
				fmt.Printf("TRCAPI:URL APIERR\n")
				fmt.Printf("TRCAPI:%+v \n", apiErr)
				fmt.Printf("TRCAPI:URL=%s\n", url)
				fmt.Printf("TRCAPI:resp.StatusCode=%s\n", strconv.Itoa(resp.StatusCode))
				fmt.Printf("\n================== APIERR ======================\n")
			}
			floodcontrol = true
			return "", errors.New("LREST Error")
		}
		floodcontrol = false

		defer func() {
			if resp.Body.Close() != nil {
				log.Info("Body.Close failure")
			}
		}()

		// lint SA1019 bodyBytes, err := ioutil.ReadAll(resp.Body)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Print(err.Error())
		}
		respData = string(bodyBytes)

		if Bit(lrpdb.Spec.Trclvl, TRCAPI) == true {
			fmt.Printf("TRCAPI: CALL API return msg.....:")
			fmt.Printf("%s\n", respData)
			fmt.Println(string(bodyBytes))
		}

		var apiResponse restSQLCollection
		//json.Unmarshal([]byte(bodyBytes), &apiResponse)
		if err = json.Unmarshal([]byte(bodyBytes), &apiResponse); err != nil {
			log.Info("Unmarshal waring")
			//return "", err
		}

		if Bit(lrpdb.Spec.Trclvl, TRCAPI) == true {
			fmt.Printf("TRCAPI: BEGIN REST API RESPONSE\n")
			fmt.Printf("TRCAPI:%#v\n", apiResponse)
			fmt.Printf("TRCAPI:%+v\n", apiResponse)
			fmt.Printf("TRCAPI: BEGIN END API RESPONSE\n")
		}

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
	}

	/* CALL FROM LREST CONTROLLER */
	if ok4 {

		if err != nil {
			log.Info("Rest server temporary unavailable")
			errmsg := err.Error()
			log.Error(err, "Failed - Could not connect to LREST Pod", "err", err.Error())
			lrest.Status.Msg = "Error: Could not connect to LREST Pod"
			e.Eventf(lrest, corev1.EventTypeWarning, "LRESTError", errmsg)
			return "", err
		}

		e.Eventf(lrest, corev1.EventTypeWarning, "Done", lrest.Spec.LRESTName)
		if resp.StatusCode != http.StatusOK {
			// lint SA1019 bb, _ := ioutil.ReadAll(resp.Body)
			bb, _ := io.ReadAll(resp.Body)

			if resp.StatusCode == 404 {
				log.Info("error 404")

			} else {
				if floodcontrol == false {
					lrest.Status.Msg = "LREST Error - HTTP Status Code:" + strconv.Itoa(resp.StatusCode)
				}
			}

			if floodcontrol == false {
				log.Info("LREST Error - HTTP Status Code :"+strconv.Itoa(resp.StatusCode), "Err", string(bb))
			}

			var apiErr LRESTError
			if err = json.Unmarshal([]byte(bb), &apiErr); err != nil {
				log.Error(err, "Warning Unmarshal error")
				//return "", err
			}

			if floodcontrol == false {
				e.Eventf(lrest, corev1.EventTypeWarning, "LRESTError", "Failed: %s", apiErr.Message)
			}
			fmt.Printf("\n================== APIERR ======================\n")
			fmt.Printf("%+v \n", apiErr)
			fmt.Printf("URL=%s\n", url)
			fmt.Printf("resp.StatusCode=%s\n", strconv.Itoa(resp.StatusCode))
			fmt.Printf("\n================== APIERR ======================\n")
			floodcontrol = true
			return "", errors.New("LREST Error")
		}
		floodcontrol = false

		//defer resp.Body.Close()
		defer func() {
			if resp.Body.Close() != nil {
				log.Info("Body.Close failure")
			}
		}()

		// lint SA1019 bodyBytes, err := ioutil.ReadAll(resp.Body)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Print(err.Error())
		}
		respData = string(bodyBytes)

		var apiResponse restSQLCollection
		//json.Unmarshal([]byte(bodyBytes), &apiResponse)
		if err = json.Unmarshal([]byte(bodyBytes), &apiResponse); err != nil {
			log.Info("Unmarshal warning")
			//return "", err

		}

		if Bit(lrest.Spec.Trclvl, TRCAPI) == true {
			fmt.Printf("TRCAPI: CALL API return msg.....:%s\n", string(bodyBytes))
			fmt.Printf("TRCAPI: apiResponse %#v\n", apiResponse)
			fmt.Printf("TRCAPI: apiResponse %+v\n", apiResponse)
		}

		errFound := false
		for _, sqlItem := range apiResponse.Items {
			if sqlItem.ErrorDetails != "" {
				log.Info("LREST Error - Oracle Error Code :" + strconv.Itoa(sqlItem.ErrorCode))
				if !errFound {
					lrest.Status.Msg = sqlItem.ErrorDetails
				}
				e.Eventf(lrest, corev1.EventTypeWarning, "OraError", "%s", sqlItem.ErrorDetails)
				errFound = true
			}
		}

		if errFound {
			return "", errors.New("Oracle Error")
		}

	}

	return respData, nil
}

/* We remove secrets regardles the status of user creation */
func (r *LRPDBReconciler) DropUserSecret(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, usrsec *corev1.Secret) error {
	log := r.Log.WithValues("dropUserSecret", req.NamespacedName)

	err := r.Delete(context.TODO(), usrsec)
	if err != nil {
		log.Info("Failed to delete app user creation secret: ")
		return err
	}

	log.Info("pdb user secrets deletion completed")
	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Secret  deleted\n")
	}

	return nil
}

// CreateApplicanUsers method creates pdb application users
// Input parameters:
//   - context
//   - request
//   - lrpdb crd
//   - pdbname
//   - Secret name
func (r *LRPDBReconciler) CreateApplicanUsers(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB, Secname string) error {
	log := r.Log.WithValues("CreateApplicanUsers", req.NamespacedName)

	// bug 	39214736
	defer func() {
		orgcp := lrpdb.DeepCopy()
		lrpdb.Spec.Pdbappuser = ""
		if err := r.Patch(ctx, lrpdb, client.MergeFrom(orgcp)); err != nil {
			log.Info("Resource patch failure")
		}

	}()

	/* Cannot create users if  pdb is not open */
	if Bit(lrpdb.Status.PDBBitMask, PDBCLS) == true {
		log.Info("Cannot create users : pdb not open")
		lrpdb.Status.Msg = "Cannot create user"
		r.UpdateStatus(ctx, req, lrpdb)
		return nil
	}

	usersec := &corev1.Secret{}

	// Testing lint correction QF1008
	// err := r.Client.Get(ctx, types.NamespacedName{Name: Secname, Namespace: lrpdb.Namespace}, usersec)
	err := r.Get(ctx, types.NamespacedName{Name: Secname, Namespace: lrpdb.Namespace}, usersec)
	if err != nil {
		log.Info("Error: cannot get secret " + Secname)
		return err
	}

	lrpdb.Status.Msg = "create user:[op. in progress]"
	r.UpdateStatus(ctx, req, lrpdb)

	log.Info("Starting user analysis")
	credential := []string{}

	cnt := 0

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: key={")
	}
	for key := range usersec.Data {
		cnt++
		if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
			fmt.Printf("%s ", key)
		}
		credential = append(credential, key)
	}

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("}\n")
	}

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Order array by key name\n")
	}

	sort.Strings(credential)
	arrsz := len(credential)

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Validate number of keys in the secret\n")
	}

	if (arrsz % 3) != 0 {
		if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
			fmt.Printf("TRCUSR: array size=%d \n", arrsz)
			fmt.Printf("TRCUSR: Worng number of elements\n")
		}

		r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "Crt usr malformed secret[0]", Secname)
		lrpdb.Status.Msg = "create user:[malformed secrets:0]"
		lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, APPERR)
		lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
		r.UpdateStatus(ctx, req, lrpdb)
		log.Info("Malformed information - Secrets will be removed")

		err = r.DropUserSecret(ctx, req, lrpdb, usersec)
		if err != nil {
			log.Info("Failed to delete app user creation secret: ")
			return err
		}

		if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
			fmt.Printf("TRCUSR: Restting lrpdb.Spec.Pdbappuser")
		}

		return errors.New("Wrong number of arguments")

	}

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Arraysize=%d\n", arrsz)
	}

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Validate key format\n")
	}
	reu := regexp.MustCompile(`^usr\d+$`)
	rep := regexp.MustCompile(`^pwd\d+$`)
	rgr := regexp.MustCompile(`^grt\d+$`)

	for cnt := 0; cnt <= (arrsz)-1; cnt++ {
		if rep.MatchString(credential[cnt]) == true {

			if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
				fmt.Printf("TRCUSR: %s \t[OK]:rep\n", credential[cnt])
			}
		} else if reu.MatchString(credential[cnt]) {
			if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
				fmt.Printf("TRCUSR: %s \t[OK]:reu\n", credential[cnt])
			}
		} else if rgr.MatchString(credential[cnt]) {
			if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
				fmt.Printf("TRCUSR: %s \t[OK]:rgr\n", credential[cnt])
			}
		} else {
			r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "Crt usr malformed secret[1]", Secname)
			lrpdb.Status.Msg = "create user:[malformed secrets:1]"
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, APPERR)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
			log.Info("Malformed information - Secrets will be removed")
			r.UpdateStatus(ctx, req, lrpdb)
			if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
				fmt.Printf("TRCUSR: Wrong secret format\n")
			}
			err = r.DropUserSecret(ctx, req, lrpdb, usersec)
			if err != nil {
				log.Info("Failed to delete app user creation secret: ")
				return err
			}
			return errors.New("Create user: Wrong secret format [1]")
		}
	}

	for cnt := 0; cnt <= (arrsz/3)-1; cnt++ {

		//b39595046
		//b39595519
		uind, _ := strconv.Atoi(credential[cnt+2*((arrsz)/3)][3:])
		pind, _ := strconv.Atoi(credential[cnt+((arrsz)/3)][3:])
		gind, _ := strconv.Atoi(credential[cnt][3:])

		if reu.MatchString(credential[cnt+2*((arrsz)/3)]) == false ||
			rep.MatchString(credential[cnt+((arrsz)/3)]) == false ||
			rgr.MatchString(credential[cnt]) == false ||
			(uind != pind) || (pind != gind) {
			r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "Crt usr malformed secret[2]", Secname)
			lrpdb.Status.Msg = "create user:[malformed secrets:2]"
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, APPERR)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
			log.Info("Malformed information - Secrets will be removed")
			r.UpdateStatus(ctx, req, lrpdb)
			if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
				fmt.Printf("TRCUSR: Wrong secret format\n")
			}
			err = r.DropUserSecret(ctx, req, lrpdb, usersec)
			if err != nil {
				log.Info("Failed to delete app user creation secret: ")
				return err
			}
			return errors.New("Create user: Wrong secret format [2]")

		}

	}
	var User string
	var Pass string
	var Grnt string
	oneError := false
	for cnt := 0; cnt <= (arrsz/3)-1; cnt++ {
		log.Info("begin:user creation ")
		if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
			// Variables schema:
			// ~~~~~~~~~~~~~~~
			// usernane: usersec.Data[credential[cnt+2*((arrsz)/3)]]
			// password: usersec.Data[credential[cnt+((arrsz)/3)]]
			// grants  : usersec.Data[credential[cnt]]

			fmt.Printf("User creation\n")
			sqlCreateUser := "create user " + string(usersec.Data[credential[cnt+2*((arrsz)/3)]]) + " identified by........."
			sqlUserPrvileges := "grant " + string(usersec.Data[credential[cnt]]) + " to " + string(usersec.Data[credential[cnt+2*((arrsz)/3)]])
			fmt.Printf("TRCUSR: SQL=%s\n", sqlCreateUser)
			fmt.Printf("TRCUSR: SQL=%s\n", sqlUserPrvileges)
		}
		User = string(usersec.Data[credential[cnt+2*((arrsz)/3)]])
		Pass = string(usersec.Data[credential[cnt+((arrsz)/3)]])
		Grnt = string(usersec.Data[credential[cnt]])

		values := map[string]string{
			"method":   "CREATEUSER",
			"appUsr":   User,
			"appPwd":   Pass,
			"appGrant": Grnt}

		url := r.BaseURL2(req, lrpdb) + lrpdb.Spec.LRPDBName

		respData, err := NewCallAPISQL(ctx, r, req, lrpdb, url, values, "POST")
		if err != nil {
			log.Error(err, "Failure NewCallAPISQL( "+url+")", "err", err.Error())
			return err
		}

		//r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl)
		if err := r.GetSQLCode(respData, &(lrpdb.Status.SqlCode), lrpdb.Spec.Trclvl); err != nil {
			log.Info("GetSQLCode Warning")
		}

		//globalsqlcode = lrpdb.Status.SqlCode
		if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
			fmt.Printf("TRCUSR: url=%s\n", url)
			fmt.Printf("TRCUSR: sqlcode=%s\n", strconv.Itoa(lrpdb.Status.SqlCode))
		}
		if lrpdb.Status.SqlCode != 0 {
			log.Info("SQL error during user creation [ORA-" + strconv.Itoa(lrpdb.Status.SqlCode) + "]")
			r.Recorder.Eventf(lrpdb, corev1.EventTypeWarning, "Create user ORA-", strconv.Itoa(lrpdb.Status.SqlCode))
			lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, APPERR)
			lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
			lrpdb.Status.Msg = "create users:[USR:" + User + " ora-" + strconv.Itoa(lrpdb.Status.SqlCode) + "]"
			oneError = true
			r.UpdateStatus(ctx, req, lrpdb)
		}

	}

	log.Info("Deleting secrets")
	err = r.Delete(context.TODO(), usersec)
	if err != nil {
		log.Info("Failed to delete app user creation secret: ")
		return err
	}

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Secret deleted\n")
	}

	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Restting lrpdb.Spec.Pdbappuser")
	}

	lrpdb.Status.PDBBitMask = Bis(lrpdb.Status.PDBBitMask, APPUSR)
	lrpdb.Status.PDBBitMaskStr = Bitmaskprint(lrpdb.Status.PDBBitMask)
	if oneError == false {
		lrpdb.Status.Msg = "create users:[op. completed]"
	}
	if Bit(lrpdb.Spec.Trclvl, TRCUSR) == true {
		fmt.Printf("TRCUSR: Bitmask:[%s]\n", lrpdb.Status.PDBBitMaskStr)
	}
	r.UpdateStatus(ctx, req, lrpdb)
	return nil
}

/* RetriveTnsAlias: this function retrives the tnsalias w/o reading lrest objects */
func (r *LRPDBReconciler) RetriveTnsAlias(ctx context.Context, req ctrl.Request, lrpdb *dbapi.LRPDB) error {
	log := r.Log.WithValues("CreateApplicanUsers", req.NamespacedName)
	tracelevel := lrpdb.Spec.Trclvl

	url := "https://" + lrpdb.Spec.CDBResName + "-lrest." + lrpdb.Spec.CDBNamespace + ":" + strconv.Itoa(8888) + "/database/lrest/ConnectString/"
	if Bit(tracelevel, TRCTNS) == true {
		fmt.Printf("TRCTNS: RetriveTnsAlias(url)..:[%s]\n", url)
	}
	ConnectioInfo, err := NewCallAPISQL(ctx, r, req, lrpdb, url, nil, "GET")
	if err != nil {
		log.Info(" NewcallAPISQL err : cannot get tns string information from rest server")
		return err
	}

	if Bit(tracelevel, TRCTNS) == true {
		fmt.Printf("TRCTNS: connection info.......:[%s]\n", ConnectioInfo)
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(ConnectioInfo), &objmap); err != nil {
		log.Error(err, "Cannot Unamarshal tnsstring connection info")
		log.Info("Cannot unmarshal connect string information from lrest server")
		lrpdb.Status.ConnString = "[get tnsstring failure]"
		if Bit(tracelevel, TRCTNS) == true {
			fmt.Printf("TRCTNS: Unmarshal failure.......[%s]\n", err.Error())
		}
		return err
	} else {
		lrpdb.Status.ConnString = objmap["tnsstring"].(string)
	}

	parseTnsAlias(&(lrpdb.Status.ConnString), &(lrpdb.Spec.LRPDBName), tracelevel)

	/* Parse the tnsstring to replace  */
	return nil
}

func (r *LRPDBReconciler) fetchCfgm2(kubeClient client.Client, namespace string, name string) (*corev1.ConfigMap, error) {
	log := r.Log.WithValues("FetchCfgm2", namespace)
	configMap := &corev1.ConfigMap{}

	log.Info("Fetching config map")
	fmt.Printf("ConfigMap:%s Namespace:%s\n", name, namespace)

	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := kubeClient.Get(context.TODO(), namespacedName, configMap); err != nil {
		log.Info("Cannot  retrieve config Map")
		return nil, err
	}

	return configMap, nil
}
