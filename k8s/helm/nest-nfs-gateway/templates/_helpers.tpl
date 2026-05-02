{{/*
Expand the name of the chart.
*/}}
{{- define "nest-nfs-gateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "nest-nfs-gateway.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
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
{{- define "nest-nfs-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nest-nfs-gateway.labels" -}}
helm.sh/chart: {{ include "nest-nfs-gateway.chart" . }}
{{ include "nest-nfs-gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "nest-nfs-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nest-nfs-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: nest-nfs-gateway
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nest-nfs-gateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nest-nfs-gateway.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
