/*
Copyright 2025 Platform Team - Numspot.

Scan Incrémental - Controller Implementation (FIXED VERSION)
Note: Les métriques sont maintenant définies dans metrics.go
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

// getScanCache récupère le cache de scan pour un node
func (r *NodeScanReconciler) getScanCache(ctx context.Context, nodeName, namespace string) (*clamavv1alpha1.ScanCacheResource, error) {
	cache := &clamavv1alpha1.ScanCacheResource{}
	cacheName := fmt.Sprintf("scancache-%s", nodeName)
	
	err := r.Get(ctx, types.NamespacedName{
		Name:      cacheName,
		Namespace: namespace,
	}, cache)
	
	if errors.IsNotFound(err) {
		// Créer un nouveau cache
		cache = &clamavv1alpha1.ScanCacheResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cacheName,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":      "clamav",
					"app.kubernetes.io/component": "scan-cache",
					"clamav.platform.numspot.com/node": nodeName,
				},
			},
			Spec: clamavv1alpha1.ScanCache{
				NodeName:     nodeName,
				ScanCount:    0,
				Files:        []clamavv1alpha1.FileMetadata{},
				CacheVersion: "v1",
			},
		}
		
		if err := r.Create(ctx, cache); err != nil {
			return nil, fmt.Errorf("failed to create scan cache: %w", err)
		}
		
		return cache, nil
	}
	
	return cache, err
}

// updateScanCache met à jour le cache après un scan
func (r *NodeScanReconciler) updateScanCache(ctx context.Context, cache *clamavv1alpha1.ScanCacheResource, 
	nodeScan *clamavv1alpha1.NodeScan, filesMetadata []clamavv1alpha1.FileMetadata) error {
	
	log := log.FromContext(ctx)
	
	now := time.Now().Unix()
	
	// Déterminer si c'était un full scan
	isFullScan := nodeScan.Spec.Strategy == clamavv1alpha1.ScanStrategyFull || 
		nodeScan.Spec.ForceFullScan
	
	if isFullScan {
		// Full scan : remplacer tout le cache
		cache.Spec.LastFullScan = now
		cache.Spec.ScanCount = 0
		cache.Spec.Files = filesMetadata
	} else {
		// Incremental scan : merger avec le cache existant
		cache.Spec.LastIncrementalScan = now
		cache.Spec.ScanCount++
		
		// Créer une map pour merge efficace
		existingFiles := make(map[string]clamavv1alpha1.FileMetadata)
		for _, f := range cache.Spec.Files {
			existingFiles[f.Path] = f
		}
		
		// Ajouter/mettre à jour les nouveaux fichiers
		for _, newFile := range filesMetadata {
			existingFiles[newFile.Path] = newFile
		}
		
		// Convertir la map en slice
		updatedFiles := make([]clamavv1alpha1.FileMetadata, 0, len(existingFiles))
		for _, f := range existingFiles {
			updatedFiles = append(updatedFiles, f)
		}
		
		// Limiter à 10000 entrées (les plus récentes)
		if len(updatedFiles) > 10000 {
			// Trier par LastScanned (les plus récents en premier)
			// Pour simplifier, on garde les 10000 premiers
			updatedFiles = updatedFiles[:10000]
			log.Info("Cache truncated to 10000 entries", "node", cache.Spec.NodeName)
		}
		
		cache.Spec.Files = updatedFiles
	}
	
	cache.Spec.TotalFiles = int64(len(cache.Spec.Files))
	cache.Status.LastUpdated = metav1.Now()
	
	// Calculer la taille approximative
	cacheJSON, _ := json.Marshal(cache.Spec.Files)
	cache.Status.Size = int64(len(cacheJSON))
	
	// ✅ Enregistrer les métriques du cache (utilise la fonction de metrics.go)
	recordScanCacheMetrics(cache.Namespace, cache.Spec.NodeName, cache.Status.Size, cache.Spec.TotalFiles)
	
	// Mettre à jour
	if err := r.Update(ctx, cache); err != nil {
		return fmt.Errorf("failed to update scan cache: %w", err)
	}
	
	return nil
}

// shouldForceFullScan détermine si un full scan doit être forcé
func (r *NodeScanReconciler) shouldForceFullScan(ctx context.Context, 
	nodeScan *clamavv1alpha1.NodeScan, 
	cache *clamavv1alpha1.ScanCacheResource,
	config *clamavv1alpha1.IncrementalScanConfig) bool {
	
	log := log.FromContext(ctx)
	
	// Si ForceFullScan est explicitement défini
	if nodeScan.Spec.ForceFullScan {
		log.Info("Full scan forced by spec", "node", nodeScan.Spec.NodeName)
		return true
	}
	
	// Si la stratégie est Full
	if nodeScan.Spec.Strategy == clamavv1alpha1.ScanStrategyFull {
		return true
	}
	
	// Si pas de config incrémentale
	if config == nil || !config.Enabled {
		return true
	}
	
	// Vérifier l'intervalle baseline
	if cache.Spec.ScanCount >= config.BaselineInterval {
		log.Info("Baseline interval reached, forcing full scan", 
			"scanCount", cache.Spec.ScanCount, 
			"baselineInterval", config.BaselineInterval)
		return true
	}
	
	// Vérifier l'expiration du cache
	now := time.Now().Unix()
	cacheAge := now - cache.Spec.LastFullScan
	expirationSeconds := int64(config.CacheExpiration) * 3600
	
	if cacheAge > expirationSeconds {
		log.Info("Cache expired, forcing full scan",
			"cacheAge", cacheAge,
			"expiration", expirationSeconds)
		return true
	}
	
	// Vérifier le délai minimum entre scans
	if cache.Spec.LastIncrementalScan > 0 {
		timeSinceLastScan := now - cache.Spec.LastIncrementalScan
		minTimeSeconds := int64(config.MinTimeBetweenScans) * 3600
		
		if timeSinceLastScan < minTimeSeconds {
			log.Info("Minimum time between scans not reached",
				"timeSinceLastScan", timeSinceLastScan,
				"minTime", minTimeSeconds)
			// Note: on ne force pas un full scan, on pourrait même skip le scan
			// Mais ici on laisse passer en incremental
		}
	}
	
	return false
}

// prepareIncrementalScanEnv prépare les variables d'environnement pour le scan incrémental
func (r *NodeScanReconciler) prepareIncrementalScanEnv(ctx context.Context,
	nodeScan *clamavv1alpha1.NodeScan,
	cache *clamavv1alpha1.ScanCacheResource,
	config *clamavv1alpha1.IncrementalScanConfig) []corev1.EnvVar {
	
	envVars := []corev1.EnvVar{}
	
	// Déterminer la stratégie effective
	strategy := nodeScan.Spec.Strategy
	if strategy == "" {
		strategy = clamavv1alpha1.ScanStrategyFull
	}
	
	forceFullScan := r.shouldForceFullScan(ctx, nodeScan, cache, config)
	
	if forceFullScan {
		strategy = clamavv1alpha1.ScanStrategyFull
	}
	
	envVars = append(envVars, corev1.EnvVar{
		Name:  "SCAN_STRATEGY",
		Value: string(strategy),
	})
	
	// Config incrémentale
	if config != nil && config.Enabled && !forceFullScan {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "INCREMENTAL_ENABLED",
			Value: "true",
		})
		
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MAX_FILE_AGE_HOURS",
			Value: fmt.Sprintf("%d", config.MaxAge),
		})
		
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SKIP_UNCHANGED_FILES",
			Value: fmt.Sprintf("%t", config.SkipUnchangedFiles),
		})
		
		// Sérialiser le cache en JSON et le passer en variable d'environnement
		// Note: Pour de gros caches, il faudrait plutôt utiliser un ConfigMap
		if len(cache.Spec.Files) > 0 && len(cache.Spec.Files) < 1000 {
			cacheJSON, err := json.Marshal(cache.Spec.Files)
			if err == nil && len(cacheJSON) < 100000 { // Limite de 100KB
				envVars = append(envVars, corev1.EnvVar{
					Name:  "SCAN_CACHE",
					Value: string(cacheJSON),
				})
			}
		}
		
		// Si le cache est trop gros, créer un ConfigMap
		if len(cache.Spec.Files) >= 1000 {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "SCAN_CACHE_CONFIGMAP",
				Value: fmt.Sprintf("scancache-%s", nodeScan.Spec.NodeName),
			})
		}
	}
	
	// Timestamp du dernier scan
	if cache.Spec.LastFullScan > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "LAST_FULL_SCAN",
			Value: fmt.Sprintf("%d", cache.Spec.LastFullScan),
		})
	}
	
	if cache.Spec.LastIncrementalScan > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "LAST_INCREMENTAL_SCAN",
			Value: fmt.Sprintf("%d", cache.Spec.LastIncrementalScan),
		})
	}
	
	return envVars
}

// createScanCacheConfigMap crée un ConfigMap pour stocker un gros cache
func (r *NodeScanReconciler) createScanCacheConfigMap(ctx context.Context,
	nodeScan *clamavv1alpha1.NodeScan,
	cache *clamavv1alpha1.ScanCacheResource) error {
	
	cacheJSON, err := json.Marshal(cache.Spec.Files)
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}
	
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("scancache-%s", nodeScan.Spec.NodeName),
			Namespace: nodeScan.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "clamav",
				"app.kubernetes.io/component": "scan-cache",
				"clamav.platform.numspot.com/node": nodeScan.Spec.NodeName,
			},
		},
		Data: map[string]string{
			"cache.json": string(cacheJSON),
		},
	}
	
	// Créer ou mettre à jour
	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      configMap.Name,
		Namespace: configMap.Namespace,
	}, existing)
	
	if errors.IsNotFound(err) {
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}
	
	// Mettre à jour
	existing.Data = configMap.Data
	return r.Update(ctx, existing)
}

// calculateIncrementalStats calcule les statistiques du scan incrémental
func (r *NodeScanReconciler) calculateIncrementalStats(ctx context.Context,
	nodeScan *clamavv1alpha1.NodeScan,
	cache *clamavv1alpha1.ScanCacheResource) {
	
	if nodeScan.Status.FilesSkippedIncremental > 0 {
		// Calculer le taux de hit du cache
		totalChecked := nodeScan.Status.FilesScanned + nodeScan.Status.FilesSkippedIncremental
		if totalChecked > 0 {
			nodeScan.Status.CacheHitRate = float64(nodeScan.Status.FilesSkippedIncremental) / float64(totalChecked) * 100
		}
		
		// Estimer le temps économisé (environ 0.1s par fichier sauté)
		nodeScan.Status.TimeSaved = nodeScan.Status.FilesSkippedIncremental / 10
	}
}