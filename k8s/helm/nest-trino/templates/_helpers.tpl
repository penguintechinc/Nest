{{- define "nest-trino.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nest-trino.fullname" -}}
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

{{- define "nest-trino.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nest-trino.labels" -}}
helm.sh/chart: {{ include "nest-trino.chart" . }}
{{ include "nest-trino.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: nest
{{- end }}

{{- define "nest-trino.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nest-trino.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "nest-trino.serviceAccountName" -}}
{{- default (include "nest-trino.fullname" .) .Values.serviceAccount.name }}
{{- end }}

{{- define "nest-trino.image" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag }}{{ if .Values.image.digest }}@{{ .Values.image.digest }}{{ end }}
{{- end }}
