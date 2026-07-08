{{- define "nest-ceph-cluster.labels" -}}
app.kubernetes.io/part-of: nest
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
