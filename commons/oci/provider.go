/*
** Copyright (c) 2022, 2024 Oracle and/or its affiliates.
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
	"errors"
	"fmt"
	"os"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oracle/oracle-database-operator/commons/k8s"
)

const (
	regionKey      = "region"
	fingerprintKey = "fingerprint"
	userKey        = "user"
	tenancyKey     = "tenancy"
	passphraseKey  = "passphrase"
	privatekeyKey  = "privatekey"
)

type ApiKeyAuth struct {
	ConfigMapName *string
	SecretName    *string
	Namespace     string
}

func GetOciProvider(kubeClient client.Client, authData ApiKeyAuth) (common.ConfigurationProvider, error) {
	if authData.ConfigMapName != nil && authData.SecretName == nil {
		return getWorkloadIdentityProvider(kubeClient, authData)
	} else if authData.ConfigMapName != nil && authData.SecretName != nil {
		provider, err := getProviderWithAPIKey(kubeClient, authData)
		if err != nil {
			return nil, err
		}

		return provider, nil
	} else if authData.ConfigMapName == nil && authData.SecretName == nil {
		return auth.InstancePrincipalConfigurationProvider()
	} else {
		return nil, errors.New("both the OCI ConfigMap and the privateKey are required to authorize with API signing key; " +
			"leave them both empty to authorize with Instance Principal")
	}
}

func getWorkloadIdentityProvider(kubeClient client.Client, authData ApiKeyAuth) (common.ConfigurationProvider, error) {
	ociConfigMap, err := k8s.FetchConfigMap(kubeClient, authData.Namespace, *authData.ConfigMapName)
	if err != nil {
		return nil, err
	}
	// Ensure configmap is set with proper data
	if len(ociConfigMap.Data) == 0 {
		return nil, fmt.Errorf("OCI ConfigMap %s has no data", ociConfigMap.Name)
	}
	region, ok := ociConfigMap.Data[regionKey]
	if !ok || len(region) == 0 {
		return nil, fmt.Errorf("OCI Region Key %s missing from OCI ConfigMap %s", regionKey, ociConfigMap.Name)
	}
	// OCI SDK requires specific, dynamic environment variables for workload identity.
	if err = os.Setenv(auth.ResourcePrincipalVersionEnvVar, auth.ResourcePrincipalVersion2_2); err != nil {

		return nil, fmt.Errorf("unable to set OCI SDK environment variable %s: %v", auth.ResourcePrincipalVersionEnvVar, err)
	}
	if err = os.Setenv(auth.ResourcePrincipalRegionEnvVar, region); err != nil {
		return nil, fmt.Errorf("unable to set OCI SDK environment variable %s: %v", auth.ResourcePrincipalRegionEnvVar, err)
	}
	return auth.OkeWorkloadIdentityConfigurationProvider()
}

func getProviderWithAPIKey(kubeClient client.Client, authData ApiKeyAuth) (common.ConfigurationProvider, error) {
	var region, fingerprint, user, tenancy, passphrase, privatekeyValue string

	// Prepare ConfigMap
	ociConfigMap, err := k8s.FetchConfigMap(kubeClient, authData.Namespace, *authData.ConfigMapName)
	if err != nil {
		return nil, err
	}

	for key, val := range ociConfigMap.Data {
		if key == regionKey {
			region = val
		} else if key == fingerprintKey {
			fingerprint = val
		} else if key == userKey {
			user = val
		} else if key == tenancyKey {
			tenancy = val
		} else if key == passphraseKey {
			passphrase = val
		} else {
			return nil, errors.New("Unable to identify the key: " + key)
		}
	}

	// Prepare privatekey value
	privatekeyValue, err = k8s.GetSecretValue(kubeClient, authData.Namespace, *authData.SecretName, privatekeyKey)
	if err != nil {
		return nil, err
	}

	return common.NewRawConfigurationProvider(tenancy, user, region, fingerprint, privatekeyValue, &passphrase), nil
}
