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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

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

	// Capture state for all status patches in this reconcile.
	// Must be after finalizer operations so that statusBase carries the current
	// ResourceVersion; using Patch (MergeFrom) instead of Update avoids
	// overwriting concurrent status changes with a stale full-object write.
	statusBase := scanSchedule.DeepCopy()

	// Parse cron schedule
	schedule, err := cron.ParseStandard(scanSchedule.Spec.Schedule)
	if err != nil {
		log.Error(err, "invalid cron schedule", "schedule", scanSchedule.Spec.Schedule)
		// Surface the parse error as a condition so users see it via
		// `kubectl get scanschedule` instead of having to grep operator logs.
		// We write the condition and stop — no point requeueing until the
		// spec is fixed (the watch will trigger a new reconcile on update).
		apimeta.SetStatusCondition(&scanSchedule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "InvalidSchedule",
			Message:            fmt.Sprintf("invalid cron expression %q: %v", scanSchedule.Spec.Schedule, err),
			ObservedGeneration: scanSchedule.Generation,
		})
		// Best-effort status patch; ignore errors (the spec fix will trigger
		// a new reconcile which will re-evaluate and update the condition).
		_ = r.Status().Patch(ctx, &scanSchedule, client.MergeFrom(statusBase))
		return ctrl.Result{}, nil // do not requeue — wait for spec change
	}
	// Clear any previous InvalidSchedule condition now that the expression is valid.
	apimeta.RemoveStatusCondition(&scanSchedule.Status.Conditions, "Ready")

	now := time.Now()
	nextRun := schedule.Next(now)

	// Update next schedule time
	scanSchedule.Status.NextScheduleTime = &metav1.Time{Time: nextRun}

	// ── Rebuild Active list from live API state FIRST ─────────────────────────
	// This must happen before the concurrency check and before the suspend early
	// return so that every code path works with a consistent, up-to-date Active
	// list.  Without this, a stale Active=[C1] (where C1 already Completed)
	// would cause Forbid to silently block every subsequent run, and unsuspending
	// a schedule would be blocked by ghost entries left over from before the
	// suspend.
	if err := r.cleanupHistory(ctx, &scanSchedule); err != nil {
		// Do NOT overwrite Status.Active with the partial/nil result — abort and
		// retry so we never persist a falsely-empty Active list.
		log.Error(err, "failed to rebuild active list; retrying")
		return ctrl.Result{}, err
	}

	// Check if suspended
	if scanSchedule.Spec.Suspend {
		log.Info("scan schedule is suspended")
		if err := r.Status().Patch(ctx, &scanSchedule, client.MergeFrom(statusBase)); err != nil {
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

	// Determine the reference point for "last known run":
	//   - If the schedule has run before, use LastScheduleTime.
	//   - On the very first reconcile (LastScheduleTime == nil) seed with the
	//     object's CreationTimestamp so that naturally-occurring cron slots after
	//     creation are discovered correctly.
	//
	// Without this seed the loop is never entered: needsRun stays false forever,
	// LastScheduleTime is never written, and the schedule never fires.
	{
		var lastRun time.Time
		if scanSchedule.Status.LastScheduleTime != nil {
			lastRun = scanSchedule.Status.LastScheduleTime.Time
		} else if !scanSchedule.CreationTimestamp.IsZero() {
			// First reconcile: anchor to creation time so slots that have elapsed
			// since the object was created are discovered on the next cron tick.
			lastRun = scanSchedule.CreationTimestamp.Time
		} else {
			// CreationTimestamp is zero (e.g. fake client in unit tests).
			// Anchor to now so no past slots are detected — the first real
			// cron tick will trigger normally via RequeueAfter.
			lastRun = now
		}
		// Walk forward from lastRun through every occurrence that has already
		// passed.  The final value of scheduledTime is the most-recent missed
		// slot; intermediate slots are intentionally skipped (no burst catch-up).
		//
		// Guard: robfig/cron returns time.Time{} when no slot is found within
		// the next 5 years (yearLimit = t.Year()+5).  Without the IsZero check,
		// a zero lastRun (year 1) causes the library to return time.Time{} after
		// year 6, which is still before now, creating an infinite loop.
		for t := schedule.Next(lastRun); !t.IsZero() && !t.After(now); t = schedule.Next(t) {
			scheduledTime = t
		}
		needsRun = !scheduledTime.IsZero()

		// startingDeadlineSeconds: if the missed slot is older than the deadline,
		// skip it rather than running a very stale scan.
		// Example: operator was down 3 days, deadline=3600 → skip all missed slots
		// that are more than 1 hour old; wait for the next on-time slot instead.
		if needsRun && scanSchedule.Spec.StartingDeadlineSeconds != nil {
			deadline := time.Duration(*scanSchedule.Spec.StartingDeadlineSeconds) * time.Second
			if now.Sub(scheduledTime) > deadline {
				log.Info("skipping missed slot: older than startingDeadlineSeconds",
					"scheduledTime", scheduledTime,
					"age", now.Sub(scheduledTime).Round(time.Second),
					"deadline", deadline)
				// Advance LastScheduleTime so the controller does not retry this
				// expired slot on subsequent reconciles.
				scanSchedule.Status.LastScheduleTime = &metav1.Time{Time: scheduledTime}
				needsRun = false
			}
		}
	}

	if needsRun {
		// Concurrency check uses the freshly-rebuilt Active list from
		// cleanupHistory above — not the potentially-stale value from the Get.
		if scanSchedule.Spec.ConcurrencyPolicy == "Forbid" && len(scanSchedule.Status.Active) > 0 {
			log.Info("skipping run due to concurrency policy", "policy", "Forbid")
			// Advance LastScheduleTime to the blocked slot so that when the active
			// scan finishes and cleanupHistory clears Active, the controller does
			// NOT immediately create a delayed scan for this same slot.
			// This matches Kubernetes CronJob Forbid semantics: a blocked run is
			// skipped, not deferred. Without this, a brief informer-cache miss in
			// cleanupHistory could allow a duplicate scan to be created.
			scanSchedule.Status.LastScheduleTime = &metav1.Time{Time: scheduledTime}
			needsRun = false
		} else if scanSchedule.Spec.ConcurrencyPolicy == "Replace" && len(scanSchedule.Status.Active) > 0 {
			// Request deletion of all active ClusterScans.
			// We do NOT clear Status.Active here — it will be rebuilt on the next
			// reconcile (triggered by the ClusterScan deletion event via .Owns()),
			// naturally excluding ClusterScans that have a DeletionTimestamp.
			// Clearing prematurely would cause a second reconcile to recreate
			// them before the GC has had a chance to remove them.
			for _, ref := range scanSchedule.Status.Active {
				cs := &clamavv1alpha1.ClusterScan{}
				if err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, cs); err == nil {
					if deleteErr := r.Delete(ctx, cs); deleteErr != nil && !errors.IsNotFound(deleteErr) {
						log.Error(deleteErr, "failed to delete active ClusterScan during Replace policy")
					}
				}
			}
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

	if err := r.Status().Patch(ctx, &scanSchedule, client.MergeFrom(statusBase)); err != nil {
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
	// Get all ClusterScans for this schedule.
	// Return early on error WITHOUT touching Status.Active — overwriting it with
	// a nil/empty slice when the List fails would silently drop the active list
	// and allow duplicate scans to be created on the next reconcile.
	clusterScans := &clamavv1alpha1.ClusterScanList{}
	if err := r.List(ctx, clusterScans, client.InNamespace(scanSchedule.Namespace),
		client.MatchingLabels{"clamav.io/schedule": scanSchedule.Name}); err != nil {
		return err
	}

	// Separate by status
	var successful, failed []clamavv1alpha1.ClusterScan
	var active []corev1.ObjectReference

	for _, cs := range clusterScans.Items {
		// ClusterScans with a DeletionTimestamp are being removed (e.g. by a
		// Replace concurrency policy). Exclude them from the active list so
		// they don't block a new scan from being created on the next reconcile.
		if !cs.DeletionTimestamp.IsZero() {
			continue
		}

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
	// The label value is always written via sanitizeLabelValue in clusterscan_controller.go,
	// so we must query with the same sanitized value to find NodeScans belonging to
	// ClusterScans whose names exceed 63 characters.
	if err := r.List(ctx, nodeScans,
		client.InNamespace(namespace),
		client.MatchingLabels{"clamav.io/clusterscan": sanitizeLabelValue(clusterScanName)}); err != nil {
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
		// No GenerationChangedPredicate here: ScanSchedule reconciliation is
		// driven primarily by RequeueAfter (cron clock), not by spec changes.
		// The predicate would cause requeues set up before a pod restart to be
		// lost — on restart the initial-sync Create event passes the predicate,
		// but subsequent timer-driven reconciles must be allowed through too.
		For(&clamavv1alpha1.ScanSchedule{}).
		Owns(&clamavv1alpha1.ClusterScan{}).
		Complete(r)
}
