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
}

// +kubebuilder:rbac:groups=clamav.io,resources=nodescans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clamav.io,resources=nodescans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clamav.io,resources=nodescans/finalizers,verbs=update
// +kubebuilder:rbac:groups=clamav.io,resources=scanpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list
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
	jobName := fmt.Sprintf("nodescan-%s", nodeScan.Name)
	if len(jobName) > 63 {
		jobName = jobName[:63]
	}

	var existingJob batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: nodeScan.Namespace}, &existingJob)

	if errors.IsNotFound(err) {
		// Initialize status if needed
		if nodeScan.Status.Phase == "" {
			nodeScan.Status.Phase = clamavv1alpha1.NodeScanPhasePending
			now := metav1.Now()
			nodeScan.Status.StartTime = &now
			if err := r.Status().Update(ctx, &nodeScan); err != nil {
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
			if err := r.parseJobResults(ctx, &nodeScan, &existingJob); err != nil {
				// Track retry count in annotations
				retryCount := 0
				if nodeScan.Annotations != nil {
					if val, ok := nodeScan.Annotations[parseRetryAnnotation]; ok {
						_, _ = fmt.Sscanf(val, "%d", &retryCount)
					}
				}
				retryCount++

				if retryCount >= maxParseRetries {
					// Max retries exceeded - mark as completed with partial results
					log.Error(err, "max parse retries exceeded, completing with partial results",
						"retries", retryCount)
					r.Recorder.Event(&nodeScan, corev1.EventTypeWarning, "ParseResultsMaxRetries",
						fmt.Sprintf("Failed to parse scan results after %d attempts: %v", retryCount, err))
					// Continue with completion - don't block on parse failures
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
					backoff := time.Duration(retryCount*10) * time.Second
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

			// Send notifications if infected files found.
			// Fire-and-forget via goroutine: HTTP/SMTP calls must never block the reconcile worker.
			if nodeScan.Status.FilesInfected > 0 && scanPolicy != nil {
				nodeScanSnap := nodeScan.DeepCopy()
				scanPolicySnap := scanPolicy.DeepCopy()
				go func() {
					notifCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					r.Notifier.Send(notifCtx, nodeScanSnap, scanPolicySnap)
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

	// Job name
	jobName := fmt.Sprintf("nodescan-%s", nodeScan.Name)
	if len(jobName) > 63 {
		jobName = jobName[:63]
	}

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
				"clamav.io/nodescan":          nodeScan.Name,
				"clamav.io/node":              nodeScan.Spec.NodeName,
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
						"target-node": nodeScan.Spec.NodeName,
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
					// ImagePullSecrets can be configured via Helm values or ScanPolicy
					ImagePullSecrets: []corev1.LocalObjectReference{},
					Containers: []corev1.Container{
						{
							Name:            "scanner",
							Image:           r.ScannerImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             envVars,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "host-root",
									MountPath: "/host",
									ReadOnly:  true,
								},
								{
									Name:      "scan-results",
									MountPath: "/results",
								},
							},
							Resources: resources,
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
					Volumes: []corev1.Volume{
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
					},
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

	pod := podList.Items[0]

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

		// JSON log structure
		type LogEntry struct {
			Level         string   `json:"level"`
			Message       string   `json:"message"`
			FilesScanned  int64    `json:"files_scanned"`
			FilesInfected int64    `json:"files_infected"`
			FilesSkipped  int64    `json:"files_skipped"`
			ErrorsCount   int64    `json:"errors_count"`
			FilePath      string   `json:"file_path"`
			VirusNames    []string `json:"virus_names"`
			FileSize      int64    `json:"file_size"`
			Alert         string   `json:"alert"`
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip non-JSON lines
		}

		// Scan completion log
		if entry.Message == "Scan terminé avec succès" {
			filesScanned = entry.FilesScanned
			filesInfected = entry.FilesInfected
			filesSkipped = entry.FilesSkipped
			errorCount = entry.ErrorsCount
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

	// Update status
	nodeScan.Status.FilesScanned = filesScanned
	nodeScan.Status.FilesInfected = filesInfected
	nodeScan.Status.FilesSkipped = filesSkipped
	nodeScan.Status.ErrorCount = errorCount

	// Limit to 100 infected files for performance
	if len(infectedFiles) > 100 {
		nodeScan.Status.InfectedFiles = infectedFiles[:100]
	} else {
		nodeScan.Status.InfectedFiles = infectedFiles
	}

	return nil
}

// updateStatus updates the NodeScan status with a condition
func (r *NodeScanReconciler) updateStatus(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan,
	phase clamavv1alpha1.NodeScanPhase, conditionType string, status metav1.ConditionStatus, message string) error {

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

	return r.Status().Update(ctx, nodeScan)
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
