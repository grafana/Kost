{{ .Prefix }}
## :dollar: Cost Report :moneybag:
{{ .Summary }}

<details>
<pre>
{{ .Details }}
</pre>
</details>

{{ if .Warnings -}}
<details>
<summary><strong>Warnings</strong>: the following errors happened while calculating the cost:</summary>
{{ range .Warnings }}
- {{ . -}}
{{ end }}
</details>
{{ end }}
