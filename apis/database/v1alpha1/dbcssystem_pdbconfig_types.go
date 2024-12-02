/*
** Copyright (c) 2022-2024 Oracle and/or its affiliates.
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

// PDBConfig defines details of PDB struct for DBCS systems
type PDBConfig struct {
	// The name for the pluggable database (PDB). The name is unique in the context of a Database. The name must begin with an alphabetic character and can contain a maximum of thirty alphanumeric characters. Special characters are not permitted. The pluggable database name should not be same as the container database name.
	PdbName *string `mandatory:"true" json:"pdbName"`

	// The OCID (https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the CDB
	// ContainerDatabaseId *string `mandatory:"false" json:"containerDatabaseId"`

	// // A strong password for PDB Admin. The password must be at least nine characters and contain at least two uppercase, two lowercase, two numbers, and two special characters. The special characters must be _, \#, or -.
	PdbAdminPassword *string `mandatory:"false" json:"pdbAdminPassword"`

	// // The existing TDE wallet password of the CDB.
	TdeWalletPassword *string `mandatory:"false" json:"tdeWalletPassword"`

	// // The locked mode of the pluggable database admin account. If false, the user needs to provide the PDB Admin Password to connect to it.
	// // If true, the pluggable database will be locked and user cannot login to it.
	ShouldPdbAdminAccountBeLocked *bool `mandatory:"false" json:"shouldPdbAdminAccountBeLocked"`

	// // Free-form tags for this resource. Each tag is a simple key-value pair with no predefined name, type, or namespace.
	// // For more information, see Resource Tags (https://docs.cloud.oracle.com/Content/General/Concepts/resourcetags.htm).
	// // Example: `{"Department": "Finance"}`
	FreeformTags map[string]string `mandatory:"false" json:"freeformTags"`

	// // Defined tags for this resource. Each key is predefined and scoped to a namespace.
	// // For more information, see Resource Tags (https://docs.cloud.oracle.com/Content/General/Concepts/resourcetags.htm).
	// DefinedTags map[string]map[string]interface{} `mandatory:"false" json:"definedTags"`

	// To specify whether to delete the PDB
	IsDelete *bool `mandatory:"false" json:"isDelete,omitempty"`

	// The OCID of the PDB for deletion purposes.
	PluggableDatabaseId *string `mandatory:"false" json:"pluggableDatabaseId,omitempty"`
}

type PDBConfigStatus struct {
	PdbName                       *string           `mandatory:"true" json:"pdbName"`
	ShouldPdbAdminAccountBeLocked *bool             `mandatory:"false" json:"shouldPdbAdminAccountBeLocked"`
	FreeformTags                  map[string]string `mandatory:"false" json:"freeformTags"`
	PluggableDatabaseId           *string           `mandatory:"false" json:"pluggableDatabaseId,omitempty"`
	PdbLifecycleState             LifecycleState    `json:"pdbState,omitempty"`
}
type PDBDetailsStatus struct {
	PDBConfigStatus []PDBConfigStatus `json:"pdbConfigStatus,omitempty"`
}
