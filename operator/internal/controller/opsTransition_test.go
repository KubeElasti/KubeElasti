/*
Copyright 2024.

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

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var _ = Describe("waitForReadyEndpoints", func() {
	const svcName = "wait-ready-svc"

	ctx := context.Background()
	namespacedName := types.NamespacedName{Name: svcName, Namespace: namespace}

	AfterEach(func() {
		slice := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: namespace},
		}
		_ = k8sClient.Delete(ctx, slice)
	})

	It("returns immediately once a ready endpoint exists", func() {
		slice := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcName,
				Namespace: namespace,
				Labels: map[string]string{
					discoveryv1.LabelServiceName: svcName,
					discoveryv1.LabelManagedBy:   "endpointslice-controller.k8s.io",
				},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses:  []string{"10.0.0.1"},
					Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
				},
			},
		}
		Expect(k8sClient.Create(ctx, slice)).To(Succeed())

		err := controllerReconciler.waitForReadyEndpoints(ctx, namespacedName, 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
	})

	It("ignores a resolver hijack EndpointSlice with the same service-name label", func() {
		// Same service-name label as a real EndpointSlice, no managed-by label, and a
		// nil-Conditions.Ready endpoint -- exactly what createOrUpdateEndpointsliceToResolver
		// creates. Should NOT be treated as a ready endpoint.
		hijack := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "elasti-" + svcName + "-endpointslice-to-resolver",
				Namespace: namespace,
				Labels:    map[string]string{discoveryv1.LabelServiceName: svcName},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"10.0.0.9"}},
			},
		}
		Expect(k8sClient.Create(ctx, hijack)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, hijack) }()

		err := controllerReconciler.waitForReadyEndpoints(ctx, namespacedName, 3*time.Second)
		Expect(err).To(HaveOccurred())
	})

	It("times out when no ready endpoint ever appears", func() {
		slice := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcName,
				Namespace: namespace,
				Labels: map[string]string{
					discoveryv1.LabelServiceName: svcName,
					discoveryv1.LabelManagedBy:   "endpointslice-controller.k8s.io",
				},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses:  []string{"10.0.0.2"},
					Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(false)},
				},
			},
		}
		Expect(k8sClient.Create(ctx, slice)).To(Succeed())

		err := controllerReconciler.waitForReadyEndpoints(ctx, namespacedName, 3*time.Second)
		Expect(err).To(HaveOccurred())
	})
})
