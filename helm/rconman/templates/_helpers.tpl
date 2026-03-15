{{/*
Expand the name of the chart.
*/}}
{{- define "rconman.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Return the fully qualified app name
*/}}
{{- define "rconman.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}-{{ .Release.Name }}
{{- end }}
{{- end }}

{{/*
Return the chart
*/}}
{{- define "rconman.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Return selector labels
*/}}
{{- define "rconman.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rconman.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Return the labels
*/}}
{{- define "rconman.labels" -}}
helm.sh/chart: {{ include "rconman.chart" . }}
{{ include "rconman.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
