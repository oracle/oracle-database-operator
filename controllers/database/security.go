// Copyright (c) 2019-2020, Oracle and/or its affiliates. All rights reserved.
//
// Certificates, TLS and other such

package controllers

import (
	"context"

	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	//"encoding/json"
)

var storePass string = "dk4kF81.fu9x"

var rootCAs *x509.CertPool

var (
	addRootCAMutex           sync.Mutex
	makeKeyMutex             sync.Mutex
	fetchKeyMutex            sync.Mutex
	getSecretMutex           sync.Mutex
	deleteSecretMutex        sync.Mutex
	makeExporterSecretsMutex sync.Mutex
)

//----------------------------------------------------------------------
// This contains the secret information that makeKey encodes into a
// Kubernetes Secret and which fetchKey fetches from such a Secret.
// Both makeKey and fetchKey return one of these, so the rest of
// the Operator can access the secret data
//----------------------------------------------------------------------

type TTSecretInfo struct {
	AgentUID   string // TimesTen db user / schema
	AgentPWD   string // used by the Agent
	HttpUID    string // HTTP basic auth credentials
	HttpPWD    string // to talk to the Agent
	ClientCert tls.Certificate
}

type TTSecrets struct {
	mu      sync.Mutex
	secrets map[string]TTSecretInfo
}

var ttSecrets = TTSecrets{secrets: make(map[string]TTSecretInfo)}

// add a secret to TTSecrets struct
func (c *TTSecrets) addSecret(ctx context.Context, secretName string, tts TTSecretInfo, objNamespace string, objName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	us := "addSecret"
	reqLogger := log.FromContext(ctx)

	defer reqLogger.V(2).Info(fmt.Sprintf("%s exited", us))

	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))

	c.secrets[secretName] = tts

	secretFile := "/tmp/" + objNamespace + ".ttc." + objName
	if _, err := os.Stat(secretFile); err == nil {
		e := os.Remove(secretFile)
		if e != nil {
			reqLogger.Error(err, fmt.Sprintf("%s: error, could not remove secretFile %s, err=%s", us, secretFile, e.Error()))
		}
	}

	err := os.WriteFile(secretFile, []byte(secretName), 0644)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("%s: error, could not write secretFile %s, err=%s", us, secretFile, err.Error()))
		//return nil, err
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("%s: wrote secretFile %s", us, secretFile))
	}

}

// getSecret fetches a secret
func getSecret(ctx context.Context, client client.Client, name types.NamespacedName) (*corev1.Secret, error) {
	getSecretMutex.Lock()
	defer getSecretMutex.Unlock()

	ourSecret := &corev1.Secret{}
	err := client.Get(ctx, name, ourSecret)
	if err != nil {
		return nil, err
	}
	return ourSecret, nil
}

// deleteSecret deletes a secret
func (c *TTSecrets) deleteSecret(ctx context.Context, secretName string, objNamespace string, objName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	us := "deleteSecret"
	reqLogger := log.FromContext(ctx)

	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))
	defer reqLogger.V(2).Info(fmt.Sprintf("%s exited", us))

	_, found := c.secrets[secretName]
	if found {
		delete(c.secrets, secretName)
		reqLogger.V(1).Info(fmt.Sprintf("%s: deleted secret %s", us, secretName))
	} else {
		reqLogger.V(2).Info(fmt.Sprintf("%s: did not find secret %v", us, secretName))
	}

	secretFile := "/tmp/" + objNamespace + ".ttc." + objName
	if _, err := os.Stat(secretFile); err == nil {
		e := os.Remove(secretFile)
		if e != nil {
			reqLogger.Error(err, fmt.Sprintf("%s: error, could not remove secretFile %s, err=%s", us, secretFile, e.Error()))
		} else {
			reqLogger.V(1).Info(fmt.Sprintf("%s: removed %s", us, secretFile))
		}
	}

	return nil
}

// newRandString returns a random String
func newRandString(lenWanted int) string {
	var bytes = make([]byte, lenWanted)
	rand.Read(bytes)
	allowedChars := "abcdefghijklmnopqrstuvwxyz1234567890"
	firstChars := "abcdefghijklmnopqrstuvwxyz"

	for i, v := range bytes {
		if i == 0 {
			bytes[i] = firstChars[v%byte(len(firstChars))]
		} else {
			bytes[i] = allowedChars[v%byte(len(allowedChars))]
		}
	}

	return string(bytes)
}

// Add a cert to the root CA list
func addCertToRootCA(ctx context.Context, dirname string, secretName string, certData []byte) (*tls.Certificate, error, string) {
	addRootCAMutex.Lock()
	defer addRootCAMutex.Unlock()

	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("addCertToRootCA entered")
	defer reqLogger.V(1).Info("addCertToRootCA returns")

	clientcert, err := tls.LoadX509KeyPair(dirname+secretName+".cert", dirname+secretName+".priv")
	if err != nil {
		reqLogger.Error(err, "Could not load keypair")
		return nil, err, "Could not load keypair: " + err.Error()
	}

	appended := rootCAs.AppendCertsFromPEM(certData)
	if !appended {
		reqLogger.Error(errors.New("Failed to append certs"), "Failed to append certs")
		// should we generate an err and return it?  we do if LoadX509KeyPair fails
	}

	return &clientcert, nil, ""

}

// This function takes a []byte, converts it to base64, then
// chunks that into an array of strings that are short enough to put into a
// wallet.
func chunkBytes(data []byte) []string {
	certEncoded := base64.StdEncoding.EncodeToString(data)
	var certArray []string
	for len(certEncoded) > 0 {
		if len(certEncoded) > 1000 {
			certArray = append(certArray, certEncoded[0:1000])
			certEncoded = certEncoded[1000:]
		} else {
			certArray = append(certArray, certEncoded)
			certEncoded = ""
		}
	}
	return certArray
}

// ----------------------------------------------------------------------
// This will create a new Secret which contains the credentials
// needed for communication between the Operator and the Agents for a
// particular TimesTenClassic object
//
// It should contain the certificates required for HTTPS, any userid/password
// combos required (for HTTP basic authentication, for example), and anything
// else that we want the Operator and a given set of Agents to have in common
// which nobody else should know.
//
// See also the fetchKey function, which can retrieve that same info
// from a Secret that was previously created by makeKey. That is required
// if, for example, the Operator is stopped or crashes and a new Operator
// takes over for it.
//
// This function makes up all of the secrets, puts them in a Secret, and then
// returns them so the caller can use them.
//
// secretInfo, err := makeKey(instance, r, reqLogger)
// ----------------------------------------------------------------------
func makeKey(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, scheme *runtime.Scheme,
	secretName string) (*TTSecretInfo, error) {
	makeKeyMutex.Lock()

	us := "makeKey"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info(us + " entered")

	defer func() {
		makeKeyMutex.Unlock()
		reqLogger.V(1).Info(us + " returns")
	}()

	objUid := string(instance.ObjectMeta.UID)

	var tts TTSecretInfo

	cn := "*." + instance.Name + "." + instance.Namespace + ".svc.cluster.local"

	// Determine what database user the agent will use for this object

	tts.AgentUID = "ttagent"
	tts.AgentPWD = newRandString(16)

	// Basic authentication

	tts.HttpUID = newRandString(16)
	tts.HttpPWD = newRandString(16)

	//----------------------------------------------------------------------
	// Make a self signed cert for this TimesTenClassic object
	//----------------------------------------------------------------------

	configFile, err := ioutil.TempFile("", objUid+".cnf")
	if err != nil {
		reqLogger.Error(err, us+": Error creating temp file: "+err.Error())
		return nil, err
	}
	defer os.Remove(configFile.Name())

	cnf := "[req]\ndistinguished_name=req\n[san]\nsubjectAltName=DNS:" + cn + "\n"
	configFile.Write([]byte(cnf))
	configFile.Close()

	rc, stdout, stderr := runShellCommand(ctx, []string{"openssl",
		"req", "-x509", "-nodes", "-newkey", "rsa:4096",
		"-keyout", "/tmp/" + secretName + ".priv",
		"-out", "/tmp/" + secretName + ".cert",
		"-days", "9999",
		"-subj", "/CN=" + cn,
		"-extensions", "san",
		"-config", configFile.Name()})
	reqLogger.V(1).Info(us + ": openssl RC=" + strconv.Itoa(rc))

	for _, l := range stdout {
		if l == "" {
			continue
		}
		reqLogger.V(1).Info(l)
	}
	for _, l := range stderr {
		if l == "" {
			continue
		}
		reqLogger.V(1).Info(l)
	}

	if rc != 0 {
		err := errors.New("openssl failed rc " + strconv.Itoa(rc))
		reqLogger.Error(err, us+": OpenSSL failed", "stdout", stdout, "stderr", stderr, "rc", rc)
		return nil, err
	}

	pkData, err := ioutil.ReadFile("/tmp/" + secretName + ".priv")
	if err != nil {
		reqLogger.Error(err, us+": Failed to read /tmp/"+secretName+".priv: "+err.Error())
		return nil, err
	}

	certData, err := ioutil.ReadFile("/tmp/" + secretName + ".cert")
	if err != nil {
		reqLogger.Error(err, us+": Failed to read /tmp/"+secretName+".cert: "+err.Error())
		return nil, err
	}

	cc, err, errmsg := addCertToRootCA(ctx, "/tmp/", secretName, certData)
	if err != nil {
		reqLogger.Error(err, us+": addCertToRootCA failed: "+err.Error())
		reqLogger.V(1).Info(fmt.Sprintf("%s: addCertToRootCA output: %v", us, errmsg))
		return nil, err
	}

	tts.ClientCert = *cc

	timestenHome, ok := os.LookupEnv("TIMESTEN_HOME")
	if ok {
		reqLogger.V(2).Info(us + ": using TimesTen instance " + timestenHome)
	} else {
		e := errors.New("Could not find the operator's TimesTen instance, TIMESTEN_HOME not found")
		reqLogger.Error(e, "Could not find the operator's TimesTen instance")
		panic(e)
	}

	reqLogger.V(2).Info(fmt.Sprintf("%s: timestenHome=%s", us, timestenHome))

	// make /tmp/<our_object_uid> first; it must exist for ttUser --zzwallet to work
	if err := safeMakeDir("/tmp/"+secretName, int(0700)); err != nil {
		reqLogger.Info(fmt.Sprintf("%s: mkdir /tmp/%s failed with error: %v", us, secretName, err.Error()))
		return nil, err
	}

	pkArray := chunkBytes(pkData)
	certArray := chunkBytes(certData)

	walArgs := []string{"-zzwallet", "/tmp/" + secretName, "-zzcreate"}
	cmd2 := exec.Command(timestenHome+"/bin/ttUser", walArgs...)

	var walout bytes.Buffer
	var walerr bytes.Buffer
	cmd2.Stdout = &walout
	cmd2.Stderr = &walerr
	inn, err := cmd2.StdinPipe()
	if err != nil {
		reqLogger.Error(err, us+": could not create StdinPipe")
		return nil, err
	}
	go func() {
		defer inn.Close()
		io.WriteString(inn, "AgentUID="+tts.AgentUID+"\n")
		io.WriteString(inn, "AgentPWD="+tts.AgentPWD+"\n")
		io.WriteString(inn, "HttpUID="+tts.HttpUID+"\n")
		io.WriteString(inn, "HttpPWD="+tts.HttpPWD+"\n")
		for i, v := range pkArray {
			io.WriteString(inn, "PK_"+strconv.Itoa(i)+"="+v+"\n")
		}
		for i, v := range certArray {
			io.WriteString(inn, "CERT_"+strconv.Itoa(i)+"="+v+"\n")
		}
	}()

	err = cmd2.Run()
	if err != nil {
		reqLogger.Error(err, us+": "+walout.String()+walerr.String())
		return nil, err
	}

	// universalInstanceGuid is the operator's GUID, objUid is the object being managed
	walletData, err := ioutil.ReadFile("/tmp/" + secretName + "/.ttwallet." + universalInstanceGuid + "/cwallet.sso")
	if err != nil {
		reqLogger.Error(err, us+": could not read wallet")
		return nil, err
	}

	err = os.RemoveAll("/tmp/" + secretName)
	if err != nil {
		reqLogger.Error(err, us+": could not erase wallet")
		return nil, err
	}

	// TODO: why do we need to keep .cert and .priv files in temp? See fetchKey, which writes them but
	//       strangely does not use them (implies we look for them somewhere else?)
	//       Keep them for now.
	//err = os.RemoveAll("/tmp/" + secretName + ".cert")
	//if err != nil {
	//    reqLogger.Error(err, us + ": could not erase " + secretName + ".cert")
	//    return nil, err
	//}
	//
	//err = os.RemoveAll("/tmp/" + secretName + ".priv")
	//if err != nil {
	//    reqLogger.Error(err, us + ": could not erase " + secretName + ".priv")
	//    return nil, err
	//}

	//----------------------------------------------------------------------

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: instance.Namespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			"cwallet.sso": walletData,
		},
	}

	if err = controllerutil.SetControllerReference(instance, newSecret, scheme); err == nil {
		err = client.Create(ctx, newSecret)
		if err == nil {
			logTTEvent(ctx, client, instance, "Create", "Secret "+newSecret.Name+" created", false)
		} else {
			//Checks if the error was because of lack of permission, if not, return the original message
			var errorMsg, isPermissionsProblem = verifyUnauthorizedError(err.Error())
			logTTEvent(ctx, client, instance, "Create", "Could not create secret "+newSecret.Name+":"+errorMsg, true)
			if isPermissionsProblem {
				// There's nothing else to do
				updateTTClassicHighLevelState(ctx, instance, "Failed", client)
			} else {
				// Put it into ManualInterventionRequired; perhaps the user can fix it.
				updateTTClassicHighLevelState(ctx, instance, "ManualInterventionRequired", client)
			}

			return nil, err
		}
	} else {
		reqLogger.Error(err, fmt.Sprintf("%s: Could not set secret owner / controller: "+err.Error()))
		return nil, err
	}

	return &tts, nil
}

//----------------------------------------------------------------------
// This will read an existing Secret created by makeKey() which contains the credentials
// needed for communication between the Operator and the Agents for a
// particular TimesTenClassic object
//
// It should contain the certificates required for HTTPS, any userid/password
// combos required (for HTTP basic authentication, for example), and anything
// else that we want the Operator and a given set of Agents to have in common
// which nobody else should know.
//
// See also the makeKey function, which can create that info and store it
// in a Secret.
//
// fetchKey is required if, for example, the Operator is stopped or crashes
// and a new Operator takes over for it. The new Operator will use fetchKey
// to retrieve the credentials it needs to communicate with and control the
// existing and already running Agents.
//
// This function will return all the secrets to the caller so it can make
// immediate use of them.
//
// secretInfo, err := fetchKey(instance, r, reqLogger)
//----------------------------------------------------------------------

func fetchKey(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, scheme *runtime.Scheme,
	secretName string) (*TTSecretInfo, error) {
	us := "fetchKey"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info(us + " entered")

	var tts TTSecretInfo

	objUid := string(instance.ObjectMeta.UID)

	// Get the secret associated with this TimesTenClassic

	s, err := getSecret(ctx, client,
		types.NamespacedName{Namespace: instance.Namespace, Name: secretName})
	if err != nil {
		return nil, errors.New(us + ": could not fetch secret " + secretName + ": " + err.Error())
	}

	// Now reconstitute the TTSecretInfo from the contents of the Kubernetes Secret

	// Make the wallet

	walletBaseDir := "/tmp/" + objUid
	walletDir := "/tmp/" + objUid + "/.ttwallet." + universalInstanceGuid

	if err := safeMakeDir(walletDir, int(0700)); err != nil {
		reqLogger.Info(fmt.Sprintf("%s: mkdir %s failed with error: %v", us, walletDir, err.Error()))
		return nil, err
	} else {
		reqLogger.V(2).Info(fmt.Sprintf("%s: mkdir %s successful", us, walletDir))
	}

	walletData := s.Data["cwallet.sso"]
	walletFile := walletDir + "/cwallet.sso"
	err = ioutil.WriteFile(walletFile, walletData, 0600)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(walletBaseDir)

	timestenHome, ok := os.LookupEnv("TIMESTEN_HOME")
	if ok {
		reqLogger.V(2).Info(fmt.Sprintf("%s: timestenHome=%s", us, timestenHome))
	} else {
		e := errors.New("Could not find the operator's TimesTen instance")
		reqLogger.Error(e, "Could not find the operator's TimesTen instance")
		panic(e)
	}

	// Now that the wallet file exists let's fetch the data back out of it

	getOne := func(what string) (string, error) {
		getRc, getOut, getErr := runShellCommand(ctx, []string{timestenHome + "/bin/ttUser", "-zzwallet", "/tmp/" + objUid, "-zzget", what})
		if getRc != 0 {
			errs := strings.Join(getErr, " / ")
			var err error
			if strings.Contains(errs, "key not found") {
				err = errors.New(us + ": key not found")
				reqLogger.V(1).Info(what + " not found")
			} else {
				err = errors.New(us + ": rc " + strconv.Itoa(getRc) + " fetching " + what + " from wallet")
				reqLogger.Error(err, "Error running ttUser zzwallet", "abc", "def", "stdout", strings.Join(getOut, " / "), "stderr", errs)
			}
			return "", err
		}
		return strings.ReplaceAll(getOut[0], "\n", ""), nil
	}

	tts.AgentUID, err = getOne("AgentUID")
	if err != nil {
		return nil, err
	}

	tts.AgentPWD, err = getOne("AgentPWD")
	if err != nil {
		return nil, err
	}

	tts.HttpUID, err = getOne("HttpUID")
	if err != nil {
		return nil, err
	}

	tts.HttpPWD, err = getOne("HttpPWD")
	if err != nil {
		return nil, err
	}

	var certCoded string
	for i := 0; ; i++ {
		d, err := getOne("CERT_" + strconv.Itoa(i))
		if err == nil {
			certCoded = certCoded + d
		} else {
			break
		}
	}
	certData, err := base64.StdEncoding.DecodeString(certCoded)
	if err != nil {
		reqLogger.Error(err, us+": could not base64 decode cert data")
		os.Exit(10)
	}

	certFile := "/tmp/" + secretName + ".cert"
	err = ioutil.WriteFile(certFile, certData, 0600)
	if err != nil {
		reqLogger.Error(err, us+": could not write "+certFile)
		os.Exit(8)
	}

	var pkCoded string
	for i := 0; ; i++ {
		d, err := getOne("PK_" + strconv.Itoa(i))
		if err == nil {
			pkCoded = pkCoded + d
		} else {
			break
		}
	}
	pkData, err := base64.StdEncoding.DecodeString(pkCoded)

	if err != nil {
		reqLogger.Error(err, us+": could not base64 decode pk data")
		os.Exit(11)
	}

	pkFile := "/tmp/" + secretName + ".priv"
	err = ioutil.WriteFile(pkFile, pkData, 0600)
	if err != nil {
		reqLogger.Error(err, us+": could not write "+pkFile)
		os.Exit(9)
	}

	cc, err, errmsg := addCertToRootCA(ctx, "/tmp/", secretName, certData)
	if err != nil {
		return nil, errors.New(us + ": could not make client cert: '" + errmsg + "': " + err.Error())
	}
	tts.ClientCert = *cc

	return &tts, nil
}

// ----------------------------------------------------------------------
// getKey is what most folks should actually use. It will:
//   - Look in the ttSecrets map to see if we already have the
//     TTSecretInfo struct for a given TimesTenClassic, and
//     return it if it's there
//   - If it's not there it'll try to fetch it from a pre-existing
//     Kubernetes Secret using fetchKey()
//   - And if there isn't one then it'll make such a Secret
//     using makeKey()
//
// secretInfo, err := getKey(instance, r, reqLogger)
func getKey(ctx context.Context, instance *timestenv2.TimesTenClassic, client client.Client, scheme *runtime.Scheme, secretName string) (*TTSecretInfo, error) {
	us := "getKey"
	reqLogger := log.FromContext(ctx)
	objUid := string(instance.ObjectMeta.UID)
	reqLogger.V(1).Info(fmt.Sprintf("%s: this is objUid %v", us, objUid))

	tts, found := ttSecrets.secrets[secretName]
	if found {
		reqLogger.V(2).Info(fmt.Sprintf("%s secretName %v found in ttSecrets", us, secretName))
		return &tts, nil
	} else {
		reqLogger.V(1).Info(fmt.Sprintf("%s secretName %v not in ttSecrets, trying fetchKey", us, secretName))
		tts, err := fetchKey(ctx, instance, client, scheme, secretName)
		if err == nil {
			reqLogger.V(1).Info(fmt.Sprintf("%s: fetchKey found secretName %s for objUid %s", us, secretName, objUid))
			ttSecrets.addSecret(ctx, secretName, *tts, instance.ObjectNamespace(), instance.ObjectName())
			return tts, nil
		} else {
			reqLogger.V(1).Info(us + ": fetchKey could not find secret, calling makeKey, err was " + err.Error())
			tts, err = makeKey(ctx, instance, client, scheme, secretName)
			if err == nil {
				reqLogger.V(1).Info(fmt.Sprintf("%s makeKey created secret %s for objUid %s", us, secretName, objUid))
				ttSecrets.addSecret(ctx, secretName, *tts, instance.ObjectNamespace(), instance.ObjectName())
				return tts, nil
			} else {
				reqLogger.Error(err, us+" makeKey failed to generate secret for objUid "+objUid+" : "+err.Error())
				return nil, err
			}
		}
	}
}

// makeSshKeypair - make an rsa key for ssh purposes
// err, privkey, pubkey = makeSshKeypair()
func makeSshKeypair() (error, []byte, []byte) {
	defer func() {
		_ = os.Remove("/tmp/z")
		_ = os.Remove("/tmp/z.pub")
	}()

	_ = os.Remove("/tmp/z")
	_ = os.Remove("/tmp/z.pub")

	genArgs := []string{"-q", "-f", "/tmp/z"}
	genCmd := exec.Command("ssh-keygen", genArgs...)
	genIn, err := genCmd.StdinPipe()
	if err != nil {
		return err, nil, nil
	}
	go func() {
		io.WriteString(genIn, "\n")
		defer genIn.Close()
	}()
	err = genCmd.Run()
	if err != nil {
		return err, nil, nil
	}

	priv, err := ioutil.ReadFile("/tmp/z")
	if err != nil {
		return err, nil, nil
	}

	pub, err := ioutil.ReadFile("/tmp/z.pub")
	if err != nil {
		return err, nil, nil
	}

	return nil, priv, pub
}

type TTCert struct {
	wallet     []byte
	serverCert []byte
	serverKey  []byte
	caCert     []byte
	caKey      []byte
	clientCert []byte
	clientKey  []byte
}

// makeExporterCert - make a certificate for use by the ttExporter
// and similar things
// err, wallet, clientCert, clientKey := makeExporterCert(ctx, "*.xyz.default.svc.cluster.local")
func makeExporterCert(ctx context.Context, cn string) (error, TTCert) {
	us := "makeExporterCert"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))

	defer reqLogger.V(2).Info(fmt.Sprintf("%s returns", us))

	var c TTCert

	dir, _ := os.MkdirTemp("", "makeExporterCert")
	//defer os.RemoveAll(dir)

	rel, err := GetTTMajorRelease(ctx)
	if err != nil {
		reqLogger.V(1).Info(fmt.Sprintf("%s could not determine TimesTen release", us))
		return err, c
	}

	if rel < 22 {
		msg := fmt.Sprintf("%s: not supported on TimesTen %d", us, rel)
		reqLogger.V(1).Info(msg)
		return errors.New(msg), c
	}
	timestenHome, _ := os.LookupEnv("TIMESTEN_HOME")
	makeCmd := []string{
		timestenHome + "/bin/ttenv",
		"ttExporter",
		"-create-server-certificate",
		"-certificate-common-name", cn,
		"-certificate-alt-names", cn,
		"-certificate-directory", dir,
	}

	rc, _ /* stdout */, stderr := runShellCommand(ctx, makeCmd)
	if rc != 0 {
		var msg string
		if len(stderr) > 0 {
			msg = fmt.Sprintf("%s: error %d creating certificate: %s", us, rc, stderr[0])
		} else {
			msg = fmt.Sprintf("%s: error %d creating certificate: no stderr", us, rc)
		}
		reqLogger.V(1).Info(msg)
		return errors.New(msg), c
	}

	// Make the client cert too

	clientDir, _ := os.MkdirTemp("", "makeExporterCertClient")
	//defer os.RemoveAll(clientDir)

	makeClientCmd := []string{
		timestenHome + "/bin/ttenv",
		"ttExporter",
		"-export-client-certificate",
		clientDir + "/client.crt",
		"-export-client-private-key",
		clientDir + "/client.key",
		"-certificate-directory", dir,
	}

	rc, _ /* stdout */, stderr = runShellCommand(ctx, makeClientCmd)
	if rc != 0 {
		var msg string
		if len(stderr) > 0 {
			msg = fmt.Sprintf("%s: error %d creating client certificate: %s", us, rc, stderr[0])
		} else {
			msg = fmt.Sprintf("%s: error %d creating client certificate: no stderr", us, rc)
		}
		reqLogger.V(1).Info(msg)
		return errors.New(msg), c
	}

	defer os.Remove(clientDir + "/client.crt")
	defer os.Remove(clientDir + "/client.key")

	// Sadly we need to rummage around a bit to find the actual wallet.

	walletFile := "/tmp"
	f, _ := os.ReadDir(dir)
	foundWalletFile := false
	for _, d := range f {
		if strings.HasPrefix(d.Name(), ".tt") {
			walletFile = fmt.Sprintf("%s/%s/cwallet.sso", dir, d.Name())
			foundWalletFile = true
		}
	}

	if foundWalletFile == false {
		reqLogger.V(1).Info(fmt.Sprintf("%s: Could not find wallet file in /tmp", us))
		return errors.New("Could not find wallet file in /tmp"), c
	}

	c.wallet, err = ioutil.ReadFile(walletFile)
	if err != nil {
		reqLogger.V(1).Info(fmt.Sprintf("%s: could not read %s: %s", us, walletFile, err.Error()))
		return err, c
	}

	err, c.serverCert = fetchWallet(reqLogger, dir, "SERVERCERT")
	if err != nil {
		reqLogger.V(1).Error(err, "Could not fetch SERVERCERT from wallet")
		return err, c
	}

	err, c.serverKey = fetchWallet(reqLogger, dir, "SERVERKEY")
	if err != nil {
		reqLogger.V(1).Error(err, "Could not fetch SERVERKEY from wallet")
		return err, c
	}

	err, c.caCert = fetchWallet(reqLogger, dir, "CACERT")
	if err != nil {
		reqLogger.V(1).Error(err, "Could not fetch CACERT from wallet")
		return err, c
	}

	err, c.caKey = fetchWallet(reqLogger, dir, "CAKEY")
	if err != nil {
		reqLogger.V(1).Error(err, "Could not fetch CAKEY from wallet")
		return err, c
	}

	ccert, err := ioutil.ReadFile(clientDir + "/client.crt")
	if err != nil {
		reqLogger.V(1).Error(err, "Could not read /tmp/client.crt")
		return err, c
	}
	c.clientCert = ccert

	ckey, err := ioutil.ReadFile(clientDir + "/client.key")
	if err != nil {
		reqLogger.V(1).Error(err, "Could not read /tmp/client.key")
		return err, c
	}
	c.clientKey = ckey

	return err, c
}

// makeExporterSecrets - make secrets for the exporter and Prometheus
// err, clientSecretName := makeExporterSecrets(ctx, instance, r)
func makeExporterSecrets(ctx context.Context, instance timestenv2.TimesTenObject, scheme *runtime.Scheme, client client.Client,
	serverSecretName string) (error, string) {
	makeExporterSecretsMutex.Lock()

	us := "makeExporterSecrets"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))

	defer func() {
		makeExporterSecretsMutex.Unlock()
		reqLogger.V(2).Info(fmt.Sprintf("%s returns", us))
	}()

	cn := fmt.Sprintf("*.%s.%s.svc.cluster.local", instance.ObjectName(), instance.ObjectNamespace())

	err, cert := makeExporterCert(ctx, cn)
	if err != nil {
		reqLogger.V(1).Info(fmt.Sprintf("%s: makeExporterCert returned %s", us, err.Error()))
		return err, ""
	}

	// This secret is used so the operator can remember the cert across restarts / multiple
	// operator instances. The CUSTOMER can also provide such a Secret if they would prefer to
	// control the certificate used.
	//serverSecretName := fmt.Sprintf("%s-metrics", instance.ObjectName())

	serverSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: instance.ObjectNamespace(),
			Name:      serverSecretName,
		},
		Data: map[string][]byte{
			"cwallet.sso": cert.wallet,
		},
	}

	switch typ := instance.(type) {
	case *timestenv2.TimesTenClassic:
		if err = controllerutil.SetControllerReference(typ, serverSecret, scheme); err != nil {
			reqLogger.V(1).Info(us + ": Could not set secret owner / controller: " + err.Error())
			return err, ""
		}
	default:
		msg := fmt.Sprintf("%s: instance type unexpected!", us)
		reqLogger.V(1).Info(msg)
		return errors.New(msg), ""
	}

	err = client.Create(ctx, serverSecret)
	if err == nil {
		logTTEvent(ctx, client, instance, "Create", "Secret "+serverSecretName+" created", false)
	} else {

		//Checks if the error was because of lack of permission, if not, return the original message
		var errorMsg, _ = verifyUnauthorizedError(err.Error())
		logTTEvent(ctx, client, instance, "Create", "Could not create secret "+serverSecretName+":"+errorMsg, true)
		return err, ""
	}

	clientSecretName := fmt.Sprintf("%s-metrics-client", instance.ObjectName())

	clientSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: instance.ObjectNamespace(),
			Name:      clientSecretName,
		},
		Data: map[string][]byte{
			"client.crt": []byte(cert.clientCert),
			"client.key": []byte(cert.clientKey),
			"ca.crt":     []byte(cert.caCert),
		},
	}

	switch typ := instance.(type) {
	case *timestenv2.TimesTenClassic:
		if err = controllerutil.SetControllerReference(typ, clientSecret, scheme); err != nil {
			reqLogger.V(1).Info(us + ": Could not set secret owner / controller: " + err.Error())
			return err, ""
		}
	default:
		msg := fmt.Sprintf("%s: instance type unexpected!", us)
		reqLogger.V(1).Info(msg)
		return errors.New(msg), ""
	}

	err = client.Create(ctx, clientSecret)
	if err == nil {
		logTTEvent(ctx, client, instance, "Create", "Secret "+clientSecretName+" created", false)
	} else {
		//Checks if the error was because of lack of permission, if not, return the original message
		var errorMsg, _ = verifyUnauthorizedError(err.Error())
		logTTEvent(ctx, client, instance, "Create", "Could not create secret "+clientSecretName+":"+errorMsg, true)
		return err, ""
	}

	return nil, clientSecretName
}

// Get TimesTen instance GUID
func getInstanceGuid(home string) (error, string) {

	ttConf := home + "/conf/timesten.conf"
	fd, err := os.Open(ttConf)
	if err != nil {
		return err, ""
	}

	defer fd.Close()
	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		line := scanner.Text()
		tok := strings.Split(line, "=")
		if tok[0] == "instance_guid" {
			return nil, tok[1]
		}
	}
	return errors.New("Could not find instance_guid in " + ttConf), ""
}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
