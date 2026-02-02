/*
Copyright 2025 Platform Team - Numspot.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1" // ✅ AJOUTÉ : Import manquant
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

// ScanScheduleReconciler reconciles a ScanSchedule object
type ScanScheduleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=scanschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=scanschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=scanschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=clusterscans,verbs=get;list;watch;create;update;patch;delete

func (r *ScanScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var scanSchedule clamavv1alpha1.ScanSchedule
	if err := r.Get(ctx, req.NamespacedName, &scanSchedule); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
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

	// Check if it's time to run
	var needsRun bool
	if scanSchedule.Status.LastScheduleTime == nil {
		needsRun = true
	} else {
		lastRun := scanSchedule.Status.LastScheduleTime.Time
		missedRun := schedule.Next(lastRun)
		if missedRun.Before(now) || missedRun.Equal(now) {
			needsRun = true
		}
	}

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
					r.Delete(ctx, cs)
				}
			}
			scanSchedule.Status.Active = []corev1.ObjectReference{}
		}
	}

	if needsRun {
		// Create new ClusterScan
		clusterScan := &clamavv1alpha1.ClusterScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", scanSchedule.Name, now.Unix()),
				Namespace: scanSchedule.Namespace,
				Labels: map[string]string{
					"clamav.platform.numspot.com/schedule": scanSchedule.Name,
				},
			},
			Spec: scanSchedule.Spec.ClusterScan,
		}

		if err := r.Create(ctx, clusterScan); err != nil {
			log.Error(err, "failed to create cluster scan")
			// Enregistrer métrique d'échec
			recordScanScheduleExecution(scanSchedule.Namespace, scanSchedule.Name, "failed")
			return ctrl.Result{}, err
		}

		// Update status
		scanSchedule.Status.LastScheduleTime = &metav1.Time{Time: now}
		scanSchedule.Status.LastClusterScan = clusterScan.Name
		scanSchedule.Status.Active = append(scanSchedule.Status.Active, corev1.ObjectReference{
			Name:      clusterScan.Name,
			Namespace: clusterScan.Namespace,
		})

		r.Recorder.Event(&scanSchedule, corev1.EventTypeNormal, "ScanCreated",
			fmt.Sprintf("Created ClusterScan %s", clusterScan.Name))

		// Enregistrer métrique de succès
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

func (r *ScanScheduleReconciler) cleanupHistory(ctx context.Context, scanSchedule *clamavv1alpha1.ScanSchedule) error {
	// Get all ClusterScans for this schedule
	clusterScans := &clamavv1alpha1.ClusterScanList{}
	if err := r.List(ctx, clusterScans, client.InNamespace(scanSchedule.Namespace),
		client.MatchingLabels{"clamav.platform.numspot.com/schedule": scanSchedule.Name}); err != nil {
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

	// Update active list
	scanSchedule.Status.Active = active

	// Clean up old successful scans
	successLimit := int32(10)
	if scanSchedule.Spec.SuccessfulScansHistoryLimit != nil {
		successLimit = *scanSchedule.Spec.SuccessfulScansHistoryLimit
	}
	if len(successful) > int(successLimit) {
		// Sort by creation timestamp
		for i := 0; i < len(successful)-int(successLimit); i++ {
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

func (r *ScanScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clamavv1alpha1.ScanSchedule{}).
		Owns(&clamavv1alpha1.ClusterScan{}).
		Complete(r)
}
