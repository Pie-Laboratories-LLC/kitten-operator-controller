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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	kittenv1alpha1 "github.com/Pie-Laboratories-LLC/kitten-operator-controller/api/v1alpha1"
)

// KittenOperatorReconciler reconciles a KittenOperator object
type KittenOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kitten.pielaboratories.com,resources=kittenoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kitten.pielaboratories.com,resources=kittenoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kitten.pielaboratories.com,resources=kittenoperators/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *KittenOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the KittenOperator instance that triggered this reconcile.
	var kitten kittenv1alpha1.KittenOperator
	if err := r.Get(ctx, req.NamespacedName, &kitten); err != nil {
		if apierrors.IsNotFound(err) {
			// Object was deleted; owned Deployment/Service get garbage
			// collected automatically via owner references. Nothing to do.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// --- Reconcile the Deployment ---
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kitten.Name,
			Namespace: kitten.Namespace,
		},
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
			return r.mutateDeployment(&kitten, deploy)
		})
		return err
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling deployment: %w", err)
	}

	// --- Reconcile the Service ---
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kitten.Name,
			Namespace: kitten.Namespace,
		},
	}
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
			return r.mutateService(&kitten, svc)
		})
		return err
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling service: %w", err)
	}

	// --- Refresh status from the Deployment we just reconciled ---
	var current appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKeyFromObject(deploy), &current); err != nil {
		return ctrl.Result{}, fmt.Errorf("re-fetching deployment: %w", err)
	}

	kitten.Status.AvailableReplicas = current.Status.AvailableReplicas

	available := metav1.Condition{
		Type:               "Available",
		Status:             metav1.ConditionFalse,
		Reason:             "DeploymentNotReady",
		Message:            "Waiting for kitten-operator pods to become ready",
		ObservedGeneration: kitten.Generation,
	}
	if current.Status.AvailableReplicas > 0 {
		available.Status = metav1.ConditionTrue
		available.Reason = "DeploymentAvailable"
		available.Message = fmt.Sprintf("%d/%d pods available", current.Status.AvailableReplicas, current.Status.Replicas)
	}
	meta.SetStatusCondition(&kitten.Status.Conditions, available)

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Status().Update(ctx, &kitten)
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	log.Info("reconciled KittenOperator", "availableReplicas", kitten.Status.AvailableReplicas)
	return ctrl.Result{}, nil
}

// mutateDeployment sets the Deployment's spec to match the desired state
// described by the KittenOperator resource. It's called by CreateOrUpdate
// both when the Deployment doesn't exist yet (create) and when it already
// exists but might have drifted (update).
func (r *KittenOperatorReconciler) mutateDeployment(kitten *kittenv1alpha1.KittenOperator, deploy *appsv1.Deployment) error {
	labels := map[string]string{
		"app.kubernetes.io/name":     "kitten-operator",
		"app.kubernetes.io/instance": kitten.Name,
	}

	deploy.Spec = appsv1.DeploymentSpec{
		Replicas: kitten.Spec.Replicas,
		Selector: &metav1.LabelSelector{MatchLabels: labels},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "kitten-operator",
						Image: kitten.Spec.Image,
						Ports: []corev1.ContainerPort{
							{Name: "http", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromString("http")},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromString("http")},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       5,
						},
					},
				},
			},
		},
	}

	// SetControllerReference makes the KittenOperator the owner of this
	// Deployment. Two effects: (1) deleting the KittenOperator cascades to
	// delete the Deployment automatically, and (2) it's what lets the
	// controller's watch mechanism notice changes to the Deployment and
	// re-trigger Reconcile.
	return controllerutil.SetControllerReference(kitten, deploy, r.Scheme)
}

func (r *KittenOperatorReconciler) mutateService(kitten *kittenv1alpha1.KittenOperator, svc *corev1.Service) error {
	labels := map[string]string{
		"app.kubernetes.io/name":     "kitten-operator",
		"app.kubernetes.io/instance": kitten.Name,
	}

	svc.Spec.Selector = labels
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "http",
			Port:       80,
			TargetPort: intstr.FromString("http"),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	return controllerutil.SetControllerReference(kitten, svc, r.Scheme)
}

// SetupWithManager sets up the controller with the Manager.
func (r *KittenOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kittenv1alpha1.KittenOperator{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("kittenoperator").
		Complete(r)
}
