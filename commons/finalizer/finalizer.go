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

package finalizer

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// name of our custom finalizer
var finalizerName = "database.oracle.com/dbcsfinalizer"

// HasFinalizer returns true if the finalizer exists in the object metadata
func HasFinalizer(obj client.Object) bool {
	finalizer := obj.GetFinalizers()
	return containsString(finalizer, finalizerName)
}

// Register adds the finalizer and patch the object
func Register(kubeClient client.Client, obj client.Object) error {
	finalizer := obj.GetFinalizers()
	finalizer = append(finalizer, finalizerName)
	return setFinalizer(kubeClient, obj, finalizer)
}

// Unregister removes the finalizer and patch the object
func Unregister(kubeClient client.Client, obj client.Object) error {
	finalizer := obj.GetFinalizers()
	finalizer = removeString(finalizer, finalizerName)
	return setFinalizer(kubeClient, obj, finalizer)
}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

type patchValue struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func setFinalizer(kubeClient client.Client, dbcs client.Object, finalizer []string) error {
	payload := []patchValue{}

	if dbcs.GetFinalizers() == nil {
		payload = append(payload, patchValue{
			Op:    "replace",
			Path:  "/metadata/finalizers",
			Value: []string{},
		})
	}

	payload = append(payload, patchValue{
		Op:    "replace",
		Path:  "/metadata/finalizers",
		Value: finalizer,
	})

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	patch := client.RawPatch(types.JSONPatchType, payloadBytes)
	return kubeClient.Patch(context.TODO(), dbcs, patch)
}
