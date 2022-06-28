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

package oci

import (
	"context"

	"github.com/oracle/oci-go-sdk/v64/common"
	"github.com/oracle/oci-go-sdk/v64/database"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

/********************************
 * Autonomous Container Database
 *******************************/
func (d *databaseService) CreateAutonomousContainerDatabase(acd *dbv1alpha1.AutonomousContainerDatabase) (database.CreateAutonomousContainerDatabaseResponse, error) {
	createAutonomousContainerDatabaseRequest := database.CreateAutonomousContainerDatabaseRequest{
		CreateAutonomousContainerDatabaseDetails: database.CreateAutonomousContainerDatabaseDetails{
			CompartmentId:              acd.Spec.CompartmentOCID,
			DisplayName:                acd.Spec.DisplayName,
			CloudAutonomousVmClusterId: acd.Spec.AutonomousExadataVMClusterOCID,
			PatchModel:                 database.CreateAutonomousContainerDatabaseDetailsPatchModelUpdates,
		},
	}

	return d.dbClient.CreateAutonomousContainerDatabase(context.TODO(), createAutonomousContainerDatabaseRequest)
}

func (d *databaseService) GetAutonomousContainerDatabase(acdOCID string) (database.GetAutonomousContainerDatabaseResponse, error) {
	getAutonomousContainerDatabaseRequest := database.GetAutonomousContainerDatabaseRequest{
		AutonomousContainerDatabaseId: common.String(acdOCID),
	}

	return d.dbClient.GetAutonomousContainerDatabase(context.TODO(), getAutonomousContainerDatabaseRequest)
}

func (d *databaseService) UpdateAutonomousContainerDatabase(acdOCID string, difACD *dbv1alpha1.AutonomousContainerDatabase) (database.UpdateAutonomousContainerDatabaseResponse, error) {
	updateAutonomousContainerDatabaseRequest := database.UpdateAutonomousContainerDatabaseRequest{
		AutonomousContainerDatabaseId: common.String(acdOCID),
		UpdateAutonomousContainerDatabaseDetails: database.UpdateAutonomousContainerDatabaseDetails{
			DisplayName:  difACD.Spec.DisplayName,
			PatchModel:   database.UpdateAutonomousContainerDatabaseDetailsPatchModelEnum(difACD.Spec.PatchModel),
			FreeformTags: difACD.Spec.FreeformTags,
		},
	}

	return d.dbClient.UpdateAutonomousContainerDatabase(context.TODO(), updateAutonomousContainerDatabaseRequest)
}

func (d *databaseService) RestartAutonomousContainerDatabase(acdOCID string) (database.RestartAutonomousContainerDatabaseResponse, error) {
	restartRequest := database.RestartAutonomousContainerDatabaseRequest{
		AutonomousContainerDatabaseId: common.String(acdOCID),
	}

	return d.dbClient.RestartAutonomousContainerDatabase(context.TODO(), restartRequest)
}

func (d *databaseService) TerminateAutonomousContainerDatabase(acdOCID string) (database.TerminateAutonomousContainerDatabaseResponse, error) {
	terminateRequest := database.TerminateAutonomousContainerDatabaseRequest{
		AutonomousContainerDatabaseId: common.String(acdOCID),
	}

	return d.dbClient.TerminateAutonomousContainerDatabase(context.TODO(), terminateRequest)
}
