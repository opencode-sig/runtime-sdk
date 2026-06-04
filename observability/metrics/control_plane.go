package metrics

import "github.com/prometheus/client_golang/prometheus"

// ControlPlaneMetrics records registry, Gateway metadata, and other
// management-plane status without affecting service liveness probes.
type ControlPlaneMetrics struct {
	service    string
	status     *prometheus.GaugeVec
	errors     *prometheus.CounterVec
	recoveries *prometheus.CounterVec
}

func NewControlPlaneMetrics(service string) *ControlPlaneMetrics {
	return &ControlPlaneMetrics{
		service: service,
		status: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "runtime_control_plane_status",
				Help: "Control-plane component status for the service. 1 means healthy, 0 means degraded.",
			},
			[]string{"service", "component"},
		),
		errors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "runtime_control_plane_errors_total",
				Help: "Total control-plane component errors.",
			},
			[]string{"service", "component", "operation"},
		),
		recoveries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "runtime_control_plane_recoveries_total",
				Help: "Total control-plane component recoveries after a degraded state.",
			},
			[]string{"service", "component", "operation"},
		),
	}
}

func (m *ControlPlaneMetrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{m.status, m.errors, m.recoveries}
}

func (m *ControlPlaneMetrics) SetStatus(component string, healthy bool) {
	if m == nil {
		return
	}
	value := 0.0
	if healthy {
		value = 1
	}
	m.status.WithLabelValues(m.service, component).Set(value)
}

func (m *ControlPlaneMetrics) RecordError(component string, operation string) {
	if m == nil {
		return
	}
	m.errors.WithLabelValues(m.service, component, operation).Inc()
}

func (m *ControlPlaneMetrics) RecordRecovery(component string, operation string) {
	if m == nil {
		return
	}
	m.recoveries.WithLabelValues(m.service, component, operation).Inc()
}
