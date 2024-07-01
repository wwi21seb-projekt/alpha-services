{{- define "alpha-chart.global.env" -}}
  {{- range $key, $value := .Values.global.env }}
    {{- if (and (not (eq $.serviceName "api-gateway")) (or (eq $key "POSTGRES_HOST") (eq $key "POSTGRES_PORT") (eq $key "POSTGRES_DB") (eq $key "POSTGRES_USER") (eq $key "POSTGRES_PASSWORD"))) }}
    {{- if (typeIs "string" $value) }}
    - name: {{ $key }}
      value: {{ $value | quote }}
    {{- else if (hasKey $value "valueFrom") }}
    - name: {{ $key }}
      valueFrom: {{ toYaml $value.valueFrom | nindent 8 }}
    {{- end }}
    {{- else if not (and (eq $.serviceName "api-gateway") (or (eq $key "POSTGRES_HOST") (eq $key "POSTGRES_PORT") (eq $key "POSTGRES_DB") (eq $key "POSTGRES_USER") (eq $key "POSTGRES_PASSWORD"))) }}
    - name: {{ $key }}
      value: {{ $value | quote }}
    {{- end }}
  {{- end }}
{{- end }}

{{- define "alpha-chart.service.env" -}}
  {{- range $key, $value := (index .Values.services $.serviceName).env }}
    {{- if (typeIs "string" $value) }}
    - name: {{ $key }}
      value: {{ $value | quote }}
    {{- else if (hasKey $value "valueFrom") }}
    - name: {{ $key }}
      valueFrom: {{ toYaml $value.valueFrom | nindent 8 }}
    {{- end }}
  {{- end }}
{{- end }}
