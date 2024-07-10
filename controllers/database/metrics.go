// Copyright (c) 2019-2020, Oracle and/or its affiliates. All rights reserved.
//
// This is the main body of the TimesTen Kubernetes Operator

package controllers

import (
	"context"
	"errors"
	"fmt"
	"os"

	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	promop "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prom "github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	//"sync"

	// Added for https metrics
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//----------------------------------------------------------------------
// METRICS
//----------------------------------------------------------------------

// Define the metrics for TimesTenClassic objects that the Operator
// will export

var (
	promGaugeTTCNormal = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "classic",
			Name:      "state_normal",
			Help:      "Replicated TimesTenClassic in 'Normal' state",
		},
		[]string{
			"name",
		},
	)
	promGaugeTTCNotNormal = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "classic",
			Name:      "state_not_normal",
			Help:      "Replicated TimesTenClassic NOT in 'Normal' state",
		},
		[]string{
			"name",
		},
	)
	promGaugeTTCState = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "classic",
			Name:      "state",
			Help:      "TimesTenClassic high level state",
		},
		[]string{
			"name",
			"state",
		},
	)

	promGaugeTTSNormal = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "scaleout",
			Name:      "state_normal",
			Help:      "TimesTenScaleout in 'Normal' state",
		},
		[]string{
			"name",
		},
	)

	promGaugeTTSNotNormal = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "scaleout",
			Name:      "state_not_normal",
			Help:      "TimesTenScaleout NOT in 'Normal' state",
		},
		[]string{
			"name",
		},
	)

	promGaugeTTSState = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "scaleout",
			Name:      "state",
			Help:      "TimesTenScaleout high level state",
		},
		[]string{
			"name",
			"state",
		},
	)

	promGaugeTTCAllReplicasReady = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "classic",
			Name:      "state_all_replicas_ready",
			Help:      "Number of non-replicated TimesTenClassic objects in AllReplicasReady state",
		},
		[]string{
			"name",
		},
	)

	promGaugeTTCNotAllReplicasReady = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "classic",
			Name:      "state_not_all_replicas_ready",
			Help:      "Number of non-replicated TimesTenClassic objects in states other than “AllReplicasReady” or “Initializing",
		},
		[]string{
			"name",
		},
	)

	promGaugeTTCSomeReplicasReady = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "classic",
			Name:      "state_some_replicas_ready",
			Help:      "Number of non-replicated TimesTenClassic objects in SomeReplicasReady state",
		},
		[]string{
			"name",
		},
	)

	promGaugeTTCNoReplicasReady = prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "timesten",
			Subsystem: "classic",
			Name:      "state_no_replicas_ready",
			Help:      "Number of non-replicated TimesTenClassic objects in NoReplicasReady  state",
		},
		[]string{
			"name",
		},
	)
)

// init() functions are called before main() and can be used for initialization
// NOTE that this is called so early in initialization that logging doesn't work!
func init() {
	registerMetrics()
}

func registerMetrics() error {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(promGaugeTTCNormal, promGaugeTTCNotNormal, promGaugeTTCState)
	metrics.Registry.MustRegister(promGaugeTTSNormal, promGaugeTTSNotNormal, promGaugeTTSState)
	metrics.Registry.MustRegister(promGaugeTTCAllReplicasReady, promGaugeTTCNotAllReplicasReady, promGaugeTTCSomeReplicasReady, promGaugeTTCNoReplicasReady)
	return nil
}

func updateClassicMetrics(ctx context.Context, instance *timestenv2.TimesTenClassic) error {
	us := "updateClassicMetrics"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	nReps := getNReplicas(instance)
	replicated := isReplicated(instance)

	if instance.Status.PrevHighLevelState != "" {
		promGaugeTTCState.WithLabelValues(instance.Name, instance.Status.PrevHighLevelState).Set(0.0)
	}
	promGaugeTTCState.WithLabelValues(instance.Name, instance.Status.HighLevelState).Set(1.0)

	if replicated {
		if instance.Status.HighLevelState == "Normal" {
			promGaugeTTCNormal.WithLabelValues(instance.Name).Set(1.0)
			promGaugeTTCNotNormal.WithLabelValues(instance.Name).Set(0.0)
		} else {
			promGaugeTTCNormal.WithLabelValues(instance.Name).Set(0.0)
			if instance.Status.HighLevelState != "Initializing" {
				promGaugeTTCNotNormal.WithLabelValues(instance.Name).Set(1.0)
			}
		}
	} else {
		if instance.Status.HighLevelState == "AllReplicasReady" {
			promGaugeTTCAllReplicasReady.WithLabelValues(instance.Name).Set(float64(nReps))
			promGaugeTTCNotAllReplicasReady.WithLabelValues(instance.Name).Set(0.0)
			promGaugeTTCSomeReplicasReady.WithLabelValues(instance.Name).Set(0.0)
			promGaugeTTCNoReplicasReady.WithLabelValues(instance.Name).Set(0.0)
		} else {
			promGaugeTTCAllReplicasReady.WithLabelValues(instance.Name).Set(0.0)
			if instance.Status.HighLevelState == "SomeReplicasReady" {
				promGaugeTTCSomeReplicasReady.WithLabelValues(instance.Name).Set(float64(nReps))
			} else {
				promGaugeTTCSomeReplicasReady.WithLabelValues(instance.Name).Set(0.0)
				if instance.Status.HighLevelState == "NoReplicasReady" {
					promGaugeTTCNoReplicasReady.WithLabelValues(instance.Name).Set(float64(nReps))
				} else {
					promGaugeTTCNoReplicasReady.WithLabelValues(instance.Name).Set(0.0)
					if instance.Status.HighLevelState != "Initializing" {
						promGaugeTTCNotAllReplicasReady.WithLabelValues(instance.Name).Set(float64(nReps))
					} else {
						promGaugeTTCNotAllReplicasReady.WithLabelValues(instance.Name).Set(0.0)
					}
				}
			}
		}
	}
	return nil
}

func deleteClassicMetrics(ctx context.Context, name string) error {
	us := "deleteClassicMetrics"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	for s, _ := range ClassicHLStates {
		//reqLogger.V(2).Info(fmt.Sprintf("Deleting %s : %s!", name, s))
		promGaugeTTCState.Delete(prom.Labels{"name": name, "state": s})
	}
	promGaugeTTCNormal.Delete(prom.Labels{"name": name})
	promGaugeTTCNotNormal.Delete(prom.Labels{"name": name})

	return nil
}

// createOperatorServiceMonitor(...) - create a ServiceMonitor to cause Prometheus
// to scrape the Operator metrics
func createOperatorServiceMonitor(s *TTServiceServer) error {
	us := "createOperatorServiceMonitor"
	reqLogger := log.FromContext(s.ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	ourScheme := *s.scheme

	// Does the ServiceMonitor already exist? Great if so!

	oldsm := &promop.ServiceMonitor{}
	err := s.client.Get(s.ctx, types.NamespacedName{Name: s.deploy.ObjectMeta.Name, Namespace: s.deploy.ObjectMeta.Namespace}, oldsm)
	if err == nil {
		reqLogger.V(1).Info(fmt.Sprintf("Operator ServiceMonitor already exists"))
		return nil
	}

	clientSecretName := s.deploy.ObjectMeta.Name + "-metrics-client"

	labelMap := map[string]string{
		"app":                 s.deploy.ObjectMeta.Name,
		"database.oracle.com": s.deploy.ObjectMeta.Name,
	}
	for k, v := range s.deploy.ObjectMeta.Labels {
		labelMap[k] = v
	}

	ourAnnotations := s.deploy.ObjectMeta.Annotations

	sm := &promop.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceMonitor",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        s.deploy.ObjectMeta.Name,
			Namespace:   s.deploy.ObjectMeta.Namespace,
			Labels:      labelMap,
			Annotations: ourAnnotations,
		},
		Spec: promop.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": s.deploy.ObjectMeta.Name,
				},
			},
			Endpoints: []promop.Endpoint{
				{
					Port:     "metrics",
					Path:     "/metrics",
					Scheme:   s.metricsHttpScheme,
					Interval: "15s",
				},
			},
		},
	}

	if s.metricsHttpScheme == "https" {
		sm.Spec.Endpoints[0].TLSConfig = &promop.TLSConfig{
			SafeTLSConfig: promop.SafeTLSConfig{
				ServerName: fmt.Sprintf("%s.%s.svc.cluster.local", s.deploy.ObjectMeta.Name, s.deploy.ObjectMeta.Namespace),
				Cert: promop.SecretOrConfigMap{
					Secret: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: clientSecretName,
						},
						Key: "client.crt",
					},
				},
				CA: promop.SecretOrConfigMap{
					Secret: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: clientSecretName,
						},
						Key: "ca.crt",
					},
				},
				KeySecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: clientSecretName,
					},
					Key: "client.key",
				},
			},
		}
	}

	if err = controllerutil.SetControllerReference(s.deploy, sm, &ourScheme); err != nil {
		reqLogger.V(1).Info(us + ": Could not set ServiceMonitor owner / controller: " + err.Error())
		return err
	}

	err = s.client.Create(s.ctx, sm)
	if err != nil {
		reqLogger.Error(err, "Could not create operator ServiceMonitor")
		return err
	}

	return nil
}

// shouldTTExporterRun - Should the exporter sidecar be provisioned?
func shouldTTExporterRun(ctx context.Context, instance timestenv2.TimesTenObject, client client.Client) bool {
	us := "shouldTTExporterRun"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	ttRel, _ := GetTTMajorRelease(ctx)

	prom := instance.GetPrometheus()

	// If the user explicitly says 'yes' or 'no', then do what they said

	if prom != nil {
		if prom.Publish != nil {
			if *prom.Publish && ttRel == 18 {
				logTTEvent(ctx, client, instance, "Create", "Not starting prometheus exporter on TimesTen 18", true)
				return false
			}
			return *prom.Publish
		}

		// If the user specified ANY other prometheus options, then they
		// are asking us to run the exporter

		if prom.Port != nil ||
			prom.Insecure != nil ||
			prom.LimitRate != nil ||
			prom.CertSecret != nil ||
			prom.CreatePodMonitors != nil {
			if ttRel == 18 {
				logTTEvent(ctx, client, instance, "Create", "Not starting prometheus exporter on TimesTen 18", true)
				return false
			}
			return true
		}

		// We should never get here (unless someone has added a new field to
		// Prometheus and hasn't updated this code). SOMETHING had to be
		// specified in .spec.ttspec.Prometheus to get it filled in.
		//
		// In any case, if this happens we'll handle it as though the user
		// had NOT specified a "prometheus" clause at all.
	}

	// The "prometheus" clause wasn't specified (or was empty).  What to do?
	//
	// If the user doesn't say, then ... has the Prometheus Operator been
	// installed in the cluster? If so then we presume that we should publish
	// metrics, unless told not to. If it wasn't then we won't.
	//
	// This preserves upwards compatibility with pre-phase 7, and is pretty
	// reasonable.

	if ttRel == 18 {
		return false // Silently, since the user never told us to
	}

	pmExists := os.Getenv("EXPOSE_METRICS")
	if pmExists == "1" {
		return true
	}
	return false
}

// getOperatorService - fetch a pre-existing Service for Operator metrics and probes
// if one exists. Create one if not.
func getOperatorService(s *TTServiceServer) error {
	us := "getOperatorService"
	reqLogger := log.FromContext(s.ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	ns := os.Getenv("WATCH_NAMESPACE")
	deployName := os.Getenv("OPERATOR_NAME")

	// Find our deployment

	s.deploy = &appsv1.Deployment{}
	err := s.client.Get(s.ctx, types.NamespacedName{Name: deployName, Namespace: ns}, s.deploy)
	if err != nil {
		reqLogger.V(1).Info(fmt.Sprintf("Could not fetch operator deployment '%s'. Operator service '%s' will not be created", deployName, deployName))
		return err
	}

	// Fetch the pre-existing service (if any)

	svc := &corev1.Service{}
	err = s.client.Get(s.ctx, types.NamespacedName{Name: deployName, Namespace: ns}, svc)
	if err != nil {
		reqLogger.V(1).Info(fmt.Sprintf("Operator service %s.%s does not exist", ns, deployName))

		err = createOperatorService(s)
		if err != nil {
			reqLogger.V(1).Error(err, "Could not create operator service")
			return err
		}
		return nil
	}

	return err
}

// createOperatorService(...) - create a Service to expose Operator metrics and probes
// If we aren't the first operator instance in this deployment we may find it's already
// there.
func createOperatorService(s *TTServiceServer) error {
	us := "createOperatorService"
	reqLogger := log.FromContext(s.ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	ns := os.Getenv("WATCH_NAMESPACE")
	deployName := os.Getenv("OPERATOR_NAME")
	exposeMetrics := os.Getenv("EXPOSE_METRICS")
	exposeProbes := os.Getenv("EXPOSE_PROBES")

	// Let's go create one

	makeService := false

	if exposeMetrics == "1" || exposeProbes == "1" {
		makeService = true
	}

	if makeService == false {
		return errors.New("No need to create service")
	}

	labelMap := map[string]string{
		"app":                 deployName,
		"database.oracle.com": deployName,
	}
	for k, v := range s.deploy.ObjectMeta.Labels {
		labelMap[k] = v
	}

	srv := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        deployName,
			Namespace:   ns,
			Labels:      labelMap,
			Annotations: s.deploy.ObjectMeta.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports:    []corev1.ServicePort{},
			Selector: s.deploy.Spec.Template.ObjectMeta.Labels,
		},
	}

	if exposeMetrics == "1" {
		srv.Spec.Ports = append(srv.Spec.Ports,
			corev1.ServicePort{
				Name:       "metrics",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
				Protocol:   corev1.ProtocolTCP})
	}

	if exposeProbes == "1" {
		srv.Spec.Ports = append(srv.Spec.Ports,
			corev1.ServicePort{
				Name:       "probe",
				Port:       8081,
				TargetPort: intstr.FromInt(8081),
				Protocol:   corev1.ProtocolTCP})
	}

	if err := controllerutil.SetControllerReference(s.deploy, srv, s.scheme); err != nil {
		reqLogger.V(1).Info(us + ": Could not set service owner / controller: " + err.Error())
		return err
	}

	err := s.client.Create(s.ctx, srv)
	if err != nil {
		reqLogger.Error(err, "Could not create operator service: "+err.Error())
		return err
	}

	reqLogger.V(1).Info(fmt.Sprintf("Operator service '%s' created", deployName))

	return nil
}

// checkPodMonitor(...) - see if our PodMonitor exists and create it if it does not
func checkPodMonitor(ctx context.Context, instance timestenv2.TimesTenObject, scheme *runtime.Scheme, client client.Client) error {
	us := "checkPodMonitor"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	oldpm := &promop.PodMonitor{}
	err := client.Get(ctx, types.NamespacedName{Name: instance.ObjectName(), Namespace: instance.ObjectNamespace()}, oldpm)
	if err == nil {
		reqLogger.V(2).Info(fmt.Sprintf("PodMonitor %s.%s already exists", instance.ObjectNamespace(), instance.ObjectName()))
		return nil
	} else {
		reqLogger.V(2).Info(fmt.Sprintf("Error getting PodMonitor %s.%s: %s", instance.ObjectNamespace(), instance.ObjectName(), err.Error()))
	}

	err = createPodMonitor(ctx, instance, scheme, client)
	return err
}

// createPodMonitor(...) - create a PodMonitor for a TimesTen object
func createPodMonitor(ctx context.Context, instance timestenv2.TimesTenObject, scheme *runtime.Scheme, client client.Client) error {
	us := "createPodMonitor"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	ourScheme := *scheme

	name := instance.ObjectName()

	prom := instance.GetPrometheus()

	var err error

	labelMap := map[string]string{
		"app":                 instance.ObjectName(),
		"database.oracle.com": instance.ObjectName(),
	}
	for k, v := range instance.ObjectLabels() {
		labelMap[k] = v
	}

	ourAnnotations := instance.ObjectAnnotations()

	pm := &promop.PodMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodMonitor",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.ObjectName(),
			Namespace:   instance.ObjectNamespace(),
			Labels:      labelMap,
			Annotations: ourAnnotations,
		},
		Spec: promop.PodMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"database.oracle.com": name,
				},
			},
			PodMetricsEndpoints: []promop.PodMetricsEndpoint{
				{
					Port:     "exporter",
					Path:     "/metrics",
					Interval: "15s",
				},
			},
		},
	}

	if prom != nil && prom.Insecure != nil && *prom.Insecure == true {
		// TLS is NOT being used
		pm.Spec.PodMetricsEndpoints[0].Scheme = "http"
	} else {
		// TLS is being used
		secretName := name + "-metrics-client"
		pm.Spec.PodMetricsEndpoints[0].Scheme = "https"
		pm.Spec.PodMetricsEndpoints[0].TLSConfig = &promop.PodMetricsEndpointTLSConfig{
			SafeTLSConfig: promop.SafeTLSConfig{
				ServerName: fmt.Sprintf("%s.%s.%s.svc.cluster.local", instance.ObjectName(), instance.ObjectName(), instance.ObjectNamespace()),
				Cert: promop.SecretOrConfigMap{
					Secret: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
						Key: "client.crt",
					},
				},
				CA: promop.SecretOrConfigMap{
					Secret: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
						Key: "ca.crt",
					},
				},
				KeySecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: "client.key",
				},
			},
		}
	}

	switch typ := instance.(type) {
	case *timestenv2.TimesTenClassic:
		if err = controllerutil.SetControllerReference(typ, pm, &ourScheme); err != nil {
			reqLogger.V(1).Info(us + ": Could not set PodMonitor owner / controller: " + err.Error())
			return err
		}
	default:
		msg := fmt.Sprintf("%s: instance type unexpected!", us)
		reqLogger.V(1).Info(msg)
		return errors.New(msg)
	}

	reqLogger.V(2).Info(fmt.Sprintf("%+v", pm))

	err = client.Create(ctx, pm)
	if err == nil {
		logTTEvent(ctx, client, instance, "Create", fmt.Sprintf("PodMonitor %s created", pm.Name), true)
	} else {
		// If the Prometheus Operator isn't installed then give a more gentle message.
		if strings.Contains(err.Error(), "the server could not find the requested resource") {
			msg := fmt.Sprintf("Prometheus Operator not installed")
			reqLogger.V(1).Info(msg)
			return errors.New(msg)
		}

		//Checks if the error was because of lack of permission, if not, return the original message
		var errorMsg, _ = verifyUnauthorizedError(err.Error())
		logTTEvent(ctx, client, instance, "FailedCreate", "Could not create PodMonitor "+errorMsg, true)
		return err
	}

	return nil
}

//----------------------------------------------------------------------

//////////////////////////////////////////////////////////////////////
// TTServiceServer below this line.
//
// This Runnable is started up if/when this operator becomes the
// 'active'. It will set up a Service to expose metrics and/or
// probes from the operator itself.
//////////////////////////////////////////////////////////////////////

type TTServiceServer struct {
	client            client.Client
	ctx               context.Context
	scheme            *runtime.Scheme
	metricsHttpScheme string
	metricsPort       int
	deploy            *appsv1.Deployment
	cert              TTCert
}

func (s TTServiceServer) New(client client.Client, scheme *runtime.Scheme, metricsHttpScheme string, metricsPort int) (*TTServiceServer, error) {
	newServer := TTServiceServer{}
	newServer.client = client
	newServer.ctx = context.Background()
	newServer.scheme = scheme
	newServer.metricsHttpScheme = metricsHttpScheme
	newServer.metricsPort = metricsPort
	newServer.deploy = nil
	return &newServer, nil
}

func (s TTServiceServer) Start(ctx context.Context) error {
	us := "TTServiceServer.Start"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(us + " entered")
	defer reqLogger.V(2).Info(us + " returns")

	go s.Server()

	return nil
}

// This is set to true when we become the leader. It's used
// by operator readiness probes.

var WeAreTheLeader bool

func setLeader(x bool) {
	WeAreTheLeader = x
}

func (s *TTServiceServer) Server() error {
	reqLogger := log.FromContext(s.ctx)
	us := "TTServiceServer.Server"
	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))
	defer reqLogger.V(2).Info(fmt.Sprintf("%s returns", us))

	defer setLeader(true) // Ready!

	exposeMetrics := os.Getenv("EXPOSE_METRICS")
	exposeProbes := os.Getenv("EXPOSE_PROBES")
	createServiceMonitor := os.Getenv("CREATE_SERVICEMONITOR")
	metricsScheme := os.Getenv("METRICS_SCHEME")

	var err error

	if exposeMetrics != "1" && exposeProbes != "1" {
		return nil
	}

	err = getOperatorService(s) // Creates one if it doesn't exist
	if err != nil {
		reqLogger.V(1).Error(err, "Error getting/creating operator metrics service")
		return err
	}

	if exposeMetrics == "1" && metricsScheme == "https" {
		err = setupOperatorExporterSecrets(s)
		if err != nil {
			reqLogger.Error(err, "Could not create operator metrics secret: "+err.Error())
			return err
		}
	}

	if createServiceMonitor == "1" {
		if exposeMetrics == "1" {
			err = createOperatorServiceMonitor(s)
			if err != nil {
				reqLogger.Error(err, "Could not create operator ServiceMonitor: "+err.Error())
				return err
			}

			reqLogger.V(1).Info(fmt.Sprintf("Operator ServiceMonitor '%s' created", s.deploy.ObjectMeta.Name))
		} else {
			reqLogger.V(1).Info("Operator ServiceMonitor not created: EXPOSE_METRICS not set by user")
		}
	}

	err = StartTLSMetricsServer(reqLogger, s)
	if err != nil {
		reqLogger.V(1).Error(err, "Error starting TLS metrics server")
	}

	return err
}

// setupOperatorExporterSecret - make (or get) secrets for the operator metrics exporter and Prometheus
// err, serverSecretName, clientSecretName := setupOperatorExporterSecret(ctx, s)
func setupOperatorExporterSecrets(s *TTServiceServer) error {
	us := "setupOperatorExporterSecrets"
	reqLogger := log.FromContext(s.ctx)
	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))
	defer reqLogger.V(2).Info(fmt.Sprintf("%s returns", us))

	// Another operator in our deployment may have already created the secret.
	// If so then we need to use it.

	metricsServerSecretName := fmt.Sprintf("%s-metrics", s.deploy.ObjectMeta.Name)

	var err error

	metricsServerSecret, err := getSecret(s.ctx, s.client,
		types.NamespacedName{Namespace: s.deploy.ObjectMeta.Namespace, Name: metricsServerSecretName})

	if err == nil {
		// Found a pre-existing secret
		s.cert.wallet = metricsServerSecret.Data["cwallet.sso"]

		// Write the wallet into a file, then fetch the CA cert and key out of it

		err, guid := getInstanceGuid("/timesten/instance1")
		if err != nil {
			return err
		}

		walDir, _ := os.MkdirTemp("", "metrics")
		//defer os.RemoveAll(walDir)

		tempdir := walDir + "/.ttwallet." + guid
		err = os.Mkdir(tempdir, 0700)
		if err != nil {
			return err
		}
		//defer os.RemoveAll(tempdir)

		err = os.WriteFile(tempdir+"/cwallet.sso", s.cert.wallet, 0700)
		if err != nil {
			return err
		}

		err, s.cert.serverCert = fetchWallet(reqLogger, walDir, "SERVERCERT")
		if err != nil {
			return err
		}

		err, s.cert.serverKey = fetchWallet(reqLogger, walDir, "SERVERKEY")
		if err != nil {
			return err
		}

		err, s.cert.caCert = fetchWallet(reqLogger, walDir, "CACERT")
		if err != nil {
			return err
		}

		err, s.cert.caKey = fetchWallet(reqLogger, walDir, "CAKEY")
		if err != nil {
			return err
		}
	} else {
		// No secret exists, gotta create one

		cn := fmt.Sprintf("%s.%s.svc.cluster.local", s.deploy.ObjectMeta.Name, s.deploy.ObjectMeta.Namespace)

		err, s.cert = makeExporterCert(s.ctx, cn)
		if err != nil {
			reqLogger.V(1).Info(fmt.Sprintf("%s: makeExporterCert returned %s", us, err.Error()))
			return err
		}

		metricsServerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: s.deploy.ObjectMeta.Namespace,
				Name:      metricsServerSecretName,
			},
			Data: map[string][]byte{
				"cwallet.sso": s.cert.wallet,
			},
		}

		if err = controllerutil.SetControllerReference(s.deploy, metricsServerSecret, s.scheme); err != nil {
			reqLogger.V(1).Info(us + ": Could not set secret owner / controller: " + err.Error())
			return err
		}

		err = s.client.Create(s.ctx, metricsServerSecret)
		if err != nil {
			reqLogger.V(1).Error(err, "Could not create operator server secret")
			return err
		}
	}

	// Then the same thing again for the client-side secret

	metricsServerClientSecretName := fmt.Sprintf("%s-metrics-client", s.deploy.ObjectMeta.Name)

	var metricsServerClientSecret *corev1.Secret

	metricsServerClientSecret, err = getSecret(s.ctx, s.client,
		types.NamespacedName{Namespace: s.deploy.ObjectMeta.Namespace, Name: metricsServerClientSecretName})

	if err == nil {
		// Found a pre-existing secret
		s.cert.clientCert = metricsServerClientSecret.Data["client.crt"]
		s.cert.clientKey = metricsServerClientSecret.Data["client.key"]
		s.cert.caCert = metricsServerClientSecret.Data["ca.crt"]
	} else {
		// No secret exists, gotta create one

		metricsServerClientSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: s.deploy.ObjectMeta.Namespace,
				Name:      metricsServerClientSecretName,
			},
			Data: map[string][]byte{
				"client.crt": s.cert.clientCert,
				"client.key": s.cert.clientKey,
				"ca.crt":     s.cert.caCert,
			},
		}

		if err = controllerutil.SetControllerReference(s.deploy, metricsServerClientSecret, s.scheme); err != nil {
			reqLogger.V(1).Info(us + ": Could not set secret owner / controller: " + err.Error())
			return err
		}

		err = s.client.Create(s.ctx, metricsServerClientSecret)
		if err != nil {
			reqLogger.V(1).Error(err, "Could not create operator metrics client secret")
			return err
		}
	}

	return nil
}

// getTimesTenInstanceGUID - fetch the instance GUID of the TimesTen instance
func getTimesTenInstanceGUID(ctx context.Context, instanceName string) string {
	us := "getTimesTenInstanceGUID"
	reqLogger := log.FromContext(ctx)
	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))
	defer reqLogger.V(2).Info(fmt.Sprintf("%s returns", us))

	f, err := os.Open("/timesten/" + instanceName + "/conf/timesten.conf")
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		tok := strings.Split(line, "=")
		if tok[0] == "instance_guid" {
			return tok[1]
		}
	}
	return ""
}

// startTLSMetricsServer - start up an https server to serve metrics
// The operator sdk doesn't support this, so we have to do it ourself
func StartTLSMetricsServer(reqLogger logr.Logger, s *TTServiceServer) error {
	us := "startTLSMetricsServer"
	reqLogger.V(2).Info(fmt.Sprintf("%s entered", us))
	defer reqLogger.V(2).Info(fmt.Sprintf("%s returns", us))

	if s.metricsHttpScheme != "https" {
		return errors.New("metrics configured for http, not starting metrics TLS server")
	}

	reqLogger.Info(fmt.Sprintf("Configuring TLS metric server on port %d", s.metricsPort))

	tlsMetricsHandler := promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	mux := http.NewServeMux()
	mux.Handle("/metrics", tlsMetricsHandler)

	//
	// Create a CA certificate pool and add our own CA certificate to it
	//
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(s.cert.caCert))

	serverCertificate, err := tls.X509KeyPair([]byte(s.cert.serverCert), []byte(s.cert.serverKey))
	if err != nil {
		reqLogger.Error(err, "Can't create X509 key pair for server certificate")
		return err
	}

	csl := tls.CipherSuites()
	var is3des = regexp.MustCompile(`3DES`)
	var istls_rsa = regexp.MustCompile(`^TLS_RSA_`)
	var isSHA1 = regexp.MustCompile(`SHA$`)
	allowed := make([]uint16, 0)
	for _, cs := range csl {
		if istls_rsa.MatchString(cs.Name) ||
			is3des.MatchString(cs.Name) ||
			isSHA1.MatchString(cs.Name) {
			continue
		}
		allowed = append(allowed, cs.ID)
	}

	tlsConfig := tls.Config{
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{serverCertificate},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: allowed,
	}

	tlsConfig.BuildNameToCertificate()

	tlsMetricsServer := http.Server{
		Addr:              fmt.Sprintf(":%d", s.metricsPort),
		Handler:           mux,
		MaxHeaderBytes:    1 << 20,
		IdleTimeout:       90 * time.Second,
		ReadHeaderTimeout: 32 * time.Second,
		TLSConfig:         &tlsConfig,
	}

	go func() {
		reqLogger.Info("Starting TLS metrics server")
		err := tlsMetricsServer.ListenAndServeTLS("", "")
		reqLogger.Error(err, "TLS metrics server exits")
	}()

	return nil
}

// Fetch a value from our wallet, decode the value, and return
func fetchWallet(reqLogger logr.Logger, certdir string, key string) (error, []byte) {
	timestenHome, _ := os.LookupEnv("TIMESTEN_HOME")
	out, err := exec.Command(timestenHome+"/bin/ttenv", "ttUser", "-zzwallet", certdir, "-zzget", key).Output()
	if err != nil {
		reqLogger.Error(err, "Failed to fetch wallet key "+key+" from "+certdir+".")
		return err, nil
	}
	return nil, []byte(strings.Replace(string(out), "|", "\n", -1))
}

//----------------------------------------------------------------------
//----------------------------------------------------------------------
// Readiness Probes
//----------------------------------------------------------------------
//----------------------------------------------------------------------

// Added as the readiness probe checker in main.c, this function
// succeeds if this is the leader and fails if not.

func ReadinessChecker(r *http.Request) error {
	if WeAreTheLeader {
		return nil
	}
	return errors.New("Not the leader")
}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
