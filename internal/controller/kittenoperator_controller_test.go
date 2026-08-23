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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kittenv1alpha1 "github.com/Pie-Laboratories-LLC/kitten-operator-controller/api/v1alpha1"
)

var _ = Describe("KittenOperator controller", func() {
	const (
		resourceName = "test-kitten-operator"
		namespace    = "default"
	)

	ctx := context.Background()
	typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: namespace}

	AfterEach(func() {
		// Clean up between specs so each test starts from a known state.
		kitten := &kittenv1alpha1.KittenOperator{}
		if err := k8sClient.Get(ctx, typeNamespacedName, kitten); err == nil {
			Expect(k8sClient.Delete(ctx, kitten)).To(Succeed())
		}
	})

	It("creates a Deployment matching the CR's spec", func() {
		replicas := int32(3)
		kitten := &kittenv1alpha1.KittenOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: kittenv1alpha1.KittenOperatorSpec{
				Replicas: &replicas,
				Image:    "kitten-operator:test",
			},
		}
		Expect(k8sClient.Create(ctx, kitten)).To(Succeed())

		deploy := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, typeNamespacedName, deploy)
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		Expect(*deploy.Spec.Replicas).To(Equal(int32(3)))
		Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal("kitten-operator:test"))
	})

	It("creates a Service exposing port 80", func() {
		replicas := int32(1)
		kitten := &kittenv1alpha1.KittenOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: kittenv1alpha1.KittenOperatorSpec{
				Replicas: &replicas,
				Image:    "kitten-operator:test",
			},
		}
		Expect(k8sClient.Create(ctx, kitten)).To(Succeed())

		svc := &corev1.Service{}
		Eventually(func() error {
			return k8sClient.Get(ctx, typeNamespacedName, svc)
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		Expect(svc.Spec.Ports).To(HaveLen(1))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(80)))
	})

	It("deletes the Deployment when the CR is deleted (owner reference / GC)", func() {
		replicas := int32(1)
		kitten := &kittenv1alpha1.KittenOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: kittenv1alpha1.KittenOperatorSpec{
				Replicas: &replicas,
				Image:    "kitten-operator:test",
			},
		}
		Expect(k8sClient.Create(ctx, kitten)).To(Succeed())

		deploy := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, typeNamespacedName, deploy)
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// Confirm the owner reference is actually set — this is what makes
		// cascade-delete and drift-watching work at all.
		Expect(deploy.OwnerReferences).To(HaveLen(1))
		Expect(deploy.OwnerReferences[0].Name).To(Equal(resourceName))
		Expect(deploy.OwnerReferences[0].Kind).To(Equal("KittenOperator"))
	})
})
