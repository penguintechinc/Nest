{{/*
Expand the name of the chart.
*/}}
{{- define "nest-iscsi-gateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "nest-iscsi-gateway.fullname" -}}
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
{{- define "nest-iscsi-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nest-iscsi-gateway.labels" -}}
helm.sh/chart: {{ include "nest-iscsi-gateway.chart" . }}
{{ include "nest-iscsi-gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "nest-iscsi-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nest-iscsi-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: nest-iscsi-gateway
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nest-iscsi-gateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nest-iscsi-gateway.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
