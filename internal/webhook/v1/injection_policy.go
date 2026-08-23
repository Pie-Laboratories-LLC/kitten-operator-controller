/*
Copyright 2026.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package v1

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// InjectionPolicy controls whether and how the kitten sidecar gets injected.
type InjectionPolicy struct {
	// Mode is "optIn" (only pods with OptInLabel=OptInValue) or "all"
	// (every pod not in ExcludeNamespaces).
	Mode string `json:"mode"`

	OptInLabel string `json:"optInLabel"`
	OptInValue string `json:"optInValue"`

	ExcludeNamespaces []string `json:"excludeNamespaces"`

	// InternalPort is the port the main container must be reconfigured to
	// listen on internally, once the sidecar takes over the original
	// externally-exposed port name/number.
	InternalPort int32 `json:"internalPort"`

	// SidecarImage is the kitten-proxy sidecar's container image.
	SidecarImage string `json:"sidecarImage"`

	// KittenServiceURL is the central kitten-operator Service the sidecar
	// fetches image URLs from.
	KittenServiceURL string `json:"kittenServiceURL"`

	// SidecarPort is the port the sidecar listens on. This is the port that
	// takes over the main container's original port *number*.
	SidecarPort int32 `json:"sidecarPort"`

	// SidecarPortName is the port *name* the sidecar claims. This must match
	// whatever name the target's Service references via targetPort — for
	// most Helm charts (including kitten-operator's and the common
	// `helm create` scaffold), that's "http".
	SidecarPortName string `json:"sidecarPortName"`
}

func defaultPolicy() *InjectionPolicy {
	return &InjectionPolicy{
		Mode:              "optIn",
		OptInLabel:        "kitten.pielaboratories.com/inject",
		OptInValue:        "true",
		ExcludeNamespaces: []string{"kube-system", "kube-node-lease", "kube-public"},
		InternalPort:      8001,
		SidecarImage:      "kitten-operator-sidecar:local",
		KittenServiceURL:  "http://kitten-operator/kittenpictures",
		SidecarPort:       8000,
		SidecarPortName:   "http",
	}
}

// loadPolicy reads the injection policy from a ConfigMap, falling back to
// sane defaults if the ConfigMap is missing or malformed. Failing open here
// (rather than rejecting admission) means a misconfigured or absent
// ConfigMap degrades to "don't inject" rather than blocking pod creation
// cluster-wide, which would be a much scarier failure mode for a webhook.
func loadPolicy(ctx context.Context, c client.Client, namespace, name string) *InjectionPolicy {
	policy := defaultPolicy()

	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &cm); err != nil {
		podlog.Info("injection policy ConfigMap not found, using defaults", "error", err.Error())
		return policy
	}

	raw, ok := cm.Data["policy.yaml"]
	if !ok {
		podlog.Info("injection policy ConfigMap missing policy.yaml key, using defaults")
		return policy
	}

	if err := yaml.Unmarshal([]byte(raw), policy); err != nil {
		podlog.Info("injection policy ConfigMap invalid, using defaults", "error", err.Error())
		return defaultPolicy()
	}

	return policy
}

func (p *InjectionPolicy) shouldInject(pod *corev1.Pod, namespace string) bool {
	for _, excluded := range p.ExcludeNamespaces {
		if namespace == excluded {
			return false
		}
	}

	switch p.Mode {
	case "all":
		return true
	case "optIn":
		return pod.Labels[p.OptInLabel] == p.OptInValue
	default:
		return false
	}
}

// alreadyInjected guards against double-injection on pod updates - the
// webhook fires on both create and update, and we only want to add the
// sidecar once.
func alreadyInjected(pod *corev1.Pod) bool {
	return pod.Annotations["kitten.pielaboratories.com/injected"] == "true"
}
