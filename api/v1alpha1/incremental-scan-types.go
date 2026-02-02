/*
Copyright 2025 Platform Team - Numspot.

Scan Incrémental - Types et Définitions
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanStrategy définit la stratégie de scan à utiliser
// +kubebuilder:validation:Enum=full;incremental;modified-only;smart
type ScanStrategy string

const (
	// ScanStrategyFull scanne tous les fichiers à chaque fois
	ScanStrategyFull ScanStrategy = "full"

	// ScanStrategyIncremental scanne uniquement les fichiers modifiés depuis le dernier scan réussi
	ScanStrategyIncremental ScanStrategy = "incremental"

	// ScanStrategyModifiedOnly scanne uniquement les fichiers modifiés dans les dernières 24h
	ScanStrategyModifiedOnly ScanStrategy = "modified-only"

	// ScanStrategySmart combine incremental + priorité aux fichiers récents
	ScanStrategySmart ScanStrategy = "smart"
)

// IncrementalScanConfig configure le comportement du scan incrémental
type IncrementalScanConfig struct {
	// Enabled active le scan incrémental
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Strategy définit la stratégie de scan
	// +kubebuilder:default=incremental
	// +optional
	Strategy ScanStrategy `json:"strategy,omitempty"`

	// BaselineInterval force un scan complet tous les X scans
	// Par exemple, si = 7, tous les 7 scans on fait un full scan
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=30
	// +optional
	BaselineInterval int32 `json:"baselineInterval,omitempty"`

	// MaxAge définit l'âge maximum (en heures) des fichiers à scanner
	// Utilisé avec modified-only et smart
	// +kubebuilder:default=24
	// +optional
	MaxAge int32 `json:"maxAge,omitempty"`

	// MinTimeBetweenScans définit le délai minimum entre deux scans (en heures)
	// Empêche de rescanner trop fréquemment le même node
	// +kubebuilder:default=6
	// +optional
	MinTimeBetweenScans int32 `json:"minTimeBetweenScans,omitempty"`

	// CacheExpiration définit la durée de validité du cache (en heures)
	// Après ce délai, un full scan est forcé
	// +kubebuilder:default=168
	// +optional
	CacheExpiration int32 `json:"cacheExpiration,omitempty"`

	// SkipUnchangedFiles saute les fichiers dont le mtime n'a pas changé
	// +kubebuilder:default=true
	// +optional
	SkipUnchangedFiles bool `json:"skipUnchangedFiles,omitempty"`
}

// FileMetadata contient les métadonnées d'un fichier scanné
type FileMetadata struct {
	// Path du fichier
	Path string `json:"path"`

	// ModTime timestamp de modification
	ModTime int64 `json:"modTime"`

	// Size en bytes
	Size int64 `json:"size"`

	// Hash SHA256 du fichier (optionnel)
	// +optional
	Hash string `json:"hash,omitempty"`

	// LastScanned timestamp du dernier scan
	LastScanned int64 `json:"lastScanned"`

	// ScanResult résultat du dernier scan (clean/infected)
	ScanResult string `json:"scanResult"`
}

// ScanCache contient le cache des fichiers scannés
type ScanCache struct {
	// NodeName du node scanné
	NodeName string `json:"nodeName"`

	// LastFullScan timestamp du dernier scan complet
	LastFullScan int64 `json:"lastFullScan"`

	// LastIncrementalScan timestamp du dernier scan incrémental
	// +optional
	LastIncrementalScan int64 `json:"lastIncrementalScan,omitempty"`

	// ScanCount nombre de scans effectués depuis le dernier full scan
	ScanCount int32 `json:"scanCount"`

	// Files métadonnées des fichiers scannés
	// Limité à 10000 entrées pour éviter une CR trop grosse
	// +optional
	Files []FileMetadata `json:"files,omitempty"`

	// TotalFiles nombre total de fichiers trackés
	TotalFiles int64 `json:"totalFiles"`

	// CacheVersion version du format de cache (pour migrations futures)
	CacheVersion string `json:"cacheVersion"`
}

// ScanCacheStatus définit le status du cache
type ScanCacheStatus struct {
	// LastUpdated timestamp de dernière mise à jour
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Size taille du cache en bytes
	// +optional
	Size int64 `json:"size,omitempty"`

	// Compressed indique si le cache est compressé
	// +optional
	Compressed bool `json:"compressed,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sc;scancache

// ScanCacheResource stocke le cache des fichiers scannés par node
type ScanCacheResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScanCache       `json:"spec,omitempty"`
	Status ScanCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScanCacheResourceList contains a list of ScanCache
type ScanCacheResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScanCacheResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScanCacheResource{}, &ScanCacheResourceList{})
}