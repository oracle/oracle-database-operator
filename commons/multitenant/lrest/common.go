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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func CommonDecryptWithPrivKey2(Key string, Buffer string, req ctrl.Request) (string, error) {

	Trclvl := 0
	block, restb := pem.Decode([]byte(Key))
	if block == nil {
		fmt.Printf("CommonDecryptWithPrivKey2 failure")
		fmt.Printf("rest =%x", restb)
		return "", errors.New("CommonDecryptWithPrivKey2 pem.Decode failure")
	}

	pkcs8PrivateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		fmt.Printf("Failed to parse private key %s \n", err.Error())
		return "", err
	}

	if Trclvl == 1 {
		fmt.Printf("======================================\n")
		fmt.Printf("%s\n", Key)
		fmt.Printf("======================================\n")
	}

	// chdx hint
	_, ok := pkcs8PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("CommonDecryptWithPrivKey2 expected RSA private key, got %T", pkcs8PrivateKey)
	}

	encString64, err := base64.StdEncoding.DecodeString(string(Buffer))
	if err != nil {
		fmt.Printf("Failed to decode encrypted string to base64: %s\n", err.Error())
		return "", err
	}

	decryptedB, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, pkcs8PrivateKey.(*rsa.PrivateKey), encString64, nil)
	//decryptedB, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, rsaPrivateKey, encString64, nil)
	if err != nil {
		fmt.Printf("Failed to decrypt string %s\n", err.Error())
		return "", err
	}
	if Trclvl == 1 {
		fmt.Printf("[%s]\n", string(decryptedB))
	}
	return strings.TrimSpace(string(decryptedB)), err

}

func ParseConfigMapData(cfgmap *corev1.ConfigMap, Trclvl int) []string {

	var tokens []string
	for Key, Value := range cfgmap.Data {
		if Bit(Trclvl, TRCCFM) == true {
			fmt.Printf("TRCCFM: (parse) key:[%s]\n", Key)
		}
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

const (
	NULL = ""
)

// * LREST STATE TABLE *//
const (
	LRSRBT = 0x00000001 /* Rebotting rest server */
	LRSCMP = 0x00000002 /* Lrest setup complete  */
	LRSINI = 0x00000004 /* Init phase */
	LRSINE = 0x00000008 /* Init phase wait */
	LRSPOD = 0x00000010 /* Pod creation */
	LRSPDE = 0x00000020 /* Pod creation wait */
	LRSPVD = 0x00000040 /* Pod validation */
	LRSPVE = 0x00000080 /* Pod validation wait */
	LRSSVC = 0x00000100 /* Svc creation */
	LRSSVE = 0x00000200 /* Svc creation wait */

)

// * LRPDB STATE TABLE *//
const (
	PDBCRT = 0x00000001 /* Create pdb */
	PDBOPN = 0x00000002 /* Open pdb read write */
	PDBCLS = 0x00000004 /* Close pdb */
	PDBDIC = 0x00000008 /* Drop pdb include datafiles */
	OCIHDL = 0x00000010 /* OCI handle allocation */
	OCICON = 0x00000020 /* Rdbms connection */
	FNALAZ = 0x00000040 /* Finalizer configured */
	PDBUPL = 0x00000080 /* Unplug pdb  */
	PDBPLG = 0x00000100 /* Plug pdb */
	APPUSR = 0x00000200 /* Application user created */
	/* Error section */
	PDBCRE = 0x00001000 /* PDB creation error */
	PDBOPE = 0x00002000 /* PDB open error */
	PDBCLE = 0x00004000 /* PDB close error */
	OCIHDE = 0x00008000 /* Allocation Handle Error */
	OCICOE = 0x00010000 /* CDD connection Error */
	FNALAE = 0x00020000
	PDBUPE = 0x00040000 /* Unplug Error */
	PDBPLE = 0x00080000 /* Plug Error */
	PDBPLW = 0x00100000 /* Plug Warining */
	PDBCNE = 0x00200000 /* Call Error */
	APPERR = 0x00400000 /* Application user error */
	/* Autodiscover */
	PDBAUT = 0x00800000 /* Autodisover */
)

// * CONFIG MAP STATUS * //
const (
	MPAPPL = 0x00000001 /* The map config has been applyed */
	MPSYNC = 0x00000002 /* The map config is in sync with v$parameters where is default=flase */
	MPEMPT = 0x00000004 /* The map is empty - not specify */
	MPWARN = 0x00000008 /* Map applied with warnings */
	MPINIT = 0x00000010 /* Config map init */
	MPRMVD = 0x00000020 /* Config map removed */
	SPARE3 = 0x00000040
)

// DEBUG OPTIONS //
const (
	TRCAPI = 0x00000001 /* Trclvl call NewcallApi */
	TRCGLR = 0x00000002 /* Trclvl call r.getLRESTResource */
	TRCSEC = 0x00000004 /* Trclvl call getGenericSecret3 */
	TRCCRT = 0x00000008 /* Trclvl pdb creation */
	TRCOPN = 0x00000010 /* Open pdb */
	TRCCLS = 0x00000020 /* Close pdb */
	TRCCFM = 0x00000040 /* Trclvl config map */
	TRCSQL = 0x00000080 /* getsqlcode and plsql related function */
	TRCCLN = 0x00000100 /* clone pdb */
	TRCPSQ = 0x00000200 /* plsql execution */
	TRCPLG = 0x00000400 /* plug pdb */
	TRCUPL = 0x00000800 /* unpplug */
	TRCAUT = 0x00001000 /* Autodiscovery */
	TRCSTK = 0x00002000 /* Print backtrace */
	TRCWEB = 0x00004000 /* Enable Webhook msg in logplane */
	TRCSTA = 0x00008000 /* Trclvl call getLRPDBState */
	TRCTNS = 0x00010000 /* Parse tnsalias - call parseTnsAlias */
	TRCUSR = 0x00020000 /* Trace User Creation */
)

// DiscardableError: we use this function in order to avoid
// bitset PDBCRE PDBOPE PDBCLE
func DiscardableError(oerr int) bool {
	// ora error white list
	// ORA-65019 pdb already open
	// ORA-65020 pdb already closed
	// ORA-1403  no data found

	var oraWhiteList = map[int]struct{}{
		65019: {},
		65020: {},
		65012: {},
		1403:  {},
	}

	_, exists := oraWhiteList[oerr]
	return exists
}

// ParseStrStatus: function used to implement reset status with tokes
func ParseStrStatus(state string) int {

	wordToNumber := map[string]int{
		"PDBCRT": 0x00000001,
		"PDBOPN": 0x00000002,
		"PDBCLS": 0x00000004,
		"PDBDIC": 0x00000008,
		"OCIHDL": 0x00000010,
		"OCICON": 0x00000020,
		"FNALAZ": 0x00000040,
		"PDBUPL": 0x00000080,
		"PDBPLG": 0x00000100,
		"APPUSR": 0x00000200,
		"PDBCRE": 0x00001000,
		"PDBOPE": 0x00002000,
		"PDBCLE": 0x00004000,
		"OCIHDE": 0x00008000,
		"OCICOE": 0x00010000,
		"FNALAE": 0x00020000,
		"PDBUPE": 0x00040000,
		"PDBPLE": 0x00080000,
		"PDBPLW": 0x00100000,
		"PDBCNE": 0x00200000,
		"APPERR": 0x00400000,
		"PDBAUT": 0x00800000,
	}

	tokens := strings.Split(strings.TrimSuffix(state, "|"), "|")
	var retbt int
	for _, tks := range tokens {
		if num, ok := wordToNumber[tks]; ok {
			/* "OR" operator */
			retbt = ((retbt) | (num))
		}
	}

	return retbt
}

func ParseTnsAlias2(tns *string, lrpdbsrv *string) {
	fmt.Printf("Analyzing string [%s]\n", *tns)
	fmt.Printf("Relacing  srv [%s]\n", *lrpdbsrv)
	var swaptns string

	if strings.Contains(strings.ToUpper(*tns), "SERVICE_NAME") == false {
		fmt.Print("Cannot generate tns alias for pdb")
		return
	}

	if strings.Contains(strings.ToUpper(*tns), "ORACLE_SID") == true {
		fmt.Print("Cannot generate tns alias for pdb")
		return
	}

	*tns = strings.ReplaceAll(*tns, " ", "")

	swaptns = fmt.Sprintf("SERVICE_NAME=%s", *lrpdbsrv)
	tnsreg := regexp.MustCompile(`SERVICE_NAME=\w+`)
	*tns = tnsreg.ReplaceAllString(*tns, swaptns)

	fmt.Printf("Newstring [%s]\n", *tns)

}

func Bid(bitmask int, bitval int) int {
	bitmask ^= ((bitval) & (bitmask))
	return bitmask
}

func Bit(bitmask int, bitval int) bool {
	if ((bitmask) & (bitval)) != 0 {
		return true
	} else {
		return false
	}
}

func Bis(bitmask int, bitval int) int {
	bitmask = ((bitmask) | (bitval))
	return bitmask
}

func Bitmaskprint2(bitmask int) string {
	BitRead := "|"
	if Bit(bitmask, LRSRBT) {
		BitRead = strings.Join([]string{BitRead, "LRSRBT|"}, "")
	}
	if Bit(bitmask, LRSCMP) {
		BitRead = strings.Join([]string{BitRead, "LRSCMP|"}, "")
	}
	if Bit(bitmask, LRSINI) {
		BitRead = strings.Join([]string{BitRead, "LRSINI|"}, "")
	}
	if Bit(bitmask, LRSINE) {
		BitRead = strings.Join([]string{BitRead, "LRSINE|"}, "")
	}
	if Bit(bitmask, LRSPOD) {
		BitRead = strings.Join([]string{BitRead, "LRSPOD|"}, "")
	}
	if Bit(bitmask, LRSPDE) {
		BitRead = strings.Join([]string{BitRead, "LRSPDE|"}, "")
	}
	if Bit(bitmask, LRSPVD) {
		BitRead = strings.Join([]string{BitRead, "LRSPVD|"}, "")
	}
	if Bit(bitmask, LRSPVE) {
		BitRead = strings.Join([]string{BitRead, "LRSPVE|"}, "")
	}
	if Bit(bitmask, LRSSVC) {
		BitRead = strings.Join([]string{BitRead, "LRSSVC|"}, "")
	}
	if Bit(bitmask, LRSSVE) {
		BitRead = strings.Join([]string{BitRead, "LRSSVE|"}, "")
	}

	BitRead = fmt.Sprintf("[%d]%s", bitmask, BitRead)
	return BitRead
}

func Bitmaskprint(bitmask int) string {
	BitRead := "|"
	if Bit(bitmask, PDBCRT) {
		BitRead = strings.Join([]string{BitRead, "PDBCRT|"}, "")
	}
	if Bit(bitmask, PDBOPN) {
		BitRead = strings.Join([]string{BitRead, "PDBOPN|"}, "")
	}
	if Bit(bitmask, PDBCLS) {
		BitRead = strings.Join([]string{BitRead, "PDBCLS|"}, "")
	}
	if Bit(bitmask, PDBDIC) {
		BitRead = strings.Join([]string{BitRead, "PDBDIC|"}, "")
	}
	if Bit(bitmask, OCIHDL) {
		BitRead = strings.Join([]string{BitRead, "OCIHDL|"}, "")
	}
	if Bit(bitmask, OCICON) {
		BitRead = strings.Join([]string{BitRead, "OCICON|"}, "")
	}
	if Bit(bitmask, FNALAZ) {
		BitRead = strings.Join([]string{BitRead, "FNALAZ|"}, "")
	}
	if Bit(bitmask, PDBUPL) {
		BitRead = strings.Join([]string{BitRead, "PDBUPL|"}, "")
	}
	if Bit(bitmask, PDBPLG) {
		BitRead = strings.Join([]string{BitRead, "PDBPLG|"}, "")
	}
	if Bit(bitmask, APPUSR) {
		BitRead = strings.Join([]string{BitRead, "APPUSR|"}, "")
	}
	if Bit(bitmask, APPERR) {
		BitRead = strings.Join([]string{BitRead, "APPERR|"}, "")
	}

	if Bit(bitmask, PDBCRE) {
		BitRead = strings.Join([]string{BitRead, "PDBCRE|"}, "")
	}
	if Bit(bitmask, PDBOPE) {
		BitRead = strings.Join([]string{BitRead, "PDBOPE|"}, "")
	}
	if Bit(bitmask, PDBCLE) {
		BitRead = strings.Join([]string{BitRead, "PDBCLE|"}, "")
	}
	if Bit(bitmask, OCIHDE) {
		BitRead = strings.Join([]string{BitRead, "OCIHDE|"}, "")
	}
	if Bit(bitmask, OCICOE) {
		BitRead = strings.Join([]string{BitRead, "OCICOE|"}, "")
	}
	if Bit(bitmask, FNALAE) {
		BitRead = strings.Join([]string{BitRead, "FNALAE|"}, "")
	}
	if Bit(bitmask, PDBUPE) {
		BitRead = strings.Join([]string{BitRead, "PDBUPE|"}, "")
	}
	if Bit(bitmask, PDBPLE) {
		BitRead = strings.Join([]string{BitRead, "PDBPLE|"}, "")
	}
	if Bit(bitmask, PDBPLW) {
		BitRead = strings.Join([]string{BitRead, "PDBPLW|"}, "")
	}
	if Bit(bitmask, PDBAUT) {
		BitRead = strings.Join([]string{BitRead, "PDBAUT|"}, "")
	}
	if Bit(bitmask, PDBCNE) {
		BitRead = strings.Join([]string{BitRead, "PDBCNE|"}, "")
	}

	BitRead = fmt.Sprintf("[%d]%s", bitmask, BitRead)
	return BitRead
}

func CMBitmaskprint(bitmask int) string {

	BitRead := "|"
	/*** Bit mask for config map ***/
	if Bit(bitmask, MPAPPL) {
		BitRead = strings.Join([]string{BitRead, "MPAPPL|"}, "")
	}
	if Bit(bitmask, MPSYNC) {
		BitRead = strings.Join([]string{BitRead, "MPSYNC|"}, "")
	}
	if Bit(bitmask, MPEMPT) {
		BitRead = strings.Join([]string{BitRead, "MPEMPT|"}, "")
	}
	if Bit(bitmask, MPWARN) {
		BitRead = strings.Join([]string{BitRead, "MPWARN|"}, "")
	}
	if Bit(bitmask, MPINIT) {
		BitRead = strings.Join([]string{BitRead, "MPINIT|"}, "")
	}
	if Bit(bitmask, MPRMVD) {
		BitRead = strings.Join([]string{BitRead, "MPRMVD|"}, "")
	}
	if Bit(bitmask, SPARE3) {
		BitRead = strings.Join([]string{BitRead, "SPARE3|"}, "")
	}

	BitRead = fmt.Sprintf("[%d]%s", bitmask, BitRead)
	return BitRead
}

func Backtrace() {

	pc := make([]uintptr, 10)
	nf := runtime.Callers(0, pc)
	if nf == 0 {
		fmt.Printf("NO PCs available cannot dump backtrace\n")
		return
	}

	pc = pc[:nf]
	frames := runtime.CallersFrames(pc)
	fmt.Printf("TRCSTK: \n BACKTRACE\n")
	fmt.Printf(" --------  -------------------\n")
	for {
		frame, more := frames.Next()

		nf--
		if nf == 0 {
			break
		}

		fmt.Printf(" FRAME[%d]  %s\n", nf, frame.Function)

		if !more {
			break
		}
	}
}

func HashStr(hnm interface{}, Beg int, End int) string {
	byteArray, err := json.Marshal(hnm)
	if err != nil {
		return ""
	}

	hash := sha256.New()
	_, err = hash.Write(byteArray)
	if err != nil {
		return ""
	}

	hashBytes := hash.Sum(nil)
	hashString := hex.EncodeToString(hashBytes[Beg:End])
	return hashString

}

func GenHash(url string, Check string) string {

	var hsh string
	if Check == "PWD" {
		hsh = HashStr(url, 17, 32)
	}
	if Check == "USR" {
		hsh = HashStr(url, 0, 17)
	}

	return hsh
}

func GetBaseName(url string) string {

	// IN     https://cdb-dev-lrest.cdbnamespace:8888/database/pdbs/"
	// OUT    cdb-dev-lrest.cdbnamespace

	start := strings.Index(url, ":")
	if start == -1 {
		return ""
	}
	start++

	end := strings.Index(url[start:], ":")
	if end == -1 {
		return ""
	}

	return url[start+2 : start+end]

}

func Gnrn() []byte {
	brn := make([]byte, 17)
	if _, err := rand.Read(brn); err != nil {
		return []byte("00000")
	}
	return []byte(hex.EncodeToString(brn))
}

// GetSQLCode reads sqlcode from resource status and copy into sqlcode pointer
func GetSQLCode02(rsp string, sqlcode *int, tracelevel int) error {

	if Bit(tracelevel, TRCSQL) == true {
		fmt.Printf("TRCSQL :Begin call\n")
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		fmt.Printf("failed to get respData from callAPI %s", err.Error())
		return err
	}

	*sqlcode = int(objmap["sqlcode"].(float64))
	if Bit(tracelevel, TRCSQL) == true {
		fmt.Printf("TRCSQL :sqlcode.......:ora-%s\n", strconv.Itoa(*sqlcode))
	}

	if *sqlcode != 0 {
		// Check the white list
		if DiscardableError(*sqlcode) == true {
			fmt.Printf("Discardable error: ora-%s \n", strconv.Itoa(*sqlcode))
			return nil
		}
		err := fmt.Errorf("%v", sqlcode)
		return err
	}
	return nil
}

func GetSQLbuffer02(rsp string, sqlmsg *string, tracelevel int) error {

	if sqlmsg == nil {
		return errors.New("sqlmsg must not be nil")
	}

	if Bit(tracelevel, TRCSQL) == true {
		fmt.Printf("TRCSQL :Begin call\n")
	}

	var objmap map[string]interface{}
	if err := json.Unmarshal([]byte(rsp), &objmap); err != nil {
		fmt.Printf("failed to get respData from callAPI %s", err.Error())
		return err
	}

	errbuffer, ok := objmap["errbuffer"].(string)
	if !ok {
		return errors.New("response is missing string errbuffer")
	}
	*sqlmsg = errbuffer

	if Bit(tracelevel, TRCSQL) == true {
		fmt.Printf("TRCSQL :errbuffer.......:%s\n", *sqlmsg)
	}

	return nil
}

// liveness probe: LrestHTTPProbe("/database/lrest/ping2/", 8888, 20, 15, 9, 0)
// readiness probe: LrestHTTPProbe("/database/lrest/ping/", 8888, 60, 15, 9, 0)
func LrestHTTPProbe(path string, portNum int, initialDelaySeconds, periodSeconds, timeoutSeconds, failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Scheme: corev1.URISchemeHTTPS,
				Path:   path,
				Port:   intstr.FromInt(portNum),
			},
		},
		InitialDelaySeconds: initialDelaySeconds,
		PeriodSeconds:       periodSeconds,
		TimeoutSeconds:      timeoutSeconds,
		FailureThreshold:    failureThreshold,
	}
}
