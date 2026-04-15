//go:build e2e

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

// Package e2e contains end-to-end tests for the ClamAV Operator.
//
// These tests run against a real Kubernetes cluster (or envtest) and exercise
// the full reconcile loop — from CRD creation to Job execution and status
// propagation.
//
// Run with:
//
//	USE_EXISTING_CLUSTER=true go test ./test/e2e/... -v -timeout 10m
//
// Or against envtest (no real cluster needed):
//
//	go test ./test/e2e/... -v -timeout 10m
package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
	clamavv1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
	"github.com/SolucTeam/clamav-operator/internal/controller"
)

// ─── Suite wiring ─────────────────────────────────────────────────────────────

var (
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	ctx        context.Context
	cancel     context.CancelFunc
	testScheme = runtime.NewScheme()
)

const (
	// defaultNamespace is the namespace used by e2e tests
	defaultNamespace = "clamav-e2e"
	// eventuallyTimeout is the default timeout for Eventually assertions
	eventuallyTimeout = 60 * time.Second
	// eventuallyInterval is the default polling interval for Eventually assertions
	eventuallyInterval = 500 * time.Millisecond
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(clamavv1beta1.AddToScheme(testScheme))
	utilruntime.Must(clamavv1alpha1.AddToScheme(testScheme))
	utilruntime.Must(batchv1.AddToScheme(testScheme))
	utilruntime.Must(corev1.AddToScheme(testScheme))
}

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode (requires kubebuilder binaries or a live cluster)")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "ClamAV Operator E2E Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	useExistingCluster := os.Getenv("USE_EXISTING_CLUSTER") == "true"

	testEnv = &envtest.Environment{
		// Point at the CRD manifests so envtest installs them automatically.
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &useExistingCluster,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create the test namespace
	ns := &corev1.Namespace{}
	ns.Name = defaultNamespace
	_ = k8sClient.Create(ctx, ns) // ignore AlreadyExists

	// Start the controller manager and register the reconcilers so that
	// resources created during tests are actually reconciled.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
	})
	Expect(err).NotTo(HaveOccurred())

	// ScanScheduleReconciler — drives the core scheduling logic under test.
	err = (&controller.ScanScheduleReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("scanschedule-controller"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	// ClusterScanReconciler — needed for any test that verifies ClusterScan phase
	// propagation back to ScanSchedule.Status.Active.
	err = (&controller.ClusterScanReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("clusterscan-controller"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	Expect(testEnv.Stop()).To(Succeed())
})
