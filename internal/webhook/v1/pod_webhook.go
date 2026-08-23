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
	"fmt"

	"k8s.io/apimachinery/pkg/util/intstr"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var podlog = logf.Log.WithName("pod-resource")

const (
	policyConfigMapNamespace = "kitten-operator-controller-system"
	policyConfigMapName      = "kitten-operator-controller-kitten-injector-config"
)

// SetupPodWebhookWithManager registers the webhook for Pod in the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1.Pod{}).
		WithDefaulter(&PodCustomDefaulter{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// PodCustomDefaulter injects a kitten-picture sidecar into pods selected by
// the cluster's injection policy.
type PodCustomDefaulter struct {
	Client client.Client
}

// Default implements webhook.CustomDefaulter.
func (d *PodCustomDefaulter) Default(ctx context.Context, pod *corev1.Pod) error {
	if alreadyInjected(pod) {
		return nil
	}

	policy := loadPolicy(ctx, d.Client, policyConfigMapNamespace, policyConfigMapName)

	// Namespace isn't always populated on pod.ObjectMeta at admission time
	// for namespaced creates via some clients; the AdmissionRequest itself
	// carries it reliably, but pod.Namespace is populated by the API server
	// before webhooks run for namespace-scoped creates, so this is safe.
	if !policy.shouldInject(pod, pod.Namespace) {
		return nil
	}

	if len(pod.Spec.Containers) == 0 {
		podlog.Info("pod has no containers, skipping injection", "name", pod.GetName())
		return nil
	}

	mainContainer := &pod.Spec.Containers[0]

	// Find the port the app is currently exposed on externally, so the
	// sidecar can take over its name/number. We take the first declared
	// port; if none is declared, there's nothing for the sidecar to
	// meaningfully proxy in front of, so skip.
	if len(mainContainer.Ports) == 0 {
		podlog.Info("main container declares no ports, skipping injection", "name", pod.GetName())
		return nil
	}

	// Relocate the main container to the policy's InternalPort. This
	// requires the main container's app to actually be configured to
	// listen there - see the cooperating-app contract in the README.
	originalPortName := mainContainer.Ports[0].Name
	mainContainer.Ports[0].ContainerPort = policy.InternalPort
	mainContainer.Ports[0].Name = "kitten-internal"

	if mainContainer.LivenessProbe != nil && mainContainer.LivenessProbe.HTTPGet != nil {
		mainContainer.LivenessProbe.HTTPGet.Port = intstr.FromString("kitten-internal")
	}
	if mainContainer.ReadinessProbe != nil && mainContainer.ReadinessProbe.HTTPGet != nil {
		mainContainer.ReadinessProbe.HTTPGet.Port = intstr.FromString("kitten-internal")
	}

	sidecar := corev1.Container{
		Name:  "kitten-sidecar",
		Image: policy.SidecarImage,
		Ports: []corev1.ContainerPort{
			{
				Name:          policy.SidecarPortName,
				ContainerPort: policy.SidecarPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: []corev1.EnvVar{
			{Name: "UPSTREAM_URL", Value: fmt.Sprintf("http://localhost:%d", policy.InternalPort)},
			{Name: "KITTEN_SERVICE_URL", Value: policy.KittenServiceURL},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(int(policy.SidecarPort))},
			},
			InitialDelaySeconds: 3,
			PeriodSeconds:       5,
		},
	}
	pod.Spec.Containers = append(pod.Spec.Containers, sidecar)

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations["kitten.pielaboratories.com/injected"] = injectedAnnotationValue
	pod.Annotations["kitten.pielaboratories.com/original-port-name"] = originalPortName

	podlog.Info("injected kitten sidecar", "pod", pod.GetName(), "namespace", pod.Namespace)
	return nil
}
