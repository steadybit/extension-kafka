{{/* vim: set filetype=mustache: */}}

{{- define "kafka.auth.secret.name" -}}
{{- default "steadybit-extension-kakfa" .Values.kafka.auth.existingSecret -}}
{{- end -}}
