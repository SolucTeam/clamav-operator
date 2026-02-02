/*
Copyright 2025 Platform Team - Numspot.

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

	clamavv1alpha1 "gitlab.tooling.cloudgouv-eu-west-1.numspot.internal/platform-iac/clamav-operator/api/v1alpha1"
	"gitlab.tooling.cloudgouv-eu-west-1.numspot.internal/platform-iac/clamav-operator/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(clamavv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var scannerImage string
	var clamavHost string
	var clamavPort int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&scannerImage, "scanner-image", "registry.tooling.cloudgouv-eu-west-1.numspot.cloud/platform-runtime/clamav-node-scanner:1.0.3",
		"Container image for the ClamAV scanner")
	flag.StringVar(&clamavHost, "clamav-host", "clamav.clamav.svc.cluster.local",
		"ClamAV service host")
	flag.IntVar(&clamavPort, "clamav-port", 3310,
		"ClamAV service port")

	opts := zap.Options{
		Development: true,
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
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "clamav-operator.platform.numspot.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// ✅ AJOUTÉ : Créer le Clientset pour accéder aux logs des pods
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes clientset")
		os.Exit(1)
	}

	// Setup controllers
	if err = (&controllers.NodeScanReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor("nodescan-controller"),
		Clientset:    clientset, // ✅ AJOUTÉ : Passer le clientset
		ScannerImage: scannerImage,
		ClamavHost:   clamavHost,
		ClamavPort:   clamavPort,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodeScan")
		os.Exit(1)
	}

	if err = (&controllers.ClusterScanReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("clusterscan-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterScan")
		os.Exit(1)
	}

	if err = (&controllers.ScanScheduleReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("scanschedule-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScanSchedule")
		os.Exit(1)
	}

	// Setup webhooks
	if err = (&clamavv1alpha1.NodeScan{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "NodeScan")
		os.Exit(1)
	}
	if err = (&clamavv1alpha1.ClusterScan{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ClusterScan")
		os.Exit(1)
	}
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
