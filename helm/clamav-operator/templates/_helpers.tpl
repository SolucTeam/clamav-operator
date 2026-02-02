{{/*
Expand the name of the chart.
*/}}
{{- define "clamav-operator.name" -}}
{{- default .Chart.Name .Values.operator.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "clamav-operator.fullname" -}}
{{- if .Values.operator.fullnameOverride }}
{{- .Values.operator.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.operator.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "clamav-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "clamav-operator.labels" -}}
helm.sh/chart: {{ include "clamav-operator.chart" . }}
{{ include "clamav-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "clamav-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "clamav-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "clamav-operator.serviceAccountName" -}}
{{- if .Values.operator.serviceAccount.create }}
{{- default (include "clamav-operator.fullname" .) .Values.operator.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.operator.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the scanner service account to use
*/}}
{{- define "clamav-operator.scannerServiceAccountName" -}}
{{- if .Values.scanner.serviceAccount.create }}
{{- default "clamav-scanner" .Values.scanner.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.scanner.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for RBAC.
*/}}
{{- define "clamav-operator.rbac.apiVersion" -}}
{{- if .Capabilities.APIVersions.Has "rbac.authorization.k8s.io/v1" -}}
rbac.authorization.k8s.io/v1
{{- else -}}
rbac.authorization.k8s.io/v1beta1
{{- end -}}
{{- end -}}

{{/*
Return the operator image
*/}}
{{- define "clamav-operator.image" -}}
{{- $tag := .Values.operator.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.operator.image.repository $tag -}}
{{- end -}}

{{/*
Return the scanner image
*/}}
{{- define "clamav-operator.scannerImage" -}}
{{- printf "%s:%s" .Values.scanner.image.repository .Values.scanner.image.tag -}}
{{- end -}}
