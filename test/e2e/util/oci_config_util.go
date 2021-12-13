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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"regexp"
	"strings"

	"github.com/oracle/oci-go-sdk/v45/common"

	corev1 "k8s.io/api/core/v1"
)

type configUtil struct {
	//The path to the test configuration file
	OCIConfigPath string

	//The profile for the configuration
	Profile string

	//ConfigFileInfo
	FileInfo *configFileInfo
}

func (p configUtil) readAndParseConfigFile() (*configFileInfo, error) {
	if p.FileInfo != nil {
		return p.FileInfo, nil
	}

	if p.OCIConfigPath == "" {
		return nil, fmt.Errorf("configuration path can not be empty")
	}

	data, err := openConfigFile(p.OCIConfigPath)
	if err != nil {
		return nil, err
	}

	p.FileInfo, err = parseConfigFile(data, p.Profile)
	return p.FileInfo, err
}

func (p configUtil) CreateOCIConfigMap(configMapNamespace string, configMapName string) (*corev1.ConfigMap, error) {
	info, err := p.readAndParseConfigFile()
	if err != nil {
		return nil, err
	}

	data := map[string]string{
		"region":      info.Region,
		"fingerprint": info.Fingerprint,
		"user":        info.UserOCID,
		"tenancy":     info.TenancyOCID,
	}

	return createKubeConfigMap(configMapNamespace, configMapName, data)
}

func (p configUtil) CreateOCISecret(secretNamespace string, secretName string) (*corev1.Secret, error) {
	info, err := p.readAndParseConfigFile()
	if err != nil {
		return nil, err
	}

	expandedKeyFilePath, err := expandPath(info.KeyFilePath)
	if err != nil {
		return nil, err
	}

	pemFileContent, err := ioutil.ReadFile(expandedKeyFilePath)
	if err != nil {
		err = fmt.Errorf("can not read PrivateKey from configuration file due to: %s", err.Error())
		return nil, err
	}

	privatekeyData := map[string]string{
		"privatekey": string(pemFileContent),
	}

	return CreateKubeSecret(secretNamespace, secretName, privatekeyData)
}

func (p configUtil) GetConfigProvider() (common.ConfigurationProvider, error) {
	return common.ConfigurationProviderFromFileWithProfile(p.OCIConfigPath, p.Profile, "")
}

func openConfigFile(configFilePath string) (data []byte, err error) {
	expandedPath, err := expandPath(configFilePath)
	if err != nil {
		return
	}
	data, err = ioutil.ReadFile(expandedPath)
	if err != nil {
		err = fmt.Errorf("can not read config file: %s due to: %s", configFilePath, err.Error())
	}

	return
}

func getHomeFolder() (string, error) {
	current, e := user.Current()
	if e != nil {
		//Give up and try to return something sensible
		home := os.Getenv("HOME")
		if home == "" {
			return "", errors.New("do not inlcude a tilde(~) in the path")
		}
		return home, nil
	}
	return current.HomeDir, nil
}

// cleans and expands the path if it contains a tilde, returns the expanded path or the input path as is if not expansion
// was performed
func expandPath(filepath string) (expandedPath string, err error) {
	cleanedPath := path.Clean(filepath)
	expandedPath = cleanedPath
	if strings.HasPrefix(cleanedPath, "~") {
		rest := cleanedPath[2:]
		home, err := getHomeFolder()
		if err != nil {
			return "", err
		}
		expandedPath = path.Join(home, rest)
	}
	return expandedPath, nil
}

var profileRegex = regexp.MustCompile(`^\[(.*)\]`)

func parseConfigFile(data []byte, profile string) (info *configFileInfo, err error) {

	if len(data) == 0 {
		return nil, fmt.Errorf("configuration file content is empty")
	}

	content := string(data)
	splitContent := strings.Split(content, "\n")

	//Look for profile
	for i, line := range splitContent {
		if match := profileRegex.FindStringSubmatch(line); len(match) > 1 && match[1] == profile {
			start := i + 1
			return parseConfigAtLine(start, splitContent)
		}
	}

	return nil, fmt.Errorf("configuration file did not contain profile: %s", profile)
}

type configFileInfo struct {
	UserOCID, Fingerprint, KeyFilePath, TenancyOCID, Region, Passphrase string
}

func parseConfigAtLine(start int, content []string) (info *configFileInfo, err error) {
	info = &configFileInfo{}
	for i := start; i < len(content); i++ {
		line := content[i]
		if profileRegex.MatchString(line) {
			break
		}

		if !strings.Contains(line, "=") {
			continue
		}

		splits := strings.Split(line, "=")
		switch key, value := strings.TrimSpace(splits[0]), strings.TrimSpace(splits[1]); strings.ToLower(key) {
		case "passphrase", "pass_phrase":
			info.Passphrase = value
		case "user":
			info.UserOCID = value
		case "fingerprint":
			info.Fingerprint = value
		case "key_file":
			info.KeyFilePath = value
		case "tenancy":
			info.TenancyOCID = value
		case "region":
			info.Region = value
		}
	}
	return
}

func GetOCIConfigUtil(configPath string, profile string) (*configUtil, error) {
	if profile == "" {
		profile = "DEFAULT"
	}

	return &configUtil{
		OCIConfigPath: configPath,
		Profile:       profile,
	}, nil
}
