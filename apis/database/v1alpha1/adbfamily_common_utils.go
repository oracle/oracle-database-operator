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

package v1alpha1

import (
	"errors"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/workrequests"
)

// LastSuccessfulSpec is an annotation key which maps to the value of last successful spec
const LastSuccessfulSpec string = "lastSuccessfulSpec"

// File the meta condition and return the meta view
func CreateMetaCondition(obj client.Object, err error, lifecycleState string, stateMsg string) metav1.Condition {

	return metav1.Condition{
		Type:               lifecycleState,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: obj.GetGeneration(),
		Reason:             stateMsg,
		Message:            err.Error(),
		Status:             metav1.ConditionTrue,
	}
}

/************************
*	OCI config
************************/
type OCIConfigSpec struct {
	ConfigMapName *string `json:"configMapName,omitempty"`
	SecretName    *string `json:"secretName,omitempty"`
}

/************************
*	ADB spec
************************/
type K8sADBSpec struct {
	Name *string `json:"name,omitempty"`
}

type OCIADBSpec struct {
	OCID *string `json:"ocid,omitempty"`
}

// TargetSpec defines the spec of the target for backup/restore runs.
type TargetSpec struct {
	K8sADB K8sADBSpec `json:"k8sADB,omitempty"`
	OCIADB OCIADBSpec `json:"ociADB,omitempty"`
}

/**************************
*	Remove Unchanged Fields
**************************/

// removeUnchangedFields removes the unchanged fields in the struct and returns if the struct is changed.
// lastSpec should be a derefereced struct that is the last successful spec, e.g. AutonomousDatabaseSpec.
// curSpec should be a pointer pointing to the struct that is being proccessed, e.g., *AutonomousDatabaseSpec.
func removeUnchangedFields(lastSpec interface{}, curSpec interface{}) (bool, error) {
	if reflect.ValueOf(lastSpec).Kind() != reflect.Struct {
		return false, errors.New("lastSpec should be a struct")
	}

	if reflect.ValueOf(curSpec).Kind() != reflect.Ptr || reflect.ValueOf(curSpec).Elem().Kind() != reflect.Struct {
		return false, errors.New("curSpec should be a struct pointer")
	}

	if reflect.ValueOf(lastSpec).Type() != reflect.ValueOf(curSpec).Elem().Type() {
		return false, errors.New("the referenced type of curSpec should be the same as the type of lastSpec")
	}

	return traverse(lastSpec, curSpec), nil
}

// Traverse and compare each fields in the lastSpec and the the curSpec.
// If unchanged, set the field in curSpec to a zero value.
// lastSpec should be a derefereced struct that is the last successful spec, e.g. AutonomousDatabaseSpec.
// curSpec should be a pointer pointing to the struct that is being proccessed, e.g., *AutonomousDatabaseSpec.
func traverse(lastSpec interface{}, curSpec interface{}) bool {
	var changed bool = false

	fields := reflect.VisibleFields(reflect.TypeOf(lastSpec))

	lastSpecValue := reflect.ValueOf(lastSpec)
	curSpecValue := reflect.ValueOf(curSpec).Elem() // deref the struct

	for _, field := range fields {
		lastField := lastSpecValue.FieldByName(field.Name)
		curField := curSpecValue.FieldByName(field.Name)

		// call traverse() if the current field is a struct
		if field.Type.Kind() == reflect.Struct {
			childrenChanged := traverse(lastField.Interface(), curField.Addr().Interface())
			if childrenChanged && !changed {
				changed = true
			}
		} else {
			fieldChanged := hasChanged(lastField, curField)

			// if fieldChanged {
			// 	if curField.Kind() == reflect.Ptr {
			// 		fmt.Printf("== field %s changed\n", field.Name)
			// 		if lastField.IsZero() {
			// 			fmt.Printf("=== lastField is nil\n")
			// 		} else {
			// 			fmt.Printf("=== lastField = %v\n", lastField.Elem().Interface())
			// 		}
			// 		if curField.IsZero() {
			// 			fmt.Printf("===== curField is nil\n")
			// 		} else {
			// 			fmt.Printf("===== curField = %v\n", curField.Elem().Interface())
			// 		}
			// 	} else {
			// 		fmt.Printf("=== lastField = %v\n", lastField.Interface())
			// 		fmt.Printf("===== curField = %v\n", curField.Interface())
			// 	}
			// }

			if fieldChanged && !changed {
				changed = true
			}

			// Set the field to zero value if unchanged
			if !fieldChanged {
				curField.Set(reflect.Zero(curField.Type()))
			}
		}
	}

	return changed
}

//  1. If the current field is with a zero value, then the field is unchanged.
//  2. If the current field is NOT with a zero value, then we want to comapre it with the last field.
//     In this case if the last field is with a zero value, then the field is changed
func hasChanged(lastField reflect.Value, curField reflect.Value) bool {
	zero := reflect.Zero(lastField.Type()).Interface()
	lastFieldIsZero := reflect.DeepEqual(lastField.Interface(), zero)
	curFieldIsZero := reflect.DeepEqual(curField.Interface(), zero)

	if curFieldIsZero {
		return false
	} else if !lastFieldIsZero {
		var lastIntrf interface{}
		var curIntrf interface{}

		if curField.Kind() == reflect.Ptr {
			lastIntrf = lastField.Elem().Interface()
			curIntrf = curField.Elem().Interface()
		} else {
			lastIntrf = lastField.Interface()
			curIntrf = curField.Interface()
		}

		return !reflect.DeepEqual(lastIntrf, curIntrf)
	}

	return true
}

/************************
*	SDKTime format
************************/

// Follow the format of the display time
const displayFormat = "2006-01-02 15:04:05 MST"

func FormatSDKTime(sdkTime *common.SDKTime) string {
	if sdkTime == nil {
		return ""
	}

	time := sdkTime.Time
	return time.Format(displayFormat)
}

func parseDisplayTime(val string) (*common.SDKTime, error) {
	parsedTime, err := time.Parse(displayFormat, val)
	if err != nil {
		return nil, err
	}
	sdkTime := common.SDKTime{Time: parsedTime}
	return &sdkTime, nil
}

/************************
*	LifecycleState check
************************/
func IsADBIntermediateState(state database.AutonomousDatabaseLifecycleStateEnum) bool {
	if state == database.AutonomousDatabaseLifecycleStateProvisioning ||
		state == database.AutonomousDatabaseLifecycleStateUpdating ||
		state == database.AutonomousDatabaseLifecycleStateScaleInProgress ||
		state == database.AutonomousDatabaseLifecycleStateStarting ||
		state == database.AutonomousDatabaseLifecycleStateStopping ||
		state == database.AutonomousDatabaseLifecycleStateTerminating ||
		state == database.AutonomousDatabaseLifecycleStateRestoreInProgress ||
		state == database.AutonomousDatabaseLifecycleStateBackupInProgress ||
		state == database.AutonomousDatabaseLifecycleStateMaintenanceInProgress ||
		state == database.AutonomousDatabaseLifecycleStateRestarting ||
		state == database.AutonomousDatabaseLifecycleStateRecreating ||
		state == database.AutonomousDatabaseLifecycleStateRoleChangeInProgress ||
		state == database.AutonomousDatabaseLifecycleStateUpgrading {
		return true
	}
	return false
}

func ValidADBTerminateState(state database.AutonomousDatabaseLifecycleStateEnum) bool {
	if state == database.AutonomousDatabaseLifecycleStateProvisioning ||
		state == database.AutonomousDatabaseLifecycleStateAvailable ||
		state == database.AutonomousDatabaseLifecycleStateStopped ||
		state == database.AutonomousDatabaseLifecycleStateUnavailable ||
		state == database.AutonomousDatabaseLifecycleStateRestoreInProgress ||
		state == database.AutonomousDatabaseLifecycleStateRestoreFailed ||
		state == database.AutonomousDatabaseLifecycleStateBackupInProgress ||
		state == database.AutonomousDatabaseLifecycleStateScaleInProgress ||
		state == database.AutonomousDatabaseLifecycleStateAvailableNeedsAttention ||
		state == database.AutonomousDatabaseLifecycleStateUpdating ||
		state == database.AutonomousDatabaseLifecycleStateMaintenanceInProgress ||
		state == database.AutonomousDatabaseLifecycleStateRoleChangeInProgress ||
		state == database.AutonomousDatabaseLifecycleStateUpgrading {
		return true
	}
	return false
}

// NextADBStableState returns the next stable state if it's an intermediate state.
// Otherwise returns the same state.
func NextADBStableState(state database.AutonomousDatabaseLifecycleStateEnum) database.AutonomousDatabaseLifecycleStateEnum {
	if state == database.AutonomousDatabaseLifecycleStateProvisioning ||
		state == database.AutonomousDatabaseLifecycleStateStarting ||
		state == database.AutonomousDatabaseLifecycleStateRestoreInProgress ||
		state == database.AutonomousDatabaseLifecycleStateBackupInProgress ||
		state == database.AutonomousDatabaseLifecycleStateScaleInProgress ||
		state == database.AutonomousDatabaseLifecycleStateUpdating ||
		state == database.AutonomousDatabaseLifecycleStateMaintenanceInProgress ||
		state == database.AutonomousDatabaseLifecycleStateRestarting ||
		state == database.AutonomousDatabaseLifecycleStateRecreating ||
		state == database.AutonomousDatabaseLifecycleStateRoleChangeInProgress ||
		state == database.AutonomousDatabaseLifecycleStateUpgrading {

		return database.AutonomousDatabaseLifecycleStateAvailable
	}

	if state == database.AutonomousDatabaseLifecycleStateStopping {
		return database.AutonomousDatabaseLifecycleStateStopped
	}

	if state == database.AutonomousDatabaseLifecycleStateTerminating {
		return database.AutonomousDatabaseLifecycleStateTerminated
	}

	return state
}

func IsBackupIntermediateState(state database.AutonomousDatabaseBackupLifecycleStateEnum) bool {
	if state == database.AutonomousDatabaseBackupLifecycleStateCreating ||
		state == database.AutonomousDatabaseBackupLifecycleStateDeleting {
		return true
	}
	return false
}

func IsRestoreIntermediateState(state workrequests.WorkRequestStatusEnum) bool {
	if state == workrequests.WorkRequestStatusAccepted ||
		state == workrequests.WorkRequestStatusInProgress ||
		state == workrequests.WorkRequestStatusCanceling {
		return true
	}
	return false
}

func IsACDIntermediateState(state database.AutonomousContainerDatabaseLifecycleStateEnum) bool {
	if state == database.AutonomousContainerDatabaseLifecycleStateProvisioning ||
		state == database.AutonomousContainerDatabaseLifecycleStateUpdating ||
		state == database.AutonomousContainerDatabaseLifecycleStateTerminating ||
		state == database.AutonomousContainerDatabaseLifecycleStateBackupInProgress ||
		state == database.AutonomousContainerDatabaseLifecycleStateRestoring ||
		state == database.AutonomousContainerDatabaseLifecycleStateRestarting ||
		state == database.AutonomousContainerDatabaseLifecycleStateMaintenanceInProgress ||
		state == database.AutonomousContainerDatabaseLifecycleStateRoleChangeInProgress {
		return true
	}
	return false
}
