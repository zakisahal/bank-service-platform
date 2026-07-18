{{- define "bank-service.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "bank-service.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "bank-service.labels" -}}
app.kubernetes.io/name: {{ include "bank-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "bank-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "bank-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "bank-service.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "bank-service.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Three distinct Secrets, one per credential's trust boundary:
  admin    - master/superuser connection, used only by the migration Job
  api      - bank_service_api role, mounted into the bank-service Deployment
  consumer - bank_service_consumer role, mounted into the consumer Deployment
*/}}
{{- define "bank-service.adminSecretName" -}}
{{- printf "%s-db-admin" (include "bank-service.fullname" .) -}}
{{- end -}}

{{- define "bank-service.appSecretName" -}}
{{- printf "%s-db-api" (include "bank-service.fullname" .) -}}
{{- end -}}

{{- define "bank-service.consumerSecretName" -}}
{{- printf "%s-db-consumer" (include "bank-service.fullname" .) -}}
{{- end -}}

{{- define "bank-service.adminDatabaseUrl" -}}
{{- printf "postgres://%s:%s@%s:%v/%s?sslmode=%s" .Values.secrets.adminUser .Values.secrets.adminPassword .Values.secrets.host .Values.secrets.port .Values.secrets.database .Values.secrets.sslmode -}}
{{- end -}}

{{- define "bank-service.apiDatabaseUrl" -}}
{{- printf "postgres://%s:%s@%s:%v/%s?sslmode=%s" .Values.secrets.apiUser .Values.secrets.apiPassword .Values.secrets.host .Values.secrets.port .Values.secrets.database .Values.secrets.sslmode -}}
{{- end -}}

{{- define "bank-service.consumerDatabaseUrl" -}}
{{- printf "postgres://%s:%s@%s:%v/%s?sslmode=%s" .Values.secrets.consumerUser .Values.secrets.consumerPassword .Values.secrets.host .Values.secrets.port .Values.secrets.database .Values.secrets.sslmode -}}
{{- end -}}

{{- define "bank-service.redisAddr" -}}
{{- printf "%s-redis:6379" (include "bank-service.fullname" .) -}}
{{- end -}}
