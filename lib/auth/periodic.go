/*
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package auth

import (
	"fmt"

	"golang.org/x/mod/semver"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	vc "github.com/gravitational/teleport/lib/versioncontrol"
)

// upgradeEnrollPeriodic is a periodic operation that aggregates per-version counts of instances
// by whether or not they are enrolled in automatic upgrades and generates a prompt to enroll
// instances if the median unenrolled version is lagging behind the median enrolled version.
type upgradeEnrollPeriodic struct {
	// enrolled/unenrolled per-version counts
	enrolled, unenrolled map[string]int
}

func newUpgradeEnrollPeriodic() *upgradeEnrollPeriodic {
	return &upgradeEnrollPeriodic{
		enrolled:   make(map[string]int),
		unenrolled: make(map[string]int),
	}
}

// VisitInstance adds an instance to ongoing aggregations.
func (u *upgradeEnrollPeriodic) VisitInstance(instance types.Instance) {
	ver := vc.Normalize(instance.GetTeleportVersion())
	if !semver.IsValid(ver) {
		return
	}

	if instance.GetExternalUpgrader() == "" {
		u.unenrolled[ver]++
	} else {
		u.enrolled[ver]++
	}
}

// GenerateEnrollPrompt generates a prompt suggesting enrollment of unenrolled instances
// in automatic upgrades if the median enrolled version is higher than the median unenrolled version.
func (u *upgradeEnrollPeriodic) GenerateEnrollPrompt() (msg string, prompt bool) {
	medianEnrolled, totalEnrolled, ok := inspectVersionCounts(u.enrolled)
	if !ok || totalEnrolled == 0 {
		return "", false
	}

	medianUnenrolled, totalUnenrolled, ok := inspectVersionCounts(u.unenrolled)
	if !ok || totalUnenrolled == 0 {
		return "", false
	}

	if semver.Compare(medianEnrolled, medianUnenrolled) != 1 {
		// unenrolled agents are not lagging behind enrolled agents
		return "", false
	}

	return fmt.Sprintf("Some agents are outdated and would benefit from enrollment in automatic upgrades."+
		" (hint: use 'tctl inventory ls --upgrader=none' or 'tctl inventory ls --older-than=%s' to see more)", medianEnrolled), true
}

// inspectVersionCounts is a helper used to determine the median version and total
// instance count from a mapping of version -> count.
func inspectVersionCounts(counts map[string]int) (median string, total int, ok bool) {
	var sum int
	var versions []string
	for version, count := range counts {
		sum += count
		versions = append(versions, version)
	}

	semver.Sort(versions)

	var cursor int
	for _, version := range versions {
		cursor += counts[version]
		if cursor > sum/2 {
			return version, sum, true
		}
	}

	return "", 0, false
}

// instanceMetricsPeriodic is an aggregator for general instance metrics.
type instanceMetricsPeriodic struct {
	upgraderCounts map[string]map[string]int
	totalInstances int
}

func newInstanceMetricsPeriodic() *instanceMetricsPeriodic {
	return &instanceMetricsPeriodic{
		upgraderCounts: make(map[string]map[string]int),
	}
}

// VisitInstance adds an instance to ongoing aggregations.
func (i *instanceMetricsPeriodic) VisitInstance(instance proto.UpstreamInventoryHello) {
	i.totalInstances++
	if upgrader := instance.GetExternalUpgrader(); upgrader != "" {
		if _, exists := i.upgraderCounts[upgrader]; !exists {
			i.upgraderCounts[upgrader] = make(map[string]int)
		}
		i.upgraderCounts[upgrader][instance.GetExternalUpgraderVersion()]++
	}
}

// TotalEnrolledInUpgrades gets the total number of instances that have some upgrader defined.
func (i *instanceMetricsPeriodic) TotalEnrolledInUpgrades() int {
	var total int
	for _, upgraderVersion := range i.upgraderCounts {
		for _, count := range upgraderVersion {
			total += count
		}
	}

	return total
}

// TotalInstances gets the total number of known instances.
func (i *instanceMetricsPeriodic) TotalInstances() int {
	return i.totalInstances
}
