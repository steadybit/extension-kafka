{{/* vim: set filetype=mustache: */}}

{{/*
Secret name for legacy single-cluster configuration
*/}}
{{- define "kafka.auth.secret.name" -}}
{{- default "steadybit-extension-kafka" .Values.kafka.auth.existingSecret -}}
{{- end -}}

{{/*
Secret name for a specific cluster in multi-cluster configuration
Usage: {{ include "kafka.cluster.secret.name" (dict "cluster" $cluster "index" $index "Release" $.Release) }}
*/}}
{{- define "kafka.cluster.secret.name" -}}
{{- if .cluster.auth }}
{{- if .cluster.auth.existingSecret -}}
{{- .cluster.auth.existingSecret -}}
{{- else -}}
{{- printf "steadybit-extension-kafka-cluster-%d" .index -}}
{{- end -}}
{{- else -}}
{{- printf "steadybit-extension-kafka-cluster-%d" .index -}}
{{- end -}}
{{- end -}}

{{/*
Check if multi-cluster mode is enabled
*/}}
{{- define "kafka.isMultiCluster" -}}
{{- if and .Values.kafka.clusters (gt (len .Values.kafka.clusters) 0) -}}
true
{{- else -}}
false
{{- end -}}
{{- end -}}
