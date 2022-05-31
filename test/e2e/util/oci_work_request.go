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

package e2eutil

import (
	"context"

	"github.com/oracle/oci-go-sdk/v64/common"
	"github.com/oracle/oci-go-sdk/v64/workrequests"

	"time"
)

func WaitUntilWorkCompleted(workClient workrequests.WorkRequestClient, opcWorkRequestID *string) error {
	retryPolicy := getCompleteWorkRetryPolicy()

	// Apply wait until work complete retryPolicy
	workRequest := workrequests.GetWorkRequestRequest{
		WorkRequestId: opcWorkRequestID,
		RequestMetadata: common.RequestMetadata{
			RetryPolicy: &retryPolicy,
		},
	}

	// GetWorkRequest retries until the work status is SUCCEEDED
	if _, err := workClient.GetWorkRequest(context.TODO(), workRequest); err != nil {
		return err
	}

	return nil
}

func getCompleteWorkRetryPolicy() common.RetryPolicy {
	// maximum times of retry
	attempts := uint(30)

	shouldRetry := func(r common.OCIOperationResponse) bool {
		if _, isServiceError := common.IsServiceError(r.Error); isServiceError {
			// Don't retry if it's service error. Sometimes it could be network error or other errors which prevents
			// request send to server; we do the retry in these cases.
			return false
		}

		if converted, ok := r.Response.(workrequests.GetWorkRequestResponse); ok {
			// do the retry until WorkReqeut Status is Succeeded  - ignore case (BMI-2652)
			return converted.Status != workrequests.WorkRequestStatusSucceeded
		}

		return true
	}

	nextDuration := func(r common.OCIOperationResponse) time.Duration {
		// // you might want wait longer for next retry when your previous one failed
		// // this function will return the duration as:
		// // 1s, 2s, 4s, 8s, 16s, 32s, 64s etc...
		// return time.Duration(math.Pow(float64(2), float64(r.AttemptNumber-1))) * time.Second
		return time.Second * 20
	}

	return common.NewRetryPolicy(attempts, shouldRetry, nextDuration)
}
