/*
** Copyright (C) 2005 Toon Toetenel
**
** Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
** documentation files (the "Software"), to deal in the Software without restriction, including without limitation the
** rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to
** permit persons to whom the Software is furnished to do so, subject to the following conditions:
**
** The above copyright notice and this permission notice shall be included in all copies or substantial portions
** of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE
** WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
** COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
** TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
**/

package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"golang.zabbix.com/sdk/errs"
	"golang.zabbix.com/sdk/metric"
	"golang.zabbix.com/sdk/plugin"
	"golang.zabbix.com/sdk/plugin/container"
)

const (
	// Name of the plugin.
	Name = "DockerSwarm"

	serviceDiscoveryMetric = swarmMetricKey("swarm.services.discovery")
	serviceReplicasDesired = swarmMetricKey("swarm.service.replicas_desired")
	serviceReplicasRunning = swarmMetricKey("swarm.service.replicas_running")
	serviceRestartCount    = swarmMetricKey("swarm.service.restarts")
	serviceTaskCount       = swarmMetricKey("swarm.service.tasks")
	serviceLastRestart     = swarmMetricKey("swarm.service.last_restart")
	stackDiscoveryMetric   = swarmMetricKey("swarm.stacks.discovery")
	stackHealthMetric      = swarmMetricKey("swarm.stack.health")
	replicaDiscoveryMetric = swarmMetricKey("swarm.replicas.discovery")
	replicaStatsMetric     = swarmMetricKey("swarm.replica.stats")
	nodeDiscoveryMetric    = swarmMetricKey("swarm.nodes.discovery")
	nodeStatusMetric       = swarmMetricKey("swarm.node.status")
)

var (
	_ plugin.Exporter = (*swarmPlugin)(nil)
	_ plugin.Runner   = (*swarmPlugin)(nil)
)

type swarmMetricKey string

type swarmMetric struct {
	metric  *metric.Metric
	handler func(ctx context.Context, params []string) (any, error)
}

type swarmPlugin struct {
	plugin.Base
	client  *client
	metrics map[swarmMetricKey]*swarmMetric
}

// Launch launches the DockerSwarm plugin. Blocks until plugin execution has finished.
func Launch() error {
	p := &swarmPlugin{
		client: newClient("/var/run/docker.sock", 30),
	}

	err := p.registerMetrics()
	if err != nil {
		return err
	}

	h, err := container.NewHandler(Name)
	if err != nil {
		return errs.Wrap(err, "failed to create new handler")
	}

	p.Logger = h

	err = h.Execute()
	if err != nil {
		return errs.Wrap(err, "failed to execute plugin handler")
	}

	return nil
}

// Start starts the Docker Swarm plugin. Required for plugin to match runner interface.
func (p *swarmPlugin) Start() {
	p.Infof("DockerSwarm plugin started")
}

// Stop stops the Docker Swarm plugin. Required for plugin to match runner interface.
func (p *swarmPlugin) Stop() {
	p.Infof("DockerSwarm plugin stopped")
}

// Export collects all the metrics.
func (p *swarmPlugin) Export(key string, rawParams []string, _ plugin.ContextProvider) (any, error) {
	m, ok := p.metrics[swarmMetricKey(key)]
	if !ok {
		return nil, errs.New("unknown metric " + key)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	res, err := m.handler(ctx, rawParams)
	if err != nil {
		return nil, errs.Wrap(err, "failed to execute handler")
	}

	return res, nil
}

func (p *swarmPlugin) registerMetrics() error {
	p.metrics = map[swarmMetricKey]*swarmMetric{
		serviceDiscoveryMetric: {
			metric: metric.New(
				"Discover Docker Swarm services with stack information.",
				nil,
				false,
			),
			handler: p.discoverServices,
		},
		serviceReplicasDesired: {
			metric: metric.New(
				"Returns the desired number of replicas for a service.",
				nil,
				false,
			),
			handler: p.getDesiredReplicas,
		},
		serviceReplicasRunning: {
			metric: metric.New(
				"Returns the number of running tasks for a service.",
				nil,
				false,
			),
			handler: p.getRunningTasks,
		},
		serviceRestartCount: {
			metric: metric.New(
				"Returns the number of task restarts for a service.",
				nil,
				false,
			),
			handler: p.getServiceRestarts,
		},
		serviceTaskCount: {
			metric: metric.New(
				"Returns the total number of tasks for a service (for debugging).",
				nil,
				false,
			),
			handler: p.getServiceTaskCount,
		},
		serviceLastRestart: {
			metric: metric.New(
				"Returns the timestamp of the most recent running task (for restart detection).",
				nil,
				false,
			),
			handler: p.getServiceLastRestart,
		},
		stackDiscoveryMetric: {
			metric: metric.New(
				"Discover Docker Compose stacks.",
				nil,
				false,
			),
			handler: p.discoverStacks,
		},
		stackHealthMetric: {
			metric: metric.New(
				"Returns health status for a Docker Compose stack.",
				nil,
				false,
			),
			handler: p.getStackHealth,
		},
		replicaDiscoveryMetric: {
			metric: metric.New(
				"Discover service replicas running on this node.",
				nil,
				false,
			),
			handler: p.discoverReplicas,
		},
		replicaStatsMetric: {
			metric: metric.New(
				"Returns CPU and memory stats for a replica running on this node.",
				nil,
				false,
			),
			handler: p.getReplicaStats,
		},
		nodeDiscoveryMetric: {
			metric: metric.New(
				"Discover all nodes in the swarm cluster.",
				nil,
				false,
			),
			handler: p.discoverNodes,
		},
		nodeStatusMetric: {
			metric: metric.New(
				"Returns availability, state and role for a swarm node.",
				nil,
				false,
			),
			handler: p.getNodeStatus,
		},
	}

	metricSet := metric.MetricSet{}

	for k, m := range p.metrics {
		metricSet[string(k)] = m.metric
	}

	err := plugin.RegisterMetrics(p, Name, metricSet.List()...)
	if err != nil {
		return errs.Wrap(err, "failed to register metrics")
	}

	return nil
}

func (p *swarmPlugin) getServices() ([]Service, error) {
	body, err := p.client.Query("services", nil)
	if err != nil {
		return nil, err
	}

	var services []Service
	if err = json.Unmarshal(body, &services); err != nil {
		return nil, errs.Wrap(err, "cannot unmarshal JSON")
	}

	return services, nil
}

func (p *swarmPlugin) discoverServices(_ context.Context, params []string) (any, error) {
	if len(params) != 0 {
		return nil, errs.New("expected no parameters for service discovery")
	}

	services, err := p.getServices()
	if err != nil {
		return nil, err
	}

	type LLDService struct {
		ID        string `json:"{#SERVICE.ID}"`
		Name      string `json:"{#SERVICE.NAME}"`
		StackName string `json:"{#STACK.NAME}"`
		// Add service name as primary identifier for stable monitoring
		ServiceKey string `json:"{#SERVICE.KEY}"` // This will be the stable identifier
	}

	lldServices := make([]LLDService, 0, len(services))
	for _, s := range services {
		stackName := "standalone"
		if s.Spec.Labels != nil {
			if namespace, exists := s.Spec.Labels["com.docker.stack.namespace"]; exists {
				stackName = namespace
			}
		}

		// Create stable service key: stackname_servicename or just servicename for standalone
		serviceKey := s.Spec.Name
		if stackName != "standalone" {
			serviceKey = stackName + "_" + s.Spec.Name
		}

		lldServices = append(lldServices, LLDService{
			ID:         s.ID,
			Name:       s.Spec.Name,
			StackName:  stackName,
			ServiceKey: serviceKey,
		})
	}

	jsonData, err := json.Marshal(lldServices)
	if err != nil {
		return nil, errs.Wrap(err, "cannot marshal JSON")
	}

	return string(jsonData), nil
}

func (p *swarmPlugin) discoverStacks(_ context.Context, params []string) (any, error) {
	if len(params) != 0 {
		return nil, errs.New("expected no parameters for stack discovery")
	}

	services, err := p.getServices()
	if err != nil {
		return nil, err
	}

	stacksMap := make(map[string]bool)
	for _, s := range services {
		stackName := "standalone"
		if s.Spec.Labels != nil {
			if namespace, exists := s.Spec.Labels["com.docker.stack.namespace"]; exists {
				stackName = namespace
			}
		}
		stacksMap[stackName] = true
	}

	type LLDStack struct {
		StackName string `json:"{#STACK.NAME}"`
	}

	lldStacks := make([]LLDStack, 0, len(stacksMap))
	for stackName := range stacksMap {
		lldStacks = append(lldStacks, LLDStack{StackName: stackName})
	}

	jsonData, err := json.Marshal(lldStacks)
	if err != nil {
		return nil, errs.Wrap(err, "cannot marshal JSON")
	}

	return string(jsonData), nil
}

func (p *swarmPlugin) getStackHealth(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for stack health")
	}

	stackName := params[0]
	services, err := p.getServices()
	if err != nil {
		return nil, err
	}

	// Filter services for this stack
	var stackServices []Service
	for _, s := range services {
		serviceStackName := "standalone"
		if s.Spec.Labels != nil {
			if namespace, exists := s.Spec.Labels["com.docker.stack.namespace"]; exists {
				serviceStackName = namespace
			}
		}
		if serviceStackName == stackName {
			stackServices = append(stackServices, s)
		}
	}

	if len(stackServices) == 0 {
		return nil, errs.New("stack not found: " + stackName)
	}

	totalServices := len(stackServices)
	healthyServices := 0
	evaluatedServices := 0

	for _, service := range stackServices {
		desired, dErr := p.getServiceDesiredReplicas(service)
		if dErr != nil {
			continue
		}

		running, rErr := p.getServiceRunningTasks(service.ID)
		if rErr != nil {
			continue
		}

		evaluatedServices++

		if running >= desired {
			healthyServices++
		}
	}

	if evaluatedServices == 0 {
		return nil, errs.New("could not evaluate any services for stack: " + stackName)
	}

	unhealthyServices := evaluatedServices - healthyServices
	healthPercentage := float64(healthyServices) / float64(evaluatedServices) * 100

	result := map[string]interface{}{
		"total_services":       totalServices,
		"evaluated_services":   evaluatedServices,
		"healthy_services":     healthyServices,
		"unhealthy_services":   unhealthyServices,
		"unevaluated_services": totalServices - evaluatedServices,
		"health_percentage":    healthPercentage,
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, errs.Wrap(err, "cannot marshal JSON")
	}

	return string(jsonData), nil
}

func (p *swarmPlugin) getDesiredReplicas(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for desired replicas")
	}

	serviceIdentifier := params[0]

	// Find the service by identifier (ID, name, or service key)
	service, err := p.findServiceByIdentifier(serviceIdentifier)
	if err != nil {
		return 0, err
	}

	return p.getServiceDesiredReplicas(*service)
}

func (p *swarmPlugin) getServiceDesiredReplicas(service Service) (int, error) {
	if service.Spec.Mode.Replicated != nil && service.Spec.Mode.Replicated.Replicas != nil {
		// #nosec G115 — replica counts are small integers
		configured := int(*service.Spec.Mode.Replicated.Replicas)
		return p.getReplicatedServiceDesiredReplicas(service, configured)
	}

	if service.Spec.Mode.Global != nil {
		return p.getGlobalServiceEligibleNodeCount(service)
	}

	return 0, errs.New("could not determine desired replicas for service " + service.ID)
}

// getReplicatedServiceDesiredReplicas caps the configured replica count when
// MaxReplicas is set and the placement constraints limit eligible nodes.
func (p *swarmPlugin) getReplicatedServiceDesiredReplicas(service Service, configured int) (int, error) {
	placement := service.Spec.TaskTemplate.Placement
	if placement == nil || placement.MaxReplicas == 0 {
		return configured, nil
	}

	nodes, err := p.getNodes()
	if err != nil {
		return 0, err
	}

	eligible, err := countEligibleNodes(nodes, placement.Constraints)
	if err != nil {
		return 0, err
	}

	return effectiveReplicatedDesired(configured, eligible, placement.MaxReplicas), nil
}

// getGlobalServiceEligibleNodeCount returns the number of active+ready nodes
// that satisfy the service's placement constraints.
func (p *swarmPlugin) getGlobalServiceEligibleNodeCount(service Service) (int, error) {
	nodes, err := p.getNodes()
	if err != nil {
		return 0, err
	}

	constraints := []string{}
	if service.Spec.TaskTemplate.Placement != nil {
		constraints = service.Spec.TaskTemplate.Placement.Constraints
	}

	return countEligibleNodes(nodes, constraints)
}

func (p *swarmPlugin) getNodes() ([]Node, error) {
	body, err := p.client.Query("nodes", nil)
	if err != nil {
		return nil, err
	}

	var nodes []Node
	if err = json.Unmarshal(body, &nodes); err != nil {
		return nil, errs.Wrap(err, "cannot unmarshal JSON")
	}

	return nodes, nil
}

func (p *swarmPlugin) getRunningTasks(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for running tasks")
	}

	serviceIdentifier := params[0]

	// Find the service by identifier (ID, name, or service key)
	service, err := p.findServiceByIdentifier(serviceIdentifier)
	if err != nil {
		return 0, err
	}

	return p.getServiceRunningTasks(service.ID)
}

func (p *swarmPlugin) getServiceRunningTasks(serviceID string) (int, error) {
	filters := map[string][]string{
		"service":       {serviceID},
		"desired-state": {"running"},
	}

	body, err := p.client.Query("tasks", filters)
	if err != nil {
		return 0, err
	}

	var tasks []Task
	if err = json.Unmarshal(body, &tasks); err != nil {
		return 0, errs.Wrap(err, "cannot unmarshal JSON")
	}

	count := 0
	for _, task := range tasks {
		if task.Status.State == "running" {
			count++
		}
	}

	return count, nil
}

// findServiceByIdentifier finds a service by ID, name, or service key
func (p *swarmPlugin) findServiceByIdentifier(identifier string) (*Service, error) {
	services, err := p.getServices()
	if err != nil {
		return nil, err
	}

	for _, s := range services {
		// Check if it's a service ID
		if s.ID == identifier {
			return &s, nil
		}

		// Check if it's a service name
		if s.Spec.Name == identifier {
			return &s, nil
		}

		// Check if it's a service key (stackname_servicename)
		stackName := "standalone"
		if s.Spec.Labels != nil {
			if namespace, exists := s.Spec.Labels["com.docker.stack.namespace"]; exists {
				stackName = namespace
			}
		}
		serviceKey := s.Spec.Name
		if stackName != "standalone" {
			serviceKey = stackName + "_" + s.Spec.Name
		}
		if serviceKey == identifier {
			return &s, nil
		}
	}

	return nil, errs.New("service not found: " + identifier)
}

func (p *swarmPlugin) getServiceRestarts(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for service restarts")
	}

	serviceIdentifier := params[0]

	// Find the service by identifier (ID, name, or service key)
	targetService, err := p.findServiceByIdentifier(serviceIdentifier)
	if err != nil {
		return 0, err
	}

	// Get all tasks for the service (not just running ones)
	filters := map[string][]string{
		"service": {targetService.ID},
	}

	body, err := p.client.Query("tasks", filters)
	if err != nil {
		return 0, err
	}

	var tasks []Task
	if err = json.Unmarshal(body, &tasks); err != nil {
		return 0, errs.Wrap(err, "cannot unmarshal JSON")
	}

	// Only count tasks that actually failed (container crashed).
	// Excludes: shutdown (scale-down/rolling-update), preparing/starting (normal lifecycle).
	restartCount := 0
	for _, task := range tasks {
		if task.Status.State == "failed" {
			restartCount++
		}
	}

	return restartCount, nil
}

func (p *swarmPlugin) getServiceTaskCount(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for service task count")
	}

	serviceIdentifier := params[0]

	// Find the service by identifier (ID, name, or service key)
	targetService, err := p.findServiceByIdentifier(serviceIdentifier)
	if err != nil {
		return 0, err
	}

	// Get all tasks for the service (not just running ones)
	filters := map[string][]string{
		"service": {targetService.ID},
	}

	body, err := p.client.Query("tasks", filters)
	if err != nil {
		return 0, err
	}

	var tasks []Task
	if err = json.Unmarshal(body, &tasks); err != nil {
		return 0, errs.Wrap(err, "cannot unmarshal JSON")
	}

	// Return total task count for debugging
	return len(tasks), nil
}

func (p *swarmPlugin) getServiceLastRestart(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for service last restart")
	}

	serviceIdentifier := params[0]

	// Find the service by identifier (ID, name, or service key)
	targetService, err := p.findServiceByIdentifier(serviceIdentifier)
	if err != nil {
		return 0, err
	}

	// Get all tasks for the service (not just running ones)
	filters := map[string][]string{
		"service": {targetService.ID},
	}

	body, err := p.client.Query("tasks", filters)
	if err != nil {
		return 0, err
	}

	var tasks []Task
	if err = json.Unmarshal(body, &tasks); err != nil {
		return 0, errs.Wrap(err, "cannot unmarshal JSON")
	}

	// Find the most recent running task and return its timestamp
	var mostRecentTimestamp int64 = 0

	for _, task := range tasks {
		if task.Status.State == "running" && task.Status.Timestamp != "" {
			// Docker uses RFC3339Nano (nanosecond precision)
			if timestamp, err := time.Parse(time.RFC3339Nano, task.Status.Timestamp); err == nil {
				if timestamp.Unix() > mostRecentTimestamp {
					mostRecentTimestamp = timestamp.Unix()
				}
			}
		}
	}

	return mostRecentTimestamp, nil
}

func (p *swarmPlugin) getLocalNodeID() (string, error) {
	body, err := p.client.Query("info", nil)
	if err != nil {
		return "", err
	}

	var info DockerInfo
	if err = json.Unmarshal(body, &info); err != nil {
		return "", errs.Wrap(err, "cannot unmarshal JSON")
	}

	if info.Swarm.NodeID == "" {
		return "", errs.New("node is not part of a swarm")
	}

	return info.Swarm.NodeID, nil
}

// replicaKey builds the stable identifier for a task.
// Replicated services: "{serviceKey}/slot/{N}"
// Global services:     "{serviceKey}/node/{nodeID[:12]}"
func replicaKey(serviceKey string, task Task) string {
	if task.Slot > 0 {
		return serviceKey + "/slot/" + strconv.Itoa(task.Slot)
	}

	nodeShort := task.NodeID
	if len(nodeShort) > 12 {
		nodeShort = nodeShort[:12]
	}

	return serviceKey + "/node/" + nodeShort
}

func (p *swarmPlugin) discoverReplicas(_ context.Context, params []string) (any, error) {
	if len(params) != 0 {
		return nil, errs.New("expected no parameters for replica discovery")
	}

	localNodeID, err := p.getLocalNodeID()
	if err != nil {
		return nil, err
	}

	services, err := p.getServices()
	if err != nil {
		return nil, err
	}

	serviceMap := make(map[string]Service, len(services))
	for _, s := range services {
		serviceMap[s.ID] = s
	}

	filters := map[string][]string{
		"node":          {localNodeID},
		"desired-state": {"running"},
	}

	body, err := p.client.Query("tasks", filters)
	if err != nil {
		return nil, err
	}

	var tasks []Task
	if err = json.Unmarshal(body, &tasks); err != nil {
		return nil, errs.Wrap(err, "cannot unmarshal JSON")
	}

	type LLDReplica struct {
		ServiceKey  string `json:"{#SERVICE.KEY}"`
		ServiceName string `json:"{#SERVICE.NAME}"`
		StackName   string `json:"{#STACK.NAME}"`
		ReplicaKey  string `json:"{#REPLICA.KEY}"`
		ReplicaSlot string `json:"{#REPLICA.SLOT}"`
	}

	lldReplicas := make([]LLDReplica, 0)

	for _, task := range tasks {
		if task.Status.State != "running" {
			continue
		}

		// Only include tasks that already have a container ID — stats require it.
		if task.Status.ContainerStatus == nil || task.Status.ContainerStatus.ContainerID == "" {
			continue
		}

		svc, ok := serviceMap[task.ServiceID]
		if !ok {
			continue
		}

		stackName := "standalone"
		if svc.Spec.Labels != nil {
			if ns, exists := svc.Spec.Labels["com.docker.stack.namespace"]; exists {
				stackName = ns
			}
		}

		svcKey := svc.Spec.Name
		if stackName != "standalone" {
			svcKey = stackName + "_" + svc.Spec.Name
		}

		slot := strconv.Itoa(task.Slot)
		if task.Slot == 0 {
			slot = task.NodeID
		}

		lldReplicas = append(lldReplicas, LLDReplica{
			ServiceKey:  svcKey,
			ServiceName: svc.Spec.Name,
			StackName:   stackName,
			ReplicaKey:  replicaKey(svcKey, task),
			ReplicaSlot: slot,
		})
	}

	jsonData, err := json.Marshal(lldReplicas)
	if err != nil {
		return nil, errs.Wrap(err, "cannot marshal JSON")
	}

	return string(jsonData), nil
}

// findTaskByReplicaKey resolves a replica key to the running task on this node.
func (p *swarmPlugin) findTaskByReplicaKey(key string) (*Task, error) {
	localNodeID, err := p.getLocalNodeID()
	if err != nil {
		return nil, err
	}

	services, err := p.getServices()
	if err != nil {
		return nil, err
	}

	// Match the service key prefix and extract the slot/node suffix.
	var targetServiceID string
	var isSlot bool
	var slot int
	var nodePrefix string

	for _, svc := range services {
		stackName := "standalone"
		if svc.Spec.Labels != nil {
			if ns, exists := svc.Spec.Labels["com.docker.stack.namespace"]; exists {
				stackName = ns
			}
		}

		svcKey := svc.Spec.Name
		if stackName != "standalone" {
			svcKey = stackName + "_" + svc.Spec.Name
		}

		if suffix, ok := strings.CutPrefix(key, svcKey+"/slot/"); ok {
			n, err := strconv.Atoi(suffix)
			if err != nil {
				return nil, errs.New("invalid slot in replica key: " + key)
			}
			targetServiceID = svc.ID
			isSlot = true
			slot = n
			break
		}

		if suffix, ok := strings.CutPrefix(key, svcKey+"/node/"); ok {
			targetServiceID = svc.ID
			isSlot = false
			nodePrefix = suffix
			break
		}
	}

	if targetServiceID == "" {
		return nil, errs.New("service not found for replica key: " + key)
	}

	filters := map[string][]string{
		"service":       {targetServiceID},
		"node":          {localNodeID},
		"desired-state": {"running"},
	}

	body, err := p.client.Query("tasks", filters)
	if err != nil {
		return nil, err
	}

	var tasks []Task
	if err = json.Unmarshal(body, &tasks); err != nil {
		return nil, errs.Wrap(err, "cannot unmarshal JSON")
	}

	for i, task := range tasks {
		if task.Status.State != "running" {
			continue
		}

		if isSlot && task.Slot == slot {
			return &tasks[i], nil
		}

		if !isSlot && strings.HasPrefix(task.NodeID, nodePrefix) {
			return &tasks[i], nil
		}
	}

	return nil, errs.New("running replica not found on this node for key: " + key)
}

func (p *swarmPlugin) getReplicaStats(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for replica stats")
	}

	task, err := p.findTaskByReplicaKey(params[0])
	if err != nil {
		return nil, err
	}

	if task.Status.ContainerStatus == nil || task.Status.ContainerStatus.ContainerID == "" {
		return nil, errs.New("container not yet running for replica: " + params[0])
	}

	body, err := p.client.QueryContainerStats(task.Status.ContainerStatus.ContainerID)
	if err != nil {
		return nil, err
	}

	var raw ContainerStats
	if err = json.Unmarshal(body, &raw); err != nil {
		return nil, errs.Wrap(err, "cannot unmarshal container stats")
	}

	stats := ReplicaStats{
		CPUPercent: calcCPUPercent(&raw),
		CPUNs:      raw.CPUStats.CPUUsage.TotalUsage,
		MemBytes:   calcMemUsage(&raw),
		MemPercent: calcMemPercent(&raw),
		MemLimit:   raw.MemoryStats.Limit,
	}

	jsonData, err := json.Marshal(stats)
	if err != nil {
		return nil, errs.Wrap(err, "cannot marshal JSON")
	}

	return string(jsonData), nil
}

func calcCPUPercent(s *ContainerStats) float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemCPUUsage) - float64(s.PreCPUStats.SystemCPUUsage)

	if sysDelta <= 0 || cpuDelta < 0 {
		return 0
	}

	numCPUs := s.CPUStats.OnlineCPUs
	if numCPUs == 0 {
		numCPUs = len(s.CPUStats.CPUUsage.PercpuUsage)
	}

	if numCPUs == 0 {
		numCPUs = 1
	}

	return (cpuDelta / sysDelta) * float64(numCPUs) * 100.0
}

func calcMemUsage(s *ContainerStats) uint64 {
	usage := s.MemoryStats.Usage

	// Subtract page cache to get actual working set memory.
	// Docker uses inactive_file on cgroups v1; cgroups v2 reports it the same way.
	if v, ok := s.MemoryStats.Stats["inactive_file"]; ok && usage > v {
		return usage - v
	}

	if v, ok := s.MemoryStats.Stats["cache"]; ok && usage > v {
		return usage - v
	}

	return usage
}

func calcMemPercent(s *ContainerStats) float64 {
	// A limit of 0 or near-maxuint64 means "no limit set".
	if s.MemoryStats.Limit == 0 || s.MemoryStats.Limit > 1e18 {
		return 0
	}

	return float64(calcMemUsage(s)) / float64(s.MemoryStats.Limit) * 100.0
}

func (p *swarmPlugin) discoverNodes(_ context.Context, params []string) (any, error) {
	if len(params) != 0 {
		return nil, errs.New("expected no parameters for node discovery")
	}

	nodes, err := p.getNodes()
	if err != nil {
		return nil, err
	}

	type LLDNode struct {
		NodeID       string `json:"{#NODE.ID}"`
		NodeHostname string `json:"{#NODE.HOSTNAME}"`
		NodeRole     string `json:"{#NODE.ROLE}"`
	}

	lldNodes := make([]LLDNode, 0, len(nodes))
	for _, n := range nodes {
		lldNodes = append(lldNodes, LLDNode{
			NodeID:       n.ID,
			NodeHostname: n.Description.Hostname,
			NodeRole:     strings.ToLower(n.Spec.Role),
		})
	}

	jsonData, err := json.Marshal(lldNodes)
	if err != nil {
		return nil, errs.Wrap(err, "cannot marshal JSON")
	}

	return string(jsonData), nil
}

func (p *swarmPlugin) getNodeStatus(_ context.Context, params []string) (any, error) {
	if len(params) != 1 {
		return nil, errs.New("expected 1 parameter for node status")
	}

	identifier := params[0]

	nodes, err := p.getNodes()
	if err != nil {
		return nil, err
	}

	var target *Node
	for i, n := range nodes {
		if n.ID == identifier || n.Description.Hostname == identifier {
			target = &nodes[i]
			break
		}
	}

	if target == nil {
		return nil, errs.New("node not found: " + identifier)
	}

	result := NodeStatusResult{
		Hostname:     target.Description.Hostname,
		Role:         strings.ToLower(target.Spec.Role),
		Availability: strings.ToLower(target.Spec.Availability),
		State:        strings.ToLower(target.Status.State),
		Addr:         target.Status.Addr,
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, errs.Wrap(err, "cannot marshal JSON")
	}

	return string(jsonData), nil
}
