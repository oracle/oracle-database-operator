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

package oci

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"time"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v54/common"
	"github.com/oracle/oci-go-sdk/v54/database"
	"github.com/oracle/oci-go-sdk/v54/secrets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

// GetWallet downloads the wallet using the given information in the AutonomousDatabase object.
// The function then unzips the wallet and returns a map object which holds the byte values of the unzipped files.
func GetWallet(logger logr.Logger, kubeClient client.Client, dbClient database.DatabaseClient, secretClient secrets.SecretsClient, adb *dbv1alpha1.AutonomousDatabase) (map[string][]byte, error) {
	// Get the wallet password from Secret then Vault Secret
	walletPassword, err := getWalletPassword(logger, kubeClient, secretClient, adb)
	if err != nil {
		return nil, err
	}

	// Request to download a wallet with the given password
	resp, err := generateAutonomousDatabaseWallet(dbClient, *adb.Spec.Details.AutonomousDatabaseOCID, walletPassword)
	if err != nil {
		return nil, err
	}

	// Unzip the file
	outZip, err := ioutil.TempFile("", "wallet*.zip")
	if err != nil {
		return nil, err
	}
	defer outZip.Close()

	if _, err := io.Copy(outZip, resp.Content); err != nil {
		return nil, err
	}

	data, err := unzipWallet(outZip.Name())
	if err != nil {
		return nil, err
	}
	return data, nil
}

func getWalletPassword(logger logr.Logger, kubeClient client.Client, secretClient secrets.SecretsClient, adb *dbv1alpha1.AutonomousDatabase) (string, error) {
	if adb.Spec.Details.Wallet.Password.K8sSecretName != nil {
		logger.Info(fmt.Sprintf("Getting wallet password from Secret %s", *adb.Spec.Details.Wallet.Password.K8sSecretName))

		namespacedName := types.NamespacedName{
			Namespace: adb.GetNamespace(),
			Name:      *adb.Spec.Details.Wallet.Password.K8sSecretName,
		}

		key := *adb.Spec.Details.Wallet.Password.K8sSecretName
		walletPassword, err := getValueFromKubeSecret(kubeClient, namespacedName, key)
		if err != nil {
			return "", err
		}
		return walletPassword, nil

	} else if adb.Spec.Details.Wallet.Password.OCISecretOCID != nil {
		logger.Info(fmt.Sprintf("Getting wallet password from OCI Vault Secret OCID %s", *adb.Spec.Details.Wallet.Password.OCISecretOCID))

		walletPassword, err := getValueFromVaultSecret(secretClient, *adb.Spec.Details.Wallet.Password.OCISecretOCID)
		if err != nil {
			return "", err
		}
		return walletPassword, nil
	}
	return "", errors.New("should provide either InstancewalletPasswordSecret or a InstancewalletPasswordId")
}

func generateAutonomousDatabaseWallet(dbClient database.DatabaseClient, adbOCID string, walletPassword string) (database.GenerateAutonomousDatabaseWalletResponse, error) {

	// maximum times of retry
	attempts := uint(10)

	// retry for all non-200 status code
	retryOnAllNon200ResponseCodes := func(r common.OCIOperationResponse) bool {
		return !(r.Error == nil && 199 < r.Response.HTTPResponse().StatusCode && r.Response.HTTPResponse().StatusCode < 300)
	}

	nextDuration := func(r common.OCIOperationResponse) time.Duration {
		// Wait longer for next retry when your previous one failed
		// this function will return the duration as:
		// 1s, 2s, 4s, 8s, 16s, 32s, 64s etc...
		return time.Duration(math.Pow(float64(2), float64(r.AttemptNumber-1))) * time.Second
	}

	walletRetryPolicy := common.NewRetryPolicy(attempts, retryOnAllNon200ResponseCodes, nextDuration)

	// Download a Wallet
	req := database.GenerateAutonomousDatabaseWalletRequest{
		AutonomousDatabaseId: common.String(adbOCID),
		GenerateAutonomousDatabaseWalletDetails: database.GenerateAutonomousDatabaseWalletDetails{
			Password: common.String(walletPassword),
		},
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &walletRetryPolicy,
		},
	}

	// Send the request using the service client
	return dbClient.GenerateAutonomousDatabaseWallet(context.TODO(), req)
}

func unzipWallet(filename string) (map[string][]byte, error) {
	data := map[string][]byte{}

	reader, err := zip.OpenReader(filename)
	if err != nil {
		return data, err
	}

	defer reader.Close()
	for _, file := range reader.File {
		reader, err := file.Open()
		if err != nil {
			return data, err
		}

		content, err := ioutil.ReadAll(reader)
		if err != nil {
			return data, err
		}

		data[file.Name] = content
	}

	return data, nil
}
