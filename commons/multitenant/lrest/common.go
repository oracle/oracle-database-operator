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
   one is included with the Software (each a "Larger Work" to which the Software
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

package lrest

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"

	ctrl "sigs.k8s.io/controller-runtime"
)

func CommonDecryptWithPrivKey(Key string, Buffer string, req ctrl.Request) (string, error) {

	Debug := 0
	block, _ := pem.Decode([]byte(Key))
	pkcs8PrivateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		fmt.Printf("Failed to parse private key %s \n", err.Error())
		return "", err
	}
	if Debug == 1 {
		fmt.Printf("======================================\n")
		fmt.Printf("%s\n", Key)
		fmt.Printf("======================================\n")
	}

	encString64, err := base64.StdEncoding.DecodeString(string(Buffer))
	if err != nil {
		fmt.Printf("Failed to decode encrypted string to base64: %s\n", err.Error())
		return "", err
	}

	decryptedB, err := rsa.DecryptPKCS1v15(nil, pkcs8PrivateKey.(*rsa.PrivateKey), encString64)
	if err != nil {
		fmt.Printf("Failed to decrypt string %s\n", err.Error())
		return "", err
	}
	if Debug == 1 {
		fmt.Printf("[%s]\n", string(decryptedB))
	}
	return strings.TrimSpace(string(decryptedB)), err

}

func ParseConfigMapData(cfgmap *corev1.ConfigMap) []string {

	var tokens []string
	for Key, Value := range cfgmap.Data {
		fmt.Printf("KEY:%s\n", Key)
		re0 := regexp.MustCompile("\\n")
		re1 := regexp.MustCompile(";")
		re2 := regexp.MustCompile(",") /* Additional separator for future use */

		Value = re0.ReplaceAllString(Value, " ")
		tokens = strings.Split(Value, " ")

		for cnt := range tokens {
			if len(tokens[cnt]) != 0 {
				tokens[cnt] = re1.ReplaceAllString(tokens[cnt], " ")
				tokens[cnt] = re2.ReplaceAllString(tokens[cnt], " ")

			}

		}

	}

	return tokens

}
