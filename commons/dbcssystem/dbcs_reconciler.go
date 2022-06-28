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

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v64/common"
	"github.com/oracle/oci-go-sdk/v64/core"
	"github.com/oracle/oci-go-sdk/v64/database"
	"github.com/oracle/oci-go-sdk/v64/workrequests"

	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/annotations"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func CreateAndGetDbcsId(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) (string, error) {

	//var provisionedDbcsSystemId string
	ctx := context.TODO()
	// Get DB System Details
	dbcsDetails := database.LaunchDbSystemDetails{}
	// Get the admin password from OCI key
	sshPublicKeys, err := getPublicSSHKey(kubeClient, dbcs)
	if err != nil {
		return "", err
	}
	// Get Db SystemOption
	dbSystemReq := GetDBSystemopts(dbcs)
	licenceModel := getLicenceModel(dbcs)

	if dbcs.Spec.DbSystem.ClusterName != "" {
		dbcsDetails.ClusterName = &dbcs.Spec.DbSystem.ClusterName
	}

	if dbcs.Spec.DbSystem.TimeZone != "" {
		dbcsDetails.TimeZone = &dbcs.Spec.DbSystem.TimeZone
	}
	// Get DB Home Details
	dbHomeReq, err := GetDbHomeDetails(kubeClient, dbClient, dbcs)
	if err != nil {
		return "", err
	}
	//tenancyOcid, _ := provider.TenancyOCID()
	dbcsDetails.AvailabilityDomain = common.String(dbcs.Spec.DbSystem.AvailabilityDomain)
	dbcsDetails.CompartmentId = common.String(dbcs.Spec.DbSystem.CompartmentId)
	dbcsDetails.SubnetId = common.String(dbcs.Spec.DbSystem.SubnetId)
	dbcsDetails.Shape = common.String(dbcs.Spec.DbSystem.Shape)
	if dbcs.Spec.DbSystem.DisplayName != "" {
		dbcsDetails.DisplayName = common.String(dbcs.Spec.DbSystem.DisplayName)
	}
	dbcsDetails.SshPublicKeys = []string{sshPublicKeys}
	dbcsDetails.Hostname = common.String(dbcs.Spec.DbSystem.HostName)
	dbcsDetails.CpuCoreCount = common.Int(dbcs.Spec.DbSystem.CpuCoreCount)
	//dbcsDetails.SourceDbSystemId = common.String(r.tenancyOcid)
	dbcsDetails.NodeCount = common.Int(GetNodeCount(dbcs))
	dbcsDetails.InitialDataStorageSizeInGB = common.Int(GetInitialStorage(dbcs))
	dbcsDetails.DbSystemOptions = &dbSystemReq
	dbcsDetails.DbHome = &dbHomeReq
	dbcsDetails.DatabaseEdition = GetDBEdition(dbcs)
	dbcsDetails.DiskRedundancy = GetDBbDiskRedundancy(dbcs)
	dbcsDetails.LicenseModel = database.LaunchDbSystemDetailsLicenseModelEnum(licenceModel)
	if len(dbcs.Spec.DbSystem.Tags) != 0 {
		dbcsDetails.FreeformTags = dbcs.Spec.DbSystem.Tags
	}

	req := database.LaunchDbSystemRequest{LaunchDbSystemDetails: dbcsDetails}

	// Send the request using the service client
	resp, err := dbClient.LaunchDbSystem(ctx, req)
	if err != nil {
		return " ", err
	}

	dbcs.Spec.Id = resp.DbSystem.Id
	// Change the phase to "Provisioning"
	if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev1alpha1.Provision, nwClient, wrClient); statusErr != nil {
		return "", statusErr
	}
	// Check the State
	_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev1alpha1.Provision), string(databasev1alpha1.Available))
	if err != nil {
		return "", err
	}

	return *resp.DbSystem.Id, nil
}

// Sync the DbcsSystem Database details

// Get admin password from Secret then OCI valut secret
func GetAdminPassword(kubeClient client.Client, dbcs *databasev1alpha1.DbcsSystem) (string, error) {
	if dbcs.Spec.DbSystem.DbAdminPaswordSecret != "" {
		// Get the Admin Secret
		adminSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbSystem.DbAdminPaswordSecret,
		}, adminSecret)

		if err != nil {
			return "", err
		}

		// Get the admin password
		key := "admin-password"
		if val, ok := adminSecret.Data[key]; ok {
			return string(val), nil
		} else {
			msg := "secret item not found: admin-password"
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Get admin password from Secret then OCI valut secret
func GetTdePassword(kubeClient client.Client, dbcs *databasev1alpha1.DbcsSystem) (string, error) {
	if dbcs.Spec.DbSystem.TdeWalletPasswordSecret != "" {
		// Get the Admin Secret
		tdeSecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbSystem.TdeWalletPasswordSecret,
		}, tdeSecret)

		if err != nil {
			return "", err
		}

		// Get the admin password
		key := "tde-password"
		if val, ok := tdeSecret.Data[key]; ok {
			return string(val), nil
		} else {
			msg := "secret item not found: tde-password"
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Get admin password from Secret then OCI valut secret
func getPublicSSHKey(kubeClient client.Client, dbcs *databasev1alpha1.DbcsSystem) (string, error) {
	if dbcs.Spec.DbSystem.SshPublicKeys[0] != "" {
		// Get the Admin Secret
		sshkeysecret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: dbcs.GetNamespace(),
			Name:      dbcs.Spec.DbSystem.SshPublicKeys[0],
		}, sshkeysecret)

		if err != nil {
			return "", err
		}

		// Get the admin password`
		key := "publickey"
		if val, ok := sshkeysecret.Data[key]; ok {
			return string(val), nil
		} else {
			msg := "secret item not found: "
			return "", errors.New(msg)
		}
	}
	return "", errors.New("should provide either a Secret name or a Valut Secret ID")
}

// Delete DbcsSystem System
func DeleteDbcsSystemSystem(dbClient database.DatabaseClient, Id string) error {

	dbcsId := Id

	dbcsReq := database.TerminateDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	_, err := dbClient.TerminateDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}

	return nil
}

// SetLifecycleState set status.state of the reosurce.
func SetLifecycleState(kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, state databasev1alpha1.LifecycleState, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		dbcs.Status.State = state
		// Set the status
		if statusErr := SetDBCSStatus(dbClient, dbcs, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		if err := kubeClient.Status().Update(context.TODO(), dbcs); err != nil {
			return err
		}

		return nil
	})
}

// SetDBCSSystem LifeCycle state when state is provisioning

func SetDBCSDatabaseLifecycleState(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {

	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	resp, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}

	// Return if the desired lifecycle state is the same as the current lifecycle state
	if string(dbcs.Status.State) == string(resp.LifecycleState) {
		return nil
	} else if string(resp.LifecycleState) == string(databasev1alpha1.Available) {
		// Change the phase to "Available"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev1alpha1.Available, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
	} else if string(resp.LifecycleState) == string(databasev1alpha1.Provision) {
		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev1alpha1.Provision, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev1alpha1.Provision), string(databasev1alpha1.Available))
		if err != nil {
			return err
		}
	} else if string(resp.LifecycleState) == string(databasev1alpha1.Update) {
		// Change the phase to "Updating"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev1alpha1.Update, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(logger, dbClient, *resp.DbSystem.Id, string(databasev1alpha1.Update), string(databasev1alpha1.Available))
		if err != nil {
			return err
		}
	} else if string(resp.LifecycleState) == string(databasev1alpha1.Failed) {
		// Change the phase to "Updating"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev1alpha1.Failed, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		return fmt.Errorf("DbSystem is in Failed State")
	} else if string(resp.LifecycleState) == string(databasev1alpha1.Terminated) {
		// Change the phase to "Terminated"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev1alpha1.Terminate, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
	}
	return nil
}

func GetDbSystemId(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem) error {
	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	response, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}

	dbcs.Spec.DbSystem.CompartmentId = *response.CompartmentId
	if response.DisplayName != nil {
		dbcs.Spec.DbSystem.DisplayName = *response.DisplayName
	}

	if response.Hostname != nil {
		dbcs.Spec.DbSystem.HostName = *response.Hostname
	}
	if response.CpuCoreCount != nil {
		dbcs.Spec.DbSystem.CpuCoreCount = *response.CpuCoreCount
	}
	dbcs.Spec.DbSystem.NodeCount = response.NodeCount
	if response.ClusterName != nil {
		dbcs.Spec.DbSystem.ClusterName = *response.ClusterName
	}
	//dbcs.Spec.DbSystem.DbUniqueName = *response.DbUniqueName
	if string(response.DbSystem.DatabaseEdition) != "" {
		dbcs.Spec.DbSystem.DbEdition = string(response.DatabaseEdition)
	}
	if string(response.DiskRedundancy) != "" {
		dbcs.Spec.DbSystem.DiskRedundancy = string(response.DiskRedundancy)
	}

	//dbcs.Spec.DbSystem.DbVersion = *response.

	if response.BackupSubnetId != nil {
		dbcs.Spec.DbSystem.BackupSubnetId = *response.BackupSubnetId
	}
	dbcs.Spec.DbSystem.Shape = *response.Shape
	dbcs.Spec.DbSystem.SshPublicKeys = []string(response.SshPublicKeys)
	if response.FaultDomains != nil {
		dbcs.Spec.DbSystem.FaultDomains = []string(response.FaultDomains)
	}
	dbcs.Spec.DbSystem.SubnetId = *response.SubnetId
	dbcs.Spec.DbSystem.AvailabilityDomain = *response.AvailabilityDomain

	err = PopulateDBDetails(logger, dbClient, dbcs)
	if err != nil {
		logger.Info("Error Occurred while collecting the DB details")
		return err
	}
	return nil
}

func PopulateDBDetails(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem) error {

	listDbHomeRsp, err := GetListDbHomeRsp(logger, dbClient, dbcs)
	if err != nil {
		logger.Info("Error Occurred while getting List of DBHomes")
		return err
	}
	dbHomeId := listDbHomeRsp.Items[0].Id
	listDBRsp, err := GetListDatabaseRsp(logger, dbClient, dbcs, *dbHomeId)
	if err != nil {
		logger.Info("Error Occurred while getting List of Databases")
		return err
	}

	dbcs.Spec.DbSystem.DbName = *listDBRsp.Items[0].DbName
	dbcs.Spec.DbSystem.DbUniqueName = *listDBRsp.Items[0].DbUniqueName
	dbcs.Spec.DbSystem.DbVersion = *listDbHomeRsp.Items[0].DbVersion

	return nil
}

func GetListDbHomeRsp(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem) (database.ListDbHomesResponse, error) {

	dbcsId := *dbcs.Spec.Id
	CompartmentId := dbcs.Spec.DbSystem.CompartmentId

	dbHomeReq := database.ListDbHomesRequest{
		DbSystemId:    &dbcsId,
		CompartmentId: &CompartmentId,
	}

	response, err := dbClient.ListDbHomes(context.TODO(), dbHomeReq)
	if err != nil {
		return database.ListDbHomesResponse{}, err
	}

	return response, nil
}

func GetListDatabaseRsp(logger logr.Logger, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, dbHomeId string) (database.ListDatabasesResponse, error) {

	CompartmentId := dbcs.Spec.DbSystem.CompartmentId

	dbReq := database.ListDatabasesRequest{
		DbHomeId:      &dbHomeId,
		CompartmentId: &CompartmentId,
	}

	response, err := dbClient.ListDatabases(context.TODO(), dbReq)
	if err != nil {
		return database.ListDatabasesResponse{}, err
	}

	return response, nil
}

func UpdateDbcsSystemIdInst(log logr.Logger, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, kubeClient client.Client, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {
	//logger := log.WithName("UpdateDbcsSystemInstance")

	updateFlag := false
	updateDbcsDetails := database.UpdateDbSystemDetails{}
	oldSpec, err := dbcs.GetLastSuccessfulSpec()
	if err != nil {
		return err
	}

	if dbcs.Spec.DbSystem.CpuCoreCount > 0 && dbcs.Spec.DbSystem.CpuCoreCount != oldSpec.DbSystem.CpuCoreCount {
		updateDbcsDetails.CpuCoreCount = common.Int(dbcs.Spec.DbSystem.CpuCoreCount)
		updateFlag = true
	}
	if dbcs.Spec.DbSystem.Shape != "" && dbcs.Spec.DbSystem.Shape != oldSpec.DbSystem.Shape {
		updateDbcsDetails.Shape = common.String(dbcs.Spec.DbSystem.Shape)
		updateFlag = true
	}

	if dbcs.Spec.DbSystem.LicenseModel != "" && dbcs.Spec.DbSystem.LicenseModel != oldSpec.DbSystem.LicenseModel {
		licenceModel := getLicenceModel(dbcs)
		updateDbcsDetails.LicenseModel = database.UpdateDbSystemDetailsLicenseModelEnum(licenceModel)
		updateFlag = true
	}

	if dbcs.Spec.DbSystem.InitialDataStorageSizeInGB != 0 && dbcs.Spec.DbSystem.InitialDataStorageSizeInGB != oldSpec.DbSystem.InitialDataStorageSizeInGB {
		updateDbcsDetails.DataStorageSizeInGBs = &dbcs.Spec.DbSystem.InitialDataStorageSizeInGB
		updateFlag = true
	}

	if updateFlag {
		updateDbcsRequest := database.UpdateDbSystemRequest{
			DbSystemId:            common.String(*dbcs.Spec.Id),
			UpdateDbSystemDetails: updateDbcsDetails,
		}

		if _, err := dbClient.UpdateDbSystem(context.TODO(), updateDbcsRequest); err != nil {
			return err
		}

		// Change the phase to "Provisioning"
		if statusErr := SetLifecycleState(kubeClient, dbClient, dbcs, databasev1alpha1.Update, nwClient, wrClient); statusErr != nil {
			return statusErr
		}
		// Check the State
		_, err = CheckResourceState(log, dbClient, *dbcs.Spec.Id, "UPDATING", "AVAILABLE")
		if err != nil {
			return err
		}

	}

	return nil
}

func UpdateDbcsSystemId(kubeClient client.Client, dbcs *databasev1alpha1.DbcsSystem) error {
	payload := []annotations.PatchValue{{
		Op:    "replace",
		Path:  "/spec/details",
		Value: dbcs.Spec,
	}}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	patch := client.RawPatch(types.JSONPatchType, payloadBytes)
	return kubeClient.Patch(context.TODO(), dbcs, patch)
}

func CheckResourceState(logger logr.Logger, dbClient database.DatabaseClient, Id string, currentState string, expectedState string) (string, error) {
	// The database OCID is not available when the provisioning is onging.
	// Retry until the new DbcsSystem is ready.
	// Retry up to 18 times every 10 seconds.

	var state string
	var err error
	for {
		state, err = GetResourceState(logger, dbClient, Id)
		if err != nil {
			logger.Info("Error occurred while collecting the resource life cycle state")
			return "", err
		}
		if string(state) == expectedState {
			break
		} else if string(state) == currentState {
			logger.Info("DB System current state is still:" + string(state) + ". Sleeping for 60 seconds.")
			time.Sleep(60 * time.Second)
			continue
		} else {
			msg := "DB System current state " + string(state) + " is not matching " + expectedState
			logger.Info(msg)
			return "", errors.New(msg)
		}
	}

	return "", nil
}

func GetResourceState(logger logr.Logger, dbClient database.DatabaseClient, Id string) (string, error) {

	dbcsId := Id
	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	response, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return "", err
	}

	state := string(response.LifecycleState)

	return state, nil
}

func SetDBCSStatus(dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, nwClient core.VirtualNetworkClient, wrClient workrequests.WorkRequestClient) error {

	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.GetDbSystemRequest{
		DbSystemId: &dbcsId,
	}

	resp, err := dbClient.GetDbSystem(context.TODO(), dbcsReq)
	if err != nil {
		return err
	}

	dbcs.Status.AvailabilityDomain = *resp.AvailabilityDomain
	dbcs.Status.CpuCoreCount = *resp.CpuCoreCount
	dbcs.Status.DataStoragePercentage = resp.DataStoragePercentage
	dbcs.Status.DataStorageSizeInGBs = resp.DataStorageSizeInGBs
	dbcs.Status.DbEdition = string(resp.DatabaseEdition)
	dbcs.Status.DisplayName = *resp.DisplayName
	dbcs.Status.LicenseModel = string(resp.LicenseModel)
	dbcs.Status.RecoStorageSizeInGB = resp.RecoStorageSizeInGB
	dbcs.Status.NodeCount = *resp.NodeCount
	dbcs.Status.StorageManagement = string(resp.DbSystemOptions.StorageManagement)
	dbcs.Status.Shape = resp.Shape
	dbcs.Status.Id = resp.Id
	dbcs.Status.SubnetId = *resp.SubnetId
	dbcs.Status.TimeZone = *resp.TimeZone
	dbcs.Status.LicenseModel = string(resp.LicenseModel)
	dbcs.Status.Network.ScanDnsName = resp.ScanDnsName
	dbcs.Status.Network.ListenerPort = resp.ListenerPort
	dbcs.Status.Network.HostName = *resp.Hostname
	dbcs.Status.Network.DomainName = *resp.Domain

	sname, vcnId, err := getSubnetName(*resp.SubnetId, nwClient)

	if err == nil {
		dbcs.Status.Network.SubnetName = sname
		vcnName, err := getVcnName(vcnId, nwClient)

		if err == nil {
			dbcs.Status.Network.VcnName = vcnName
		}

	}

	// Work Request Ststaus
	dbWorkRequest := databasev1alpha1.DbWorkrequests{}

	dbWorks, err := getWorkRequest(*resp.OpcRequestId, wrClient, dbcs)
	if err == nil {
		for _, dbWork := range dbWorks {
			//status := checkValue(dbcs, dbWork.Id)
			//	if status != 0 {
			dbWorkRequest.OperationId = dbWork.Id
			dbWorkRequest.OperationType = dbWork.OperationType
			dbWorkRequest.PercentComplete = fmt.Sprint(*dbWork.PercentComplete) //strconv.FormatFloat(dbWork.PercentComplete, 'E', -1, 32)
			if dbWork.TimeAccepted != nil {
				dbWorkRequest.TimeAccepted = dbWork.TimeAccepted.String()
			}
			if dbWork.TimeFinished != nil {
				dbWorkRequest.TimeFinished = dbWork.TimeFinished.String()
			}
			if dbWork.TimeStarted != nil {
				dbWorkRequest.TimeStarted = dbWork.TimeStarted.String()
			}

			if dbWorkRequest != (databasev1alpha1.DbWorkrequests{}) {
				status := checkValue(dbcs, dbWork.Id)
				if status == 0 {
					dbcs.Status.WorkRequests = append(dbcs.Status.WorkRequests, dbWorkRequest)
					dbWorkRequest = databasev1alpha1.DbWorkrequests{}
				} else {
					setValue(dbcs, dbWorkRequest)
				}
			}
			//}
		}
	}

	// DB Home Status
	dbcs.Status.DbInfo = dbcs.Status.DbInfo[:0]
	dbStatus := databasev1alpha1.DbStatus{}

	dbHomes, err := getDbHomeList(dbClient, dbcs)

	if err == nil {
		for _, dbHome := range dbHomes {
			dbDetails, err := getDList(dbClient, dbcs, dbHome.Id)
			for _, dbDetail := range dbDetails {
				if err == nil {
					dbStatus.Id = dbDetail.Id
					dbStatus.DbHomeId = *dbDetail.DbHomeId
					dbStatus.DbName = *dbDetail.DbName
					dbStatus.DbUniqueName = *dbDetail.DbUniqueName
					dbStatus.DbWorkload = *dbDetail.DbWorkload
				}
				dbcs.Status.DbInfo = append(dbcs.Status.DbInfo, dbStatus)
				dbStatus = databasev1alpha1.DbStatus{}
			}
		}
	}
	return nil
}

func getDbHomeList(dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem) ([]database.DbHomeSummary, error) {

	var items []database.DbHomeSummary
	dbcsId := *dbcs.Spec.Id

	dbcsReq := database.ListDbHomesRequest{
		DbSystemId:    &dbcsId,
		CompartmentId: &dbcs.Spec.DbSystem.CompartmentId,
	}

	resp, err := dbClient.ListDbHomes(context.TODO(), dbcsReq)
	if err != nil {
		return items, err
	}

	return resp.Items, nil
}

func getDList(dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, dbHomeId *string) ([]database.DatabaseSummary, error) {

	dbcsId := *dbcs.Spec.Id
	var items []database.DatabaseSummary
	dbcsReq := database.ListDatabasesRequest{
		SystemId:      &dbcsId,
		CompartmentId: &dbcs.Spec.DbSystem.CompartmentId,
		DbHomeId:      dbHomeId,
	}

	resp, err := dbClient.ListDatabases(context.TODO(), dbcsReq)
	if err != nil {
		return items, err
	}

	return resp.Items, nil
}

func getSubnetName(subnetId string, nwClient core.VirtualNetworkClient) (*string, *string, error) {

	req := core.GetSubnetRequest{SubnetId: common.String(subnetId)}

	// Send the request using the service client
	resp, err := nwClient.GetSubnet(context.Background(), req)

	if err != nil {
		return nil, nil, err
	}
	// Retrieve value from the response.

	return resp.DisplayName, resp.VcnId, nil
}

func getVcnName(vcnId *string, nwClient core.VirtualNetworkClient) (*string, error) {

	req := core.GetVcnRequest{VcnId: common.String(*vcnId)}

	// Send the request using the service client
	resp, err := nwClient.GetVcn(context.Background(), req)

	if err != nil {
		return nil, err
	}
	// Retrieve value from the response.

	return resp.DisplayName, nil
}

// =========== validate Specs ============
func ValidateSpex(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, dbcs *databasev1alpha1.DbcsSystem, nwClient core.VirtualNetworkClient, eRecord record.EventRecorder) error {

	//var str1 string
	var eventMsg string
	var eventErr string = "Spec Error"
	lastSuccSpec, err := dbcs.GetLastSuccessfulSpec()
	if err != nil {
		return err
	}
	// Check if last Successful update nil or not
	if lastSuccSpec == nil {
		if dbcs.Spec.DbSystem.DbVersion != "" {
			_, err = GetDbLatestVersion(dbClient, dbcs, "")
			if err != nil {
				eventMsg = "DBCS CRD resource  " + GetFmtStr(dbcs.Name) + " DbVersion " + GetFmtStr(dbcs.Spec.DbSystem.DbVersion) + " is not matching available DB releases."
				eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
				return err
			}
		} else {
			eventMsg = "DBCS CRD resource  " + "DbVersion  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.DbVersion") + " cannot be a empty string."
			eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
			return err
		}
		if dbcs.Spec.DbSystem.DbWorkload != "" {
			_, err = getDbWorkLoadType(dbcs)
			if err != nil {
				eventMsg = "DBCS CRD resource  " + GetFmtStr(dbcs.Name) + " DbWorkload " + GetFmtStr(dbcs.Spec.DbSystem.DbWorkload) + " is not matching the DBworkload type OLTP|DSS."
				eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
				return err
			}
		} else {
			eventMsg = "DBCS CRD resource  " + "DbWorkload  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.DbWorkload") + " cannot be a empty string."
			eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
			return err
		}

		if dbcs.Spec.DbSystem.NodeCount != nil {
			switch *dbcs.Spec.DbSystem.NodeCount {
			case 1:
			case 2:
			default:
				eventMsg = "DBCS CRD resource  " + "NodeCount  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.NodeCount") + " can be either 1 or 2."
				eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
				return err
			}
		}

	} else {
		if lastSuccSpec.DbSystem.DbVersion != dbcs.Spec.DbSystem.DbVersion {
			eventMsg = "DBCS CRD resource  " + "DbVersion  " + GetFmtStr(dbcs.Name) + GetFmtStr("dbcs.Spec.DbSystem.DbVersion") + " cannot be a empty string."
			eRecord.Eventf(dbcs, corev1.EventTypeWarning, eventErr, eventMsg)
			return err
		}

	}

	return nil

}
