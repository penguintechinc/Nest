{{- define "nest-injector.fullname" -}}
{{- printf "%s-%s" .Release.Name "nest-injector" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nest-injector.labels" -}}
app.kubernetes.io/name: nest-injector
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "nest-injector.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nest-injector.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
