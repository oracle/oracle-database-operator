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

import "encoding/json"

type KMSConfig struct {
	VaultName      string `json:"vaultName,omitempty"`
	CompartmentId  string `json:"compartmentId,omitempty"`
	KeyName        string `json:"keyName,omitempty"`
	EncryptionAlgo string `json:"encryptionAlgo,omitempty"`
	VaultType      string `json:"vaultType,omitempty"`
}
type KMSDetailsStatus struct {
	VaultId            string `json:"vaultId,omitempty"`
	ManagementEndpoint string `json:"managementEndpoint,omitempty"`
	KeyId              string `json:"keyId,omitempty"`
	VaultName          string `json:"vaultName,omitempty"`
	CompartmentId      string `json:"compartmentId,omitempty"`
	KeyName            string `json:"keyName,omitempty"`
	EncryptionAlgo     string `json:"encryptionAlgo,omitempty"`
	VaultType          string `json:"vaultType,omitempty"`
}

const (
	lastSuccessfulKMSConfig = "lastSuccessfulKMSConfig"
	lastSuccessfulKMSStatus = "lastSuccessfulKMSStatus"
)

// GetLastSuccessfulKMSConfig returns the KMS config from the last successful reconciliation.
// Returns nil, nil if there is no lastSuccessfulKMSConfig.
func (dbcs *DbcsSystem) GetLastSuccessfulKMSConfig() (*KMSConfig, error) {
	val, ok := dbcs.GetAnnotations()[lastSuccessfulKMSConfig]
	if !ok {
		return nil, nil
	}

	configBytes := []byte(val)
	kmsConfig := KMSConfig{}

	err := json.Unmarshal(configBytes, &kmsConfig)
	if err != nil {
		return nil, err
	}

	return &kmsConfig, nil
}

// GetLastSuccessfulKMSStatus returns the KMS status from the last successful reconciliation.
// Returns nil, nil if there is no lastSuccessfulKMSStatus.
func (dbcs *DbcsSystem) GetLastSuccessfulKMSStatus() (*KMSDetailsStatus, error) {
	val, ok := dbcs.GetAnnotations()[lastSuccessfulKMSStatus]
	if !ok {
		return nil, nil
	}

	statusBytes := []byte(val)
	kmsStatus := KMSDetailsStatus{}

	err := json.Unmarshal(statusBytes, &kmsStatus)
	if err != nil {
		return nil, err
	}

	return &kmsStatus, nil
}

// SetLastSuccessfulKMSConfig saves the given KMSConfig to the annotations.
func (dbcs *DbcsSystem) SetLastSuccessfulKMSConfig(kmsConfig *KMSConfig) error {
	configBytes, err := json.Marshal(kmsConfig)
	if err != nil {
		return err
	}

	annotations := dbcs.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[lastSuccessfulKMSConfig] = string(configBytes)
	dbcs.SetAnnotations(annotations)
	return nil
}

// SetLastSuccessfulKMSStatus saves the given KMSDetailsStatus to the annotations.
func (dbcs *DbcsSystem) SetLastSuccessfulKMSStatus(kmsStatus *KMSDetailsStatus) error {
	statusBytes, err := json.Marshal(kmsStatus)
	if err != nil {
		return err
	}

	annotations := dbcs.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[lastSuccessfulKMSStatus] = string(statusBytes)
	dbcs.SetAnnotations(annotations)
	// Update KMSDetailsStatus in DbcsSystemStatus
	dbcs.Status.KMSDetailsStatus = KMSDetailsStatus{
		VaultName:      kmsStatus.VaultName,
		CompartmentId:  kmsStatus.CompartmentId,
		KeyName:        kmsStatus.KeyName,
		EncryptionAlgo: kmsStatus.EncryptionAlgo,
		VaultType:      kmsStatus.VaultType,
	}
	return nil
}
