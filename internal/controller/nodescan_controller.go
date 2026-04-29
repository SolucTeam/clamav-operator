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

package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
	"github.com/SolucTeam/clamav-operator/internal/notification"
)

const (
	nodeScanFinalizer = "clamav.io/finalizer"
	// maxParseRetries is the maximum number of times to retry parsing job results
	maxParseRetries = 5
	// parseRetryAnnotation tracks the number of parse retry attempts
	parseRetryAnnotation = "clamav.io/parse-retries"
)

// NodeScanReconciler reconciles a NodeScan object
type NodeScanReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Clientset    kubernetes.Interface
	Notifier     *notification.Notifier
	ScannerImage string
	ClamavHost   string
	ClamavPort   int
	// ScannerImagePullSecrets is forwarded from the operator's Helm values
	// (scanner.imagePullSecrets) and injected into every scanner Job pod spec.
	// Required when the scanner image lives in a private registry.
	ScannerImagePullSecrets []corev1.LocalObjectReference
	// SignaturesPVCName is the name of the PVC that holds ClamAV signature
	// databases (set by Helm when scanner.signatures.persistent=true).
	// When non-empty, scanner job pods mount this PVC at CLAMAV_DB_PATH so
	// that signatures written by the freshclam CronJob are immediately visible
	// to scanner jobs — without baking signatures into the image.
	// The PVC must exist in the same namespace as the NodeScan object.
	// Leave empty to use signatures baked into the scanner image (air-gap /
	// image-embedded workflow).
	SignaturesPVCName string

	// ParseMaxRetries overrides maxParseRetries when non-zero.
	// Set to a small value (e.g. 1) in tests to avoid multi-minute backoff.
	ParseMaxRetries int
	// ParseRetryBaseSeconds overrides the per-attempt backoff multiplier (seconds)
	// when non-zero. Set to 1 in tests for near-instant completion.
	ParseRetryBaseSeconds int
}

// parseRetrySettings returns the effective retry ceiling and backoff multiplier,
// falling back to the package-level constants when the reconciler fields are unset.
func (r *NodeScanReconciler) parseRetrySettings() (maxRetries int, baseSeconds int) {
	maxRetries = maxParseRetries
	if r.ParseMaxRetries > 0 {
		maxRetries = r.ParseMaxRetries
	}
	baseSeconds = 10
	if r.ParseRetryBaseSeconds > 0 {
		baseSeconds = r.ParseRetryBaseSeconds
	}
	return
}

// +kubebuilder:rbac:groups=clamav.io,resources=nodescans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clamav.io,resources=nodescans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clamav.io,resources=nodescans/finalizers,verbs=update
// +kubebuilder:rbac:groups=clamav.io,resources=scanpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop
func (r *NodeScanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the NodeScan instance
	var nodeScan clamavv1alpha1.NodeScan
	if err := r.Get(ctx, req.NamespacedName, &nodeScan); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch NodeScan")
		return ctrl.Result{}, err
	}

	// Capture the original state for Patch operations (avoids 409 Conflict on concurrent updates)
	original := nodeScan.DeepCopy()

	// Handle deletion
	if !nodeScan.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&nodeScan, nodeScanFinalizer) {
			if err := r.cleanupNodeScan(ctx, &nodeScan); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&nodeScan, nodeScanFinalizer)
			if err := r.Patch(ctx, &nodeScan, client.MergeFrom(original)); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&nodeScan, nodeScanFinalizer) {
		controllerutil.AddFinalizer(&nodeScan, nodeScanFinalizer)
		if err := r.Patch(ctx, &nodeScan, client.MergeFrom(original)); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Verify node exists
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nodeScan.Spec.NodeName}, &node); err != nil {
		if errors.IsNotFound(err) {
			r.Recorder.Event(&nodeScan, corev1.EventTypeWarning, "NodeNotFound",
				fmt.Sprintf("Node %s not found", nodeScan.Spec.NodeName))
			return ctrl.Result{}, r.updateStatus(ctx, &nodeScan, clamavv1alpha1.NodeScanPhaseFailed,
				"NodeNotFound", metav1.ConditionFalse, "Node does not exist")
		}
		return ctrl.Result{}, err
	}

	// Get the scan policy if specified
	var scanPolicy *clamavv1alpha1.ScanPolicy
	if nodeScan.Spec.ScanPolicy != "" {
		scanPolicy = &clamavv1alpha1.ScanPolicy{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      nodeScan.Spec.ScanPolicy,
			Namespace: nodeScan.Namespace,
		}, scanPolicy); err != nil {
			if errors.IsNotFound(err) {
				r.Recorder.Event(&nodeScan, corev1.EventTypeWarning, "ScanPolicyNotFound",
					fmt.Sprintf("ScanPolicy %s not found", nodeScan.Spec.ScanPolicy))
				return ctrl.Result{}, r.updateStatus(ctx, &nodeScan, clamavv1alpha1.NodeScanPhaseFailed,
					"ScanPolicyNotFound", metav1.ConditionFalse, "ScanPolicy does not exist")
			}
			return ctrl.Result{}, err
		}
	}

	// Check if Job already exists
	jobName := sanitizeLabelValue(fmt.Sprintf("nodescan-%s", nodeScan.Name))

	var existingJob batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: nodeScan.Namespace}, &existingJob)

	if errors.IsNotFound(err) {
		// Initialize status if needed.
		// Use Patch instead of Update so we only send the diff and avoid
		// overwriting concurrent status changes (e.g. from a webhook or
		// another controller replica) with a stale full-object write.
		if nodeScan.Status.Phase == "" {
			statusBase := nodeScan.DeepCopy()
			nodeScan.Status.Phase = clamavv1alpha1.NodeScanPhasePending
			now := metav1.Now()
			nodeScan.Status.StartTime = &now
			if err := r.Status().Patch(ctx, &nodeScan, client.MergeFrom(statusBase)); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Create the Job
		job, err := r.constructJobForNodeScan(&nodeScan, scanPolicy)
		if err != nil {
			log.Error(err, "unable to construct job")
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, job); err != nil {
			log.Error(err, "unable to create Job for NodeScan", "job", job)
			r.Recorder.Event(&nodeScan, corev1.EventTypeWarning, "JobCreationFailed",
				fmt.Sprintf("Failed to create Job: %v", err))
			return ctrl.Result{}, err
		}

		// Update status
		nodeScan.Status.Phase = clamavv1alpha1.NodeScanPhaseRunning
		nodeScan.Status.JobRef = &corev1.ObjectReference{
			APIVersion: job.APIVersion,
			Kind:       job.Kind,
			Name:       job.Name,
			Namespace:  job.Namespace,
			UID:        job.UID,
		}

		r.Recorder.Event(&nodeScan, corev1.EventTypeNormal, "JobCreated",
			fmt.Sprintf("Scan job created for node %s", nodeScan.Spec.NodeName))

		if err := r.updateStatus(ctx, &nodeScan, clamavv1alpha1.NodeScanPhaseRunning,
			"JobCreated", metav1.ConditionTrue, "Scan job has been created"); err != nil {
			return ctrl.Result{}, err
		}

		// Record metrics
		recordNodeScanMetrics(&nodeScan, clamavv1alpha1.NodeScanPhaseRunning)
		incNodeScanRunning(nodeScan.Namespace)

		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Job exists, check its status
	if existingJob.Status.Succeeded > 0 {
		if nodeScan.Status.Phase != clamavv1alpha1.NodeScanPhaseCompleted {
			now := metav1.Now()
			nodeScan.Status.Phase = clamavv1alpha1.NodeScanPhaseCompleted
			nodeScan.Status.CompletionTime = &now
			if nodeScan.Status.StartTime != nil {
				nodeScan.Status.Duration = int64(now.Sub(nodeScan.Status.StartTime.Time).Seconds())
			}

			// Parse results from Job with retry on transient errors
			effectiveMaxRetries, effectiveBaseSeconds := r.parseRetrySettings()
			if err := r.parseJobResults(ctx, &nodeScan, &existingJob); err != nil {
				// Track retry count in annotations
				retryCount := 0
				if nodeScan.Annotations != nil {
					if val, ok := nodeScan.Annotations[parseRetryAnnotation]; ok {
						_, _ = fmt.Sscanf(val, "%d", &retryCount)
					}
				}
				retryCount++

				if retryCount >= effectiveMaxRetries {
					// Max retries exceeded — mark the scan as completed but flag it as
					// partial so consumers know the data cannot be trusted as complete.
					log.Error(err, "max parse retries exceeded, completing with partial results",
						"retries", retryCount)
					r.Recorder.Event(&nodeScan, corev1.EventTypeWarning, "ParseResultsMaxRetries",
						fmt.Sprintf("Failed to parse scan results after %d attempts: %v. "+
							"Status.ResultsPartial=true — do NOT treat this as a clean scan.", retryCount, err))
					nodeScan.Status.ResultsPartial = true
					// Add a typed condition so GitOps tools and alert rules can detect this.
					partialCond := metav1.Condition{
						Type:               "PartialResults",
						Status:             metav1.ConditionTrue,
						Reason:             "ParseMaxRetriesExceeded",
						Message:            fmt.Sprintf("Scan output could not be parsed after %d attempts. Results are incomplete.", effectiveMaxRetries),
						LastTransitionTime: metav1.Now(),
					}
					found := false
					for i, c := range nodeScan.Status.Conditions {
						if c.Type == "PartialResults" {
							nodeScan.Status.Conditions[i] = partialCond
							found = true
							break
						}
					}
					if !found {
						nodeScan.Status.Conditions = append(nodeScan.Status.Conditions, partialCond)
					}
					// Continue with completion — don't block the reconcile loop indefinitely.
				} else {
					// Update retry count annotation using Patch to avoid 409 on concurrent updates
					annotationBase := nodeScan.DeepCopy()
					if nodeScan.Annotations == nil {
						nodeScan.Annotations = make(map[string]string)
					}
					nodeScan.Annotations[parseRetryAnnotation] = fmt.Sprintf("%d", retryCount)
					if err := r.Patch(ctx, &nodeScan, client.MergeFrom(annotationBase)); err != nil {
						log.Error(err, "failed to update retry annotation")
					}

					log.Error(err, "failed to parse job results, will retry",
						"attempt", retryCount, "maxRetries", maxParseRetries)
					r.Recorder.Event(&nodeScan, corev1.EventTypeWarning, "ParseResultsFailed",
						fmt.Sprintf("Failed to parse scan results (attempt %d/%d): %v", retryCount, maxParseRetries, err))
					// Requeue with exponential backoff
					backoff := time.Duration(retryCount*effectiveBaseSeconds) * time.Second
					return ctrl.Result{RequeueAfter: backoff}, nil
				}
			}

			r.Recorder.Event(&nodeScan, corev1.EventTypeNormal, "ScanCompleted",
				fmt.Sprintf("Scan completed: %d files scanned, %d infected",
					nodeScan.Status.FilesScanned, nodeScan.Status.FilesInfected))

			if err := r.updateStatus(ctx, &nodeScan, clamavv1alpha1.NodeScanPhaseCompleted,
				"ScanCompleted", metav1.ConditionTrue, "Scan completed successfully"); err != nil {
				return ctrl.Result{}, err
			}

			// Reduce the Job TTL to 1 h now that results are recorded in the NodeScan Status.
			// The CRD is the source of truth; the Job pod is now disposable.
			r.patchJobTTL(ctx, &existingJob, TTLSecondsAfterSucceeded)

			// Record metrics
			recordNodeScanMetrics(&nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
			decNodeScanRunning(nodeScan.Namespace)

			// Record partial-results metric regardless of infection status.
			recordPartialResults(nodeScan.Namespace, nodeScan.Spec.NodeName, nodeScan.Status.ResultsPartial)

			// Send notifications if infected files found.
			// Runs in a goroutine so HTTP/SMTP retries never block the reconcile worker.
			// SendWithRetry attempts each channel up to 3 times with exponential backoff.
			// The 5-minute deadline gives 3 channels × ~35 s of retries comfortable headroom.
			if nodeScan.Status.FilesInfected > 0 && scanPolicy != nil {
				nodeScanSnap := nodeScan.DeepCopy()
				scanPolicySnap := scanPolicy.DeepCopy()
				namespace := nodeScan.Namespace
				// context.WithoutCancel keeps tracing/log values without inheriting
				// the reconcile-loop cancellation.
				baseCtx := context.WithoutCancel(ctx)
				go func() {
					notifCtx, cancel := context.WithTimeout(baseCtx, 5*time.Minute)
					defer cancel()
					results := r.Notifier.SendWithRetry(notifCtx, nodeScanSnap, scanPolicySnap)
					for _, res := range results {
						recordNotificationAttempt(namespace, res.Channel)
						if res.Err != nil {
							recordNotificationFailed(namespace, res.Channel)
						}
					}
				}()
			}

			// Update ScanPolicy usage stats
			if scanPolicy != nil {
				r.updatePolicyStats(ctx, scanPolicy)
			}
		}
		return ctrl.Result{}, nil

	} else if existingJob.Status.Failed > 0 {
		if nodeScan.Status.Phase != clamavv1alpha1.NodeScanPhaseFailed {
			now := metav1.Now()
			nodeScan.Status.Phase = clamavv1alpha1.NodeScanPhaseFailed
			nodeScan.Status.CompletionTime = &now
			if nodeScan.Status.StartTime != nil {
				nodeScan.Status.Duration = int64(now.Sub(nodeScan.Status.StartTime.Time).Seconds())
			}

			// Capture exit code and failure reason from the scanner container
			// so the information survives Job / Pod GC.
			reason, exitCode := r.getJobFailureInfo(ctx, &existingJob)
			nodeScan.Status.FailureReason = reason
			nodeScan.Status.ExitCode = exitCode

			r.Recorder.Event(&nodeScan, corev1.EventTypeWarning, "ScanFailed",
				fmt.Sprintf("Scan job failed: %s (exit %d)", reason, exitCode))

			if err := r.updateStatus(ctx, &nodeScan, clamavv1alpha1.NodeScanPhaseFailed,
				"ScanFailed", metav1.ConditionFalse,
				fmt.Sprintf("Scan job failed: %s (exit %d)", reason, exitCode)); err != nil {
				return ctrl.Result{}, err
			}

			// Keep the failed Job for 24 h to allow log inspection (TTL already set at creation).
			// Explicit patch ensures the value is correct even if someone changed it.
			r.patchJobTTL(ctx, &existingJob, TTLSecondsAfterFailed)

			// Record metrics
			recordNodeScanMetrics(&nodeScan, clamavv1alpha1.NodeScanPhaseFailed)
			decNodeScanRunning(nodeScan.Namespace)
		}
		return ctrl.Result{}, nil
	}

	// Job is still running
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// constructJobForNodeScan creates a Job for scanning a node
func (r *NodeScanReconciler) constructJobForNodeScan(nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) (*batchv1.Job, error) {
	// Determine paths to scan
	paths := nodeScan.Spec.Paths
	if len(paths) == 0 && scanPolicy != nil {
		paths = scanPolicy.Spec.Paths
	}
	if len(paths) == 0 {
		paths = []string{"/host/var/lib", "/host/opt"}
	}

	// Exclude patterns: NodeScan.Spec → ScanPolicy → empty list.
	// The scanner always applies its own hardcoded system exclusions (/proc, /sys,
	// etc.) on top; EXCLUDE_PATTERNS carries only the user-defined additions.
	excludePatterns := nodeScan.Spec.ExcludePatterns
	if len(excludePatterns) == 0 && scanPolicy != nil {
		excludePatterns = scanPolicy.Spec.ExcludePatterns
	}
	excludePatternsJSON := "[]"
	if len(excludePatterns) > 0 {
		if b, err := json.Marshal(excludePatterns); err == nil {
			excludePatternsJSON = string(b)
		}
	}

	// Determine other parameters
	maxConcurrent := nodeScan.Spec.MaxConcurrent
	if maxConcurrent == 0 && scanPolicy != nil {
		maxConcurrent = scanPolicy.Spec.MaxConcurrent
	}
	if maxConcurrent == 0 {
		maxConcurrent = DefaultMaxConcurrent
	}

	fileTimeout := nodeScan.Spec.FileTimeout
	if fileTimeout == 0 && scanPolicy != nil {
		fileTimeout = scanPolicy.Spec.FileTimeout
	}
	if fileTimeout == 0 {
		fileTimeout = DefaultFileTimeout
	}

	maxFileSize := nodeScan.Spec.MaxFileSize
	if maxFileSize == 0 && scanPolicy != nil {
		maxFileSize = scanPolicy.Spec.MaxFileSize
	}
	if maxFileSize == 0 {
		maxFileSize = DefaultMaxFileSize
	}

	connectTimeout := int64(DefaultConnectTimeout)
	if scanPolicy != nil && scanPolicy.Spec.ConnectTimeout > 0 {
		connectTimeout = scanPolicy.Spec.ConnectTimeout
	}

	// Environment variables
	//
	// Scanner-mode settings are forwarded from the operator pod's own env vars
	// (set by Helm via deployment.yaml). This avoids duplicating config values
	// between Helm values and CRD fields, and keeps the operator binary flag
	// surface minimal.
	scanMode := getEnvOrDefault("SCANNER_MODE", "standalone")
	clamscanPath := getEnvOrDefault("SCANNER_CLAMSCAN_PATH", "/usr/bin/clamscan")
	clamavDBPath := getEnvOrDefault("SCANNER_CLAMAV_DB_PATH", "/var/lib/clamav")
	// In standalone mode a freshclam CronJob manages signature updates on a
	// schedule. The scanner itself must NOT attempt to update signatures during
	// a scan run — that would create write conflicts and slow every scan.
	// In remote mode the ClamAV daemon handles updates independently, so the
	// same logic applies: always "false" here, rely on the daemon's own schedule.
	updateSigs := "false"
	if scanMode != "standalone" {
		// Preserve any explicit override from the operator env in remote mode.
		updateSigs = getEnvOrDefault("SCANNER_UPDATE_SIGNATURES", "false")
	}
	// Incremental scan settings follow the same CRD-over-global priority pattern
	// used for maxConcurrent, fileTimeout, etc.:
	//   1. NodeScan.Spec value  (per-scan override, set by the user in the CR)
	//   2. Operator env var     (global Helm default, applied when spec is absent)
	//
	// Previously these values were read exclusively from env vars, causing any
	// IncrementalConfig or Strategy set on a NodeScan CR to be silently ignored.
	scanStrategy := getEnvOrDefault("SCANNER_SCAN_STRATEGY", "full")
	if nodeScan.Spec.Strategy != "" {
		scanStrategy = string(nodeScan.Spec.Strategy)
	}

	incrEnabled := getEnvOrDefault("SCANNER_INCREMENTAL_ENABLED", "false")
	// Derive INCREMENTAL_ENABLED from Strategy rather than from
	// IncrementalConfig.Enabled. The v1alpha1 Enabled field is dropped during
	// the v1alpha1→v1beta1 conversion (it has no v1beta1 counterpart) and is
	// always zero-valued when the object is read back from storage.  Strategy,
	// by contrast, is a top-level NodeScanSpec field that survives the round-
	// trip, making it the authoritative signal for whether incremental mode
	// should be active for this scan.
	if nodeScan.Spec.Strategy != "" {
		if nodeScan.Spec.Strategy == clamavv1alpha1.ScanStrategyFull {
			incrEnabled = "false"
		} else {
			incrEnabled = "true"
		}
	}

	fullScanInterv := getEnvOrDefault("SCANNER_FULL_SCAN_INTERVAL", "10")
	maxFileAgeHours := getEnvOrDefault("SCANNER_MAX_FILE_AGE_HOURS", "24")
	skipUnchanged := getEnvOrDefault("SCANNER_SKIP_UNCHANGED_FILES", "true")
	if nodeScan.Spec.IncrementalConfig != nil {
		cfg := nodeScan.Spec.IncrementalConfig
		if cfg.BaselineInterval > 0 {
			fullScanInterv = fmt.Sprintf("%d", cfg.BaselineInterval)
		}
		if cfg.MaxAge > 0 {
			maxFileAgeHours = fmt.Sprintf("%d", cfg.MaxAge)
		}
		// SkipUnchangedFiles survives the v1beta1 round-trip; always override.
		if cfg.SkipUnchangedFiles {
			skipUnchanged = "true"
		} else {
			skipUnchanged = "false"
		}
	}

	// ForceFullScan overrides any incremental strategy for this specific run.
	// Must be wired explicitly: a ClusterScan that sets forceFullScan on the
	// template copies it to each NodeScan, but the scanner only learns about it
	// via the FORCE_FULL_SCAN env var.
	forceFullScan := "false"
	if nodeScan.Spec.ForceFullScan {
		forceFullScan = "true"
	}

	envVars := []corev1.EnvVar{
		{Name: "NODE_NAME", Value: nodeScan.Spec.NodeName},
		{Name: "HOST_ROOT", Value: "/host"},
		{Name: "RESULTS_DIR", Value: "/results"},
		{Name: "CLAMAV_HOST", Value: r.ClamavHost},
		{Name: "CLAMAV_PORT", Value: fmt.Sprintf("%d", r.ClamavPort)},
		{Name: "PATHS_TO_SCAN", Value: strings.Join(paths, ",")},
		{Name: "MAX_CONCURRENT", Value: fmt.Sprintf("%d", maxConcurrent)},
		{Name: "FILE_TIMEOUT", Value: fmt.Sprintf("%d", fileTimeout)},
		{Name: "CONNECT_TIMEOUT", Value: fmt.Sprintf("%d", connectTimeout)},
		{Name: "MAX_FILE_SIZE", Value: fmt.Sprintf("%d", maxFileSize)},
		// Scanner mode & standalone paths
		{Name: "SCAN_MODE", Value: scanMode},
		{Name: "CLAMSCAN_PATH", Value: clamscanPath},
		{Name: "CLAMAV_DB_PATH", Value: clamavDBPath},
		{Name: "UPDATE_SIGNATURES", Value: updateSigs},
		// Incremental scan settings
		{Name: "INCREMENTAL_ENABLED", Value: incrEnabled},
		{Name: "SCAN_STRATEGY", Value: scanStrategy},
		{Name: "FULL_SCAN_INTERVAL", Value: fullScanInterv},
		{Name: "MAX_FILE_AGE_HOURS", Value: maxFileAgeHours},
		{Name: "SKIP_UNCHANGED_FILES", Value: skipUnchanged},
		{Name: "FORCE_FULL_SCAN", Value: forceFullScan},
		// User-defined exclude patterns (JSON array of regex strings).
		// The scanner merges these with its built-in system exclusions.
		{Name: "EXCLUDE_PATTERNS", Value: excludePatternsJSON},
	}

	// Resources - apply in priority order:
	// 1. NodeScan.Spec.Resources (explicit)
	// 2. ScanPolicy.Spec.Resources (policy-defined)
	// 3. Priority-based defaults (high/medium/low)
	var resources corev1.ResourceRequirements
	if nodeScan.Spec.Resources != nil {
		resources = *nodeScan.Spec.Resources
	} else if scanPolicy != nil && scanPolicy.Spec.Resources != nil {
		resources = *scanPolicy.Spec.Resources
	} else {
		// Use priority-based default resources
		resources = GetResourcesForPriority(nodeScan.Spec.Priority)
	}

	// Job name — sanitized to ≤ 63 chars using a hash suffix when the NodeScan
	// name (which includes the node name) exceeds the Kubernetes label limit.
	jobName := sanitizeLabelValue(fmt.Sprintf("nodescan-%s", nodeScan.Name))

	// Initial TTL: use the value from spec, or the default failed TTL.
	// Will be patched to TTLSecondsAfterSucceeded once the Job succeeds.
	ttl := nodeScan.Spec.TTLSecondsAfterFinished
	if ttl == nil {
		ttl = ptr.To(int32(TTLSecondsAfterFailed))
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: nodeScan.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "clamav",
				"app.kubernetes.io/component": "scanner",
				"clamav.io/nodescan":          sanitizeLabelValue(nodeScan.Name),
				"clamav.io/node":              sanitizeLabelValue(nodeScan.Spec.NodeName),
				"clamav.io/scan-priority":     nodeScan.Spec.Priority,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(3)),
			TTLSecondsAfterFinished: ttl,
			// Hard wall-clock deadline: prevents zombie pods from blocking a node indefinitely.
			ActiveDeadlineSeconds: ptr.To(int64(JobActiveDeadlineSeconds)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":         "clamav-node-scanner",
						"target-node": sanitizeLabelValue(nodeScan.Spec.NodeName),
						"security":    "clamav",
						"clamav":      "scanner",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "clamav-scanner",
					NodeName:           nodeScan.Spec.NodeName,
					HostPID:            true,
					HostIPC:            true,
					DNSPolicy:          corev1.DNSClusterFirst,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(false),
						RunAsUser:    ptr.To(int64(0)),
						FSGroup:      ptr.To(int64(0)),
					},
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists},
					},
					// ScannerImagePullSecrets is forwarded from Helm values (scanner.imagePullSecrets).
					// Required for scanner images stored in private registries.
					ImagePullSecrets: r.ScannerImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "scanner",
							Image:           r.ScannerImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             envVars,
							VolumeMounts:    r.buildScannerVolumeMounts(clamavDBPath),
							Resources:       resources,
							SecurityContext: &corev1.SecurityContext{
								Privileged:             ptr.To(true),
								ReadOnlyRootFilesystem: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"SYS_ADMIN",
										"DAC_READ_SEARCH",
									},
								},
							},
						},
					},
					Volumes: r.buildScannerVolumes(),
				},
			},
		},
	}

	// Set NodeScan as owner
	if err := controllerutil.SetControllerReference(nodeScan, job, r.Scheme); err != nil {
		return nil, err
	}

	return job, nil
}

// buildScannerVolumeMounts returns the VolumeMount list for a scanner Job container.
// When SignaturesPVCName is set the signatures PVC is mounted at dbPath so that
// freshclam-written databases are immediately visible to the scanner.
func (r *NodeScanReconciler) buildScannerVolumeMounts(dbPath string) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{Name: "host-root", MountPath: "/host", ReadOnly: true},
		{Name: "scan-results", MountPath: "/results"},
	}
	if r.SignaturesPVCName != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "clamav-signatures",
			MountPath: dbPath,
			ReadOnly:  true,
		})
	}
	return mounts
}

// buildScannerVolumes returns the Volume list for a scanner Job pod.
// When SignaturesPVCName is set the signatures PVC is included so freshclam
// updates are shared with scanner job pods without rebuilding the image.
func (r *NodeScanReconciler) buildScannerVolumes() []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "host-root",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
					Type: ptr.To(corev1.HostPathDirectory),
				},
			},
		},
		{
			Name: "scan-results",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/clamav-scans",
					Type: ptr.To(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
	}
	if r.SignaturesPVCName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "clamav-signatures",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.SignaturesPVCName,
					ReadOnly:  true,
				},
			},
		})
	}
	return volumes
}

// parseJobResults parses the scan results from the completed Job
func (r *NodeScanReconciler) parseJobResults(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, job *batchv1.Job) error {
	log := log.FromContext(ctx)

	// Get the Pod from the Job
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(job.Namespace), client.MatchingLabels(job.Spec.Selector.MatchLabels)); err != nil {
		return err
	}

	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods found for job")
	}

	// A Job with BackoffLimit > 0 can have multiple pods (original + retries).
	// The API does not guarantee ordering, so Items[0] may be a failed pod.
	// Find the Succeeded pod explicitly to ensure we parse the right logs.
	var pod *corev1.Pod
	for i := range podList.Items {
		if podList.Items[i].Status.Phase == corev1.PodSucceeded {
			pod = &podList.Items[i]
			break
		}
	}
	if pod == nil {
		// Fallback: no pod in Succeeded phase yet (still completing) — caller will retry.
		return fmt.Errorf("no succeeded pod found for job (found %d pods)", len(podList.Items))
	}

	// Get pod logs using clientset
	req := r.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "scanner",
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer stream.Close()

	// Parse log lines to extract JSON
	scanner := bufio.NewScanner(stream)

	var (
		filesScanned  int64
		filesInfected int64
		filesSkipped  int64
		errorCount    int64
		infectedFiles []clamavv1alpha1.InfectedFile
	)

	for scanner.Scan() {
		line := scanner.Text()

		// Scanner JSON log contract
		//
		// Each line of the scanner container's stdout must be valid JSON.
		// Non-JSON lines are silently skipped for forward compatibility.
		//
		// Two event types are relevant to the controller:
		//
		//   type == "scan_complete"  (added in scanner v0.2)
		//     Emitted once at the end of the scan run. Carries aggregate counters.
		//     The controller falls back to matching message=="Scan completed successfully"
		//     for scanner images older than v0.2.
		//
		//   alert == "INFECTED_FILE"
		//     Emitted once per infected file. Must include file_path.
		//
		// Full schema (all fields optional unless noted):
		//
		//   level            string   — log level (info/warn/error)
		//   type             string   — event type; "scan_complete" is the only machine-read value  [REQUIRED for aggregate]
		//   message          string   — human-readable description (backward compat with pre-v0.2)
		//   files_scanned    int64    — total files examined
		//   files_infected   int64    — total files with a positive match
		//   files_skipped    int64    — total files skipped (size, pattern, etc.)
		//   errors_count     int64    — total errors during scan
		//   file_path        string   — absolute path on the host  [REQUIRED for INFECTED_FILE]
		//   virus_names      []string — list of virus signatures matched
		//   file_size        int64    — size of the infected file in bytes
		//   alert            string   — "INFECTED_FILE" triggers infected-file recording
		//   strategy         string   — scan strategy used (full/incremental/smart) [since v0.3]
		//   files_skipped_incremental int64 — files skipped by incremental logic   [since v0.3]
		//   cache_hits       int64    — incremental cache hits                      [since v0.3]
		//   cache_misses     int64    — incremental cache misses                    [since v0.3]
		//
		// Any field not present defaults to its zero value (0 / "" / nil).
		// New fields may be added by the scanner without breaking this controller.
		type LogEntry struct {
			Level         string   `json:"level"`
			Type          string   `json:"type"`
			Message       string   `json:"message"`
			FilesScanned  int64    `json:"files_scanned"`
			FilesInfected int64    `json:"files_infected"`
			FilesSkipped  int64    `json:"files_skipped"`
			ErrorsCount   int64    `json:"errors_count"`
			FilePath      string   `json:"file_path"`
			VirusNames    []string `json:"virus_names"`
			FileSize      int64    `json:"file_size"`
			Alert         string   `json:"alert"`
			// Incremental scanning fields (emitted since scanner v0.3)
			Strategy                string `json:"strategy"`
			FilesSkippedIncremental int64  `json:"files_skipped_incremental"`
			CacheHits               int64  `json:"cache_hits"`
			CacheMisses             int64  `json:"cache_misses"`
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip non-JSON lines
		}

		// Scan completion log: prefer structured `type` field; fall back to message
		// text to stay compatible with scanner images older than v0.2.
		isScanComplete := entry.Type == "scan_complete" ||
			entry.Message == "Scan completed successfully"
		if isScanComplete {
			filesScanned = entry.FilesScanned
			filesInfected = entry.FilesInfected
			filesSkipped = entry.FilesSkipped
			errorCount = entry.ErrorsCount

			// Populate incremental stats emitted since scanner v0.3.
			// These are zero-value for full scans or older scanner images,
			// which is the correct behavior (no incremental metrics emitted).
			if entry.Strategy != "" {
				nodeScan.Status.StrategyUsed = clamavv1alpha1.ScanStrategy(entry.Strategy)
			}
			nodeScan.Status.FilesSkippedIncremental = entry.FilesSkippedIncremental
			if total := entry.CacheHits + entry.CacheMisses; total > 0 {
				// Result is always in [0, 100]; clamp before narrowing to silence G115.
				rate := entry.CacheHits * 100 / total
				if rate > 100 {
					rate = 100
				}
				nodeScan.Status.CacheHitRate = int32(rate) //nolint:gosec // rate is clamped to [0,100]
			}
		}

		// Individual infected file log
		if entry.Alert == "INFECTED_FILE" && entry.FilePath != "" {
			infectedFile := clamavv1alpha1.InfectedFile{
				Path:    entry.FilePath,
				Viruses: entry.VirusNames,
				Size:    entry.FileSize,
			}

			infectedFiles = append(infectedFiles, infectedFile)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error(err, "error reading logs")
		return fmt.Errorf("error reading logs: %w", err)
	}

	const maxStoredInfected = 100

	// Detect partial output: the scanner summary line reported N infected files
	// but we only parsed M individual INFECTED_FILE entries (M < N), and M is
	// not simply due to the maxStoredInfected cap. This happens when the scanner
	// Job is killed while streaming its log output, leaving per-file records
	// incomplete. Mark the result as partial so that consumers and the retry
	// mechanism know the InfectedFiles list is not exhaustive.
	if filesInfected > 0 &&
		int64(len(infectedFiles)) < filesInfected &&
		int64(len(infectedFiles)) < maxStoredInfected {
		log.Info("partial infected file list detected — scanner output may be truncated",
			"reported_infected", filesInfected, "parsed_entries", len(infectedFiles))
		nodeScan.Status.ResultsPartial = true
	}

	// Update status
	nodeScan.Status.FilesScanned = filesScanned
	nodeScan.Status.FilesInfected = filesInfected
	nodeScan.Status.FilesSkipped = filesSkipped
	nodeScan.Status.ErrorCount = errorCount

	// Cap stored infected files at 100 to keep the CRD status object manageable.
	// The full count is preserved in FilesInfected. When truncation occurs, we
	// set InfectedFilesTruncated=true and emit a Warning event so no consumer
	// silently treats a partial list as exhaustive.
	if len(infectedFiles) > maxStoredInfected {
		nodeScan.Status.InfectedFiles = infectedFiles[:maxStoredInfected]
		nodeScan.Status.InfectedFilesTruncated = true
		r.Recorder.Event(nodeScan, corev1.EventTypeWarning, "InfectedFilesTruncated",
			fmt.Sprintf("%d infected files detected but only %d are stored in status. "+
				"Check FilesInfected for the full count.",
				len(infectedFiles), maxStoredInfected))
	} else {
		nodeScan.Status.InfectedFiles = infectedFiles
		nodeScan.Status.InfectedFilesTruncated = false
	}

	return nil
}

// updateStatus updates the NodeScan status with a condition.
// It uses Status().Patch (MergeFrom) instead of Status().Update so that only
// the fields changed here are sent to the API server, avoiding overwriting
// concurrent modifications and reducing 409 Conflict risk.
func (r *NodeScanReconciler) updateStatus(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan,
	phase clamavv1alpha1.NodeScanPhase, conditionType string, status metav1.ConditionStatus, message string) error {

	// Capture current state before any modification so Patch can compute the diff.
	original := nodeScan.DeepCopy()

	nodeScan.Status.Phase = phase
	// Inform GitOps tools (ArgoCD, Flux…) that this generation has been fully processed.
	nodeScan.Status.ObservedGeneration = nodeScan.Generation
	now := metav1.Now()
	nodeScan.Status.LastTransitionTime = &now

	// Update or add condition
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             string(phase),
		Message:            message,
		LastTransitionTime: now,
	}

	found := false
	for i, c := range nodeScan.Status.Conditions {
		if c.Type == conditionType {
			if c.Status != status {
				nodeScan.Status.Conditions[i] = condition
			}
			found = true
			break
		}
	}
	if !found {
		nodeScan.Status.Conditions = append(nodeScan.Status.Conditions, condition)
	}

	return r.Status().Patch(ctx, nodeScan, client.MergeFrom(original))
}

// updatePolicyStats updates the usage statistics of a ScanPolicy
func (r *NodeScanReconciler) updatePolicyStats(ctx context.Context, scanPolicy *clamavv1alpha1.ScanPolicy) {
	now := metav1.Now()
	scanPolicy.Status.LastUsed = &now
	scanPolicy.Status.UsageCount++
	if err := r.Status().Update(ctx, scanPolicy); err != nil {
		log.FromContext(ctx).Error(err, "failed to update ScanPolicy stats")
	}
}

// cleanupNodeScan cleans up resources when NodeScan is deleted
func (r *NodeScanReconciler) cleanupNodeScan(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan) error {
	// Delete associated Job if it exists
	if nodeScan.Status.JobRef != nil {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodeScan.Status.JobRef.Name,
				Namespace: nodeScan.Status.JobRef.Namespace,
			},
		}
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// getJobFailureInfo reads the scanner container's termination state from the Job's pods
// and returns a human-readable reason and the container exit code.
// This information is captured into the NodeScan Status so it survives Job / Pod GC.
func (r *NodeScanReconciler) getJobFailureInfo(ctx context.Context, job *batchv1.Job) (reason string, exitCode int32) {
	if job.Spec.Selector == nil {
		return "UnknownError", 0
	}
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(job.Namespace),
		client.MatchingLabels(job.Spec.Selector.MatchLabels)); err != nil {
		return "UnknownError", 0
	}
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == "scanner" && cs.State.Terminated != nil {
				failReason := cs.State.Terminated.Reason
				if failReason == "" {
					failReason = "Error"
				}
				return failReason, cs.State.Terminated.ExitCode
			}
		}
	}
	return "UnknownError", 0
}

// patchJobTTL updates the TTLSecondsAfterFinished field of a Job without replacing the whole object.
// Best-effort: errors are logged but do not fail the reconcile loop.
func (r *NodeScanReconciler) patchJobTTL(ctx context.Context, job *batchv1.Job, ttlSeconds int32) {
	log := log.FromContext(ctx)
	original := job.DeepCopy()
	job.Spec.TTLSecondsAfterFinished = ptr.To(ttlSeconds)
	if err := r.Patch(ctx, job, client.MergeFrom(original)); err != nil {
		log.Error(err, "failed to patch job TTL", "job", job.Name, "ttl", ttlSeconds)
	}
}

// SetupWithManager sets up the controller with the Manager.
// It also lazily initializes the Notifier if the caller did not provide one,
// so that main.go stays concise.
func (r *NodeScanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Notifier == nil {
		r.Notifier = notification.New(r.Client, r.Recorder)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&clamavv1alpha1.NodeScan{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// getEnvOrDefault returns the value of the environment variable named by key,
// or fallback if the variable is unset or empty.
// Used to forward operator-pod env vars (set by Helm) into scanner Job pods.
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
