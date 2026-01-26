{{/*
Expand the name of the chart.
*/}}
{{- define "gpu-telemetry-pipeline.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "gpu-telemetry-pipeline.fullname" -}}
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
{{- define "gpu-telemetry-pipeline.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "gpu-telemetry-pipeline.labels" -}}
helm.sh/chart: {{ include "gpu-telemetry-pipeline.chart" . }}
{{ include "gpu-telemetry-pipeline.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "gpu-telemetry-pipeline.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gpu-telemetry-pipeline.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "gpu-telemetry-pipeline.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "gpu-telemetry-pipeline.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
MQ Server name
*/}}
{{- define "gpu-telemetry-pipeline.mqServer.name" -}}
{{- printf "%s-mq-server" (include "gpu-telemetry-pipeline.fullname" .) }}
{{- end }}

{{/*
Streamer name
*/}}
{{- define "gpu-telemetry-pipeline.streamer.name" -}}
{{- printf "%s-streamer" (include "gpu-telemetry-pipeline.fullname" .) }}
{{- end }}

{{/*
Collector name
*/}}
{{- define "gpu-telemetry-pipeline.collector.name" -}}
{{- printf "%s-collector" (include "gpu-telemetry-pipeline.fullname" .) }}
{{- end }}

{{/*
API name
*/}}
{{- define "gpu-telemetry-pipeline.api.name" -}}
{{- printf "%s-api" (include "gpu-telemetry-pipeline.fullname" .) }}
{{- end }}

{{/*
Image pull secrets
*/}}
{{- define "gpu-telemetry-pipeline.imagePullSecrets" -}}
{{- if .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.global.imagePullSecrets }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- end }}
