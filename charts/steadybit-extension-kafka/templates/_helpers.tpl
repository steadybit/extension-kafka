{{/* vim: set filetype=mustache: */}}

{{- define "kafka.auth.secret.name" -}}
{{- default "steadybit-extension-kafka" .Values.kafka.auth.existingSecret -}}
{{- end -}}
