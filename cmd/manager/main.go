/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
	clamavv1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
	"github.com/SolucTeam/clamav-operator/internal/controller"
	//+kubebuilder:scaffold:imports
)

// splitHostname parses a Kubernetes service hostname like "svc.namespace.svc.cluster.local"
// and returns the components [serviceName, namespace, ...]
func splitHostname(hostname string) []string {
	return strings.Split(hostname, ".")
}

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Register v1beta1 first: it is the hub (storage version).
	// The API server routes conversion webhook calls between v1alpha1 and v1beta1.
	utilruntime.Must(clamavv1beta1.AddToScheme(scheme))
	utilruntime.Must(clamavv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var webhookPort int
	var scannerImage string
	var clamavHost string
	var clamavPort int
	var skipStartupChecks bool
	var scannerServiceAccount string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&webhookPort, "webhook-port", 9443,
		"Port the webhook server listens on. Must match the containerPort in the Helm Deployment "+
			"(webhook.port value) and the Service targetPort.")
	flag.StringVar(&scannerImage, "scanner-image", "ghcr.io/solucteam/clamav-node-scanner:latest",
		"Container image for the ClamAV scanner (overridden by Helm via --scanner-image flag)")
	flag.StringVar(&clamavHost, "clamav-host", "clamav.clamav.svc.cluster.local",
		"ClamAV service host")
	flag.IntVar(&clamavPort, "clamav-port", 3310,
		"ClamAV service port")
	flag.BoolVar(&skipStartupChecks, "skip-startup-checks", false,
		"Skip startup validation checks (not recommended for production)")
	flag.StringVar(&scannerServiceAccount, "scanner-service-account", "clamav-scanner",
		"Name of the ServiceAccount used by scanner jobs")

	// Development mode enables human-readable logs; disable in production for JSON output.
	// Automatically controlled by zap flags (--zap-devel=true|false).
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Get the Kubernetes config
	config := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: webhookPort,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "clamav-operator.clamav.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create the Clientset for accessing pod logs and performing startup checks
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes clientset")
		os.Exit(1)
	}

	// Obtain the signal-handling context once and reuse it for both startup
	// checks and the manager lifecycle. Calling ctrl.SetupSignalHandler()
	// multiple times is safe in recent controller-runtime versions but is
	// semantically incorrect (we want a single shared context).
	ctx := ctrl.SetupSignalHandler()

	// Run startup validation checks
	if !skipStartupChecks {
		namespace := controller.GetNamespace()
		setupLog.Info("Running startup validation checks", "namespace", namespace)

		checker := controller.NewStartupChecker(clientset, namespace, scannerServiceAccount)

		if err := checker.RunAllChecks(ctx); err != nil {
			setupLog.Error(err, "Startup validation failed",
				"hint", "Use --skip-startup-checks to bypass (not recommended for production)")
			os.Exit(1)
		}

		// Optional: Check ClamAV connectivity (warning only, not fatal)
		clamavNamespace := "clamav"
		if parts := splitHostname(clamavHost); len(parts) >= 2 {
			clamavNamespace = parts[1]
		}
		clamavServiceName := "clamav"
		if parts := splitHostname(clamavHost); len(parts) >= 1 {
			clamavServiceName = parts[0]
		}
		if err := controller.ValidateClamAVConnectivity(ctx, clientset, clamavNamespace, clamavServiceName, int32(clamavPort)); err != nil { //nolint:gosec // port numbers are always in valid int32 range
			setupLog.Info("ClamAV connectivity check warning", "error", err)
		}

		setupLog.Info("All startup validation checks passed")
	} else {
		setupLog.Info("Skipping startup validation checks (--skip-startup-checks=true)")
	}

	// Parse SCANNER_IMAGE_PULL_SECRETS env var (set by Helm from scanner.imagePullSecrets).
	// Format: comma-separated secret names, e.g. "regcred,other-secret".
	// Empty string → no pull secrets (public registry).
	var scannerPullSecrets []corev1.LocalObjectReference
	if raw := os.Getenv("SCANNER_IMAGE_PULL_SECRETS"); raw != "" {
		for _, name := range strings.Split(raw, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				scannerPullSecrets = append(scannerPullSecrets, corev1.LocalObjectReference{Name: name})
			}
		}
	}

	// Setup controllers
	if err = (&controller.NodeScanReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		Recorder:                mgr.GetEventRecorderFor("nodescan-controller"),
		Clientset:               clientset,
		ScannerImage:            scannerImage,
		ClamavHost:              clamavHost,
		ClamavPort:              clamavPort,
		ScannerImagePullSecrets: scannerPullSecrets,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodeScan")
		os.Exit(1)
	}

	if err = (&controller.ClusterScanReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("clusterscan-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterScan")
		os.Exit(1)
	}

	if err = (&controller.ScanScheduleReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("scanschedule-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScanSchedule")
		os.Exit(1)
	}

	// Setup admission webhooks (validation for v1alpha1 types)
	if err = (&clamavv1alpha1.NodeScan{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "NodeScan")
		os.Exit(1)
	}
	if err = (&clamavv1alpha1.ClusterScan{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterScan")
		os.Exit(1)
	}
	if err = (&clamavv1alpha1.ScanSchedule{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ScanSchedule")
		os.Exit(1)
	}

	// NOTE: /convert is registered automatically by controller-runtime when
	// SetupWebhookWithManager is called for any type that implements
	// conversion.Convertible (ConvertTo / ConvertFrom). Registering it again
	// here would cause "panic: can't register duplicate path: /convert".
	// Do NOT add an explicit mgr.GetWebhookServer().Register("/convert", ...) call.

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
