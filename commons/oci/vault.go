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
	"context"
	"encoding/base64"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v63/common"
	"github.com/oracle/oci-go-sdk/v63/secrets"
)

type VaultService interface {
	GetSecretValue(vaultSecretOCID string) (string, error)
}

type vaultService struct {
	logger       logr.Logger
	secretClient secrets.SecretsClient
}

func NewVaultService(
	logger logr.Logger,
	provider common.ConfigurationProvider) (VaultService, error) {

	secretClient, err := secrets.NewSecretsClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, err
	}

	return &vaultService{
		logger:       logger.WithName("vaultService"),
		secretClient: secretClient,
	}, nil
}

func (v *vaultService) GetSecretValue(vaultSecretOCID string) (string, error) {
	request := secrets.GetSecretBundleRequest{
		SecretId: common.String(vaultSecretOCID),
	}

	response, err := v.secretClient.GetSecretBundle(context.TODO(), request)
	if err != nil {
		return "", err
	}

	base64content := response.SecretBundle.SecretBundleContent.(secrets.Base64SecretBundleContentDetails)
	base64String := *base64content.Content
	decoded, err := base64.StdEncoding.DecodeString(base64String)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}
