package main

import (
	"strings"

	"golang.zabbix.com/sdk/errs"
)

type placementConstraint struct {
	key      string
	operator string // "==" or "!="
	value    string
}

// countEligibleNodes returns the number of active+ready nodes that satisfy all constraints.
func countEligibleNodes(nodes []Node, rawConstraints []string) (int, error) {
	count := 0

	for _, node := range nodes {
		if strings.ToLower(node.Spec.Availability) != "active" || strings.ToLower(node.Status.State) != "ready" {
			continue
		}

		matches, err := nodeMatchesConstraints(node, rawConstraints)
		if err != nil {
			return 0, err
		}

		if matches {
			count++
		}
	}

	return count, nil
}

func nodeMatchesConstraints(node Node, rawConstraints []string) (bool, error) {
	for _, raw := range rawConstraints {
		c, err := parsePlacementConstraint(raw)
		if err != nil {
			return false, err
		}

		match, err := evaluatePlacementConstraint(node, c)
		if err != nil {
			return false, err
		}

		if !match {
			return false, nil
		}
	}

	return true, nil
}

func parsePlacementConstraint(raw string) (placementConstraint, error) {
	for _, op := range []string{"==", "!="} {
		idx := strings.Index(raw, op)
		if idx >= 0 {
			return placementConstraint{
				key:      strings.TrimSpace(raw[:idx]),
				operator: op,
				value:    strings.TrimSpace(raw[idx+len(op):]),
			}, nil
		}
	}

	return placementConstraint{}, errs.New("unsupported constraint format: " + raw)
}

func evaluatePlacementConstraint(node Node, c placementConstraint) (bool, error) {
	actualValue, exists, err := getConstraintValue(node, c.key)
	if err != nil {
		return false, err
	}

	expected := normalizeConstraintExpectedValue(c.key, c.value)

	switch c.operator {
	case "==":
		return exists && actualValue == expected, nil
	case "!=":
		return !exists || actualValue != expected, nil
	default:
		return false, errs.New("unsupported constraint operator: " + c.operator)
	}
}

// normalizeConstraintExpectedValue lowercases the expected value for keys where
// Docker stores the actual value in lowercase.
func normalizeConstraintExpectedValue(key, value string) string {
	switch key {
	case "node.role", "node.availability",
		"node.platform.os", "node.platform.arch", "node.platform.architecture":
		return strings.ToLower(value)
	}

	return value
}

func getConstraintValue(node Node, key string) (value string, exists bool, err error) {
	switch key {
	case "node.id":
		return node.ID, true, nil
	case "node.hostname":
		return node.Description.Hostname, true, nil
	case "node.role":
		return strings.ToLower(node.Spec.Role), true, nil
	case "node.availability":
		return strings.ToLower(node.Spec.Availability), true, nil
	case "node.platform.os":
		return strings.ToLower(node.Description.Platform.OS), true, nil
	case "node.platform.arch", "node.platform.architecture":
		return strings.ToLower(node.Description.Platform.Architecture), true, nil
	}

	if strings.HasPrefix(key, "node.labels.") {
		labelKey := strings.TrimPrefix(key, "node.labels.")
		v, ok := node.Spec.Labels[labelKey]
		return v, ok, nil
	}

	if strings.HasPrefix(key, "engine.labels.") {
		labelKey := strings.TrimPrefix(key, "engine.labels.")
		v, ok := getEngineLabelValue(node.Description.Engine.Labels, labelKey)
		return v, ok, nil
	}

	return "", false, errs.New("unsupported constraint key: " + key)
}

func getEngineLabelValue(labels []string, key string) (string, bool) {
	for _, label := range labels {
		k, v, found := strings.Cut(label, "=")
		if found && k == key {
			return v, true
		}
	}

	return "", false
}

// effectiveReplicatedDesired caps configured replicas by eligibleNodes * maxReplicasPerNode.
// When maxReplicasPerNode is 0, no cap applies.
func effectiveReplicatedDesired(configuredReplicas, eligibleNodes int, maxReplicasPerNode uint64) int {
	if maxReplicasPerNode == 0 {
		return configuredReplicas
	}

	// #nosec G115 — eligible node counts are small integers, overflow not possible
	cap := eligibleNodes * int(maxReplicasPerNode)
	if configuredReplicas > cap {
		return cap
	}

	return configuredReplicas
}
