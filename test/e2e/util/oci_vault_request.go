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

package e2eutil

import (
	"context"
	"encoding/base64"
	"math"
	"time"

	"github.com/oracle/oci-go-sdk/v51/common"
	"github.com/oracle/oci-go-sdk/v51/keymanagement"
	"github.com/oracle/oci-go-sdk/v51/vault"
)

func waitForVaultStatePolicy(state keymanagement.VaultLifecycleStateEnum) common.RetryPolicy {
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if _, isServiceError := common.IsServiceError(r.Error); isServiceError {
			// not service error, could be network error or other errors which prevents
			// request send to server, will do retry here
			return true
		}

		if vaultResponse, ok := r.Response.(keymanagement.GetVaultResponse); ok {
			// do the retry until lifecycle state reaches the passed terminal state
			return vaultResponse.Vault.LifecycleState != state
		}

		return true
	}

	return newRetryPolicy(shouldRetry)
}

func CreateOCIVault(kmsVaultClient keymanagement.KmsVaultClient, compartmentID *string, vaultName *string) (*keymanagement.Vault, error) {
	vaultDetails := keymanagement.CreateVaultDetails{
		CompartmentId: compartmentID,
		DisplayName:   vaultName,
		VaultType:     keymanagement.CreateVaultDetailsVaultTypeDefault,
	}

	request := keymanagement.CreateVaultRequest{
		CreateVaultDetails: vaultDetails,
	}
	response, err := kmsVaultClient.CreateVault(context.TODO(), request)
	if err != nil {
		return nil, err
	}

	return &response.Vault, nil
}

func CreateOCIKey(vaultManagementClient keymanagement.KmsManagementClient, compartmentID *string, keyName *string) (*keymanagement.Key, error) {
	keyLength := 32

	keyShape := keymanagement.KeyShape{
		Algorithm: keymanagement.KeyShapeAlgorithmAes,
		Length:    &keyLength,
	}

	createKeyDetails := keymanagement.CreateKeyDetails{
		CompartmentId: compartmentID,
		KeyShape:      &keyShape,
		DisplayName:   keyName,
	}

	request := keymanagement.CreateKeyRequest{
		CreateKeyDetails: createKeyDetails,
	}
	response, err := vaultManagementClient.CreateKey(context.TODO(), request)
	if err != nil {
		return nil, err
	}

	return &response.Key, nil
}

func CreateOCISecret(vaultClient vault.VaultsClient, compartmentID *string, secretName *string, vaultID *string, keyID *string, content *string) (*string, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(*content))

	base64Content := vault.Base64SecretContentDetails{
		Name:    secretName,
		Content: common.String(encoded),
	}

	details := vault.CreateSecretDetails{
		CompartmentId: compartmentID,
		SecretContent: base64Content,
		SecretName:    secretName,
		VaultId:       vaultID,
		KeyId:         keyID,
	}

	request := vault.CreateSecretRequest{
		CreateSecretDetails: details,
	}

	// Send the request using the service client
	response, err := vaultClient.CreateSecret(context.TODO(), request)
	if err != nil {
		return nil, err
	}

	return response.Secret.Id, nil
}

func getVault(ctx context.Context, client keymanagement.KmsVaultClient, retryPolicy *common.RetryPolicy, vaultID *string) error {
	request := keymanagement.GetVaultRequest{
		VaultId: vaultID,
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: retryPolicy,
		},
	}
	if _, err := client.GetVault(ctx, request); err != nil {
		return err
	}
	return nil
}

func getKey(client keymanagement.KmsManagementClient, retryPolicy *common.RetryPolicy, keyID *string) error {
	request := keymanagement.GetKeyRequest{
		KeyId: keyID,
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: retryPolicy,
		},
	}
	if _, err := client.GetKey(context.TODO(), request); err != nil {
		return err
	}
	return nil
}

func getSecret(client vault.VaultsClient, retryPolicy *common.RetryPolicy, secretID *string) error {
	request := vault.GetSecretRequest{
		SecretId: secretID,
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: retryPolicy,
		},
	}
	if _, err := client.GetSecret(context.TODO(), request); err != nil {
		return err
	}
	return nil
}

func newRetryPolicy(retryOperation func(common.OCIOperationResponse) bool) common.RetryPolicy {
	// maximum times of retry
	attempts := uint(10)

	nextDuration := func(r common.OCIOperationResponse) time.Duration {
		// you might want wait longer for next retry when your previous one failed
		// this function will return the duration as:
		// 1s, 2s, 4s, 8s, 16s, 32s, 64s etc...
		return time.Duration(math.Pow(float64(2), float64(r.AttemptNumber-1))) * time.Second
	}

	return common.NewRetryPolicy(attempts, retryOperation, nextDuration)
}

func WaitForVaultState(client keymanagement.KmsVaultClient, vaultID *string, state keymanagement.VaultLifecycleStateEnum) error {
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if vaultResponse, ok := r.Response.(keymanagement.GetVaultResponse); ok {
			// do the retry until lifecycle state reaches the passed terminal state
			return vaultResponse.Vault.LifecycleState != state
		}

		return true
	}

	lifecycleStateCheckRetryPolicy := newRetryPolicy(shouldRetry)

	return getVault(context.TODO(), client, &lifecycleStateCheckRetryPolicy, vaultID)
}

func WaitForKeyState(client keymanagement.KmsManagementClient, keyID *string, state keymanagement.KeyLifecycleStateEnum) error {
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if keyResponse, ok := r.Response.(keymanagement.GetKeyResponse); ok {
			// do the retry until lifecycle state reaches the passed terminal state
			return keyResponse.Key.LifecycleState != state
		}

		return true
	}

	lifecycleStateCheckRetryPolicy := newRetryPolicy(shouldRetry)

	return getKey(client, &lifecycleStateCheckRetryPolicy, keyID)
}

func WaitForSecretState(client vault.VaultsClient, secretID *string, state vault.SecretLifecycleStateEnum) error {
	shouldRetry := func(r common.OCIOperationResponse) bool {
		if secretResponse, ok := r.Response.(vault.GetSecretResponse); ok {
			// do the retry until lifecycle state reaches the passed terminal state
			return secretResponse.Secret.LifecycleState != state
		}

		return true
	}

	lifecycleStateCheckRetryPolicy := newRetryPolicy(shouldRetry)

	return getSecret(client, &lifecycleStateCheckRetryPolicy, secretID)
}

// CleanupVault deletes the vault
// Anything encrypted by the keys contained within this vault will be unusable or irretrievable after the vault has been deleted
func CleanupVault(kmsVaultClient keymanagement.KmsVaultClient, vaultID *string) error {
	if vaultID == nil {
		return nil
	}

	request := keymanagement.ScheduleVaultDeletionRequest{
		VaultId: vaultID,
	}
	if _, err := kmsVaultClient.ScheduleVaultDeletion(context.TODO(), request); err != nil {
		return err
	}
	return nil
}
