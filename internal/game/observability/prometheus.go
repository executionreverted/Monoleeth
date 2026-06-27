package observability

import (
	"fmt"
	"strings"
	"time"
)

// PrometheusContentType is the Prometheus 0.0.4 text exposition content type.
const PrometheusContentType = "text/plain; version=0.0.4; charset=utf-8"

// PrometheusText renders a metric snapshot in Prometheus text exposition format.
func PrometheusText(snapshot MetricSnapshot) string {
	var b strings.Builder
	seen := make(map[string]struct{})
	for _, counter := range snapshot.Counters {
		name := prometheusMetricName(counter.Name)
		writePrometheusHeader(&b, seen, name, "counter")
		writePrometheusSample(&b, name, counter.Labels, float64(counter.Value))
	}
	for _, gauge := range snapshot.Gauges {
		name := prometheusMetricName(gauge.Name)
		writePrometheusHeader(&b, seen, name, "gauge")
		writePrometheusSample(&b, name, gauge.Labels, float64(gauge.Value))
	}
	for _, duration := range snapshot.Durations {
		name := prometheusMetricName(duration.Name)
		writePrometheusHeader(&b, seen, name, "summary")
		writePrometheusSample(&b, name, appendPrometheusLabel(duration.Labels, Label{Name: "quantile", Value: "0.5"}), prometheusDurationValue(duration.Name, duration.P50))
		writePrometheusSample(&b, name, appendPrometheusLabel(duration.Labels, Label{Name: "quantile", Value: "0.95"}), prometheusDurationValue(duration.Name, duration.P95))
		writePrometheusSample(&b, name, appendPrometheusLabel(duration.Labels, Label{Name: "quantile", Value: "0.99"}), prometheusDurationValue(duration.Name, duration.P99))
		writePrometheusSample(&b, name+"_sum", duration.Labels, prometheusDurationValue(duration.Name, duration.Total))
		writePrometheusSample(&b, name+"_count", duration.Labels, float64(duration.Count))
	}
	return b.String()
}

func writePrometheusHeader(b *strings.Builder, seen map[string]struct{}, name, metricType string) {
	if _, ok := seen[name]; ok {
		return
	}
	seen[name] = struct{}{}
	fmt.Fprintf(b, "# HELP %s %s metric from game runtime.\n", name, name)
	fmt.Fprintf(b, "# TYPE %s %s\n", name, metricType)
}

func writePrometheusSample(b *strings.Builder, name string, labels []Label, value float64) {
	b.WriteString(name)
	if len(labels) > 0 {
		b.WriteByte('{')
		for i, label := range labels {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, `%s="%s"`, prometheusLabelName(label.Name), escapePrometheusLabelValue(label.Value))
		}
		b.WriteByte('}')
	}
	fmt.Fprintf(b, " %g\n", value)
}

func appendPrometheusLabel(labels []Label, label Label) []Label {
	appended := make([]Label, 0, len(labels)+1)
	appended = append(appended, labels...)
	appended = append(appended, label)
	return appended
}

func prometheusDurationValue(name string, duration time.Duration) float64 {
	if strings.HasSuffix(name, "_ms") {
		return float64(duration) / float64(time.Millisecond)
	}
	return duration.Seconds()
}

func prometheusMetricName(name string) string {
	return prometheusIdentifier(name, true)
}

func prometheusLabelName(name string) string {
	return prometheusIdentifier(name, false)
}

func prometheusIdentifier(name string, allowColon bool) string {
	if name == "" {
		return "_"
	}
	var b strings.Builder
	for index, char := range name {
		if prometheusIdentifierCharOK(char, allowColon) && (index > 0 || prometheusIdentifierFirstCharOK(char, allowColon)) {
			b.WriteRune(char)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == "" || !prometheusIdentifierFirstCharOK(rune(out[0]), allowColon) {
		return "_" + out
	}
	return out
}

func prometheusIdentifierFirstCharOK(char rune, allowColon bool) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		char == '_' ||
		(allowColon && char == ':')
}

func prometheusIdentifierCharOK(char rune, allowColon bool) bool {
	return prometheusIdentifierFirstCharOK(char, allowColon) || (char >= '0' && char <= '9')
}

func escapePrometheusLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}
