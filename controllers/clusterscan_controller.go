/*
Copyright 2025 Platform Team - Numspot.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

const (
	clusterScanFinalizer = "clamav.platform.numspot.com/clusterscan-finalizer"
)

// ClusterScanReconciler reconciles a ClusterScan object
type ClusterScanReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=clusterscans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=clusterscans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=clusterscans/finalizers,verbs=update
// +kubebuilder:rbac:groups=clamav.platform.numspot.com,resources=nodescans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *ClusterScanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var clusterScan clamavv1alpha1.ClusterScan
	if err := r.Get(ctx, req.NamespacedName, &clusterScan); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !clusterScan.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&clusterScan, clusterScanFinalizer) {
			if err := r.cleanupClusterScan(ctx, &clusterScan); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&clusterScan, clusterScanFinalizer)
			if err := r.Update(ctx, &clusterScan); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&clusterScan, clusterScanFinalizer) {
		controllerutil.AddFinalizer(&clusterScan, clusterScanFinalizer)
		if err := r.Update(ctx, &clusterScan); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Initialize status
	if clusterScan.Status.Phase == "" {
		clusterScan.Status.Phase = clamavv1alpha1.ClusterScanPhasePending
		now := metav1.Now()
		clusterScan.Status.StartTime = &now
		if err := r.Status().Update(ctx, &clusterScan); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Get nodes to scan
	nodes, err := r.getNodesForScan(ctx, &clusterScan)
	if err != nil {
		return ctrl.Result{}, err
	}

	clusterScan.Status.TotalNodes = int32(len(nodes))

	// Create or check NodeScans
	existingNodeScans := &clamavv1alpha1.NodeScanList{}
	if err := r.List(ctx, existingNodeScans, client.InNamespace(clusterScan.Namespace),
		client.MatchingLabels{"clamav.platform.numspot.com/clusterscan": clusterScan.Name}); err != nil {
		return ctrl.Result{}, err
	}

	// Update counters
	var completed, running, failed, infected int32
	var totalScanned, totalInfected int64
	nodeRefs := []clamavv1alpha1.NodeScanReference{}

	for _, ns := range existingNodeScans.Items {
		switch ns.Status.Phase {
		case clamavv1alpha1.NodeScanPhaseCompleted:
			completed++
			totalScanned += ns.Status.FilesScanned
			totalInfected += ns.Status.FilesInfected
			if ns.Status.FilesInfected > 0 {
				infected++
			}
		case clamavv1alpha1.NodeScanPhaseRunning:
			running++
		case clamavv1alpha1.NodeScanPhaseFailed:
			failed++
		}

		nodeRefs = append(nodeRefs, clamavv1alpha1.NodeScanReference{
			Name:           ns.Name,
			NodeName:       ns.Spec.NodeName,
			Phase:          ns.Status.Phase,
			FilesInfected:  ns.Status.FilesInfected,
			FilesScanned:   ns.Status.FilesScanned,
			StartTime:      ns.Status.StartTime,
			CompletionTime: ns.Status.CompletionTime,
		})
	}

	// Create NodeScans for nodes that don't have one yet
	concurrent := clusterScan.Spec.Concurrent
	if concurrent == 0 {
		concurrent = 3
	}

	if running < concurrent {
		for _, node := range nodes {
			// Check if NodeScan already exists for this node
			exists := false
			for _, ns := range existingNodeScans.Items {
				if ns.Spec.NodeName == node.Name {
					exists = true
					break
				}
			}

			if !exists && running < concurrent {
				if err := r.createNodeScanForNode(ctx, &clusterScan, node.Name); err != nil {
					log.Error(err, "failed to create NodeScan", "node", node.Name)
					continue
				}
				running++
			}
		}
	}

	// Update status
	clusterScan.Status.CompletedNodes = completed
	clusterScan.Status.RunningNodes = running
	clusterScan.Status.FailedNodes = failed
	clusterScan.Status.InfectedNodes = infected
	clusterScan.Status.TotalFilesScanned = totalScanned
	clusterScan.Status.TotalFilesInfected = totalInfected
	clusterScan.Status.NodeScans = nodeRefs

	// Update phase
	if completed+failed == clusterScan.Status.TotalNodes {
		if failed == 0 {
			clusterScan.Status.Phase = clamavv1alpha1.ClusterScanPhaseCompleted
		} else if completed > 0 {
			clusterScan.Status.Phase = clamavv1alpha1.ClusterScanPhasePartiallyComplete
		} else {
			clusterScan.Status.Phase = clamavv1alpha1.ClusterScanPhaseFailed
		}
		now := metav1.Now()
		clusterScan.Status.CompletionTime = &now
		
		// Record metrics
		recordClusterScanMetrics(&clusterScan, clusterScan.Status.Phase)
	} else {
		clusterScan.Status.Phase = clamavv1alpha1.ClusterScanPhaseRunning
	}

	if err := r.Status().Update(ctx, &clusterScan); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if still running
	if clusterScan.Status.Phase == clamavv1alpha1.ClusterScanPhaseRunning {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ClusterScanReconciler) getNodesForScan(ctx context.Context, clusterScan *clamavv1alpha1.ClusterScan) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}

	if clusterScan.Spec.NodeSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(clusterScan.Spec.NodeSelector)
		if err != nil {
			return nil, err
		}
		if err := r.List(ctx, nodeList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return nil, err
		}
	} else {
		if err := r.List(ctx, nodeList); err != nil {
			return nil, err
		}
	}

	return nodeList.Items, nil
}

func (r *ClusterScanReconciler) createNodeScanForNode(ctx context.Context, clusterScan *clamavv1alpha1.ClusterScan, nodeName string) error {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", clusterScan.Name, nodeName),
			Namespace: clusterScan.Namespace,
			Labels: map[string]string{
				"clamav.platform.numspot.com/clusterscan": clusterScan.Name,
				"clamav.platform.numspot.com/node":        nodeName,
			},
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName:   nodeName,
			ScanPolicy: clusterScan.Spec.ScanPolicy,
			Priority:   clusterScan.Spec.Priority,
		},
	}

	// Apply template if provided
	if clusterScan.Spec.NodeScanTemplate != nil {
		// Copier les champs du template
		if clusterScan.Spec.NodeScanTemplate.Paths != nil {
			nodeScan.Spec.Paths = clusterScan.Spec.NodeScanTemplate.Paths
		}
		if clusterScan.Spec.NodeScanTemplate.MaxConcurrent != 0 {
			nodeScan.Spec.MaxConcurrent = clusterScan.Spec.NodeScanTemplate.MaxConcurrent
		}
		if clusterScan.Spec.NodeScanTemplate.Resources != nil {
			nodeScan.Spec.Resources = clusterScan.Spec.NodeScanTemplate.Resources
		}
		
		// ✅ NOUVEAU : Copier la configuration incrémentale
		if clusterScan.Spec.NodeScanTemplate.Strategy != "" {
			nodeScan.Spec.Strategy = clusterScan.Spec.NodeScanTemplate.Strategy
		}
		if clusterScan.Spec.NodeScanTemplate.IncrementalConfig != nil {
			nodeScan.Spec.IncrementalConfig = clusterScan.Spec.NodeScanTemplate.IncrementalConfig
		}
		if clusterScan.Spec.NodeScanTemplate.ForceFullScan {
			nodeScan.Spec.ForceFullScan = clusterScan.Spec.NodeScanTemplate.ForceFullScan
		}
	}

	if err := controllerutil.SetControllerReference(clusterScan, nodeScan, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, nodeScan)
}

func (r *ClusterScanReconciler) cleanupClusterScan(ctx context.Context, clusterScan *clamavv1alpha1.ClusterScan) error {
	// Delete all NodeScans owned by this ClusterScan
	nodeScans := &clamavv1alpha1.NodeScanList{}
	if err := r.List(ctx, nodeScans, client.InNamespace(clusterScan.Namespace),
		client.MatchingLabels{"clamav.platform.numspot.com/clusterscan": clusterScan.Name}); err != nil {
		return err
	}

	for _, ns := range nodeScans.Items {
		if err := r.Delete(ctx, &ns); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *ClusterScanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clamavv1alpha1.ClusterScan{}).
		Owns(&clamavv1alpha1.NodeScan{}).
		Complete(r)
}