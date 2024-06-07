{{ commentPrefix }}
{{- if eq 0.0 .Delta }}
{{- template "unchanged" . -}}
{{ else }}
{{- template "changes" . -}}
{{ end }}

{{- if .Errors }}
<details>
  <summary><strong>:exclamation: Errors</strong>: the following errors happened while calculating the cost:</summary>
  {{ range .Errors -}}
  - {{ . }}<br/>
  {{- end }}
</details>
{{ end }}

{{- if .Warnings }}
<details>
  <summary><strong>:warning: Warnings</strong>: the following resources had warnings, while calculating cost:</summary>
  {{ range .Warnings -}}
  - {{ . }}<br/>
  {{- end }}
</details>
{{ end }}


<sub>See the [FAQ](https://github.com/grafana/deployment_tools/blob/master/docker/k8s-cost-estimator/FAQ.md) for any questions!
<sub>Still need help? Then join us in the [`#platform-capacity-chat`](https://raintank-corp.slack.com/archives/C03PDLFK29K) channel.</sub>

<sub></sub>
{{- /* TEMPLATES */ -}}
{{ define "unchanged" }}
## :dollar: Cost Estimation Report
No changes in monthly cost for the affected resources. Here are the current estimated costs.

{{ if gt (len .Summary) 1 }}
<details>
  <summary>Details by cluster and resource type</summary>

{{ template "unchanged_details" .Reports }}
</details>
{{ else }}
{{ template "unchanged_details" .Reports }}
{{ end }}
{{ end }}

{{ define "changes" }}
{{- $increased := gt .Delta 0.0 }}
## :dollar: Cost Estimation Report {{ if $increased }}:chart_with_upwards_trend:{{ else }}:chart_with_downwards_trend:{{ end }}
Monthly cost for the affected resources will {{ if $increased }}increase by {{ dollars .Delta }} ({{ ratio .Delta .OldTotal | percentage }}){{ else }}decrease by {{ dollars (multiply .Delta -1) }} ({{ multiply (ratio .Delta .OldTotal) -1 | percentage }}){{ end }}

{{ if gt (len .Summary) 1 }}

| Cluster | Previous | New | Delta |
| - | - | - | - |
{{ range $cluster, $report := .Summary -}}
{{- if eq $report.Delta 0.0 -}}{{ continue }}{{- end -}}
| `{{ $cluster }}` | {{ dollars $report.Old }} | {{ dollars $report.New }} | {{ if eq $report.Delta 0.0 }}N/A{{ else }}{{ dollars $report.Delta }} ({{ ratio .Delta $report.Old | percentage }}){{ end }} |
{{ end }}

<details>
  <summary>Details by cluster and resource type</summary>

  {{ template "change_details" .Reports }}
</details>
{{ else }}
{{ template "change_details" .Reports }}
{{ end }}
{{ end }}

{{ define "unchanged_details" }}
{{ range $cluster, $resources := . }}
<details>
  <summary> Details for <code class="notranslate">{{ $cluster}}</code></summary>

| Namespace | Resource | CPU | Memory | Storage | Total | 
| - | - | - | - | - | - | 
{{ range $resources -}}| `{{ .New.Namespace }}` | `{{ .New.Kind }}`<br/>`{{ .New.Name }}` | {{ dollars .New.CPU }} | {{ dollars .New.Memory }} | {{ dollars .New.Storage }} | {{ dollars .New.Total }} |
{{ end }} 
</details>
{{ end }}
{{ end }}

{{ define "change_details" }}
{{ range $cluster, $resources := . -}}
<details>
  <summary> Details for <code class="notranslate">{{ $cluster}}</code></summary>

| Namespace | Resource | CPU | Memory | Storage | Total | Delta |
| - | - | - | - | - | - | - |
{{ range $resources -}}
| `{{ .New.Namespace}}` | `{{ .New.Kind }}`<br/>`{{.New.Name}}` | {{ dollars .Old.CPU }}→<br/>{{ dollars .New.CPU }} | {{ dollars .Old.Memory }}→<br/>{{ dollars .New.Memory }} | {{ dollars .Old.Storage }}→<br/>{{ dollars .New.Storage }} | {{ dollars .Old.Total }}→<br/>{{ dollars .New.Total }} | {{ if eq 0.0 .Delta }}N/A{{ else }}{{ dollars .Delta }}<br/>({{ ratio .Delta .Old.Total | percentage }}) {{ end }}|
{{ end }}
</details>
{{ end }}

<p><em>Legend: previous cost on top, expected cost below.</em></p>
{{ end }}
