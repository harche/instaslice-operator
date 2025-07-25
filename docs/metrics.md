# InstaSlice Operator Metrics

The InstaSlice operator exports comprehensive Prometheus metrics to monitor the health, performance, and resource utilization of the GPU slicing and scheduling system.

## Metrics Endpoint

Metrics are exposed on the `/metrics` endpoint on port 8080 by default. The port can be configured using the `METRICS_PORT` environment variable.

```
http://localhost:8080/metrics
```

A health check endpoint is also available at `/health`.

## Metric Categories

### 1. GPU & MIG Slice Inventory

Track the physical GPU resources and their MIG slice capacity and utilization.

#### `das_instaslice_gpu_total`
- **Type**: Gauge
- **Labels**: `node`, `gpu_uuid`
- **Description**: Count of physical GPUs per node
- **Example**: `das_instaslice_gpu_total{node="worker-1",gpu_uuid="GPU-12345678-1234-1234-1234-123456789abc"} 1`

#### `das_instaslice_gpu_slice_capacity`
- **Type**: Gauge
- **Labels**: `node`, `gpu_uuid`, `profile`
- **Description**: Maximum number of MIG slices a GPU supports for each profile
- **Example**: `das_instaslice_gpu_slice_capacity{node="worker-1",gpu_uuid="GPU-12345678-1234-1234-1234-123456789abc",profile="1g.5gb"} 7`

#### `das_instaslice_gpu_slice_allocated`
- **Type**: Gauge
- **Labels**: `node`, `gpu_uuid`, `profile`
- **Description**: Current number of slices in use
- **Example**: `das_instaslice_gpu_slice_allocated{node="worker-1",gpu_uuid="GPU-12345678-1234-1234-1234-123456789abc",profile="1g.5gb"} 3`

#### `das_instaslice_gpu_slice_free`
- **Type**: Gauge
- **Labels**: `node`, `gpu_uuid`, `profile`
- **Description**: Remaining capacity per profile
- **Example**: `das_instaslice_gpu_slice_free{node="worker-1",gpu_uuid="GPU-12345678-1234-1234-1234-123456789abc",profile="1g.5gb"} 4`

### 2. AllocationClaim Lifecycle

Monitor the lifecycle of GPU slice allocation claims.

#### `das_instaslice_allocationclaim_state_total`
- **Type**: Gauge
- **Labels**: `state`, `namespace`
- **Description**: Number of claims by state (staged, created, inUse, etc.)
- **Example**: `das_instaslice_allocationclaim_state_total{state="inUse",namespace="default"} 5`

#### `das_instaslice_allocationclaim_transitions_total`
- **Type**: Counter
- **Labels**: `from_state`, `to_state`, `namespace`
- **Description**: Number of state transitions
- **Example**: `das_instaslice_allocationclaim_transitions_total{from_state="staged",to_state="created",namespace="default"} 12`

#### `das_instaslice_allocationclaim_duration_seconds`
- **Type**: Histogram
- **Labels**: `final_state`, `namespace`
- **Description**: Time from claim creation until final state
- **Example**: `das_instaslice_allocationclaim_duration_seconds_bucket{final_state="inUse",namespace="default",le="1"} 8`

### 3. Scheduler Plugin Activity

Track the performance and behavior of the MIG scheduler plugin.

#### `das_instaslice_scheduler_filter_attempts_total`
- **Type**: Counter
- **Labels**: `result`, `node`
- **Description**: Success/failure counts for the Filter phase
- **Example**: `das_instaslice_scheduler_filter_attempts_total{result="success",node="worker-1"} 45`

#### `das_instaslice_scheduler_score_invocations_total`
- **Type**: Counter
- **Labels**: `node`
- **Description**: How often nodes are scored
- **Example**: `das_instaslice_scheduler_score_invocations_total{node="worker-1"} 23`

#### `das_instaslice_scheduler_prebind_latency_seconds`
- **Type**: Histogram
- **Labels**: `node`
- **Description**: Time to promote staged claims during PreBind
- **Example**: `das_instaslice_scheduler_prebind_latency_seconds_bucket{node="worker-1",le="0.1"} 15`

### 4. Device Plugin & Slice Provisioning

Monitor the creation and deletion of MIG slices.

#### `das_instaslice_slice_provision_total`
- **Type**: Counter
- **Labels**: `result`, `profile`, `node`
- **Description**: Slices created or failed
- **Example**: `das_instaslice_slice_provision_total{result="success",profile="1g.5gb",node="worker-1"} 8`

#### `das_instaslice_slice_provision_latency_seconds`
- **Type**: Histogram
- **Labels**: `profile`, `node`
- **Description**: Time for MIG slice creation
- **Example**: `das_instaslice_slice_provision_latency_seconds_bucket{profile="1g.5gb",node="worker-1",le="5"} 6`

#### `das_instaslice_slice_deletion_total`
- **Type**: Counter
- **Labels**: `result`, `profile`, `node`
- **Description**: Slices deleted or failed to delete
- **Example**: `das_instaslice_slice_deletion_total{result="success",profile="1g.5gb",node="worker-1"} 3`

#### `das_instaslice_slice_deletion_latency_seconds`
- **Type**: Histogram
- **Labels**: `profile`, `node`
- **Description**: Time for MIG slice deletion
- **Example**: `das_instaslice_slice_deletion_latency_seconds_bucket{profile="1g.5gb",node="worker-1",le="2"} 2`

### 5. Operator Controller Health

Monitor the health and performance of operator controllers.

#### `das_instaslice_reconcile_total`
- **Type**: Counter
- **Labels**: `controller`, `result`
- **Description**: Reconciliation outcomes for controllers
- **Example**: `das_instaslice_reconcile_total{controller="TargetConfigReconciler",result="success"} 156`

#### `das_instaslice_reconcile_duration_seconds`
- **Type**: Histogram
- **Labels**: `controller`
- **Description**: Time spent in each reconcile loop
- **Example**: `das_instaslice_reconcile_duration_seconds_bucket{controller="TargetConfigReconciler",le="1"} 142`

#### `das_instaslice_errors_total`
- **Type**: Counter
- **Labels**: `component`, `error_type`
- **Description**: Errors from webhook, scheduler, or device plugin components
- **Example**: `das_instaslice_errors_total{component="target_config_reconciler",error_type="cert_manager_not_installed"} 1`

### 6. Build and Configuration Info

Information about the operator build and configuration.

#### `das_instaslice_info`
- **Type**: Gauge
- **Labels**: `version`, `git_commit`, `build_date`
- **Description**: Operator version, git commit, build date
- **Example**: `das_instaslice_info{version="v1.0.0",git_commit="abc123",build_date="2024-01-15T10:30:00Z"} 1`

#### `das_instaslice_emulated_mode`
- **Type**: Gauge
- **Labels**: `mode`
- **Description**: Indicates whether hardware emulation is active
- **Example**: `das_instaslice_emulated_mode{mode="disabled"} 1`

## Usage Examples

### Prometheus Configuration

Add the following to your Prometheus configuration:

```yaml
scrape_configs:
  - job_name: 'instaslice-operator'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```

### Grafana Dashboards

Create dashboards to visualize:

1. **GPU Utilization**: Track GPU slice capacity vs allocated
2. **Scheduler Performance**: Monitor filter success rates and latency
3. **AllocationClaim Lifecycle**: Track state transitions and duration
4. **Error Rates**: Monitor component errors and failures
5. **Slice Provisioning**: Track creation/deletion success rates and latency

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: instaslice-alerts
    rules:
      - alert: HighErrorRate
        expr: rate(das_instaslice_errors_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in InstaSlice operator"

      - alert: SchedulerFilterFailure
        expr: rate(das_instaslice_scheduler_filter_attempts_total{result="error"}[5m]) > 0.05
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High scheduler filter failure rate"

      - alert: SliceProvisionFailure
        expr: rate(das_instaslice_slice_provision_total{result="error"}[5m]) > 0.1
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High MIG slice provision failure rate"
```

## Integration with Components

The metrics are automatically integrated into:

- **Operator Controllers**: Track reconciliation performance and errors
- **MIG Scheduler Plugin**: Monitor filter, score, and prebind operations
- **Device Plugin**: Track slice provisioning and deletion
- **AllocationClaim Management**: Monitor lifecycle events

## Environment Variables

- `METRICS_PORT`: Port for metrics server (default: 8080)
- `EMULATED_MODE`: Set to "enabled" or "disabled" to control emulation mode

## Troubleshooting

1. **Metrics not available**: Check if the metrics server is running on the correct port
2. **High error rates**: Review component logs and check for configuration issues
3. **Performance issues**: Monitor latency histograms and identify bottlenecks
4. **Resource utilization**: Use GPU inventory metrics to track capacity and usage