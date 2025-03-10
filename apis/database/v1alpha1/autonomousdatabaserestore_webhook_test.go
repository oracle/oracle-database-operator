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
	"github.com/oracle/oci-go-sdk/v65/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("test AutonomousDatabaseRestore webhook", func() {
	Describe("Test ValidateCreate of the AutonomousDatabaseRestore validating webhook", func() {
		var (
			resourceName = "testadbrestore"
			namespace    = "default"

			restore *AutonomousDatabaseRestore
		)

		BeforeEach(func() {
			restore = &AutonomousDatabaseRestore{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "database.oracle.com/v1alpha1",
					Kind:       "AutonomousDatabaseRestore",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: AutonomousDatabaseRestoreSpec{
					Target: TargetSpec{},
				},
			}
		})

		It("Should specify at least one of the k8sADB and ociADB", func() {
			var errMsg string = "target ADB is empty"

			restore.Spec.Target.K8sAdb.Name = nil
			restore.Spec.Target.OciAdb.Ocid = nil

			validateInvalidTest(restore, false, errMsg)
		})

		It("Should specify either k8sADB.name or ociADB.ocid, but not both", func() {
			var errMsg string = "specify either k8sADB.name or ociADB.ocid, but not both"

			restore.Spec.Target.K8sAdb.Name = common.String("fake-target-adb")
			restore.Spec.Target.OciAdb.Ocid = common.String("fake.ocid1.autonomousdatabase.oc1...")

			validateInvalidTest(restore, false, errMsg)
		})

		It("Should select at least one restore source", func() {
			var errMsg string = "retore source is empty"

			restore.Spec.Source.K8sAdbBackup.Name = nil
			restore.Spec.Source.PointInTime.Timestamp = nil

			validateInvalidTest(restore, false, errMsg)
		})

		It("Cannot apply backupName and the PITR parameters at the same time", func() {
			var errMsg string = "cannot apply backupName and the PITR parameters at the same time"

			restore.Spec.Source.K8sAdbBackup.Name = common.String("fake-source-adb-backup")
			restore.Spec.Source.PointInTime.Timestamp = common.String("2021-12-23 11:03:13 UTC")

			validateInvalidTest(restore, false, errMsg)
		})

		It("Invalid timestamp format", func() {
			var errMsg string = "invalid timestamp format"

			restore.Spec.Source.PointInTime.Timestamp = common.String("12/23/2021 11:03:13")

			validateInvalidTest(restore, false, errMsg)
		})
	})
})
