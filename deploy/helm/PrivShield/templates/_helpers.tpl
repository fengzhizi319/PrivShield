{{/*
=============================================================================
Helm 模板辅助函数（_helpers.tpl）
=============================================================================
定义 Chart 中复用的模板函数（template），通过 include 调用。
命名规范："<chart-name>.<function-name>"，避免与其他 Chart 冲突。
注意：以下函数在所有模板文件中均可通过 include 引用。
=============================================================================
*/}}

{{/*
PrivShield.name — 展开 Chart 名称
─────────────────────────────────────────────────────────────────────────────
逻辑：优先使用 values.yaml 中的 nameOverride，否则使用 Chart.yaml 中的 .Chart.Name
后处理：转小写 → 截断到 63 字符（K8s DNS 名称限制）→ 去除尾部连字符
*/}}
{{- define "PrivShield.name" -}}
{{- default .Chart.Name .Values.nameOverride | lower | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
PrivShield.fullname — 创建完全限定的应用名称
─────────────────────────────────────────────────────────────────────────────
用途：Deployment/Service/ConfigMap 等资源的 metadata.name
逻辑：
  1. 若设置 fullnameOverride → 直接使用（截断 63 字符）
  2. 否则 → 拼接 "<release-name>-<chart-name>"
     - 若 release name 已包含 chart name → 仅用 release name（避免重复）
     - 否则 → "<release>-<chart>" 格式
后处理：转小写 → 截断 63 字符 → 去除尾部连字符
示例：helm install my-release ./PrivShield → "my-release-privshield"
*/}}
{{- define "PrivShield.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | lower | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride | lower }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
PrivShield.chart — 生成 chart 标签值
─────────────────────────────────────────────────────────────────────────────
用途：helm.sh/chart 标签的值（标识 Chart 名称+版本）
格式："<chart-name>-<version>"，版本号中的 "+" 替换为 "_"（K8s 标签不允许 "+"）
示例："PrivShield-0.1.0"
*/}}
{{- define "PrivShield.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
PrivShield.labels — 生成通用标签集
─────────────────────────────────────────────────────────────────────────────
用途：所有资源 metadata.labels 的通用部分，遵循 K8s 推荐标签规范
包含：
  - helm.sh/chart:        Chart 名称+版本（标识资源由哪个 Chart 生成）
  - app.kubernetes.io/name:       Chart 名称（selectorLabels 中定义）
  - app.kubernetes.io/instance:   Release 名称（selectorLabels 中定义）
  - app.kubernetes.io/version:    应用版本（来自 Chart.yaml appVersion）
  - app.kubernetes.io/managed-by: 固定为 "Helm"（标识管理工具）
*/}}
{{- define "PrivShield.labels" -}}
helm.sh/chart: {{ include "PrivShield.chart" . }}
{{ include "PrivShield.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
PrivShield.selectorLabels — 生成选择器标签
─────────────────────────────────────────────────────────────────────────────
用途：Deployment.spec.selector.matchLabels / Service.spec.selector
作用：将 Deployment 与 Service 关联（Service 通过相同标签找到 Pod）
包含：
  - app.kubernetes.io/name:   Chart 名称
  - app.kubernetes.io/instance: Release 名称
注意：同一 Release 的所有资源共享这对标签，确保 Service 能路由到正确 Pod
*/}}
{{- define "PrivShield.selectorLabels" -}}
app.kubernetes.io/name: {{ include "PrivShield.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
PrivShield.serviceAccountName — 生成 ServiceAccount 名称
─────────────────────────────────────────────────────────────────────────────
逻辑：
  - serviceAccount.create=true  → 使用 fullname（自动创建）或自定义 name
  - serviceAccount.create=false → 使用自定义 name 或 "default"（使用已有 SA）
*/}}
{{- define "PrivShield.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "PrivShield.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
PrivShield.imageTag — 生成镜像 tag（支持 flavor 切换）
─────────────────────────────────────────────────────────────────────────────
逻辑：
  1. 若 image.tag 非空 → 直接使用（用户显式指定）
  2. 否则若 flavor=ml → 自动附加 "-ml" 后缀（如 "0.1.0-ml"）
  3. 否则 → 使用 Chart.yaml 中的 appVersion（如 "0.1.0"）
示例：
  - flavor=core, tag=""  → "0.1.0"
  - flavor=ml,   tag=""  → "0.1.0-ml"
  - flavor=*,    tag="v1" → "v1"（用户覆盖优先）
*/}}
{{- define "PrivShield.imageTag" -}}
{{- if .Values.image.tag }}
{{- .Values.image.tag }}
{{- else if eq .Values.flavor "ml" }}
{{- printf "%s-ml" .Chart.AppVersion }}
{{- else }}
{{- .Chart.AppVersion }}
{{- end }}
{{- end }}
