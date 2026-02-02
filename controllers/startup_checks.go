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

package controllers

import (
	"context"
	"fmt"
	"os"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// StartupChecker performs validation checks at operator startup
type StartupChecker struct {
	clientset        kubernetes.Interface
	namespace        string
	scannerSAName    string
	requiredRBACRules []RBACRule
}

// RBACRule defines a required RBAC permission
type RBACRule struct {
	APIGroup  string
	Resource  string
	Verbs     []string
	Namespace string // empty for cluster-scoped
}

// NewStartupChecker creates a new StartupChecker
func NewStartupChecker(clientset kubernetes.Interface, namespace, scannerSAName string) *StartupChecker {
	return &StartupChecker{
		clientset:     clientset,
		namespace:     namespace,
		scannerSAName: scannerSAName,
		requiredRBACRules: []RBACRule{
			// Scanner needs to be able to read from host filesystem (implicit via pod security)
			{APIGroup: "", Resource: "pods", Verbs: []string{"get", "list"}, Namespace: namespace},
			{APIGroup: "", Resource: "pods/log", Verbs: []string{"get"}, Namespace: namespace},
			{APIGroup: "batch", Resource: "jobs", Verbs: []string{"create", "get", "list", "watch", "delete"}, Namespace: namespace},
			{APIGroup: "", Resource: "nodes", Verbs: []string{"get", "list", "watch"}, Namespace: ""},
			{APIGroup: "clamav.platform.numspot.com", Resource: "nodescans", Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}, Namespace: namespace},
			{APIGroup: "clamav.platform.numspot.com", Resource: "nodescans/status", Verbs: []string{"get", "update", "patch"}, Namespace: namespace},
		},
	}
}

// RunAllChecks executes all startup validation checks
func (c *StartupChecker) RunAllChecks(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Running startup validation checks")

	// Check 1: Validate Scanner ServiceAccount exists
	if err := c.checkScannerServiceAccount(ctx); err != nil {
		return fmt.Errorf("scanner ServiceAccount check failed: %w", err)
	}
	logger.Info("Scanner ServiceAccount check passed", "serviceAccount", c.scannerSAName)

	// Check 2: Validate operator has required RBAC permissions
	if err := c.checkOperatorRBACPermissions(ctx); err != nil {
		return fmt.Errorf("RBAC permissions check failed: %w", err)
	}
	logger.Info("RBAC permissions check passed")

	// Check 3: Validate we can connect to the API server
	if err := c.checkAPIServerConnectivity(ctx); err != nil {
		return fmt.Errorf("API server connectivity check failed: %w", err)
	}
	logger.Info("API server connectivity check passed")

	logger.Info("All startup validation checks passed")
	return nil
}

// checkScannerServiceAccount verifies that the scanner ServiceAccount exists
func (c *StartupChecker) checkScannerServiceAccount(ctx context.Context) error {
	logger := log.FromContext(ctx)

	sa, err := c.clientset.CoreV1().ServiceAccounts(c.namespace).Get(ctx, c.scannerSAName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("scanner ServiceAccount '%s' not found in namespace '%s'. "+
				"Please create it or ensure the Helm chart is correctly installed", c.scannerSAName, c.namespace)
		}
		return fmt.Errorf("failed to get ServiceAccount: %w", err)
	}

	logger.V(1).Info("Found scanner ServiceAccount",
		"name", sa.Name,
		"namespace", sa.Namespace,
		"created", sa.CreationTimestamp)

	return nil
}

// checkOperatorRBACPermissions verifies the operator has required permissions
func (c *StartupChecker) checkOperatorRBACPermissions(ctx context.Context) error {
	logger := log.FromContext(ctx)

	var missingPermissions []string

	for _, rule := range c.requiredRBACRules {
		for _, verb := range rule.Verbs {
			allowed, err := c.canI(ctx, rule.APIGroup, rule.Resource, verb, rule.Namespace)
			if err != nil {
				logger.Error(err, "Failed to check permission",
					"resource", rule.Resource,
					"verb", verb)
				continue
			}

			if !allowed {
				permission := fmt.Sprintf("%s/%s:%s", rule.APIGroup, rule.Resource, verb)
				if rule.Namespace != "" {
					permission += fmt.Sprintf(" (namespace: %s)", rule.Namespace)
				}
				missingPermissions = append(missingPermissions, permission)
			}
		}
	}

	if len(missingPermissions) > 0 {
		return fmt.Errorf("operator is missing required RBAC permissions: %v. "+
			"Please ensure the ClusterRole and ClusterRoleBinding are correctly configured",
			missingPermissions)
	}

	return nil
}

// canI checks if the current service account has permission to perform an action
func (c *StartupChecker) canI(ctx context.Context, apiGroup, resource, verb, namespace string) (bool, error) {
	sar := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     apiGroup,
				Resource:  resource,
			},
		},
	}

	response, err := c.clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}

	return response.Status.Allowed, nil
}

// checkAPIServerConnectivity verifies connectivity to the API server
func (c *StartupChecker) checkAPIServerConnectivity(ctx context.Context) error {
	// Simple API server health check - list namespaces (usually allowed for service accounts)
	_, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("cannot connect to Kubernetes API server: %w", err)
	}
	return nil
}

// GetNamespace returns the current namespace from the environment or file
func GetNamespace() string {
	// Try environment variable first
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	// Try reading from the mounted service account
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		return string(data)
	}

	// Default fallback
	return "clamav-system"
}

// ValidateClamAVConnectivity checks if ClamAV service is reachable
func ValidateClamAVConnectivity(ctx context.Context, clientset kubernetes.Interface, namespace, serviceName string, port int32) error {
	logger := log.FromContext(ctx)

	// Check if the ClamAV service exists
	svc, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ClamAV service not found - scans will fail until service is available",
				"service", serviceName,
				"namespace", namespace)
			// Don't fail startup, just warn - ClamAV might be deployed separately
			return nil
		}
		return fmt.Errorf("failed to check ClamAV service: %w", err)
	}

	// Check if service has endpoints
	endpoints, err := clientset.CoreV1().Endpoints(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		logger.Info("Could not check ClamAV endpoints",
			"service", serviceName,
			"error", err)
		return nil
	}

	hasReadyEndpoints := false
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 {
			hasReadyEndpoints = true
			break
		}
	}

	if !hasReadyEndpoints {
		logger.Info("ClamAV service has no ready endpoints - scans will fail until endpoints are available",
			"service", svc.Name)
	} else {
		logger.Info("ClamAV service is available",
			"service", svc.Name,
			"clusterIP", svc.Spec.ClusterIP)
	}

	return nil
}

// CheckResult represents the result of a startup check
type CheckResult struct {
	Name    string
	Passed  bool
	Message string
	Warning bool
}

// RunChecksWithResults runs all checks and returns detailed results
func (c *StartupChecker) RunChecksWithResults(ctx context.Context) []CheckResult {
	var results []CheckResult

	// Check ServiceAccount
	if err := c.checkScannerServiceAccount(ctx); err != nil {
		results = append(results, CheckResult{
			Name:    "ScannerServiceAccount",
			Passed:  false,
			Message: err.Error(),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "ScannerServiceAccount",
			Passed:  true,
			Message: fmt.Sprintf("ServiceAccount '%s' exists in namespace '%s'", c.scannerSAName, c.namespace),
		})
	}

	// Check RBAC
	if err := c.checkOperatorRBACPermissions(ctx); err != nil {
		results = append(results, CheckResult{
			Name:    "RBACPermissions",
			Passed:  false,
			Message: err.Error(),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "RBACPermissions",
			Passed:  true,
			Message: "All required RBAC permissions are granted",
		})
	}

	// Check API server
	if err := c.checkAPIServerConnectivity(ctx); err != nil {
		results = append(results, CheckResult{
			Name:    "APIServerConnectivity",
			Passed:  false,
			Message: err.Error(),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "APIServerConnectivity",
			Passed:  true,
			Message: "Successfully connected to Kubernetes API server",
		})
	}

	return results
}
