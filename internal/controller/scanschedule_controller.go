/*
Copyright 2025 The ClamAV Operator Authors.
*/

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

const scanScheduleFinalizer = "clamav.io/scanschedule-finalizer"

// ScanScheduleReconciler reconciles a ScanSchedule object
type ScanScheduleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=clamav.io,resources=scanschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clamav.io,resources=scanschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clamav.io,resources=scanschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=clamav.io,resources=clusterscans,verbs=get;list;watch;create;update;patch;delete

func (r *ScanScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var scanSchedule clamavv1alpha1.ScanSchedule
	if err := r.Get(ctx, req.NamespacedName, &scanSchedule); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// ── Deletion handling ──────────────────────────────────────────────────────
	if !scanSchedule.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&scanSchedule, scanScheduleFinalizer) {
			if err := r.deleteOwnedClusterScans(ctx, &scanSchedule); err != nil {
				return ctrl.Result{}, err
			}
			original := scanSchedule.DeepCopy()
			controllerutil.RemoveFinalizer(&scanSchedule, scanScheduleFinalizer)
			if err := r.Patch(ctx, &scanSchedule, client.MergeFrom(original)); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// ── Ensure finalizer ───────────────────────────────────────────────────────
	if !controllerutil.ContainsFinalizer(&scanSchedule, scanScheduleFinalizer) {
		original := scanSchedule.DeepCopy()
		controllerutil.AddFinalizer(&scanSchedule, scanScheduleFinalizer)
		if err := r.Patch(ctx, &scanSchedule, client.MergeFrom(original)); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Parse cron schedule
	schedule, err := cron.ParseStandard(scanSchedule.Spec.Schedule)
	if err != nil {
		log.Error(err, "invalid cron schedule")
		return ctrl.Result{}, err
	}

	now := time.Now()
	nextRun := schedule.Next(now)

	// Update next schedule time
	scanSchedule.Status.NextScheduleTime = &metav1.Time{Time: nextRun}

	// Check if suspended
	if scanSchedule.Spec.Suspend {
		log.Info("scan schedule is suspended")
		if err := r.Status().Update(ctx, &scanSchedule); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Until(nextRun)}, nil
	}

	// Check if it's time to run.
	//
	// On first reconcile (LastScheduleTime == nil) we intentionally do NOT run
	// immediately: we wait for the first occurrence of the cron expression.
	// This mirrors the behavior of Kubernetes CronJobs and prevents a
	// surprise scan burst every time a ScanSchedule is (re)created.
	//
	// When the operator was down for multiple intervals we advance through ALL
	// missed runs but only trigger ONE scan (the most recent scheduled time).
	// We record the last *scheduled* time (not time.Now()) so that subsequent
	// runs are anchored to the cron grid and the schedule never drifts.
	var needsRun bool
	var scheduledTime time.Time

	if scanSchedule.Status.LastScheduleTime != nil {
		lastRun := scanSchedule.Status.LastScheduleTime.Time
		// Walk forward from lastRun through every occurrence that has already
		// passed.  The final value of scheduledTime is the most-recent missed
		// slot; intermediate slots are intentionally skipped (no burst catch-up).
		for t := schedule.Next(lastRun); !t.After(now); t = schedule.Next(t) {
			scheduledTime = t
		}
		needsRun = !scheduledTime.IsZero()
	}
	// needsRun remains false when LastScheduleTime is nil (first reconcile).
	// The controller will requeue at nextRun and trigger at the correct time.

	if needsRun {
		// Check concurrency policy
		if scanSchedule.Spec.ConcurrencyPolicy == "Forbid" && len(scanSchedule.Status.Active) > 0 {
			log.Info("skipping run due to concurrency policy", "policy", "Forbid")
			needsRun = false
		} else if scanSchedule.Spec.ConcurrencyPolicy == "Replace" && len(scanSchedule.Status.Active) > 0 {
			// Delete active scans
			for _, ref := range scanSchedule.Status.Active {
				cs := &clamavv1alpha1.ClusterScan{}
				if err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, cs); err == nil {
					if deleteErr := r.Delete(ctx, cs); deleteErr != nil {
						log.Error(deleteErr, "failed to delete active ClusterScan during Replace policy")
					}
				}
			}
			scanSchedule.Status.Active = []corev1.ObjectReference{}
		}
	}

	if needsRun {
		// Create new ClusterScan owned by this ScanSchedule so that:
		//   1. The GC cascade deletes it when the ScanSchedule is deleted.
		//   2. The .Owns() watch in SetupWithManager triggers reconciliation
		//      whenever a child ClusterScan changes phase.
		clusterScan := &clamavv1alpha1.ClusterScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", scanSchedule.Name, now.Unix()),
				Namespace: scanSchedule.Namespace,
				Labels: map[string]string{
					"clamav.io/schedule": scanSchedule.Name,
				},
			},
			Spec: scanSchedule.Spec.ClusterScan,
		}

		// Set the ScanSchedule as the controller owner so that Kubernetes GC
		// and the .Owns() watch both work correctly.
		if err := controllerutil.SetControllerReference(&scanSchedule, clusterScan, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, clusterScan); err != nil {
			log.Error(err, "failed to create cluster scan")
			recordScanScheduleExecution(scanSchedule.Namespace, scanSchedule.Name, "failed")
			return ctrl.Result{}, err
		}

		// Anchor LastScheduleTime to the scheduled slot, not to time.Now().
		// This keeps subsequent runs on the cron grid and prevents drift when
		// the operator was temporarily unavailable.
		scanSchedule.Status.LastScheduleTime = &metav1.Time{Time: scheduledTime}
		scanSchedule.Status.LastClusterScan = clusterScan.Name
		scanSchedule.Status.Active = append(scanSchedule.Status.Active, corev1.ObjectReference{
			Name:      clusterScan.Name,
			Namespace: clusterScan.Namespace,
		})

		r.Recorder.Event(&scanSchedule, corev1.EventTypeNormal, "ScanCreated",
			fmt.Sprintf("Created ClusterScan %s", clusterScan.Name))

		recordScanScheduleExecution(scanSchedule.Namespace, scanSchedule.Name, "success")
	}

	// Clean up completed scans
	if err := r.cleanupHistory(ctx, &scanSchedule); err != nil {
		log.Error(err, "failed to cleanup history")
	}

	if err := r.Status().Update(ctx, &scanSchedule); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Until(nextRun)}, nil
}

// deleteOwnedClusterScans deletes all ClusterScans owned by the given ScanSchedule.
// Called during finalizer processing to ensure synchronous cleanup before the
// ScanSchedule object itself is removed from the API server.
func (r *ScanScheduleReconciler) deleteOwnedClusterScans(ctx context.Context, scanSchedule *clamavv1alpha1.ScanSchedule) error {
	log := log.FromContext(ctx)
	clusterScans := &clamavv1alpha1.ClusterScanList{}
	if err := r.List(ctx, clusterScans,
		client.InNamespace(scanSchedule.Namespace),
		client.MatchingLabels{"clamav.io/schedule": scanSchedule.Name},
	); err != nil {
		return err
	}
	for i := range clusterScans.Items {
		cs := &clusterScans.Items[i]
		if err := r.Delete(ctx, cs); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "failed to delete ClusterScan during ScanSchedule cleanup", "clusterScan", cs.Name)
			return err
		}
	}
	return nil
}

func (r *ScanScheduleReconciler) cleanupHistory(ctx context.Context, scanSchedule *clamavv1alpha1.ScanSchedule) error {
	// Get all ClusterScans for this schedule
	clusterScans := &clamavv1alpha1.ClusterScanList{}
	if err := r.List(ctx, clusterScans, client.InNamespace(scanSchedule.Namespace),
		client.MatchingLabels{"clamav.io/schedule": scanSchedule.Name}); err != nil {
		return err
	}

	// Separate by status
	var successful, failed []clamavv1alpha1.ClusterScan
	var active []corev1.ObjectReference

	for _, cs := range clusterScans.Items {
		switch cs.Status.Phase {
		case clamavv1alpha1.ClusterScanPhaseCompleted:
			successful = append(successful, cs)
		case clamavv1alpha1.ClusterScanPhaseFailed, clamavv1alpha1.ClusterScanPhasePartiallyComplete:
			failed = append(failed, cs)
		default:
			active = append(active, corev1.ObjectReference{
				Name:      cs.Name,
				Namespace: cs.Namespace,
			})
		}
	}

	// Sort by CreationTimestamp ascending (oldest first) so that pruning always
	// removes the oldest entries.  The Kubernetes API does not guarantee any
	// ordering for List results, so an explicit sort is required.
	sortByAge := func(items []clamavv1alpha1.ClusterScan) {
		sort.Slice(items, func(i, j int) bool {
			return items[i].CreationTimestamp.Before(&items[j].CreationTimestamp)
		})
	}
	sortByAge(successful)
	sortByAge(failed)

	// Update active list
	scanSchedule.Status.Active = active

	// Clean up old successful scans
	successLimit := int32(10)
	if scanSchedule.Spec.SuccessfulScansHistoryLimit != nil {
		successLimit = *scanSchedule.Spec.SuccessfulScansHistoryLimit
	}
	if len(successful) > int(successLimit) {
		for i := 0; i < len(successful)-int(successLimit); i++ {
			// Proactively delete child NodeScans before the ClusterScan to avoid
			// accumulation while waiting for the GC / finalizer cascade.
			r.cleanupNodeScansForClusterScan(ctx, successful[i].Name, scanSchedule.Namespace)
			if err := r.Delete(ctx, &successful[i]); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	// Clean up old failed scans
	failedLimit := int32(3)
	if scanSchedule.Spec.FailedScansHistoryLimit != nil {
		failedLimit = *scanSchedule.Spec.FailedScansHistoryLimit
	}
	if len(failed) > int(failedLimit) {
		for i := 0; i < len(failed)-int(failedLimit); i++ {
			r.cleanupNodeScansForClusterScan(ctx, failed[i].Name, scanSchedule.Namespace)
			if err := r.Delete(ctx, &failed[i]); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	// Update last successful time if there are successful scans
	if len(successful) > 0 {
		lastSuccessful := successful[len(successful)-1]
		if lastSuccessful.Status.CompletionTime != nil {
			scanSchedule.Status.LastSuccessfulTime = lastSuccessful.Status.CompletionTime
		}
	}

	return nil
}

// cleanupNodeScansForClusterScan proactively deletes all NodeScan objects owned by the given
// ClusterScan. This is called before deleting the ClusterScan itself so that terminated
// NodeScan objects don't accumulate in the cluster while waiting for the GC cascade.
func (r *ScanScheduleReconciler) cleanupNodeScansForClusterScan(ctx context.Context, clusterScanName, namespace string) {
	log := log.FromContext(ctx)
	nodeScans := &clamavv1alpha1.NodeScanList{}
	if err := r.List(ctx, nodeScans,
		client.InNamespace(namespace),
		client.MatchingLabels{"clamav.io/clusterscan": clusterScanName}); err != nil {
		log.Error(err, "failed to list NodeScans for cleanup", "clusterScan", clusterScanName)
		return
	}
	for i := range nodeScans.Items {
		ns := &nodeScans.Items[i]
		if err := r.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "failed to delete NodeScan during cleanup", "nodeScan", ns.Name)
		}
	}
}

func (r *ScanScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clamavv1alpha1.ScanSchedule{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&clamavv1alpha1.ClusterScan{}).
		Complete(r)
}
